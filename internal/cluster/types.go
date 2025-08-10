// Package cluster provides the core distributed system functionality for Torua.
// See doc.go for complete package documentation.
package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// NodeInfo represents a storage node in the Torua cluster, containing the
// essential metadata needed for node identification, communication, and
// cluster membership management.
//
// NodeInfo instances are used throughout the system for:
//   - Node registration with the coordinator
//   - Health check targeting
//   - Request routing decisions
//   - Cluster state broadcasts
//
// Thread Safety:
// NodeInfo is safe for concurrent read access once initialized.
// Modifications should be protected by external synchronization.
//
// Example:
//
//	node := &NodeInfo{
//	    ID:   "node-1",
//	    Addr: "192.168.1.10:8081",
//	}
type NodeInfo struct {
	// LastHealthCheck records when the node was last checked by the coordinator.
	// Used to track monitoring freshness and detect stale health data.
	// Zero value indicates the node has never been health checked.
	LastHealthCheck time.Time `json:"last_health_check,omitempty"`

	// ID is the unique identifier for this node within the cluster.
	// It must be unique across all nodes and stable across restarts.
	// Format: typically "node-{number}" or UUID.
	// Example: "node-1", "node-2", "550e8400-e29b-41d4-a716-446655440000"
	ID string `json:"id"`

	// Addr is the network address where this node can be reached.
	// Must be accessible from both coordinator and other nodes.
	// Format: "host:port" or "ip:port"
	// Example: "localhost:8081", "192.168.1.10:8081", "node1.example.com:8081"
	Addr string `json:"addr"`

	// Status indicates the current health status of the node.
	// Possible values: "healthy", "unhealthy", "unknown"
	// Updated by the coordinator's health monitoring system.
	// Example: "healthy" for responsive nodes, "unhealthy" after failures
	Status string `json:"status,omitempty"`
}

// RegisterRequest encapsulates the data sent by a node when registering
// with the coordinator to join the cluster.
//
// The registration process:
// 1. Node creates RegisterRequest with its NodeInfo
// 2. Node POSTs request to coordinator's /cluster/register endpoint
// 3. Coordinator assigns shards and returns updated NodeInfo
// 4. Node begins serving assigned shards
//
// Example request body:
//
//	{
//	  "node": {
//	    "id": "node-1",
//	    "addr": "localhost:8081"
//	  }
//	}
type RegisterRequest struct {
	// Node contains the registering node's metadata.
	// The coordinator may modify this (e.g., assign shards) in response.
	Node NodeInfo `json:"node"`
}

// BroadcastRequest represents a message to be broadcast from the coordinator
// to all nodes in the cluster, typically used for propagating cluster state
// changes, configuration updates, or control commands.
//
// Broadcast mechanism:
// 1. Coordinator creates BroadcastRequest with target path and payload
// 2. Coordinator sends POST request to each node's broadcast endpoint
// 3. Nodes process the payload based on the path
// 4. Failed broadcasts are logged but don't stop other broadcasts
//
// Common broadcast types:
//   - Cluster state updates (path: "/cluster/state")
//   - Configuration changes (path: "/config/update")
//   - Shard assignments (path: "/shards/assign")
//
// Example:
//
//	broadcast := &BroadcastRequest{
//	    Path: "/cluster/state",
//	    Payload: json.RawMessage(`{"nodes":["node-1","node-2"]}`),
//	}
type BroadcastRequest struct {
	// Path specifies the target endpoint or message type.
	// Used by nodes to route the payload to appropriate handlers.
	// Format: URL path starting with "/"
	// Example: "/cluster/state", "/shards/reassign", "/config/reload"
	Path string `json:"path"`

	// Payload contains the actual message data in JSON format.
	// Using json.RawMessage allows deferring parsing until needed
	// and preserves the original JSON structure.
	// Content depends on Path and is defined by the handler.
	Payload json.RawMessage `json:"payload"`
}

// httpClient is the shared HTTP client used for all cluster communication.
// It's configured with a 5-second timeout to prevent hanging on unresponsive
// nodes and to enable quick failure detection.
//
// Performance characteristics:
//   - Connection pooling enabled by default
//   - Maximum of 100 idle connections
//   - Idle connection timeout of 90 seconds
//   - Supports HTTP/2 when available
//
// Note: This is a package-level variable to enable connection reuse
// across multiple requests, improving performance.
var httpClient = &http.Client{Timeout: 5 * time.Second}

// PostJSON sends a JSON-encoded POST request to the specified URL and
// decodes the JSON response into the provided output structure.
//
// This function is the primary mechanism for node-to-node and
// node-to-coordinator communication in the cluster, handling:
//   - Request body JSON encoding
//   - Context-based cancellation
//   - Response status validation
//   - Response body JSON decoding
//
// Parameters:
//   - ctx: Context for request cancellation and timeout control.
//     Should have a deadline set for production use.
//   - url: Complete URL to send the request to.
//     Example: "http://coordinator:8080/cluster/register"
//   - body: Go structure to be JSON-encoded as request body.
//     Must be JSON-serializable (exported fields, valid types).
//   - out: Pointer to structure for JSON response decoding.
//     Pass nil if response body should be ignored.
//
// Returns:
//   - nil on success (HTTP 2xx status and successful decode if out != nil)
//   - Error on failure, which may be:
//   - JSON marshaling error (invalid body structure)
//   - Network error (connection failure, timeout)
//   - HTTP error (non-2xx status code)
//   - JSON unmarshaling error (invalid response format)
//
// Thread Safety:
// This function is thread-safe and can be called concurrently.
// The shared httpClient handles connection pooling safely.
//
// Example:
//
//	req := &RegisterRequest{Node: NodeInfo{ID: "node-1", Addr: "localhost:8081"}}
//	var resp NodeInfo
//	err := PostJSON(ctx, "http://coordinator:8080/cluster/register", req, &resp)
//	if err != nil {
//	    log.Printf("Registration failed: %v", err)
//	}
func PostJSON(ctx context.Context, url string, body, out any) error {
	// Marshal request body to JSON
	reqBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	// Create HTTP request with context for cancellation
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute request using shared client
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for HTTP errors (status >= 300)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %s: %d", url, resp.StatusCode)
	}

	// Skip decoding if caller doesn't want response
	if out == nil {
		return nil
	}

	// Decode JSON response into output structure
	return json.NewDecoder(resp.Body).Decode(out)
}

// GetJSON sends a GET request to the specified URL and decodes the
// JSON response into the provided output structure.
//
// This function is primarily used for:
//   - Health checks (GET /health)
//   - Status queries (GET /status)
//   - Data retrieval (GET /data/{key})
//   - Metrics collection (GET /metrics)
//
// Parameters:
//   - ctx: Context for request cancellation and timeout control.
//     Should have a deadline set to prevent indefinite waits.
//   - url: Complete URL to send the request to.
//     Example: "http://node1:8081/health"
//   - out: Pointer to structure for JSON response decoding.
//     The structure should match the expected response format.
//
// Returns:
//   - nil on success (HTTP 2xx status and successful decode)
//   - Error on failure, which may be:
//   - Network error (connection failure, DNS resolution, timeout)
//   - HTTP error (non-2xx status code)
//   - JSON unmarshaling error (response doesn't match out structure)
//
// Thread Safety:
// This function is thread-safe and can be called concurrently.
// Multiple goroutines can safely make GET requests simultaneously.
//
// Performance Notes:
//   - Uses connection pooling for efficiency
//   - Streams response body (doesn't buffer entirely in memory)
//   - Suitable for responses up to several MB
//   - For large responses, consider streaming or pagination
//
// Example:
//
//	var health HealthStatus
//	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
//	defer cancel()
//	err := GetJSON(ctx, "http://node1:8081/health", &health)
//	if err != nil {
//	    log.Printf("Health check failed: %v", err)
//	}
func GetJSON(ctx context.Context, url string, out any) error {
	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}

	// Execute request using shared client
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %s: %d", url, resp.StatusCode)
	}

	// Decode JSON response
	return json.NewDecoder(resp.Body).Decode(out)
}
