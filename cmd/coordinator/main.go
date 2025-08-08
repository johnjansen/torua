package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/exp/slices"

	"github.com/dreamware/torua/internal/cluster"
	"github.com/dreamware/torua/internal/coordinator"
)

func main() {
	addr := getenv("COORDINATOR_ADDR", ":8080")
	srv := newServer()

	mux := http.NewServeMux()
	mux.HandleFunc("/register", srv.handleRegister)
	mux.HandleFunc("/nodes", srv.handleListNodes)
	mux.HandleFunc("/broadcast", srv.handleBroadcast)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Data routing endpoints
	mux.HandleFunc("/data/", srv.handleData)
	// Shard management endpoints
	mux.HandleFunc("/shards", srv.handleShards)
	mux.HandleFunc("/shards/assign", srv.handleShardAssign)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("coordinator listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	log.Println("coordinator stopped")
}

type server struct {
	mu       sync.RWMutex
	nodes    []cluster.NodeInfo
	registry *coordinator.ShardRegistry
}

func newServer() *server {
	// Start with 4 shards by default (can be made configurable later)
	return &server{
		registry: coordinator.NewShardRegistry(4),
	}
}

func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req cluster.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Node.ID == "" || req.Node.Addr == "" {
		http.Error(w, "missing id/addr", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := slices.IndexFunc(s.nodes, func(n cluster.NodeInfo) bool { return n.ID == req.Node.ID })
	if idx >= 0 {
		s.nodes[idx] = req.Node
	} else {
		s.nodes = append(s.nodes, req.Node)
		// Auto-assign shards to new nodes (simple round-robin for now)
		s.autoAssignShards()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_ = json.NewEncoder(w).Encode(struct {
		Nodes []cluster.NodeInfo `json:"nodes"`
	}{Nodes: s.nodes})
}

func (s *server) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	var req cluster.BroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Path == "" || req.Path[0] != '/' {
		http.Error(w, "path must start with '/'", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	targets := append([]cluster.NodeInfo(nil), s.nodes...)
	s.mu.RUnlock()

	type result struct {
		NodeID string `json:"node_id"`
		Err    string `json:"err,omitempty"`
	}
	out := make([]result, 0, len(targets))

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	for _, n := range targets {
		url := n.Addr + req.Path
		err := cluster.PostJSON(ctx, url, req.Payload, nil)
		res := result{NodeID: n.ID}
		if err != nil {
			res.Err = err.Error()
		}
		out = append(out, res)
	}

	_ = json.NewEncoder(w).Encode(struct {
		SentTo  int      `json:"sent_to"`
		Results []result `json:"results"`
	}{SentTo: len(targets), Results: out})
}

// handleData routes data operations to the appropriate shard/node
func (s *server) handleData(w http.ResponseWriter, r *http.Request) {
	// Extract key from path: /data/{key}
	key := r.URL.Path[len("/data/"):]
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	// Find which node owns this key
	nodeID, err := s.registry.GetNodeForKey(key)
	if err != nil {
		http.Error(w, fmt.Sprintf("no node assigned for key: %v", err), http.StatusServiceUnavailable)
		return
	}

	// Find the node's address
	s.mu.RLock()
	var nodeAddr string
	for _, node := range s.nodes {
		if node.ID == nodeID {
			nodeAddr = node.Addr
			break
		}
	}
	s.mu.RUnlock()

	if nodeAddr == "" {
		http.Error(w, fmt.Sprintf("node %s not found", nodeID), http.StatusServiceUnavailable)
		return
	}

	// Determine which shard owns this key
	shardID := s.registry.GetShardForKey(key)

	// Forward the request to the node's shard
	targetURL := fmt.Sprintf("%s/shard/%d/store/%s", nodeAddr, shardID, key)

	switch r.Method {
	case http.MethodGet:
		s.forwardGet(targetURL, w, r)
	case http.MethodPut:
		s.forwardPut(targetURL, w, r)
	case http.MethodDelete:
		s.forwardDelete(targetURL, w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// forwardGet forwards a GET request to a node
func (s *server) forwardGet(targetURL string, w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to forward request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response back to client
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// forwardPut forwards a PUT request to a node
func (s *server) forwardPut(targetURL string, w http.ResponseWriter, r *http.Request) {
	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to forward request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response back to client
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// forwardDelete forwards a DELETE request to a node
func (s *server) forwardDelete(targetURL string, w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, targetURL, nil)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to forward request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response back to client
	w.WriteHeader(resp.StatusCode)
}

// handleShards returns current shard assignments
func (s *server) handleShards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	assignments := s.registry.GetAllAssignments()

	response := struct {
		Shards    []*coordinator.ShardAssignment `json:"shards"`
		NumShards int                            `json:"num_shards"`
	}{
		Shards:    assignments,
		NumShards: s.registry.NumShards(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleShardAssign manually assigns a shard to a node (admin operation)
func (s *server) handleShardAssign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ShardID   int    `json:"shard_id"`
		NodeID    string `json:"node_id"`
		IsPrimary bool   `json:"is_primary"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	if err := s.registry.AssignShard(req.ShardID, req.NodeID, req.IsPrimary); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// autoAssignShards automatically assigns unassigned shards to nodes
// This is a simple round-robin assignment for now
func (s *server) autoAssignShards() {
	if len(s.nodes) == 0 {
		return
	}

	// Get all current assignments
	assignments := s.registry.GetAllAssignments()
	assignedShards := make(map[int]bool)
	for _, a := range assignments {
		assignedShards[a.ShardID] = true
	}

	// Assign any unassigned shards
	nodeIndex := 0
	for shardID := 0; shardID < s.registry.NumShards(); shardID++ {
		if !assignedShards[shardID] {
			nodeID := s.nodes[nodeIndex].ID
			s.registry.AssignShard(shardID, nodeID, true)
			log.Printf("Auto-assigned shard %d to node %s", shardID, nodeID)
			nodeIndex = (nodeIndex + 1) % len(s.nodes)
		}
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
