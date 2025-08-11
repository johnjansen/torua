package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamware/torua/internal/cluster"
	"github.com/dreamware/torua/internal/coordinator"
)

// TestMarkNodeUnhealthy tests the markNodeUnhealthy function
func TestMarkNodeUnhealthy(t *testing.T) {
	tests := []struct {
		name         string
		initialNodes []cluster.NodeInfo
		nodeID       string
		wantNodes    int
		wantStatus   string
	}{
		{
			name: "mark existing node as unhealthy",
			initialNodes: []cluster.NodeInfo{
				{ID: "node1", Addr: "http://localhost:8081", Status: "healthy"},
				{ID: "node2", Addr: "http://localhost:8082", Status: "healthy"},
			},
			nodeID:     "node1",
			wantNodes:  2,
			wantStatus: healthStatusUnhealthy,
		},
		{
			name: "mark non-existent node",
			initialNodes: []cluster.NodeInfo{
				{ID: "node1", Addr: "http://localhost:8081", Status: "healthy"},
			},
			nodeID:    "node3",
			wantNodes: 1,
		},
		{
			name: "already unhealthy node",
			initialNodes: []cluster.NodeInfo{
				{ID: "node1", Addr: "http://localhost:8081", Status: healthStatusUnhealthy},
			},
			nodeID:     "node1",
			wantNodes:  1,
			wantStatus: healthStatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			srv.nodes = tt.initialNodes

			srv.markNodeUnhealthy(tt.nodeID)

			if len(srv.nodes) != tt.wantNodes {
				t.Errorf("nodes count = %d, want %d", len(srv.nodes), tt.wantNodes)
			}

			// Check if the node was marked unhealthy
			for _, node := range srv.nodes {
				if node.ID == tt.nodeID && tt.wantStatus != "" {
					if node.Status != tt.wantStatus {
						t.Errorf("node status = %s, want %s", node.Status, tt.wantStatus)
					}
				}
			}
		})
	}
}

// TestHandleData tests the data routing handler
func TestHandleData(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		setupServer    func(*server)
		wantStatusCode int
	}{
		{
			name:   "GET request with valid key",
			method: http.MethodGet,
			path:   "/data/test-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // test-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
				}
			},
			wantStatusCode: http.StatusBadGateway, // No actual node to forward to
		},
		{
			name:   "PUT request with valid key",
			method: http.MethodPut,
			path:   "/data/test-key",
			body:   "test-value",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // test-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
				}
			},
			wantStatusCode: http.StatusBadGateway,
		},
		{
			name:   "DELETE request with valid key",
			method: http.MethodDelete,
			path:   "/data/test-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // test-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
				}
			},
			wantStatusCode: http.StatusBadGateway,
		},
		{
			name:           "unsupported method POST",
			method:         http.MethodPost,
			path:           "/data/test-key",
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:           "unsupported method HEAD",
			method:         http.MethodHead,
			path:           "/data/test-key",
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:           "missing key in path",
			method:         http.MethodGet,
			path:           "/data/",
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "missing key in path (just /data)",
			method:         http.MethodGet,
			path:           "/data",
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:   "key with slashes",
			method: http.MethodGet,
			path:   "/data/path/to/key",
			setupServer: func(s *server) {
				s.registry.AssignShard(2, "node1", true) // path/to/key hashes to shard 2
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
				}
			},
			wantStatusCode: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			if tt.setupServer != nil {
				tt.setupServer(srv)
			}

			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			rec := httptest.NewRecorder()

			srv.handleData(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
				t.Errorf("response body: %s", rec.Body.String())
			}
		})
	}
}

// TestHandleShards tests the shard listing handler
func TestHandleShards(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		setupServer    func(*server)
		wantStatusCode int
		wantShards     int
		wantNumShards  int
	}{
		{
			name:   "GET shards successfully with assignments",
			method: http.MethodGet,
			setupServer: func(s *server) {
				s.registry.AssignShard(0, "node1", true)
				s.registry.AssignShard(1, "node2", true)
				s.registry.AssignShard(2, "node1", false)
			},
			wantStatusCode: 200,
			wantShards:     3,
			wantNumShards:  4, // Default shard count
		},
		{
			name:           "GET shards with no assignments",
			method:         http.MethodGet,
			setupServer:    func(s *server) {},
			wantStatusCode: 200,
			wantShards:     0,
			wantNumShards:  4,
		},
		{
			name:           "unsupported method POST",
			method:         http.MethodPost,
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:           "unsupported method PUT",
			method:         http.MethodPut,
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:           "unsupported method DELETE",
			method:         http.MethodDelete,
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			if tt.setupServer != nil {
				tt.setupServer(srv)
			}

			req := httptest.NewRequest(tt.method, "/shards", nil)
			rec := httptest.NewRecorder()

			srv.handleShards(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if rec.Code == http.StatusOK {
				var resp struct {
					Shards    []*coordinator.ShardAssignment `json:"shards"`
					NumShards int                            `json:"num_shards"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(resp.Shards) != tt.wantShards {
					t.Errorf("shards count = %d, want %d", len(resp.Shards), tt.wantShards)
				}
				if resp.NumShards != tt.wantNumShards {
					t.Errorf("num_shards = %d, want %d", resp.NumShards, tt.wantNumShards)
				}
			}
		})
	}
}

// TestHandleShardAssign tests manual shard assignment
func TestHandleShardAssign(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           string
		setupServer    func(*server)
		wantStatusCode int
		checkResult    func(*server) error
	}{
		{
			name:   "successful primary shard assignment",
			method: http.MethodPost,
			body:   `{"shard_id": 0, "node_id": "node1", "is_primary": true}`,
			setupServer: func(s *server) {
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
				}
			},
			wantStatusCode: http.StatusOK,
			checkResult: func(s *server) error {
				assignment := s.registry.GetAssignment(0)
				if assignment == nil {
					return io.EOF
				}
				if assignment.NodeID != "node1" {
					return io.ErrUnexpectedEOF
				}
				return nil
			},
		},
		{
			name:   "successful replica shard assignment",
			method: http.MethodPost,
			body:   `{"shard_id": 1, "node_id": "node2", "is_primary": false}`,
			setupServer: func(s *server) {
				s.nodes = []cluster.NodeInfo{
					{ID: "node2", Addr: "http://localhost:8082"},
				}
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPost,
			body:           `{invalid json}`,
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "empty body",
			method:         http.MethodPost,
			body:           ``,
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:   "invalid shard ID (negative)",
			method: http.MethodPost,
			body:   `{"shard_id": -1, "node_id": "node1", "is_primary": true}`,
			setupServer: func(s *server) {
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
				}
			},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:   "invalid shard ID (too large)",
			method: http.MethodPost,
			body:   `{"shard_id": 999, "node_id": "node1", "is_primary": true}`,
			setupServer: func(s *server) {
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
				}
			},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "non-existent node",
			method:         http.MethodPost,
			body:           `{"shard_id": 0, "node_id": "non-existent", "is_primary": true}`,
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "empty node ID",
			method:         http.MethodPost,
			body:           `{"shard_id": 0, "node_id": "", "is_primary": true}`,
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "unsupported method GET",
			method:         http.MethodGet,
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:           "unsupported method PUT",
			method:         http.MethodPut,
			setupServer:    func(s *server) {},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			if tt.setupServer != nil {
				tt.setupServer(srv)
			}

			req := httptest.NewRequest(tt.method, "/shards/assign", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			srv.handleShardAssign(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if tt.checkResult != nil {
				if err := tt.checkResult(srv); err != nil {
					t.Errorf("result check failed: %v", err)
				}
			}
		})
	}
}

// TestForwardGet tests GET request forwarding
func TestForwardGet(t *testing.T) {
	// Create a mock node server
	mockNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/shards/3/data/test-key" {
			w.Write([]byte("test-value"))
		} else if r.URL.Path == "/shards/3/data/error-key" {
			http.Error(w, "internal error", http.StatusInternalServerError)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer mockNode.Close()

	tests := []struct {
		name           string
		key            string
		setupServer    func(*server)
		wantStatusCode int
		wantBody       string
	}{
		{
			name: "successful GET",
			key:  "test-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // test-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: mockNode.URL},
				}
			},
			wantStatusCode: http.StatusOK,
			wantBody:       "test-value",
		},
		{
			name: "key not found",
			key:  "missing-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(1, "node1", true) // missing-key hashes to shard 1
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: mockNode.URL},
				}
			},
			wantStatusCode: http.StatusNotFound,
		},
		{
			name: "node returns error",
			key:  "error-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // error-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: mockNode.URL},
				}
			},
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name: "no node assigned to shard",
			key:  "test-key",
			setupServer: func(s *server) {
				// No shard assignment
			},
			wantStatusCode: http.StatusServiceUnavailable,
		},
		{
			name: "node not found in cluster",
			key:  "test-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(0, "node2", true)
				// node2 doesn't exist in nodes list
			},
			wantStatusCode: http.StatusServiceUnavailable,
		},
		{
			name: "node with invalid URL",
			key:  "test-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // test-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "not-a-valid-url"},
				}
			},
			wantStatusCode: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			if tt.setupServer != nil {
				tt.setupServer(srv)
			}

			req := httptest.NewRequest(http.MethodGet, "/data/"+tt.key, nil)
			rec := httptest.NewRecorder()

			// Build target URL using registry
			shardID := srv.registry.GetShardForKey(tt.key)
			nodeID, _ := srv.registry.GetNodeForKey(tt.key)
			targetURL := ""
			for _, node := range srv.nodes {
				if node.ID == nodeID {
					targetURL = fmt.Sprintf("%s/shards/%d/data/%s", node.Addr, shardID, tt.key)
					break
				}
			}
			srv.forwardGet(targetURL, rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %s, want %s", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestForwardPut tests PUT request forwarding
func TestForwardPut(t *testing.T) {
	var receivedBody []byte
	// Create a mock node server
	mockNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/shards/3/data/test-key" {
			body, _ := io.ReadAll(r.Body)
			receivedBody = body
			w.WriteHeader(http.StatusOK)
		} else if r.Method == http.MethodPut && r.URL.Path == "/shards/3/data/error-key" {
			http.Error(w, "storage full", http.StatusInsufficientStorage)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer mockNode.Close()

	tests := []struct {
		name           string
		key            string
		value          string
		setupServer    func(*server)
		wantStatusCode int
		checkBody      bool
	}{
		{
			name:  "successful PUT",
			key:   "test-key",
			value: "test-value",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // test-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: mockNode.URL},
				}
			},
			wantStatusCode: http.StatusOK,
			checkBody:      true,
		},
		{
			name:  "PUT with empty value",
			key:   "test-key",
			value: "",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // test-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: mockNode.URL},
				}
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:  "node returns error",
			key:   "error-key",
			value: "test-value",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // error-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: mockNode.URL},
				}
			},
			wantStatusCode: http.StatusInsufficientStorage,
		},
		{
			name:  "no node assigned to shard",
			key:   "test-key",
			value: "test-value",
			setupServer: func(s *server) {
				// No shard assignment
			},
			wantStatusCode: http.StatusServiceUnavailable,
		},
		{
			name:  "node not found in cluster",
			key:   "test-key",
			value: "test-value",
			setupServer: func(s *server) {
				s.registry.AssignShard(0, "node2", true)
				// node2 doesn't exist in nodes list
			},
			wantStatusCode: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			receivedBody = nil
			if tt.setupServer != nil {
				tt.setupServer(srv)
			}

			req := httptest.NewRequest(http.MethodPut, "/data/"+tt.key, strings.NewReader(tt.value))
			rec := httptest.NewRecorder()

			// Build target URL using registry
			shardID := srv.registry.GetShardForKey(tt.key)
			nodeID, _ := srv.registry.GetNodeForKey(tt.key)
			targetURL := ""
			for _, node := range srv.nodes {
				if node.ID == nodeID {
					targetURL = fmt.Sprintf("%s/shards/%d/data/%s", node.Addr, shardID, tt.key)
					break
				}
			}
			srv.forwardPut(targetURL, rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if tt.checkBody && string(receivedBody) != tt.value {
				t.Errorf("forwarded body = %s, want %s", string(receivedBody), tt.value)
			}
		})
	}
}

// TestForwardDelete tests DELETE request forwarding
func TestForwardDelete(t *testing.T) {
	var deletedPath string
	// Create a mock node server
	mockNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/shards/") {
			deletedPath = r.URL.Path
			if r.URL.Path == "/shards/3/data/test-key" {
				w.WriteHeader(http.StatusOK)
			} else if r.URL.Path == "/shards/3/data/protected-key" {
				http.Error(w, "forbidden", http.StatusForbidden)
			} else {
				http.NotFound(w, r)
			}
		}
	}))
	defer mockNode.Close()

	tests := []struct {
		name           string
		key            string
		setupServer    func(*server)
		wantStatusCode int
		wantPath       string
	}{
		{
			name: "successful DELETE",
			key:  "test-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // test-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: mockNode.URL},
				}
			},
			wantStatusCode: http.StatusOK,
			wantPath:       "/shards/3/data/test-key",
		},
		{
			name: "key not found",
			key:  "missing-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(1, "node1", true) // missing-key hashes to shard 1
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: mockNode.URL},
				}
			},
			wantStatusCode: http.StatusNotFound,
		},
		{
			name: "protected key returns forbidden",
			key:  "protected-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(3, "node1", true) // protected-key hashes to shard 3
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: mockNode.URL},
				}
			},
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "no node assigned to shard",
			key:  "test-key",
			setupServer: func(s *server) {
				// No shard assignment
			},
			wantStatusCode: http.StatusServiceUnavailable,
		},
		{
			name: "node not found in cluster",
			key:  "test-key",
			setupServer: func(s *server) {
				s.registry.AssignShard(0, "node2", true)
				// node2 doesn't exist in nodes list
			},
			wantStatusCode: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			deletedPath = ""
			if tt.setupServer != nil {
				tt.setupServer(srv)
			}

			req := httptest.NewRequest(http.MethodDelete, "/data/"+tt.key, nil)
			rec := httptest.NewRecorder()

			// Build target URL using registry
			shardID := srv.registry.GetShardForKey(tt.key)
			nodeID, _ := srv.registry.GetNodeForKey(tt.key)
			targetURL := ""
			for _, node := range srv.nodes {
				if node.ID == nodeID {
					targetURL = fmt.Sprintf("%s/shards/%d/data/%s", node.Addr, shardID, tt.key)
					break
				}
			}
			srv.forwardDelete(targetURL, rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if tt.wantPath != "" && deletedPath != tt.wantPath {
				t.Errorf("deleted path = %s, want %s", deletedPath, tt.wantPath)
			}
		})
	}
}

// TestAutoAssignShards tests automatic shard assignment
func TestAutoAssignShards(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(*server)
		wantShards  map[string]int // nodeID -> shard count
	}{
		{
			name: "single node gets all shards",
			setupServer: func(s *server) {
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
				}
			},
			wantShards: map[string]int{
				"node1": 4, // Default 4 shards
			},
		},
		{
			name: "two nodes share shards evenly",
			setupServer: func(s *server) {
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
					{ID: "node2", Addr: "http://localhost:8082"},
				}
			},
			wantShards: map[string]int{
				"node1": 2,
				"node2": 2,
			},
		},
		{
			name: "three nodes distribute shards",
			setupServer: func(s *server) {
				s.nodes = []cluster.NodeInfo{
					{ID: "node1", Addr: "http://localhost:8081"},
					{ID: "node2", Addr: "http://localhost:8082"},
					{ID: "node3", Addr: "http://localhost:8083"},
				}
			},
			wantShards: map[string]int{
				// With 4 shards and 3 nodes, distribution is 2-1-1
				"node1": 2,
				"node2": 1,
				"node3": 1,
			},
		},
		{
			name:        "no nodes means no assignments",
			setupServer: func(s *server) {},
			wantShards:  map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			if tt.setupServer != nil {
				tt.setupServer(srv)
			}

			srv.autoAssignShards()

			// Count shards per node
			shardCounts := make(map[string]int)
			assignments := srv.registry.GetAllAssignments()
			for _, assignment := range assignments {
				if assignment.IsPrimary {
					shardCounts[assignment.NodeID]++
				}
			}

			// Verify counts match expectations
			for nodeID, expectedCount := range tt.wantShards {
				if shardCounts[nodeID] != expectedCount {
					t.Errorf("node %s has %d shards, want %d", nodeID, shardCounts[nodeID], expectedCount)
				}
			}

			// Verify no unexpected nodes have shards
			for nodeID, count := range shardCounts {
				if _, expected := tt.wantShards[nodeID]; !expected {
					t.Errorf("unexpected node %s has %d shards", nodeID, count)
				}
			}
		})
	}
}

// TestForwardRequestBodyHandling tests that request bodies are properly forwarded
func TestForwardRequestBodyHandling(t *testing.T) {
	var capturedBody []byte
	var capturedContentType string

	mockNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer mockNode.Close()

	tests := []struct {
		name        string
		body        []byte
		contentType string
		key         string
	}{
		{
			name:        "JSON body",
			body:        []byte(`{"name":"test","value":123}`),
			contentType: "application/json",
			key:         "json-key",
		},
		{
			name:        "Plain text body",
			body:        []byte("plain text content"),
			contentType: "text/plain",
			key:         "text-key",
		},
		{
			name:        "Binary body",
			body:        []byte{0x89, 0x50, 0x4E, 0x47}, // PNG header
			contentType: "image/png",
			key:         "binary-key",
		},
		{
			name:        "Empty body with content type",
			body:        []byte{},
			contentType: "application/octet-stream",
			key:         "empty-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer()
			// Assign correct shards based on key hashing
			switch tt.key {
			case "json-key":
				srv.registry.AssignShard(1, "node1", true) // json-key hashes to shard 1
			case "text-key", "empty-key":
				srv.registry.AssignShard(0, "node1", true) // text-key and empty-key hash to shard 0
			case "binary-key":
				srv.registry.AssignShard(2, "node1", true) // binary-key hashes to shard 2
			}
			srv.nodes = []cluster.NodeInfo{
				{ID: "node1", Addr: mockNode.URL},
			}

			capturedBody = nil
			capturedContentType = ""

			req := httptest.NewRequest(http.MethodPut, "/data/"+tt.key, bytes.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rec := httptest.NewRecorder()

			// Build target URL using registry
			shardID := srv.registry.GetShardForKey(tt.key)
			targetURL := fmt.Sprintf("%s/shards/%d/data/%s", mockNode.URL, shardID, tt.key)
			srv.forwardPut(targetURL, rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
			}

			if !bytes.Equal(capturedBody, tt.body) {
				t.Errorf("forwarded body = %v, want %v", capturedBody, tt.body)
			}

			if capturedContentType != tt.contentType {
				t.Errorf("forwarded Content-Type = %s, want %s", capturedContentType, tt.contentType)
			}
		})
	}
}
