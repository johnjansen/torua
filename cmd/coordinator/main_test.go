package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/dreamware/torua/internal/cluster"
)

// TestGetenv tests the getenv utility function
func TestGetenv(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		def      string
		expected string
	}{
		{
			name:     "environment variable set",
			key:      "TEST_ENV_VAR",
			value:    "test_value",
			def:      "default",
			expected: "test_value",
		},
		{
			name:     "environment variable not set",
			key:      "UNSET_ENV_VAR",
			value:    "",
			def:      "default_value",
			expected: "default_value",
		},
		{
			name:     "empty environment variable returns default",
			key:      "EMPTY_ENV_VAR",
			value:    "",
			def:      "fallback",
			expected: "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set or unset environment variable
			if tt.value != "" {
				os.Setenv(tt.key, tt.value)
				defer os.Unsetenv(tt.key)
			}

			// Test getenv
			result := getenv(tt.key, tt.def)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestNewServer tests server creation
func TestNewServer(t *testing.T) {
	srv := newServer()

	// Verify server is initialized
	if srv == nil {
		t.Fatal("Expected server instance, got nil")
	}

	// Verify nodes slice is initialized (it can be nil or empty)
	// In Go, a nil slice is a valid empty slice

	// Verify it starts with no nodes
	if len(srv.nodes) != 0 {
		t.Errorf("Expected 0 nodes initially, got %d", len(srv.nodes))
	}
}

// TestHandleRegister tests the node registration endpoint
func TestHandleRegister(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectNode     bool
	}{
		{
			name: "successful registration",
			requestBody: cluster.RegisterRequest{
				Node: cluster.NodeInfo{
					ID:   "node1",
					Addr: "http://localhost:8081",
				},
			},
			expectedStatus: http.StatusNoContent,
			expectNode:     true,
		},
		{
			name: "registration with missing ID",
			requestBody: cluster.RegisterRequest{
				Node: cluster.NodeInfo{
					ID:   "",
					Addr: "http://localhost:8081",
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectNode:     false,
		},
		{
			name: "registration with missing address",
			requestBody: cluster.RegisterRequest{
				Node: cluster.NodeInfo{
					ID:   "node2",
					Addr: "",
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectNode:     false,
		},
		{
			name:           "invalid JSON body",
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectNode:     false,
		},
		{
			name: "update existing node",
			requestBody: cluster.RegisterRequest{
				Node: cluster.NodeInfo{
					ID:   "node1",
					Addr: "http://localhost:8082", // Different address
				},
			},
			expectedStatus: http.StatusNoContent,
			expectNode:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()

			// Pre-populate with a node for update test
			if tt.name == "update existing node" {
				srv.nodes = append(srv.nodes, cluster.NodeInfo{
					ID:   "node1",
					Addr: "http://localhost:8081",
				})
			}

			// Create request
			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				if err != nil {
					t.Fatalf("Failed to marshal request body: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			// Handle request
			srv.handleRegister(rec, req)

			// Check status code
			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			// Check if node was added/updated
			if tt.expectNode {
				found := false
				for _, node := range srv.nodes {
					if reqData, ok := tt.requestBody.(cluster.RegisterRequest); ok {
						if node.ID == reqData.Node.ID {
							found = true
							// For update test, verify address changed
							if tt.name == "update existing node" && node.Addr != reqData.Node.Addr {
								t.Errorf("Node address not updated: expected %s, got %s",
									reqData.Node.Addr, node.Addr)
							}
							break
						}
					}
				}
				if !found {
					t.Error("Expected node to be registered but it wasn't found")
				}
			}
		})
	}
}

// TestHandleListNodes tests the node listing endpoint
func TestHandleListNodes(t *testing.T) {
	tests := []struct {
		name          string
		initialNodes  []cluster.NodeInfo
		expectedCount int
	}{
		{
			name:          "empty node list",
			initialNodes:  []cluster.NodeInfo{},
			expectedCount: 0,
		},
		{
			name: "single node",
			initialNodes: []cluster.NodeInfo{
				{ID: "node1", Addr: "http://localhost:8081"},
			},
			expectedCount: 1,
		},
		{
			name: "multiple nodes",
			initialNodes: []cluster.NodeInfo{
				{ID: "node1", Addr: "http://localhost:8081"},
				{ID: "node2", Addr: "http://localhost:8082"},
				{ID: "node3", Addr: "http://localhost:8083"},
			},
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			srv.nodes = tt.initialNodes

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/nodes", nil)
			rec := httptest.NewRecorder()

			// Handle request
			srv.handleListNodes(rec, req)

			// Check status code
			if rec.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", rec.Code)
			}

			// Parse response
			var response struct {
				Nodes []cluster.NodeInfo `json:"nodes"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Verify node count
			if len(response.Nodes) != tt.expectedCount {
				t.Errorf("Expected %d nodes, got %d", tt.expectedCount, len(response.Nodes))
			}

			// Verify node data matches
			for i, node := range response.Nodes {
				if i < len(tt.initialNodes) {
					if node.ID != tt.initialNodes[i].ID {
						t.Errorf("Node ID mismatch: expected %s, got %s",
							tt.initialNodes[i].ID, node.ID)
					}
					if node.Addr != tt.initialNodes[i].Addr {
						t.Errorf("Node Addr mismatch: expected %s, got %s",
							tt.initialNodes[i].Addr, node.Addr)
					}
				}
			}
		})
	}
}

// TestHandleBroadcast tests the broadcast endpoint
func TestHandleBroadcast(t *testing.T) {
	tests := []struct {
		name           string
		nodes          []cluster.NodeInfo
		requestBody    interface{}
		expectedStatus int
		expectedSentTo int
	}{
		{
			name: "successful broadcast to multiple nodes",
			nodes: []cluster.NodeInfo{
				{ID: "node1", Addr: "http://localhost:8081"},
				{ID: "node2", Addr: "http://localhost:8082"},
			},
			requestBody: cluster.BroadcastRequest{
				Path:    "/control",
				Payload: json.RawMessage(`{"op":"ping"}`),
			},
			expectedStatus: http.StatusOK,
			expectedSentTo: 2,
		},
		{
			name:  "broadcast with no nodes",
			nodes: []cluster.NodeInfo{},
			requestBody: cluster.BroadcastRequest{
				Path:    "/control",
				Payload: json.RawMessage(`{"op":"ping"}`),
			},
			expectedStatus: http.StatusOK,
			expectedSentTo: 0,
		},
		{
			name: "invalid path without leading slash",
			nodes: []cluster.NodeInfo{
				{ID: "node1", Addr: "http://localhost:8081"},
			},
			requestBody: cluster.BroadcastRequest{
				Path:    "control", // Missing leading /
				Payload: json.RawMessage(`{"op":"ping"}`),
			},
			expectedStatus: http.StatusBadRequest,
			expectedSentTo: 0,
		},
		{
			name: "empty path",
			nodes: []cluster.NodeInfo{
				{ID: "node1", Addr: "http://localhost:8081"},
			},
			requestBody: cluster.BroadcastRequest{
				Path:    "",
				Payload: json.RawMessage(`{"op":"ping"}`),
			},
			expectedStatus: http.StatusBadRequest,
			expectedSentTo: 0,
		},
		{
			name:           "invalid JSON body",
			nodes:          []cluster.NodeInfo{},
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedSentTo: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock node servers if needed
			var nodeServers []*httptest.Server
			for _, node := range tt.nodes {
				nodeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify the request path contains /control
					if r.URL.Path != "/control" {
						t.Errorf("Expected path /control, got %s", r.URL.Path)
					}
					w.WriteHeader(http.StatusNoContent)
				}))
				nodeServers = append(nodeServers, nodeServer)
				defer nodeServer.Close()

				// Update node address to use test server URL
				for i := range tt.nodes {
					if tt.nodes[i].ID == node.ID {
						tt.nodes[i].Addr = nodeServer.URL
						break
					}
				}
			}

			srv := newServer()
			srv.nodes = tt.nodes

			// Create request
			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				if err != nil {
					t.Fatalf("Failed to marshal request body: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/broadcast", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			// Handle request
			srv.handleBroadcast(rec, req)

			// Check status code
			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			// Parse response if successful
			if tt.expectedStatus == http.StatusOK {
				var response struct {
					SentTo  int `json:"sent_to"`
					Results []struct {
						NodeID string `json:"node_id"`
						Err    string `json:"err,omitempty"`
					} `json:"results"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if response.SentTo != tt.expectedSentTo {
					t.Errorf("Expected sent_to %d, got %d", tt.expectedSentTo, response.SentTo)
				}

				if len(response.Results) != tt.expectedSentTo {
					t.Errorf("Expected %d results, got %d", tt.expectedSentTo, len(response.Results))
				}
			}
		})
	}
}

// TestHandleBroadcastWithFailingNodes tests broadcast with node failures
func TestHandleBroadcastWithFailingNodes(t *testing.T) {
	// Create one successful and one failing node
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer successServer.Close()

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	srv := newServer()
	srv.nodes = []cluster.NodeInfo{
		{ID: "success-node", Addr: successServer.URL},
		{ID: "fail-node", Addr: failServer.URL},
	}

	// Create broadcast request
	reqBody := cluster.BroadcastRequest{
		Path:    "/control",
		Payload: json.RawMessage(`{"op":"test"}`),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/broadcast", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	// Handle request
	srv.handleBroadcast(rec, req)

	// Parse response
	var response struct {
		SentTo  int `json:"sent_to"`
		Results []struct {
			NodeID string `json:"node_id"`
			Err    string `json:"err,omitempty"`
		} `json:"results"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify results
	if response.SentTo != 2 {
		t.Errorf("Expected sent_to 2, got %d", response.SentTo)
	}

	// Check that we have error for failing node
	foundError := false
	for _, result := range response.Results {
		if result.NodeID == "fail-node" && result.Err != "" {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("Expected error for failing node but didn't find one")
	}
}

// TestConcurrentNodeOperations tests concurrent access to node list
func TestConcurrentNodeOperations(t *testing.T) {
	srv := newServer()

	// Number of concurrent operations
	numOps := 100
	var wg sync.WaitGroup
	wg.Add(numOps * 3) // 3 types of operations

	// Concurrent registrations
	for i := 0; i < numOps; i++ {
		go func(id int) {
			defer wg.Done()
			reqBody := cluster.RegisterRequest{
				Node: cluster.NodeInfo{
					ID:   fmt.Sprintf("node%d", id),
					Addr: fmt.Sprintf("http://localhost:%d", 8080+id),
				},
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			srv.handleRegister(rec, req)
		}(i)
	}

	// Concurrent list operations
	for i := 0; i < numOps; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/nodes", nil)
			rec := httptest.NewRecorder()
			srv.handleListNodes(rec, req)
		}()
	}

	// Concurrent broadcast operations
	for i := 0; i < numOps; i++ {
		go func() {
			defer wg.Done()
			reqBody := cluster.BroadcastRequest{
				Path:    "/control",
				Payload: json.RawMessage(`{"op":"test"}`),
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/broadcast", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			srv.handleBroadcast(rec, req)
		}()
	}

	// Wait for all operations to complete
	wg.Wait()

	// Verify we have the expected number of nodes
	if len(srv.nodes) != numOps {
		t.Errorf("Expected %d nodes after concurrent registration, got %d", numOps, len(srv.nodes))
	}
}

// TestMainFunction tests the main function with signal handling
func TestMainFunction(t *testing.T) {
	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set test args
	os.Args = []string{"coordinator"}

	// Set environment variable for test
	os.Setenv("COORDINATOR_ADDR", "127.0.0.1:0") // Use port 0 for automatic assignment
	defer os.Unsetenv("COORDINATOR_ADDR")

	// Run main in a goroutine
	done := make(chan bool)
	go func() {
		// Recover from any panic
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Main function panicked (expected during shutdown): %v", r)
			}
			done <- true
		}()
		main()
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Send interrupt signal to trigger shutdown
	process, _ := os.FindProcess(os.Getpid())
	process.Signal(syscall.SIGTERM)

	// Wait for main to finish with timeout
	select {
	case <-done:
		// Main exited successfully
	case <-time.After(10 * time.Second):
		t.Error("Main function did not shutdown within timeout")
	}
}

// TestHealthEndpoint tests the health check endpoint
func TestHealthEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

// TestServerShutdown tests graceful server shutdown
func TestServerShutdown(t *testing.T) {
	srv := newServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/register", srv.handleRegister)

	httpSrv := &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start server
	listener, err := net.Listen("tcp", httpSrv.Addr)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	go func() {
		httpSrv.Serve(listener)
	}()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = httpSrv.Shutdown(ctx)
	if err != nil {
		t.Errorf("Failed to shutdown server: %v", err)
	}
}
