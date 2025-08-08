package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamware/torua/internal/cluster"
)

// logFatal is a variable to allow mocking log.Fatal in tests
var logFatal = log.Fatalf

func main() {
	nodeID := mustGetenv("NODE_ID")
	listen := getenv("NODE_LISTEN", ":8081")
	public := getenv("NODE_ADDR", "http://127.0.0.1:8081")
	coord := mustGetenv("COORDINATOR_ADDR")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/control", handleControl)

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
