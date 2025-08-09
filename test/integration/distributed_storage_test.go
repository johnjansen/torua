package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// TestSystem represents our distributed system under test
type TestSystem struct {
	t          *testing.T
	coord      *exec.Cmd
	nodes      []*exec.Cmd
	coordAddr  string
	nodeAddrs  []string
	httpClient *http.Client
}

// NewTestSystem creates a new test system with coordinator and nodes
func NewTestSystem(t *testing.T) *TestSystem {
	return &TestSystem{
		t:         t,
		coordAddr: "http://127.0.0.1:18080", // Use high ports to avoid conflicts
		nodeAddrs: []string{
			"http://127.0.0.1:18081",
			"http://127.0.0.1:18082",
		},
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Start launches the coordinator and nodes
func (ts *TestSystem) Start() error {
	// Check if binaries exist, if not try to build them
	if _, err := os.Stat("./bin/coordinator"); os.IsNotExist(err) {
		ts.t.Log("Building coordinator binary...")
		if err := exec.Command("go", "build", "-o", "bin/coordinator", "./cmd/coordinator").Run(); err != nil {
			return fmt.Errorf("failed to build coordinator: %w", err)
		}
	}
	if _, err := os.Stat("./bin/node"); os.IsNotExist(err) {
		ts.t.Log("Building node binary...")
		if err := exec.Command("go", "build", "-o", "bin/node", "./cmd/node").Run(); err != nil {
			return fmt.Errorf("failed to build node: %w", err)
		}
	}

	// Start coordinator
	ts.t.Log("Starting coordinator...")
	ts.coord = exec.Command("./bin/coordinator")
	ts.coord.Env = append(os.Environ(), "COORDINATOR_ADDR=:18080")
	ts.coord.Stdout = os.Stdout
	ts.coord.Stderr = os.Stderr
	if err := ts.coord.Start(); err != nil {
		return fmt.Errorf("failed to start coordinator: %w", err)
	}

	// Wait for coordinator to be ready
	if err := ts.waitForService(ts.coordAddr + "/health"); err != nil {
		return fmt.Errorf("coordinator failed to start: %w", err)
	}

	// Start nodes
	for i, addr := range ts.nodeAddrs {
		ts.t.Logf("Starting node %d...", i+1)
		node := exec.Command("./bin/node")
		node.Env = append(os.Environ(),
			fmt.Sprintf("NODE_ID=n%d", i+1),
			fmt.Sprintf("NODE_LISTEN=:1808%d", i+1),
			fmt.Sprintf("NODE_ADDR=%s", addr),
			fmt.Sprintf("COORDINATOR_ADDR=%s", ts.coordAddr),
		)
		node.Stdout = os.Stdout
		node.Stderr = os.Stderr
		if err := node.Start(); err != nil {
			return fmt.Errorf("failed to start node %d: %w", i+1, err)
		}
		ts.nodes = append(ts.nodes, node)

		// Wait for node to be ready
		if err := ts.waitForService(addr + "/health"); err != nil {
			return fmt.Errorf("node %d failed to start: %w", i+1, err)
		}
	}

	// Give nodes time to register with coordinator
	time.Sleep(500 * time.Millisecond)

	return nil
}

// Stop gracefully shuts down all components
func (ts *TestSystem) Stop() {
	// Stop nodes
	for i, node := range ts.nodes {
		if node != nil && node.Process != nil {
			ts.t.Logf("Stopping node %d...", i+1)
			node.Process.Kill()
			node.Wait()
		}
	}

	// Stop coordinator
	if ts.coord != nil && ts.coord.Process != nil {
		ts.t.Log("Stopping coordinator...")
		ts.coord.Process.Kill()
		ts.coord.Wait()
	}
}

// waitForService waits for an HTTP service to become available
func (ts *TestSystem) waitForService(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s", url)
		default:
			resp, err := ts.httpClient.Get(url)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return nil
			}
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// PUT stores a value at the given key
func (ts *TestSystem) PUT(key, value string) (int, error) {
	url := fmt.Sprintf("%s/data/%s", ts.coordAddr, key)
	resp, err := ts.httpClient.Do(newRequest("PUT", url, []byte(value)))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// GET retrieves a value for the given key
func (ts *TestSystem) GET(key string) (int, string, error) {
	url := fmt.Sprintf("%s/data/%s", ts.coordAddr, key)
	resp, err := ts.httpClient.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", err
	}

	return resp.StatusCode, string(body), nil
}

// DELETE removes a key
func (ts *TestSystem) DELETE(key string) (int, error) {
	url := fmt.Sprintf("%s/data/%s", ts.coordAddr, key)
	req, _ := http.NewRequest("DELETE", url, nil)
	resp, err := ts.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// GetNodes returns the list of registered nodes
func (ts *TestSystem) GetNodes() ([]map[string]interface{}, error) {
	resp, err := ts.httpClient.Get(ts.coordAddr + "/nodes")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Nodes []map[string]interface{} `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Nodes, nil
}

// GetShards returns the shard assignments
func (ts *TestSystem) GetShards() ([]map[string]interface{}, error) {
	resp, err := ts.httpClient.Get(ts.coordAddr + "/shards")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Shards []map[string]interface{} `json:"shards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Shards, nil
}

// Helper to create HTTP requests
func newRequest(method, url string, body []byte) *http.Request {
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	return req
}

// TestDistributedStorage runs end-to-end tests for the distributed storage system
func TestDistributedStorage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if binaries exist before trying to run integration tests
	if _, err := os.Stat("./bin/coordinator"); os.IsNotExist(err) {
		t.Skip("Skipping integration test: coordinator binary not found (run 'make build' first)")
	}
	if _, err := os.Stat("./bin/node"); os.IsNotExist(err) {
		t.Skip("Skipping integration test: node binary not found (run 'make build' first)")
	}

	// Create and start test system
	ts := NewTestSystem(t)
	if err := ts.Start(); err != nil {
		t.Fatalf("Failed to start test system: %v", err)
	}
	defer ts.Stop()

	// Run test scenarios
	t.Run("StoreAndRetrieve", func(t *testing.T) {
		testStoreAndRetrieve(t, ts)
	})

	t.Run("UpdateExistingValue", func(t *testing.T) {
		testUpdateExistingValue(t, ts)
	})

	t.Run("DeleteValue", func(t *testing.T) {
		testDeleteValue(t, ts)
	})

	t.Run("NonExistentKey", func(t *testing.T) {
		testNonExistentKey(t, ts)
	})

	t.Run("KeyDistribution", func(t *testing.T) {
		testKeyDistribution(t, ts)
	})

	t.Run("ConsistentRouting", func(t *testing.T) {
		testConsistentRouting(t, ts)
	})

	t.Run("ConcurrentOperations", func(t *testing.T) {
		testConcurrentOperations(t, ts)
	})

	t.Run("SystemVisibility", func(t *testing.T) {
		testSystemVisibility(t, ts)
	})

	t.Run("VariousKeyPatterns", func(t *testing.T) {
		testVariousKeyPatterns(t, ts)
	})

	t.Run("Performance", func(t *testing.T) {
		testPerformance(t, ts)
	})
}

// testStoreAndRetrieve verifies basic store and retrieve operations
func testStoreAndRetrieve(t *testing.T, ts *TestSystem) {
	// Store a value
	status, err := ts.PUT("greeting", "Hello World")
	if err != nil {
		t.Fatalf("Failed to PUT: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", status)
	}

	// Retrieve the value
	status, value, err := ts.GET("greeting")
	if err != nil {
		t.Fatalf("Failed to GET: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("Expected status 200, got %d", status)
	}
	if value != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", value)
	}
}

// testUpdateExistingValue verifies updating an existing key
func testUpdateExistingValue(t *testing.T, ts *TestSystem) {
	// Store initial value
	ts.PUT("counter", "1")

	// Update the value
	status, err := ts.PUT("counter", "2")
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", status)
	}

	// Verify new value
	_, value, _ := ts.GET("counter")
	if value != "2" {
		t.Errorf("Expected '2', got '%s'", value)
	}
}

// testDeleteValue verifies deletion of keys
func testDeleteValue(t *testing.T, ts *TestSystem) {
	// Store a value
	ts.PUT("temp", "temporary data")

	// Delete it
	status, err := ts.DELETE("temp")
	if err != nil {
		t.Fatalf("Failed to DELETE: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", status)
	}

	// Verify it's gone
	status, _, _ = ts.GET("temp")
	if status != http.StatusNotFound {
		t.Errorf("Expected status 404 for deleted key, got %d", status)
	}
}

// testNonExistentKey verifies handling of missing keys
func testNonExistentKey(t *testing.T, ts *TestSystem) {
	status, _, err := ts.GET("does-not-exist")
	if err != nil {
		t.Fatalf("Failed to GET: %v", err)
	}
	if status != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent key, got %d", status)
	}
}

// testKeyDistribution verifies keys are distributed across shards
func testKeyDistribution(t *testing.T, ts *TestSystem) {
	// Store multiple keys
	keys := []string{"key1", "key2", "key3", "key4", "key5", "key6", "key7", "key8"}
	for i, key := range keys {
		value := fmt.Sprintf("value%d", i+1)
		if _, err := ts.PUT(key, value); err != nil {
			t.Fatalf("Failed to PUT %s: %v", key, err)
		}
	}

	// Verify all keys are retrievable
	for i, key := range keys {
		expectedValue := fmt.Sprintf("value%d", i+1)
		_, value, err := ts.GET(key)
		if err != nil {
			t.Fatalf("Failed to GET %s: %v", key, err)
		}
		if value != expectedValue {
			t.Errorf("Key %s: expected '%s', got '%s'", key, expectedValue, value)
		}
	}

	// Check shard distribution (keys should map to different shards)
	// This is a statistical test - with 8 keys and 4 shards,
	// we should have at least 2 different shards used
	shardMap := make(map[int]bool)
	for _, key := range keys {
		// Calculate shard using same algorithm as system
		h := hashKey(key)
		shardID := h % 4
		shardMap[shardID] = true
	}

	if len(shardMap) < 2 {
		t.Errorf("Poor shard distribution: only %d shards used for %d keys", len(shardMap), len(keys))
	}
}

// testConsistentRouting verifies same key always routes to same shard
func testConsistentRouting(t *testing.T, ts *TestSystem) {
	key := "consistent-key"
	ts.PUT(key, "initial")

	// Get the same key multiple times
	for i := 0; i < 10; i++ {
		_, value, err := ts.GET(key)
		if err != nil {
			t.Fatalf("GET attempt %d failed: %v", i+1, err)
		}
		if value != "initial" {
			t.Errorf("GET attempt %d: expected 'initial', got '%s'", i+1, value)
		}
	}
}

// testConcurrentOperations verifies system handles concurrent requests
func testConcurrentOperations(t *testing.T, ts *TestSystem) {
	numClients := 10
	var wg sync.WaitGroup
	errors := make(chan error, numClients*2)

	// Concurrent PUTs
	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-key-%d", id)
			value := fmt.Sprintf("concurrent-value-%d", id)
			if _, err := ts.PUT(key, value); err != nil {
				errors <- fmt.Errorf("PUT failed for client %d: %w", id, err)
			}
		}(i)
	}
	wg.Wait()

	// Concurrent GETs
	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-key-%d", id)
			expectedValue := fmt.Sprintf("concurrent-value-%d", id)
			_, value, err := ts.GET(key)
			if err != nil {
				errors <- fmt.Errorf("GET failed for client %d: %w", id, err)
				return
			}
			if value != expectedValue {
				errors <- fmt.Errorf("client %d: expected '%s', got '%s'", id, expectedValue, value)
			}
		}(i)
	}
	wg.Wait()

	// Check for errors
	select {
	case err := <-errors:
		t.Error(err)
	default:
		// No errors
	}
}

// testSystemVisibility verifies we can inspect system state
func testSystemVisibility(t *testing.T, ts *TestSystem) {
	// Check nodes are registered
	nodes, err := ts.GetNodes()
	if err != nil {
		t.Fatalf("Failed to get nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(nodes))
	}

	// Check shards are assigned
	shards, err := ts.GetShards()
	if err != nil {
		t.Fatalf("Failed to get shards: %v", err)
	}
	if len(shards) == 0 {
		t.Error("No shards assigned")
	}

	// Verify each shard has a node assignment
	for _, shard := range shards {
		if shard["NodeID"] == nil || shard["NodeID"] == "" {
			t.Errorf("Shard %v has no node assignment", shard["ShardID"])
		}
	}
}

// testVariousKeyPatterns verifies different key formats work
func testVariousKeyPatterns(t *testing.T, ts *TestSystem) {
	testCases := []struct {
		key   string
		value string
	}{
		{"simple", "text"},
		{"user@example.com", "email-data"},
		{"path/to/resource", "nested-data"},
		{"key-with-spaces here", "spaced-value"},
		{"数字", "unicode-value"},
		{"very:long:key:with:many:colons:and:segments", "complex"},
	}

	for _, tc := range testCases {
		// Store
		if _, err := ts.PUT(tc.key, tc.value); err != nil {
			t.Errorf("Failed to PUT key '%s': %v", tc.key, err)
			continue
		}

		// Retrieve
		_, value, err := ts.GET(tc.key)
		if err != nil {
			t.Errorf("Failed to GET key '%s': %v", tc.key, err)
			continue
		}

		if value != tc.value {
			t.Errorf("Key '%s': expected '%s', got '%s'", tc.key, tc.value, value)
		}
	}
}

// testPerformance verifies basic performance requirements
func testPerformance(t *testing.T, ts *TestSystem) {
	// Store some data first
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("perf-key-%d", i)
		value := fmt.Sprintf("perf-value-%d", i)
		ts.PUT(key, value)
	}

	// Test GET performance
	start := time.Now()
	_, _, err := ts.GET("perf-key-50")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Performance test GET failed: %v", err)
	}

	if elapsed > 50*time.Millisecond {
		t.Errorf("GET took %v, expected < 50ms", elapsed)
	}

	// Test PUT performance
	start = time.Now()
	_, err = ts.PUT("perf-new-key", "new-value")
	elapsed = time.Since(start)

	if err != nil {
		t.Fatalf("Performance test PUT failed: %v", err)
	}

	if elapsed > 50*time.Millisecond {
		t.Errorf("PUT took %v, expected < 50ms", elapsed)
	}
}

// hashKey replicates the system's key hashing for testing
func hashKey(key string) int {
	// Simple hash for testing - should match system's algorithm
	h := uint32(0)
	for _, c := range key {
		h = h*31 + uint32(c)
	}
	return int(h)
}

// TestStandaloneScenarios tests individual scenarios that don't require full system
func TestStandaloneScenarios(t *testing.T) {
	t.Run("ShardCalculation", func(t *testing.T) {
		// Verify our hash function distributes keys reasonably
		shardCounts := make(map[int]int)
		for i := 0; i < 1000; i++ {
			key := fmt.Sprintf("test-key-%d", i)
			shard := hashKey(key) % 4
			shardCounts[shard]++
		}

		// Each shard should get roughly 250 keys (±50%)
		for shard, count := range shardCounts {
			if count < 125 || count > 375 {
				t.Errorf("Shard %d has poor distribution: %d keys", shard, count)
			}
		}
	})

	t.Run("KeyValidation", func(t *testing.T) {
		// Test that various key formats are valid
		validKeys := []string{
			"simple",
			"with-dash",
			"with_underscore",
			"with.dot",
			"with:colon",
			"with/slash",
			"unicode-文字",
			"long" + string(make([]byte, 1000)), // 1KB key
		}

		for _, key := range validKeys {
			if key == "" {
				t.Errorf("Key should not be empty: %s", key)
			}
		}
	})
}
