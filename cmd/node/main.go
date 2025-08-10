// Package main implements the Torua node service, which manages data storage
// shards and handles distributed storage operations as part of the cluster.
//
// The node is a worker in the Torua distributed system, responsible for:
//   - Managing assigned storage shards
//   - Executing data operations (GET, PUT, DELETE)
//   - Registering with the coordinator
//   - Responding to health checks
//   - Creating shards on-demand when requests arrive
//
// Architecture:
//
//	┌─────────────────────────────────────────┐
//	│                Node                      │
//	├─────────────────────────────────────────┤
//	│  HTTP API:                              │
//	│    /health       - Health check         │
//	│    /control      - Control messages     │
//	│    /shard/*      - Shard operations     │
//	│    /info         - Node information     │
//	├─────────────────────────────────────────┤
//	│  Components:                            │
//	│    Node          - Runtime state        │
//	│    shards map    - Active shards        │
//	│    Registration  - Coordinator link     │
//	└─────────────────────────────────────────┘
//
// Configuration:
//   - NODE_ID: Unique node identifier (required)
//   - NODE_LISTEN: Listen address (default: ":8081")
//   - NODE_ADDR: Public address for coordinator (default: "http://127.0.0.1:8081")
//   - COORDINATOR_ADDR: Coordinator URL (required)
//
// Example usage:
//
//	# Start node
//	NODE_ID=node-1 \
//	NODE_LISTEN=:8081 \
//	NODE_ADDR=http://localhost:8081 \
//	COORDINATOR_ADDR=http://localhost:8080 \
//	./node
//
//	# Store data (through coordinator)
//	curl -X PUT localhost:8080/data/user:123 \
//	  -d '{"name":"Alice","age":30}'
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

// logFatal is a variable to allow mocking log.Fatal in tests.
// This indirection enables test code to intercept fatal errors
// without actually terminating the test process.
var logFatal = log.Fatalf

// Node represents a storage node in the distributed cluster, managing multiple
// shards and coordinating with the coordinator for data operations.
//
// Each node:
//   - Has a unique identifier within the cluster
//   - Manages zero or more storage shards
//   - Creates shards on-demand when requests arrive
//   - Handles concurrent shard operations safely
//
// Shard management:
//   - Shards are created lazily when first accessed
//   - Each shard has independent storage and state
//   - Shards can be primary or replica (future)
//   - Thread-safe access through RWMutex
//
// Concurrency model:
//   - Multiple readers can access shard map concurrently
//   - Write operations (add/remove shard) require exclusive lock
//   - Individual shards handle their own synchronization
//
// Memory usage:
//   - Base overhead: ~1KB per node
//   - Per shard: Depends on storage backend (currently in-memory)
//   - 100 shards with 1000 keys each: ~10-100MB
type Node struct {
	// shards maps shard IDs to their runtime instances.
	// Created on-demand when coordinator routes requests.
	// Protected by mu for thread-safe access.
	shards map[int]*shard.Shard

	// ID uniquely identifies this node in the cluster.
	// Format: typically "node-{number}" or UUID.
	// Immutable after creation.
	ID string

	// mu protects concurrent access to the shards map.
	// Uses RWMutex to allow multiple concurrent readers.
	mu sync.RWMutex
}

// NewNode creates a new node instance ready to manage shards and handle
// distributed storage operations.
//
// The node starts with:
//   - Empty shard map (shards created on-demand)
//   - Configured ID for cluster identification
//   - Thread-safe state management
//
// Parameters:
//   - id: Unique identifier for this node (must not be empty)
//
// Returns:
//   - Initialized Node ready for shard operations
//
// Example:
//
//	node := NewNode("node-1")
//	node.AddShard(shard.NewShard(0, true))
func NewNode(id string) *Node {
	return &Node{
		ID:     id,
		shards: make(map[int]*shard.Shard),
	}
}

// AddShard adds a shard to the node's management, making it available for
// data operations and request handling.
//
// Behavior:
//   - Overwrites existing shard with same ID (if any)
//   - Shard becomes immediately available for operations
//   - Thread-safe through exclusive locking
//
// Use cases:
//   - Initial shard assignment from coordinator
//   - On-demand shard creation for incoming requests
//   - Shard migration (receiving new shard)
//
// Parameters:
//   - s: Shard instance to add (must not be nil)
//
// Thread safety:
//   - Acquires exclusive lock for map modification
//   - Safe for concurrent calls
func (n *Node) AddShard(s *shard.Shard) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.shards[s.ID] = s
}

// GetShard retrieves a shard by ID for executing operations, returning nil
// if the shard doesn't exist on this node.
//
// Behavior:
//   - Returns nil for non-existent shards
//   - Caller should check for nil before use
//   - Read-only operation allows concurrent access
//
// Use cases:
//   - Handling incoming data requests
//   - Health checks on specific shards
//   - Administrative queries
//
// Parameters:
//   - id: Shard ID to retrieve
//
// Returns:
//   - Shard instance if exists, nil otherwise
//
// Thread safety:
//   - Uses read lock for concurrent access
//   - Multiple goroutines can retrieve shards simultaneously
func (n *Node) GetShard(id int) *shard.Shard {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.shards[id]
}

// main initializes and runs the node service, registering with the coordinator
// and serving shard operations until shutdown.
//
// The main function:
//  1. Reads configuration from environment variables
//  2. Creates node instance with shard management
//  3. Sets up HTTP endpoints for operations
//  4. Registers with coordinator (with retries)
//  5. Serves requests until shutdown signal
//  6. Performs graceful shutdown
//
// Required environment:
//   - NODE_ID: Unique identifier for this node
//   - COORDINATOR_ADDR: URL of coordinator service
//
// Optional environment:
//   - NODE_LISTEN: Local listen address (default: ":8081")
//   - NODE_ADDR: Public address for coordinator (default: "http://127.0.0.1:8081")
//
// Exit codes:
//   - 0: Normal shutdown via signal
//   - 1: Missing required configuration
//   - 1: Failed to register with coordinator
//   - 1: Failed to start HTTP server
func main() {
	// Read required configuration
	nodeID := mustGetenv("NODE_ID")
	listen := getenv("NODE_LISTEN", ":8081")
	public := getenv("NODE_ADDR", "http://127.0.0.1:8081")
	coord := mustGetenv("COORDINATOR_ADDR")

	// Create node with shard management
	node := NewNode(nodeID)

	// Shards will be created on-demand when coordinator routes requests
	// This avoids the need for explicit shard assignment protocol
	log.Printf("node[%s] initialized (shards will be created on demand)", nodeID)

	// Configure HTTP routes
	mux := http.NewServeMux()

	// Health check endpoint for monitoring
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Control endpoint for coordinator commands
	mux.HandleFunc("/control", handleControl)

	// Shard storage endpoints for data operations
	// Path: /shard/{shardID}/store/{key}
	mux.HandleFunc("/shard/", func(w http.ResponseWriter, r *http.Request) {
		handleShardRequest(node, w, r)
	})

	// Node info endpoint for debugging and monitoring
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		handleNodeInfo(node, w, r)
	})

	// Configure HTTP server with security timeouts
	s := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second, // Prevent slowloris attacks
	}

	// Start server in goroutine for non-blocking operation
	go func() {
		log.Printf("node[%s] listening on %s (public %s)", nodeID, listen, public)
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logFatal("listen: %v", err)
		}
	}()

	// Register with coordinator (with retries)
	ctx := context.Background()
	register(ctx, coord, nodeID, public)

	// Set up signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	<-stop

	// Initiate graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("node stopped")
}

// register attempts to register the node with the coordinator, retrying on
// failure to handle coordinator startup delays or temporary network issues.
//
// Registration process:
//  1. Sends node ID and public address to coordinator
//  2. Retries up to 10 times with exponential backoff
//  3. Logs success or terminates on persistent failure
//  4. Enables coordinator to route requests to this node
//
// Retry strategy:
//   - 10 attempts maximum
//   - 400ms delay between attempts
//   - Total retry window: ~4 seconds
//   - Fatal error if all attempts fail
//
// Parameters:
//   - ctx: Context for cancellation (currently unused)
//   - coord: Coordinator base URL
//   - id: Node's unique identifier
//   - addr: Node's public address for incoming requests
//
// Side effects:
//   - Logs registration attempts and results
//   - Terminates process on persistent failure
//
// Error handling:
//   - Network errors trigger retry
//   - 4xx/5xx responses trigger retry
//   - Persistent failure is fatal (node can't operate without registration)
func register(ctx context.Context, coord, id, addr string) {
	// Build registration request with node information
	body := cluster.RegisterRequest{Node: cluster.NodeInfo{ID: id, Addr: addr}}
	var lastErr error

	// Retry registration with backoff
	for i := 0; i < 10; i++ {
		lastErr = cluster.PostJSON(ctx, coord+"/register", body, nil)
		if lastErr == nil {
			log.Printf("registered with coordinator @ %s", coord)
			return
		}
		log.Printf("register retry %d: %v", i+1, lastErr)
		time.Sleep(400 * time.Millisecond)
	}

	// Fatal error if registration fails after all retries
	// Node cannot operate without coordinator registration
	logFatal("failed to register with coordinator: %v", lastErr)
}

// handleControl processes control messages from the coordinator for cluster
// management operations like configuration updates or maintenance commands.
//
// Endpoint: POST /control
//
// Current implementation:
//   - Logs payload for debugging
//   - Always returns success
//   - No actual control operations yet
//
// Future use cases:
//   - Shard assignment notifications
//   - Configuration updates
//   - Maintenance mode triggers
//   - Rebalancing commands
//
// Request body:
//   - Any JSON payload from coordinator
//
// Response:
//   - 204 No Content: Message received
//   - 400 Bad Request: Failed to read body
//
// Thread safety:
//   - Stateless operation, inherently thread-safe
func handleControl(w http.ResponseWriter, r *http.Request) {
	// Read entire body for logging
	var raw bytes.Buffer
	if _, err := raw.ReadFrom(r.Body); err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Log control message for debugging
	// Future: Parse and act on control commands
	log.Printf("control payload: %s", raw.Bytes())

	// Acknowledge receipt
	w.WriteHeader(http.StatusNoContent)
}

// handleShardRequest routes shard-specific storage requests, creating shards
// on-demand and delegating operations to the appropriate shard instance.
//
// Endpoint: /shard/{shardID}/store/{key}
//
// Path structure:
//   - /shard/0/store/user:123 → Shard 0, key "user:123"
//   - /shard/1/store/doc/path → Shard 1, key "doc/path"
//   - Keys can contain slashes for hierarchical organization
//
// Request routing:
//  1. Parse shard ID from path
//  2. Create shard if doesn't exist (on-demand)
//  3. Delegate to shard-specific handler
//  4. Return shard's response to client
//
// On-demand shard creation:
//   - Shards created when first request arrives
//   - Avoids explicit shard assignment protocol
//   - Simplifies node implementation
//   - Trade-off: No pre-warming of shards
//
// Supported operations:
//   - GET: Retrieve value by key
//   - PUT: Store key-value pair
//   - DELETE: Remove key
//
// Error handling:
//   - 400 Bad Request: Invalid path format or shard ID
//   - 404 Not Found: Key doesn't exist (GET only)
//   - 500 Internal Server Error: Storage operation failed
//
// Thread safety:
//   - Shard creation protected by node's mutex
//   - Shard operations handle their own synchronization
//
// Parameters:
//   - node: Node instance managing shards
//   - w: HTTP response writer
//   - r: HTTP request to process
func handleShardRequest(node *Node, w http.ResponseWriter, r *http.Request) {
	// Parse path: /shard/{shardID}/store/{key}
	pathWithoutPrefix := strings.TrimPrefix(r.URL.Path, "/shard/")

	// Find the first slash to separate shardID from the rest
	firstSlash := strings.Index(pathWithoutPrefix, "/")
	if firstSlash == -1 {
		http.Error(w, "invalid path format", http.StatusBadRequest)
		return
	}

	// Parse shard ID from path
	shardIDStr := pathWithoutPrefix[:firstSlash]
	remainingPath := pathWithoutPrefix[firstSlash+1:]

	shardID, err := strconv.Atoi(shardIDStr)
	if err != nil {
		http.Error(w, "invalid shard ID", http.StatusBadRequest)
		return
	}

	// Get or create the shard on demand
	// This workaround handles the lack of explicit shard assignment protocol
	s := node.GetShard(shardID)
	if s == nil {
		// Create shard on demand when coordinator routes to it
		// This ensures nodes can handle requests immediately without
		// waiting for explicit shard assignment from coordinator
		log.Printf("Creating shard %d on demand", shardID)
		newShard := shard.NewShard(shardID, true)
		node.AddShard(newShard)
		s = newShard
	}

	// Route to appropriate handler based on path
	if strings.HasPrefix(remainingPath, "store") {
		storePath := strings.TrimPrefix(remainingPath, "store")
		if storePath == "" || storePath == "/" {
			// List keys: GET /shard/{shardID}/store
			if r.Method == http.MethodGet {
				handleListKeys(s, w, r)
				return
			}
		} else if strings.HasPrefix(storePath, "/") {
			// Key operations: /shard/{shardID}/store/{key}
			key := strings.TrimPrefix(storePath, "/")
			switch r.Method {
			case http.MethodGet:
				handleGet(s, key, w, r)
			case http.MethodPut:
				handlePut(s, key, w, r)
			case http.MethodDelete:
				handleDelete(s, key, w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
	} else if remainingPath == "stats" {
		// Stats: GET /shard/{shardID}/stats
		if r.Method == http.MethodGet {
			handleShardStats(s, w, r)
			return
		}
	}

	http.Error(w, "not found", http.StatusNotFound)
}

// handleGet retrieves a value from the shard's storage backend, returning
// the stored data or an appropriate error status.
//
// Endpoint: GET /shard/{shardID}/store/{key}
//
// Response:
//   - 200 OK: Value found, body contains data
//   - 404 Not Found: Key doesn't exist
//   - 500 Internal Server Error: Storage backend error
//
// Response headers:
//   - Content-Type: application/octet-stream
//
// The function returns raw bytes without interpretation, allowing storage
// of any data type (JSON, binary, text, etc.).
//
// Parameters:
//   - s: Shard instance to query
//   - key: Storage key to retrieve
//   - w: HTTP response writer
//   - r: HTTP request (unused but kept for consistency)
func handleGet(s *shard.Shard, key string, w http.ResponseWriter, _ *http.Request) {
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
	if _, err := w.Write(value); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// handlePut stores a value in the shard's storage backend, creating or
// updating the key-value pair.
//
// Endpoint: PUT /shard/{shardID}/store/{key}
//
// Request body:
//   - Raw bytes to store (any format)
//   - Empty body stores empty value
//
// Response:
//   - 204 No Content: Value stored successfully
//   - 400 Bad Request: Failed to read request body
//   - 500 Internal Server Error: Storage backend error
//
// Storage behavior:
//   - Overwrites existing values without warning
//   - Accepts any byte sequence (JSON, binary, text)
//   - No size limits enforced (depends on backend)
//   - Synchronous write (returns after storage)
//
// Parameters:
//   - s: Shard instance to write to
//   - key: Storage key for the value
//   - w: HTTP response writer
//   - r: HTTP request containing value in body
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

// handleDelete removes a key-value pair from the shard's storage backend,
// freeing the associated resources.
//
// Endpoint: DELETE /shard/{shardID}/store/{key}
//
// Response:
//   - 204 No Content: Key deleted (or didn't exist)
//   - 500 Internal Server Error: Storage backend error
//
// Delete behavior:
//   - Idempotent: Deleting non-existent key succeeds
//   - Immediate: No soft delete or tombstones
//   - No undo: Deletion is permanent
//
// Parameters:
//   - s: Shard instance to delete from
//   - key: Storage key to remove
//   - w: HTTP response writer
//   - r: HTTP request (unused but kept for consistency)
func handleDelete(s *shard.Shard, key string, w http.ResponseWriter, _ *http.Request) {
	if err := s.Delete(key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListKeys returns all keys stored in the shard, useful for debugging,
// maintenance, and data exploration.
//
// Endpoint: GET /shard/{shardID}/store
//
// Response body:
//
//	{
//	  "keys": ["user:1", "user:2", "doc:xyz"],
//	  "count": 3
//	}
//
// Response:
//   - 200 OK: JSON array of keys (empty array if no keys)
//
// Performance considerations:
//   - O(n) operation where n = number of keys
//   - No pagination (returns all keys at once)
//   - May be slow/memory-intensive for large shards
//   - Consider adding pagination for production use
//
// Parameters:
//   - s: Shard instance to list keys from
//   - w: HTTP response writer
//   - r: HTTP request (unused but kept for consistency)
func handleListKeys(s *shard.Shard, w http.ResponseWriter, _ *http.Request) {
	keys := s.ListKeys()

	response := struct {
		Keys  []string `json:"keys"`
		Count int      `json:"count"`
	}{
		Keys:  keys,
		Count: len(keys),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// handleShardStats returns operational statistics for monitoring shard
// performance and resource usage.
//
// Endpoint: GET /shard/{shardID}/stats
//
// Response body:
//
//	{
//	  "shard_id": 0,
//	  "operations": {
//	    "gets": 1234,
//	    "puts": 567,
//	    "deletes": 89
//	  },
//	  "storage": {
//	    "keys": 100,
//	    "bytes": 10240
//	  }
//	}
//
// Statistics tracked:
//   - Operation counts: Total gets, puts, deletes
//   - Storage metrics: Key count, total bytes
//   - Cumulative since shard creation
//
// Use cases:
//   - Performance monitoring
//   - Capacity planning
//   - Load balancing decisions
//   - Debugging hot spots
//
// Response:
//   - 200 OK: JSON statistics object
//
// Parameters:
//   - s: Shard instance to get stats from
//   - w: HTTP response writer
//   - r: HTTP request (unused but kept for consistency)
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
	_ = json.NewEncoder(w).Encode(response)
}

// handleNodeInfo returns comprehensive information about the node and all its
// managed shards for monitoring and debugging purposes.
//
// Endpoint: GET /info
//
// Response body:
//
//	{
//	  "node_id": "node-1",
//	  "shard_count": 2,
//	  "shards": [
//	    {
//	      "id": 0,
//	      "primary": true,
//	      "state": "active",
//	      "keys": 150,
//	      "bytes": 15360
//	    },
//	    {
//	      "id": 1,
//	      "primary": true,
//	      "state": "active",
//	      "keys": 200,
//	      "bytes": 20480
//	    }
//	  ]
//	}
//
// Information provided:
//   - Node identifier
//   - Total shard count
//   - Per-shard details (ID, role, state, metrics)
//
// Use cases:
//   - Health monitoring dashboards
//   - Debugging shard distribution
//   - Capacity analysis
//   - Load balancing verification
//
// Response:
//   - 200 OK: JSON node information
//
// Thread safety:
//   - Acquires read lock on node's shard map
//   - Safe for concurrent access
//
// Parameters:
//   - node: Node instance to query
//   - w: HTTP response writer
//   - r: HTTP request (unused but kept for consistency)
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

// getenv retrieves an environment variable with a default fallback value,
// simplifying configuration management for optional settings.
//
// The function checks if the environment variable is set and non-empty,
// returning its value if so, otherwise returning the default value.
//
// Parameters:
//   - k: Environment variable name to look up
//   - def: Default value if variable is unset or empty
//
// Returns:
//   - Environment variable value if set and non-empty
//   - Default value otherwise
//
// Example:
//
//	listen := getenv("NODE_LISTEN", ":8081")
//	// Returns $NODE_LISTEN if set, otherwise ":8081"
func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// mustGetenv retrieves a required environment variable, terminating the
// program if it's not set to ensure configuration completeness.
//
// Use this for critical configuration that the node cannot operate without,
// such as node ID or coordinator address.
//
// Parameters:
//   - k: Environment variable name to look up
//
// Returns:
//   - Environment variable value if set and non-empty
//
// Side effects:
//   - Calls log.Fatal if variable is unset or empty
//   - Program terminates with exit code 1
//
// Example:
//
//	nodeID := mustGetenv("NODE_ID")
//	// Returns $NODE_ID or terminates with error message
func mustGetenv(k string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	logFatal("missing env %s", k)
	return ""
}
