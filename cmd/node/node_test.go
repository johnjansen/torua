package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dreamware/torua/internal/shard"
)

// TestNodeAddShard tests the AddShard method of Node
func TestNodeAddShard(t *testing.T) {
	tests := []struct {
		name        string
		shardID     int
		isPrimary   bool
		setupNode   func(*Node)
		checkResult func(*Node, int) error
	}{
		{
			name:      "add new primary shard",
			shardID:   1,
			isPrimary: true,
			setupNode: func(n *Node) {
				// Empty node, no shards
			},
			checkResult: func(n *Node, shardID int) error {
				if n.GetShard(shardID) == nil {
					return fmt.Errorf("shard %d was not added", shardID)
				}
				return nil
			},
		},
		{
			name:      "add new replica shard",
			shardID:   2,
			isPrimary: false,
			setupNode: func(n *Node) {
				// Empty node
			},
			checkResult: func(n *Node, shardID int) error {
				sh := n.GetShard(shardID)
				if sh == nil {
					return fmt.Errorf("shard %d was not added", shardID)
				}
				if sh.Primary {
					return fmt.Errorf("shard %d should be replica, not primary", shardID)
				}
				return nil
			},
		},
		{
			name:      "overwrite existing shard",
			shardID:   1,
			isPrimary: true,
			setupNode: func(n *Node) {
				// Pre-add shard 1 as replica
				n.AddShard(shard.NewShard(1, false))
			},
			checkResult: func(n *Node, shardID int) error {
				sh := n.GetShard(shardID)
				if sh == nil {
					return fmt.Errorf("shard %d was not found", shardID)
				}
				if !sh.Primary {
					return fmt.Errorf("shard %d should be primary after overwrite", shardID)
				}
				return nil
			},
		},
		{
			name:      "add multiple shards",
			shardID:   5,
			isPrimary: true,
			setupNode: func(n *Node) {
				n.AddShard(shard.NewShard(1, true))
				n.AddShard(shard.NewShard(2, false))
				n.AddShard(shard.NewShard(3, true))
			},
			checkResult: func(n *Node, shardID int) error {
				// Check all shards exist
				for _, id := range []int{1, 2, 3, 5} {
					if n.GetShard(id) == nil {
						return fmt.Errorf("shard %d not found", id)
					}
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNode("test-node")

			if tt.setupNode != nil {
				tt.setupNode(node)
			}

			// Add the shard
			sh := shard.NewShard(tt.shardID, tt.isPrimary)
			node.AddShard(sh)

			// Check result
			if err := tt.checkResult(node, tt.shardID); err != nil {
				t.Errorf("AddShard() check failed: %v", err)
			}
		})
	}
}

// TestNodeGetShard tests the GetShard method of Node
func TestNodeGetShard(t *testing.T) {
	tests := []struct {
		name      string
		shardID   int
		setupNode func(*Node)
		wantNil   bool
	}{
		{
			name:    "get existing shard",
			shardID: 1,
			setupNode: func(n *Node) {
				n.AddShard(shard.NewShard(1, true))
			},
			wantNil: false,
		},
		{
			name:    "get non-existent shard",
			shardID: 99,
			setupNode: func(n *Node) {
				n.AddShard(shard.NewShard(1, true))
			},
			wantNil: true,
		},
		{
			name:      "get shard from empty node",
			shardID:   0,
			setupNode: func(n *Node) {},
			wantNil:   true,
		},
		{
			name:    "get shard after adding multiple",
			shardID: 3,
			setupNode: func(n *Node) {
				n.AddShard(shard.NewShard(1, true))
				n.AddShard(shard.NewShard(2, false))
				n.AddShard(shard.NewShard(3, true))
				n.AddShard(shard.NewShard(4, false))
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNode("test-node")

			if tt.setupNode != nil {
				tt.setupNode(node)
			}

			sh := node.GetShard(tt.shardID)

			if tt.wantNil && sh != nil {
				t.Errorf("GetShard(%d) = %v, want nil", tt.shardID, sh)
			}
			if !tt.wantNil && sh == nil {
				t.Errorf("GetShard(%d) = nil, want shard", tt.shardID)
			}
		})
	}
}

// TestHandleShardRequest tests the HTTP handler for shard operations
func TestHandleShardRequest(t *testing.T) {
	// Create a test node
	node := NewNode("test-node")

	// Create handler function that captures the node
	handler := func(w http.ResponseWriter, r *http.Request) {
		handleShardRequest(node, w, r)
	}

	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		setupNode      func(*Node)
		wantStatusCode int
		wantBody       string
		checkBody      bool
	}{
		{
			name:   "GET existing key",
			method: http.MethodGet,
			path:   "/shard/1/store/test-key",
			setupNode: func(n *Node) {
				sh := shard.NewShard(1, true)
				sh.Put("test-key", []byte("test-value"))
				n.AddShard(sh)
			},
			wantStatusCode: http.StatusOK,
			wantBody:       "test-value",
			checkBody:      true,
		},
		{
			name:   "GET non-existent key",
			method: http.MethodGet,
			path:   "/shard/1/store/missing-key",
			setupNode: func(n *Node) {
				n.AddShard(shard.NewShard(1, true))
			},
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:           "PUT new key creates shard on demand",
			method:         http.MethodPut,
			path:           "/shard/2/store/new-key",
			body:           "new-value",
			setupNode:      func(n *Node) {},
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:   "PUT update existing key",
			method: http.MethodPut,
			path:   "/shard/1/store/test-key",
			body:   "updated-value",
			setupNode: func(n *Node) {
				sh := shard.NewShard(1, true)
				sh.Put("test-key", []byte("old-value"))
				n.AddShard(sh)
			},
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:   "DELETE existing key",
			method: http.MethodDelete,
			path:   "/shard/1/store/test-key",
			setupNode: func(n *Node) {
				sh := shard.NewShard(1, true)
				sh.Put("test-key", []byte("test-value"))
				n.AddShard(sh)
			},
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:   "DELETE non-existent key",
			method: http.MethodDelete,
			path:   "/shard/1/store/missing-key",
			setupNode: func(n *Node) {
				n.AddShard(shard.NewShard(1, true))
			},
			wantStatusCode: http.StatusNoContent, // Delete is idempotent, returns success even for non-existent keys
		},
		{
			name:   "GET list all keys",
			method: http.MethodGet,
			path:   "/shard/1/store",
			setupNode: func(n *Node) {
				sh := shard.NewShard(1, true)
				sh.Put("key1", []byte("value1"))
				sh.Put("key2", []byte("value2"))
				sh.Put("key3", []byte("value3"))
				n.AddShard(sh)
			},
			wantStatusCode: http.StatusOK,
			wantBody:       `{"keys":["key1","key2","key3"],"count":3}`,
			checkBody:      true,
		},
		{
			name:           "GET list keys from non-existent shard creates it",
			method:         http.MethodGet,
			path:           "/shard/99/store",
			setupNode:      func(n *Node) {},
			wantStatusCode: http.StatusOK,
			wantBody:       `{"keys":[],"count":0}`,
			checkBody:      true,
		},
		{
			name:           "GET shard stats",
			method:         http.MethodGet,
			path:           "/shard/1/stats",
			setupNode:      func(n *Node) {},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "GET shard info",
			method:         http.MethodGet,
			path:           "/shard/1",
			setupNode:      func(n *Node) {},
			wantStatusCode: http.StatusBadRequest, // Path without /store or /stats is invalid
		},
		{
			name:           "invalid shard ID",
			method:         http.MethodGet,
			path:           "/shard/invalid/store/key",
			setupNode:      func(n *Node) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:   "key with slashes",
			method: http.MethodPut,
			path:   "/shard/1/store/path/to/key",
			body:   "value-with-path",
			setupNode: func(n *Node) {
				// Shard will be created on demand
			},
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:   "GET key with slashes",
			method: http.MethodGet,
			path:   "/shard/1/store/path/to/key",
			setupNode: func(n *Node) {
				sh := shard.NewShard(1, true)
				sh.Put("path/to/key", []byte("value-with-path"))
				n.AddShard(sh)
			},
			wantStatusCode: http.StatusOK,
			wantBody:       "value-with-path",
			checkBody:      true,
		},
		{
			name:           "unsupported method",
			method:         http.MethodPost,
			path:           "/shard/1/store/key",
			setupNode:      func(n *Node) {},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset node state
			node.mu.Lock()
			node.shards = make(map[int]*shard.Shard)
			node.mu.Unlock()

			if tt.setupNode != nil {
				tt.setupNode(node)
			}

			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
			}

			if tt.checkBody {
				gotBody := strings.TrimSpace(rec.Body.String())
				wantBody := strings.TrimSpace(tt.wantBody)

				// For JSON responses, parse and compare
				if strings.HasPrefix(wantBody, "{") && strings.Contains(wantBody, "keys") {
					var gotResp, wantResp struct {
						Keys  []string `json:"keys"`
						Count int      `json:"count"`
					}
					if err := json.Unmarshal([]byte(gotBody), &gotResp); err == nil {
						if err := json.Unmarshal([]byte(wantBody), &wantResp); err == nil {
							if gotResp.Count != wantResp.Count {
								t.Errorf("count mismatch: got %d, want %d", gotResp.Count, wantResp.Count)
							}
							if len(gotResp.Keys) != len(wantResp.Keys) {
								t.Errorf("keys length mismatch: got %d, want %d", len(gotResp.Keys), len(wantResp.Keys))
							}
							// Keys might be in different order, so check presence
							for _, key := range wantResp.Keys {
								found := false
								for _, gk := range gotResp.Keys {
									if gk == key {
										found = true
										break
									}
								}
								if !found {
									t.Errorf("missing expected key: %s", key)
								}
							}
							return
						}
					}
				}

				if gotBody != wantBody {
					t.Errorf("body = %s, want %s", gotBody, wantBody)
				}
			}
		})
	}
}

// TestConcurrentShardOperations tests concurrent access to shards
func TestConcurrentShardOperations(t *testing.T) {
	node := NewNode("test-node")

	// Number of concurrent operations
	numOps := 100
	numShards := 5

	var wg sync.WaitGroup

	// Concurrent shard additions
	wg.Add(numShards)
	for i := 0; i < numShards; i++ {
		go func(shardID int) {
			defer wg.Done()
			sh := shard.NewShard(shardID, true)
			node.AddShard(sh)
		}(i)
	}
	wg.Wait()

	// Verify all shards were added
	for i := 0; i < numShards; i++ {
		if node.GetShard(i) == nil {
			t.Errorf("shard %d was not added", i)
		}
	}

	// Concurrent read/write operations on shards
	wg.Add(numOps * 3)

	// Concurrent PUTs
	for i := 0; i < numOps; i++ {
		go func(i int) {
			defer wg.Done()
			shardID := i % numShards
			sh := node.GetShard(shardID)
			if sh != nil {
				key := fmt.Sprintf("key-%d", i)
				value := fmt.Sprintf("value-%d", i)
				sh.Put(key, []byte(value))
			}
		}(i)
	}

	// Concurrent GETs
	for i := 0; i < numOps; i++ {
		go func(i int) {
			defer wg.Done()
			shardID := i % numShards
			sh := node.GetShard(shardID)
			if sh != nil {
				key := fmt.Sprintf("key-%d", i)
				sh.Get(key) // Result doesn't matter for concurrency test
			}
		}(i)
	}

	// Concurrent GetShard calls
	for i := 0; i < numOps; i++ {
		go func(i int) {
			defer wg.Done()
			shardID := i % numShards
			node.GetShard(shardID)
		}(i)
	}

	wg.Wait()

	// Verify data integrity - at least some keys should exist
	keyCount := 0
	for i := 0; i < numShards; i++ {
		sh := node.GetShard(i)
		if sh != nil {
			keys := sh.ListKeys()
			keyCount += len(keys)
		}
	}

	if keyCount == 0 {
		t.Errorf("no keys were stored despite %d PUT operations", numOps)
	}
}

// TestNodeInfo tests the node info endpoint
func TestNodeInfo(t *testing.T) {
	node := NewNode("test-node")
	node.AddShard(shard.NewShard(1, true))
	node.AddShard(shard.NewShard(2, false))
	node.AddShard(shard.NewShard(3, true))

	handler := func(w http.ResponseWriter, r *http.Request) {
		handleNodeInfo(node, w, r)
	}

	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	// Check response structure based on actual handleNodeInfo implementation
	var info struct {
		NodeID string            `json:"node_id"`
		Shards []shard.ShardInfo `json:"shards"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if info.NodeID != "test-node" {
		t.Errorf("node ID = %s, want test-node", info.NodeID)
	}

	if len(info.Shards) != 3 {
		t.Errorf("shard count = %d, want 3", len(info.Shards))
	}
}

// TestLargeDataHandling tests handling of large values
func TestLargeDataHandling(t *testing.T) {
	node := NewNode("test-node")
	sh := shard.NewShard(1, true)
	node.AddShard(sh)

	// Test with 1MB value
	largeValue := bytes.Repeat([]byte("x"), 1024*1024)
	key := "large-key"

	// Store large value
	err := sh.Put(key, largeValue)
	if err != nil {
		t.Fatalf("failed to store large value: %v", err)
	}

	// Retrieve large value
	retrieved, err := sh.Get(key)
	if err != nil {
		t.Fatalf("failed to retrieve large value: %v", err)
	}

	if !bytes.Equal(retrieved, largeValue) {
		t.Errorf("retrieved value doesn't match original (size: got %d, want %d)",
			len(retrieved), len(largeValue))
	}
}

// TestSpecialCharacterKeys tests keys with special characters
func TestSpecialCharacterKeys(t *testing.T) {
	node := NewNode("test-node")
	sh := shard.NewShard(1, true)
	node.AddShard(sh)

	specialKeys := []string{
		"key-with-dash",
		"key_with_underscore",
		"key.with.dots",
		"key:with:colons",
		"key@with@at",
		"key#with#hash",
		"key$with$dollar",
		"key%with%percent",
		"key&with&ampersand",
		"key=with=equals",
		"key+with+plus",
		"key with spaces",
		"key/with/slashes",
		"key\\with\\backslashes",
		"key|with|pipes",
		"key?with?questions",
		"key*with*asterisks",
		"key(with)parens",
		"key[with]brackets",
		"key{with}braces",
		"key<with>angles",
		"key'with'quotes",
		`key"with"doublequotes`,
		"key`with`backticks",
		"key~with~tilde",
		"key!with!exclamation",
		"key;with;semicolon",
		"key,with,comma",
	}

	for _, key := range specialKeys {
		t.Run(fmt.Sprintf("key=%s", key), func(t *testing.T) {
			value := fmt.Sprintf("value-for-%s", key)

			// Store
			if err := sh.Put(key, []byte(value)); err != nil {
				t.Errorf("failed to store key %s: %v", key, err)
			}

			// Retrieve
			retrieved, err := sh.Get(key)
			if err != nil {
				t.Errorf("failed to retrieve key %s: %v", key, err)
			}

			if string(retrieved) != value {
				t.Errorf("value mismatch for key %s: got %s, want %s", key, retrieved, value)
			}

			// Delete
			if err := sh.Delete(key); err != nil {
				t.Errorf("failed to delete key %s: %v", key, err)
			}

			// Verify deletion
			if _, err := sh.Get(key); err == nil {
				t.Errorf("key %s should have been deleted", key)
			}
		})
	}
}
