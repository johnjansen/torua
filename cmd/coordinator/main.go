package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/exp/slices"

	"github.com/dreamware/torua/internal/cluster"
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
	mu    sync.RWMutex
	nodes []cluster.NodeInfo
}

func newServer() *server { return &server{} }

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

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
