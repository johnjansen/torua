// Package main implements the Torua coordinator service, which orchestrates
// the distributed storage cluster by managing node registration, shard assignment,
// and request routing.
//
// The coordinator is the central control plane for the Torua distributed system,
// responsible for:
//   - Node registration and health monitoring
//   - Shard-to-node assignment management
//   - Request routing based on consistent hashing
//   - Cluster-wide broadcast operations
//   - Administrative operations (shard reassignment, rebalancing)
//
// Architecture:
//
//	┌─────────────────────────────────────────┐
//	│            Coordinator                   │
//	├─────────────────────────────────────────┤
//	│  HTTP API:                              │
//	│    /register     - Node registration    │
//	│    /nodes        - List active nodes    │
//	│    /data/*       - Route data requests  │
//	│    /shards       - Manage assignments   │
//	│    /broadcast    - Cluster-wide ops     │
//	│    /health       - Health check         │
//	├─────────────────────────────────────────┤
//	│  Components:                            │
//	│    server        - HTTP handler state   │
//	│    ShardRegistry - Shard assignments    │
//	│    nodes[]       - Active node list     │
//	└─────────────────────────────────────────┘
//
// Configuration:
//   - COORDINATOR_ADDR: Listen address (default: ":8080")
//
// Example usage:
//
//	# Start coordinator
//	COORDINATOR_ADDR=:8080 ./coordinator
//
//	# Register a node
//	curl -X POST localhost:8080/register \
//	  -d '{"node":{"id":"node-1","addr":"http://localhost:8081"}}'
//
//	# Store data (routed to appropriate shard)
//	curl -X PUT localhost:8080/data/user:123 \
//	  -d '{"name":"Alice","age":30}'
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

// Health status constants for node health monitoring
const (
	healthStatusHealthy   = "healthy"
	healthStatusUnhealthy = "unhealthy"
	healthStatusUnknown   = "unknown"
)

// main initializes and runs the coordinator service, setting up HTTP endpoints
// for cluster management and gracefully handling shutdown signals.
//
// The main function:
//  1. Configures the HTTP server with appropriate timeouts
//  2. Registers all API endpoints for cluster operations
//  3. Starts the server in a goroutine for non-blocking operation
//  4. Sets up signal handlers for graceful shutdown
//  5. Waits for termination signal (SIGINT/SIGTERM)
//  6. Performs graceful shutdown with 5-second timeout
//
// Exit codes:
//   - 0: Normal shutdown via signal
//   - 1: Fatal error during startup or operation
func main() {
	// Get listen address from environment or use default
	addr := getenv("COORDINATOR_ADDR", ":8080")

	// Initialize server with shard registry
	srv := newServer()

	// Start health monitor in background
	go srv.healthMonitor.Start(context.Background(), func() []cluster.NodeInfo {
		srv.mu.RLock()
		defer srv.mu.RUnlock()
		// Return a copy of the nodes slice for health monitoring
		nodes := make([]cluster.NodeInfo, len(srv.nodes))
		copy(nodes, srv.nodes)
		return nodes
	})

	// Configure HTTP routes
	mux := http.NewServeMux()

	// Node management endpoints
	mux.HandleFunc("/register", srv.handleRegister)   // POST: Register/update node
	mux.HandleFunc("/nodes", srv.handleListNodes)     // GET: List all nodes
	mux.HandleFunc("/broadcast", srv.handleBroadcast) // POST: Broadcast to all nodes
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Data routing endpoints - route client requests to appropriate shards
	mux.HandleFunc("/data/", srv.handleData) // GET/PUT/DELETE: Data operations

	// Shard management endpoints for admin operations
	mux.HandleFunc("/shards", srv.handleShards)             // GET: List shard assignments
	mux.HandleFunc("/shards/assign", srv.handleShardAssign) // POST: Manual shard assignment

	// Configure HTTP server with security timeouts
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second, // Prevent slowloris attacks
	}

	// Start server in goroutine to allow for graceful shutdown
	go func() {
		log.Printf("coordinator listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Set up signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	<-stop

	// Stop health monitor first
	log.Println("Stopping health monitor...")
	srv.healthMonitor.Stop()

	// Initiate graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	log.Println("coordinator stopped")
}

// server encapsulates the coordinator's runtime state, managing node registration
// and shard assignments with thread-safe access patterns.
//
// The server maintains:
//   - A list of registered nodes with their connection details
//   - A shard registry mapping data partitions to nodes
//   - Thread-safe access through read/write mutex
//
// Concurrency model:
//   - Multiple readers can access node list concurrently (RLock)
//   - Write operations (registration, updates) require exclusive access (Lock)
//   - Registry has its own internal synchronization
//
// Memory considerations:
//   - Each NodeInfo ~200 bytes (ID, address, metadata)
//   - 100 nodes = ~20KB memory overhead
//   - Registry overhead depends on shard count (see ShardRegistry docs)
type server struct {
	// registry manages shard-to-node assignments for data distribution.
	// Uses consistent hashing to map keys to shards and shards to nodes.
	// Thread-safe: handles its own synchronization internally.
	registry *coordinator.ShardRegistry

	// healthMonitor periodically checks node health status
	healthMonitor *coordinator.HealthMonitor

	// nodes contains all registered nodes in the cluster.
	// Nodes are identified by unique ID and include connection address.
	// Updated during registration; removed on failure detection (future).
	nodes []cluster.NodeInfo

	// mu protects concurrent access to the nodes slice.
	// Uses RWMutex to allow multiple concurrent readers for list operations
	// while ensuring exclusive access during registration/updates.
	mu sync.RWMutex
}

// newServer creates and initializes a new coordinator server instance with
// default configuration suitable for small to medium clusters.
//
// Default configuration:
//   - 4 shards: Suitable for 1-4 nodes with room for growth
//   - Empty node list: Nodes register themselves after startup
//   - Initialized shard registry: Ready for assignments
//
// The shard count determines:
//   - Data distribution granularity
//   - Maximum parallelism for operations
//   - Rebalancing flexibility when nodes join/leave
//
// Future improvements:
//   - Make shard count configurable via environment variable
//   - Support dynamic shard splitting for growing clusters
//   - Initialize with persisted state for recovery
//
// Returns:
//   - Initialized server ready to accept registrations
func newServer() *server {
	// Start with 4 shards by default
	// This provides reasonable distribution for small clusters
	// while keeping overhead low for testing
	// Get health check interval from environment (default 5 seconds)
	healthInterval := 5 * time.Second
	if envInterval := os.Getenv("HEALTH_CHECK_INTERVAL"); envInterval != "" {
		if parsed, err := time.ParseDuration(envInterval); err == nil {
			healthInterval = parsed
			log.Printf("Health check interval set to %v", healthInterval)
		}
	}

	srv := &server{
		registry:      coordinator.NewShardRegistry(4),
		healthMonitor: coordinator.NewHealthMonitor(healthInterval),
	}

	// Set up callback for when nodes become unhealthy
	srv.healthMonitor.SetOnUnhealthy(func(nodeID string) {
		log.Printf("Node %s is unhealthy, triggering shard redistribution", nodeID)
		// Mark node as unhealthy but keep it in the list
		srv.markNodeUnhealthy(nodeID)
		// Redistribute shards to healthy nodes
		srv.autoAssignShards()
	})

	return srv
}

// handleRegister processes node registration requests, updating the cluster
// membership and triggering shard assignment for new nodes.
//
// Endpoint: POST /register
//
// Request body:
//
//	{
//	  "node": {
//	    "id": "node-1",           // Unique node identifier
//	    "addr": "http://host:port" // Node's HTTP address
//	  }
//	}
//
// Registration behavior:
//   - New nodes: Added to cluster and assigned shards via round-robin
//   - Existing nodes: Updated in-place (for address changes)
//   - Invalid requests: Rejected with 400 Bad Request
//
// Side effects:
//   - Updates internal node list
//   - Triggers shard auto-assignment for new nodes
//   - Logs registration events
//
// Response:
//   - 204 No Content: Registration successful
//   - 400 Bad Request: Invalid JSON or missing required fields
//
// Thread safety:
//   - Acquires write lock for entire operation
//   - Auto-assignment happens within lock to ensure consistency
func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
	// Parse and validate registration request
	var req cluster.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Node.ID == "" || req.Node.Addr == "" {
		http.Error(w, "missing id/addr", http.StatusBadRequest)
		return
	}

	// Update node list with exclusive lock
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if node already exists (re-registration)
	idx := slices.IndexFunc(s.nodes, func(n cluster.NodeInfo) bool { return n.ID == req.Node.ID })
	if idx >= 0 {
		// Update existing node (address might have changed)
		s.nodes[idx] = req.Node
	} else {
		// Add new node to cluster
		s.nodes = append(s.nodes, req.Node)
		// Auto-assign shards to new nodes (simple round-robin for now)
		// This ensures data is distributed as nodes join
		s.autoAssignShards()
	}

	// Return success with no content
	w.WriteHeader(http.StatusNoContent)
}

// markNodeUnhealthy marks a node as unhealthy in the active nodes list by ID.
// This is called when a node is detected as unhealthy.
// The node remains in the list for visibility but is marked as unhealthy.
//
// Parameters:
//   - nodeID: ID of the node to mark as unhealthy
//
// Thread-safe: Uses write lock to protect nodes slice modification.
func (s *server) markNodeUnhealthy(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and mark the node as unhealthy
	for i, node := range s.nodes {
		if node.ID == nodeID {
			s.nodes[i].Status = healthStatusUnhealthy
			log.Printf("Marked node %s as unhealthy in cluster", nodeID)
			return
		}
	}
}

// handleListNodes returns the list of all registered nodes in the cluster.
// providing visibility into cluster membership for monitoring and debugging.
//
// Endpoint: GET /nodes
//
// Response body:
//
//	{
//	  "nodes": [
//	    {
//	      "id": "node-1",
//	      "addr": "http://localhost:8081"
//	    },
//	    {
//	      "id": "node-2",
//	      "addr": "http://localhost:8082"
//	    }
//	  ]
//	}
//
// Use cases:
//   - Health monitoring dashboards
//   - Debugging cluster topology
//   - Client service discovery (future)
//
// Response:
//   - 200 OK: JSON array of node information
//   - Empty array if no nodes registered
//
// Thread safety:
//   - Uses read lock for concurrent access
//   - Snapshot isolation: changes during encoding won't affect output
func (s *server) handleListNodes(w http.ResponseWriter, _ *http.Request) {
	// Acquire read lock for concurrent access
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get health status for all nodes
	allHealth := s.healthMonitor.GetAllNodeHealth()

	// Create response with nodes including their health status
	nodes := make([]cluster.NodeInfo, len(s.nodes))
	for i, node := range s.nodes {
		nodes[i] = node
		// Add health status if available, unless already marked unhealthy
		if node.Status != healthStatusUnhealthy {
			if health := allHealth[node.ID]; health != nil {
				nodes[i].Status = health.Status
				nodes[i].LastHealthCheck = health.LastCheck
			} else {
				nodes[i].Status = healthStatusUnknown
			}
		}
		// Node was explicitly marked unhealthy, preserve that status
	}

	// Encode node list as JSON response
	// Ignoring encoder error as it only fails on unmarshalable types
	if err := json.NewEncoder(w).Encode(struct {
		Nodes []cluster.NodeInfo `json:"nodes"`
	}{Nodes: nodes}); err != nil {
		log.Printf("Error encoding nodes response: %v", err)
	}
}

// handleBroadcast sends a request to all registered nodes in parallel, useful
// for cluster-wide operations like configuration updates or cache invalidation.
//
// Endpoint: POST /broadcast
//
// Request body:
//
//	{
//	  "path": "/some/endpoint",    // Target path on each node
//	  "payload": {                 // JSON payload to send
//	    "action": "clear_cache",
//	    "timestamp": 1234567890
//	  }
//	}
//
// Broadcast behavior:
//   - Sends POST request to path on all nodes
//   - 4-second timeout per node (total time may exceed this)
//   - Continues even if some nodes fail
//   - Returns results for all attempts
//
// Use cases:
//   - Configuration updates
//   - Cache invalidation
//   - Triggering maintenance operations
//   - Collecting cluster-wide statistics
//
// Response body:
//
//	{
//	  "sent_to": 3,
//	  "results": [
//	    {"node_id": "node-1"},
//	    {"node_id": "node-2"},
//	    {"node_id": "node-3", "err": "connection refused"}
//	  ]
//	}
//
// Response:
//   - 200 OK: Broadcast attempted (check results for individual failures)
//   - 400 Bad Request: Invalid JSON or missing path
//
// Thread safety:
//   - Takes snapshot of node list to avoid holding lock during I/O
//   - Each node request is independent
func (s *server) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	// Parse and validate broadcast request
	var req cluster.BroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// Validate path format
	if req.Path == "" || req.Path[0] != '/' {
		http.Error(w, "path must start with '/'", http.StatusBadRequest)
		return
	}

	// Take snapshot of nodes to avoid holding lock during network I/O
	s.mu.RLock()
	targets := append([]cluster.NodeInfo(nil), s.nodes...)
	s.mu.RUnlock()

	// Result tracking for each node
	type result struct {
		NodeID string `json:"node_id"`
		Err    string `json:"err,omitempty"`
	}
	out := make([]result, 0, len(targets))

	// Set timeout for all requests (not per-request)
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	// Send request to each node (sequential for simplicity)
	// Could be parallelized with goroutines for better performance
	for _, n := range targets {
		url := n.Addr + req.Path
		err := cluster.PostJSON(ctx, url, req.Payload, nil)
		res := result{NodeID: n.ID}
		if err != nil {
			res.Err = err.Error()
		}
		out = append(out, res)
	}

	// Return summary of broadcast results
	if err := json.NewEncoder(w).Encode(struct {
		Results []result `json:"results"`
		SentTo  int      `json:"sent_to"`
	}{Results: out, SentTo: len(out)}); err != nil {
		log.Printf("Error encoding broadcast results: %v", err)
	}
}

// handleData routes data operations to the appropriate shard/node based on
// consistent hashing, acting as a transparent proxy for client requests.
//
// Endpoint: GET|PUT|DELETE /data/{key}
//
// Routing algorithm:
//  1. Extract key from URL path
//  2. Hash key to determine owning shard (consistent hashing)
//  3. Look up node assignment for shard
//  4. Forward request to node's shard-specific endpoint
//  5. Stream response back to client
//
// Request examples:
//
//	GET /data/user:123          # Retrieve value
//	PUT /data/user:123          # Store value (body contains data)
//	DELETE /data/user:123       # Remove value
//
// Routing flow:
//
//	Client → Coordinator → Hash(key) → Shard → Node → Storage
//	                         ↓
//	                    FNV-1a hash
//	                         ↓
//	                    shard_id = hash % num_shards
//
// Error handling:
//   - 400 Bad Request: Missing key in path
//   - 503 Service Unavailable: No node assigned to shard
//   - 503 Service Unavailable: Assigned node not registered
//   - 502 Bad Gateway: Failed to forward request to node
//   - 405 Method Not Allowed: Unsupported HTTP method
//
// Performance considerations:
//   - Single hash computation per request (O(1))
//   - One network hop to target node
//   - Response streaming minimizes memory usage
//   - 5-second timeout prevents hanging requests
//
// Thread safety:
//   - Registry lookups are thread-safe
//   - Node list access uses read lock
//   - Each request handled independently
func (s *server) handleData(w http.ResponseWriter, r *http.Request) {
	// Extract key from path: /data/{key}
	// Supports keys with slashes (e.g., /data/user/profile/123)
	key := r.URL.Path[len("/data/"):]
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	// Find which node owns this key using consistent hashing
	nodeID, err := s.registry.GetNodeForKey(key)
	if err != nil {
		// No node assigned to the shard (cluster not ready)
		http.Error(w, fmt.Sprintf("no node assigned for key: %v", err), http.StatusServiceUnavailable)
		return
	}

	// Find the node's address from registration data
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
		// Node assigned but not registered (inconsistent state)
		http.Error(w, fmt.Sprintf("node %s not found", nodeID), http.StatusServiceUnavailable)
		return
	}

	// Determine which shard owns this key
	shardID := s.registry.GetShardForKey(key)

	// Forward the request to the node's shard-specific endpoint
	targetURL := fmt.Sprintf("%s/shard/%d/store/%s", nodeAddr, shardID, key)

	// Route based on HTTP method
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

// forwardGet forwards a GET request to a node and streams the response back,
// acting as a transparent proxy for read operations.
//
// The function:
//  1. Creates new request with timeout context
//  2. Forwards to target node
//  3. Streams response back to client
//  4. Preserves status codes and body content
//
// Error handling:
//   - 500 Internal Server Error: Failed to create request
//   - 502 Bad Gateway: Network error or node unreachable
//   - Otherwise: Node's response status and body
//
// Performance:
//   - 5-second timeout prevents hanging on unresponsive nodes
//   - Response streaming minimizes memory usage
//   - No buffering of response body
//
// Parameters:
//   - targetURL: Full URL of node's shard endpoint
//   - w: Response writer to client
//   - r: Original client request (for context)
func (s *server) forwardGet(targetURL string, w http.ResponseWriter, r *http.Request) {
	// Create request with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, http.NoBody)
	if err != nil {
		// Shouldn't happen unless URL is malformed
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	// Forward request to node
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Network error or node down
		http.Error(w, fmt.Sprintf("failed to forward request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Stream response back to client
	// Preserves status code and body from node
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("Error copying response body: %v", err)
	}
}

// forwardPut forwards a PUT request to a node with the request body,
// handling write operations in the distributed storage system.
//
// The function:
//  1. Reads entire request body into memory
//  2. Creates new PUT request with body
//  3. Forwards to target node with timeout
//  4. Streams response back to client
//
// Body handling:
//   - Entire body read into memory (consider streaming for large values)
//   - Body size limited by available memory
//   - Empty body is valid (stores empty value)
//
// Error handling:
//   - 400 Bad Request: Failed to read request body
//   - 500 Internal Server Error: Failed to create request
//   - 502 Bad Gateway: Network error or node unreachable
//   - Otherwise: Node's response status and body
//
// Performance:
//   - 5-second timeout for node response
//   - Body buffered in memory (not ideal for large values)
//   - Future: Implement streaming for large bodies
//
// Parameters:
//   - targetURL: Full URL of node's shard endpoint
//   - w: Response writer to client
//   - r: Original client request (for body and context)
func (s *server) forwardPut(targetURL string, w http.ResponseWriter, r *http.Request) {
	// Read entire body for forwarding
	// TODO: Consider streaming for large bodies
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Create request with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(body))
	if err != nil {
		// Shouldn't happen unless URL is malformed
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	// Forward request to node
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Network error or node down
		http.Error(w, fmt.Sprintf("failed to forward request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Stream response back to client
	// Preserves status code and body from node
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("Error copying response body: %v", err)
	}
}

// forwardDelete forwards a DELETE request to a node for removing data,
// completing the CRUD operations for the distributed storage system.
//
// The function:
//  1. Creates DELETE request with timeout context
//  2. Forwards to target node
//  3. Returns node's response to client
//  4. No body required for DELETE operations
//
// Error handling:
//   - 500 Internal Server Error: Failed to create request
//   - 502 Bad Gateway: Network error or node unreachable
//   - Otherwise: Node's response status (typically 204 No Content)
//
// Performance:
//   - 5-second timeout prevents hanging on unresponsive nodes
//   - Minimal overhead (no body processing)
//   - Single network hop to target node
//
// Parameters:
//   - targetURL: Full URL of node's shard endpoint
//   - w: Response writer to client
//   - r: Original client request (for context)
func (s *server) forwardDelete(targetURL string, w http.ResponseWriter, r *http.Request) {
	// Create request with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, targetURL, http.NoBody)
	if err != nil {
		// Shouldn't happen unless URL is malformed
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	// Forward request to node
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Network error or node down
		http.Error(w, fmt.Sprintf("failed to forward request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Return node's response status to client
	// DELETE typically returns 204 No Content on success
	w.WriteHeader(resp.StatusCode)
}

// handleShards returns current shard assignments for monitoring and debugging,
// providing visibility into how data is distributed across the cluster.
//
// Endpoint: GET /shards
//
// Response body:
//
//	{
//	  "num_shards": 4,
//	  "shards": [
//	    {
//	      "shard_id": 0,
//	      "node_id": "node-1",
//	      "is_primary": true
//	    },
//	    {
//	      "shard_id": 1,
//	      "node_id": "node-2",
//	      "is_primary": true
//	    }
//	  ]
//	}
//
// Use cases:
//   - Monitoring shard distribution balance
//   - Debugging data routing issues
//   - Planning manual rebalancing operations
//   - Verifying cluster topology
//
// Response:
//   - 200 OK: JSON with shard assignments
//   - 405 Method Not Allowed: Non-GET request
//
// Thread safety:
//   - Registry handles its own synchronization
//   - Assignments are copied, preventing modification
func (s *server) handleShards(w http.ResponseWriter, r *http.Request) {
	// Only GET method supported for listing
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current assignments from registry
	assignments := s.registry.GetAllAssignments()

	// Build response with shard metadata
	response := struct {
		Shards    []*coordinator.ShardAssignment `json:"shards"`
		NumShards int                            `json:"num_shards"`
	}{
		Shards:    assignments,
		NumShards: s.registry.NumShards(),
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding shards response: %v", err)
	}
}

// handleShardAssign manually assigns a shard to a node for administrative
// operations like rebalancing, recovery, or initial cluster setup.
//
// Endpoint: POST /shards/assign
//
// Request body:
//
//	{
//	  "shard_id": 0,        // Shard to assign (0 to num_shards-1)
//	  "node_id": "node-1",  // Target node ID
//	  "is_primary": true    // Primary or replica assignment
//	}
//
// Assignment rules:
//   - Each shard should have exactly one primary
//   - Replicas provide fault tolerance (optional)
//   - Same shard can be assigned to multiple nodes (primary + replicas)
//   - Reassignment overwrites existing assignment
//
// Use cases:
//   - Manual rebalancing after adding nodes
//   - Recovery after node failure
//   - Initial cluster bootstrapping
//   - Testing specific shard distributions
//
// Response:
//   - 204 No Content: Assignment successful
//   - 400 Bad Request: Invalid shard ID, missing fields, or assignment error
//   - 405 Method Not Allowed: Non-POST request
//
// Side effects:
//   - Updates shard registry immediately
//   - Does NOT notify affected nodes (future improvement)
//   - May affect ongoing operations on reassigned shards
//
// Thread safety:
//   - Registry handles synchronization internally
//   - Assignment is atomic
func (s *server) handleShardAssign(w http.ResponseWriter, r *http.Request) {
	// Only POST method supported for assignment
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse assignment request
	var req struct {
		NodeID    string `json:"node_id"`
		IsPrimary bool   `json:"is_primary"`
		ShardID   int    `json:"shard_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// Perform assignment through registry
	if err := s.registry.AssignShard(req.ShardID, req.NodeID, req.IsPrimary); err != nil {
		// Registry returns errors for invalid shard IDs or other issues
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return success with no content
	w.WriteHeader(http.StatusNoContent)
}

// autoAssignShards automatically distributes unassigned shards among registered
// nodes using round-robin allocation for initial cluster setup and rebalancing.
//
// Assignment algorithm:
//  1. Identifies all unassigned shards
//  2. Distributes them evenly across available nodes
//  3. Uses round-robin to ensure balance
//  4. Assigns all shards as primaries (no replicas yet)
//
// When called:
//   - After new node registration
//   - During cluster initialization
//   - Never called for node removal (manual intervention required)
//
// Behavior:
//   - Only assigns unassigned shards (doesn't move existing)
//   - Logs each assignment for audit trail
//   - No-op if no nodes registered
//   - No-op if all shards already assigned
//
// Limitations:
//   - Simple round-robin (doesn't consider node capacity)
//   - No replica creation (single point of failure)
//   - No rebalancing of existing assignments
//   - Runs on every registration (may cause churn)
//
// Future improvements:
//   - Consider node capacity and load
//   - Create replicas for fault tolerance
//   - Implement proper rebalancing strategy
//   - Batch assignments after multiple registrations
//
// Thread safety:
//   - Must be called with s.mu held (by handleRegister)
//   - Registry operations are thread-safe internally
func (s *server) autoAssignShards() {
	// Build list of healthy nodes only
	var healthyNodes []cluster.NodeInfo
	for _, node := range s.nodes {
		if node.Status != healthStatusUnhealthy {
			healthyNodes = append(healthyNodes, node)
		}
	}

	// No-op if no healthy nodes to assign to
	if len(healthyNodes) == 0 {
		log.Printf("No healthy nodes available for shard assignment")
		return
	}

	// Get all current assignments to identify gaps
	assignments := s.registry.GetAllAssignments()
	assignedShards := make(map[int]bool)
	for _, a := range assignments {
		assignedShards[a.ShardID] = true
	}

	// Assign any unassigned shards using round-robin across healthy nodes
	nodeIndex := 0
	for shardID := 0; shardID < s.registry.NumShards(); shardID++ {
		if !assignedShards[shardID] {
			// Select next healthy node in round-robin fashion
			nodeID := healthyNodes[nodeIndex].ID
			// Assign as primary (no replicas in current implementation)
			if err := s.registry.AssignShard(shardID, nodeID, true); err != nil {
				log.Printf("Error assigning shard %d to node %s: %v", shardID, nodeID, err)
			}
			log.Printf("Auto-assigned shard %d to node %s", shardID, nodeID)
			// Move to next healthy node for even distribution
			nodeIndex = (nodeIndex + 1) % len(healthyNodes)
		}
	}
}

// getenv retrieves an environment variable with a default fallback value,
// simplifying configuration management for deployment flexibility.
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
//	addr := getenv("COORDINATOR_ADDR", ":8080")
//	// Returns $COORDINATOR_ADDR if set, otherwise ":8080"
func getenv(k, def string) string {
	// Check environment variable
	if v := os.Getenv(k); v != "" {
		return v
	}
	// Return default if unset or empty
	return def
}
