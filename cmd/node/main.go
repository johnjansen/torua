package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dreamware/torua/internal/cluster"
	"github.com/dreamware/torua/internal/shard"
)

// logFatal is a variable to allow mocking log.Fatal in tests
var logFatal = log.Fatalf

// Node represents a node with its shards
type Node struct {
	ID     string
	shards map[int]*shard.Shard
	mu     sync.RWMutex
}

// NewNode creates a new node instance
func NewNode(id string) *Node {
	return &Node{
		ID:     id,
		shards: make(map[int]*shard.Shard),
	}
}

// AddShard adds a shard to the node
func (n *Node) AddShard(s *shard.Shard) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.shards[s.ID] = s
}

// GetShard retrieves a shard by ID
func (n *Node) GetShard(id int) *shard.Shard {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.shards[id]
}

func main() {
	nodeID := mustGetenv("NODE_ID")
	listen := getenv("NODE_LISTEN", ":8081")
	public := getenv("NODE_ADDR", "http://127.0.0.1:8081")
	coord := mustGetenv("COORDINATOR_ADDR")

	// Create node with initial shard
	node := NewNode(nodeID)

	// Start with shard 0 for now (will be assigned by coordinator later)
	initialShard := shard.NewShard(0, true)
	node.AddShard(initialShard)
	log.Printf("node[%s] initialized with shard 0", nodeID)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/control", handleControl)

	// Shard storage endpoints
	mux.HandleFunc("/shard/", func(w http.ResponseWriter, r *http.Request) {
		handleShardRequest(node, w, r)
	})

	// Node info endpoint
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		handleNodeInfo(node, w, r)
	})

	s := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("node[%s] listening on %s (public %s)", nodeID, listen, public)
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logFatal("listen: %v", err)
		}
	}()

	ctx := context.Background()
	register(ctx, coord, nodeID, public)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Shutdown(ctx)
	log.Println("node stopped")
}

func register(ctx context.Context, coord, id, addr string) {
	body := cluster.RegisterRequest{Node: cluster.NodeInfo{ID: id, Addr: addr}}
	var lastErr error
	for i := 0; i < 10; i++ {
		lastErr = cluster.PostJSON(ctx, coord+"/register", body, nil)
		if lastErr == nil {
			log.Printf("registered with coordinator @ %s", coord)
			return
		}
		log.Printf("register retry %d: %v", i+1, lastErr)
		time.Sleep(400 * time.Millisecond)
	}
	logFatal("failed to register with coordinator: %v", lastErr)
}

func handleControl(w http.ResponseWriter, r *http.Request) {
	var raw bytes.Buffer
	if _, err := raw.ReadFrom(r.Body); err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	log.Printf("control payload: %s", raw.Bytes())
	w.WriteHeader(http.StatusNoContent)
}

// handleShardRequest routes shard-specific requests
// Path format: /shard/{shardID}/store/{key}
func handleShardRequest(node *Node, w http.ResponseWriter, r *http.Request) {
	// Parse path: /shard/{shardID}/store/{key}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/shard/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path format", http.StatusBadRequest)
		return
	}

	// Parse shard ID
	shardID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "invalid shard ID", http.StatusBadRequest)
		return
	}

	// Get the shard
	shard := node.GetShard(shardID)
	if shard == nil {
		http.Error(w, "shard not found", http.StatusNotFound)
		return
	}

	// Handle based on remaining path
	if parts[1] == "store" {
		if len(parts) == 2 {
			// List keys: GET /shard/{shardID}/store
			if r.Method == http.MethodGet {
				handleListKeys(shard, w, r)
				return
			}
		} else if len(parts) == 3 {
			// Key operations: /shard/{shardID}/store/{key}
			key := parts[2]
			switch r.Method {
			case http.MethodGet:
				handleGet(shard, key, w, r)
			case http.MethodPut:
				handlePut(shard, key, w, r)
			case http.MethodDelete:
				handleDelete(shard, key, w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
	} else if parts[1] == "stats" {
		// Stats: GET /shard/{shardID}/stats
		if r.Method == http.MethodGet {
			handleShardStats(shard, w, r)
			return
		}
	}

	http.Error(w, "not found", http.StatusNotFound)
}

// handleGet retrieves a value from the shard
func handleGet(s *shard.Shard, key string, w http.ResponseWriter, r *http.Request) {
	value, err := s.Get(key)
	if err != nil {
		if err.Error() == "key not found" {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(value)
}

// handlePut stores a value in the shard
func handlePut(s *shard.Shard, key string, w http.ResponseWriter, r *http.Request) {
	// Read body
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r.Body); err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Store the value
	if err := s.Put(key, buf.Bytes()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDelete removes a key from the shard
func handleDelete(s *shard.Shard, key string, w http.ResponseWriter, r *http.Request) {
	if err := s.Delete(key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListKeys returns all keys in the shard
func handleListKeys(s *shard.Shard, w http.ResponseWriter, r *http.Request) {
	keys := s.ListKeys()

	response := struct {
		Keys  []string `json:"keys"`
		Count int      `json:"count"`
	}{
		Keys:  keys,
		Count: len(keys),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleShardStats returns shard statistics
func handleShardStats(s *shard.Shard, w http.ResponseWriter, r *http.Request) {
	stats := s.GetStats()

	response := struct {
		ShardID int                  `json:"shard_id"`
		Ops     shard.OperationStats `json:"operations"`
		Storage struct {
			Keys  int `json:"keys"`
			Bytes int `json:"bytes"`
		} `json:"storage"`
	}{
		ShardID: s.ID,
		Ops:     stats.Ops,
		Storage: struct {
			Keys  int `json:"keys"`
			Bytes int `json:"bytes"`
		}{
			Keys:  stats.Storage.Keys,
			Bytes: stats.Storage.Bytes,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleNodeInfo returns information about the node and its shards
func handleNodeInfo(node *Node, w http.ResponseWriter, r *http.Request) {
	node.mu.RLock()
	defer node.mu.RUnlock()

	shardInfos := make([]shard.ShardInfo, 0, len(node.shards))
	for _, s := range node.shards {
		shardInfos = append(shardInfos, s.Info())
	}

	response := struct {
		NodeID string            `json:"node_id"`
		Shards []shard.ShardInfo `json:"shards"`
		Count  int               `json:"shard_count"`
	}{
		NodeID: node.ID,
		Shards: shardInfos,
		Count:  len(shardInfos),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func mustGetenv(k string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	logFatal("missing env %s", k)
	return ""
}
