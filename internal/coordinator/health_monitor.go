// Package coordinator provides the cluster coordination server functionality.
// This file implements health monitoring for registered nodes in the cluster.
package coordinator

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dreamware/torua/internal/cluster"
)

// NodeHealth tracks the health status of a single node in the cluster.
// It maintains the current status, last successful check time, and failure count.
// Thread-safe: Protected by HealthMonitor's mutex when accessed.
type NodeHealth struct {
	LastCheck        time.Time // Timestamp of the last health check attempt
	LastHealthy      time.Time // Timestamp of the last successful health check
	NodeID           string    // Unique identifier of the node
	Status           string    // Current status: "healthy", "unhealthy", "unknown"
	ConsecutiveFails int       // Number of consecutive failed health checks
}

// HealthMonitor performs periodic health checks on all registered nodes in the cluster.
// It tracks node health status and triggers shard redistribution when nodes fail.
// Thread-safe: All methods are safe for concurrent access.
type HealthMonitor struct {
	nodes       map[string]*NodeHealth  // Current health status per node
	httpClient  *http.Client            // HTTP client for health checks
	checkFunc   func(addr string) error // Function to perform health check
	onUnhealthy func(nodeID string)     // Callback when node becomes unhealthy
	ctx         context.Context         // Context for cancellation
	cancel      context.CancelFunc      // Cancel function for shutdown
	interval    time.Duration           // How often to check node health
	timeout     time.Duration           // HTTP timeout for health checks
	mu          sync.RWMutex            // Protects nodes map
	wg          sync.WaitGroup          // Wait group for graceful shutdown
	maxFailures int                     // Failures before marking unhealthy
}

// NewHealthMonitor creates a new health monitor with the specified check interval.
// The monitor will check each node's /health endpoint every interval.
// Nodes are marked unhealthy after 3 consecutive failures.
//
// Parameters:
//   - interval: How often to perform health checks (recommended: 5s)
//
// Returns:
//   - *HealthMonitor: Configured health monitor ready to start
//
// Example:
//
//	monitor := NewHealthMonitor(5 * time.Second)
//	go monitor.Start(ctx, nodeProvider)
func NewHealthMonitor(interval time.Duration) *HealthMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &HealthMonitor{
		interval:    interval,
		timeout:     2 * time.Second, // 2 second timeout for health checks
		maxFailures: 3,               // Mark unhealthy after 3 failures
		nodes:       make(map[string]*NodeHealth),
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetOnUnhealthy sets the callback function to be invoked when a node becomes unhealthy.
// This is typically used to trigger shard redistribution.
//
// Parameters:
//   - callback: Function to call with the node ID when it becomes unhealthy
//
// Example:
//
//	monitor.SetOnUnhealthy(func(nodeID string) {
//	    log.Printf("Node %s is unhealthy, redistributing shards", nodeID)
//	    shardRegistry.RedistributeShards(nodeID)
//	})
func (h *HealthMonitor) SetOnUnhealthy(callback func(nodeID string)) {
	h.onUnhealthy = callback
}

// Start begins the health monitoring process in the current goroutine.
// It periodically checks all nodes provided by the nodeProvider function.
// This method blocks until the context is canceled.
//
// Parameters:
//   - ctx: Context for cancellation (use monitor's internal context)
//   - nodeProvider: Function that returns current list of nodes
//
// Example:
//
//	go monitor.Start(ctx, func() []cluster.NodeInfo {
//	    return srv.nodes.GetAll()
//	})
func (h *HealthMonitor) Start(ctx context.Context, nodeProvider func() []cluster.NodeInfo) {
	h.wg.Add(1)
	defer h.wg.Done()

	// Use the provided context or fall back to internal
	if ctx == nil {
		ctx = h.ctx
	}

	// Set default health check function if not configured
	if h.checkFunc == nil {
		h.checkFunc = h.defaultHealthCheck
	}

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	log.Printf("Health monitor started with interval %v", h.interval)

	// Perform initial health check immediately
	h.checkAllNodes(nodeProvider())

	for {
		select {
		case <-ticker.C:
			h.checkAllNodes(nodeProvider())
		case <-ctx.Done():
			log.Println("Health monitor stopping due to context cancellation")
			return
		case <-h.ctx.Done():
			log.Println("Health monitor stopping due to internal cancellation")
			return
		}
	}
}

// Stop gracefully shuts down the health monitor.
// It cancels the monitoring goroutine and waits for it to complete.
//
// Example:
//
//	monitor.Stop()
//	// Monitor is now safely stopped
func (h *HealthMonitor) Stop() {
	h.cancel()
	h.wg.Wait()
	log.Println("Health monitor stopped")
}

// checkAllNodes performs health checks on all provided nodes.
// It updates the health status for each node and triggers callbacks for state changes.
//
// Parameters:
//   - nodes: List of nodes to check
//
// Implementation:
//  1. Iterate through all provided nodes
//  2. Perform health check on each node
//  3. Update health status based on result
//  4. Trigger callback if node becomes unhealthy
//  5. Clean up removed nodes from tracking
func (h *HealthMonitor) checkAllNodes(nodes []cluster.NodeInfo) {
	// Track which nodes are still in the cluster
	currentNodes := make(map[string]bool)

	for _, node := range nodes {
		currentNodes[node.ID] = true
		h.checkNode(node)
	}

	// Clean up nodes that are no longer in the cluster
	h.mu.Lock()
	for nodeID := range h.nodes {
		if !currentNodes[nodeID] {
			delete(h.nodes, nodeID)
			log.Printf("Removed node %s from health monitoring", nodeID)
		}
	}
	h.mu.Unlock()
}

// checkNode performs a health check on a single node.
// It updates the node's health status based on the check result.
//
// Parameters:
//   - node: Node information including ID and address
//
// Implementation:
//  1. Get or create health record for the node
//  2. Perform HTTP health check
//  3. Update status based on result
//  4. Track consecutive failures
//  5. Trigger unhealthy callback if threshold exceeded
func (h *HealthMonitor) checkNode(node cluster.NodeInfo) {
	h.mu.Lock()
	health, exists := h.nodes[node.ID]
	if !exists {
		health = &NodeHealth{
			NodeID:      node.ID,
			Status:      "unknown",
			LastCheck:   time.Now(),
			LastHealthy: time.Now(),
		}
		h.nodes[node.ID] = health
	}
	h.mu.Unlock()

	// Perform the health check
	err := h.checkFunc(node.Addr)

	h.mu.Lock()
	defer h.mu.Unlock()

	health.LastCheck = time.Now()

	if err != nil {
		// Health check failed
		health.ConsecutiveFails++
		log.Printf("Health check failed for node %s (attempt %d/%d): %v",
			node.ID, health.ConsecutiveFails, h.maxFailures, err)

		// Mark as unhealthy if exceeded max failures
		if health.ConsecutiveFails >= h.maxFailures {
			previousStatus := health.Status
			health.Status = "unhealthy"

			// Trigger callback if this is a state change
			if previousStatus != "unhealthy" && h.onUnhealthy != nil {
				log.Printf("Node %s marked as unhealthy after %d failures",
					node.ID, health.ConsecutiveFails)
				// Call callback without holding the lock
				go h.onUnhealthy(node.ID)
			}
		}
	} else {
		// Health check succeeded
		if health.Status == "unhealthy" {
			log.Printf("Node %s recovered and is now healthy", node.ID)
		}
		health.Status = "healthy"
		health.ConsecutiveFails = 0
		health.LastHealthy = time.Now()
	}
}

// defaultHealthCheck performs an HTTP GET request to the node's /health endpoint.
// It returns an error if the node is not healthy.
//
// Parameters:
//   - addr: Node address (e.g., "localhost:8081")
//
// Returns:
//   - error: nil if healthy, error otherwise
//
// Implementation:
//  1. Construct health check URL
//  2. Perform HTTP GET with timeout
//  3. Check response status code
//  4. Return error if not 200 OK
func (h *HealthMonitor) defaultHealthCheck(addr string) error {
	// Handle both full URLs and host:port formats
	url := addr
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		url = fmt.Sprintf("http://%s", addr)
	}
	// Ensure the URL ends with /health
	if !strings.HasSuffix(url, "/health") {
		url = strings.TrimRight(url, "/") + "/health"
	}

	resp, err := h.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// GetNodeHealth returns the current health status of a specific node.
// Returns nil if the node is not being monitored.
//
// Parameters:
//   - nodeID: ID of the node to check
//
// Returns:
//   - *NodeHealth: Current health status or nil if not found
//
// Example:
//
//	health := monitor.GetNodeHealth("node-1")
//	if health != nil && health.Status == "healthy" {
//	    // Node is healthy
//	}
func (h *HealthMonitor) GetNodeHealth(nodeID string) *NodeHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()

	health, exists := h.nodes[nodeID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	return &NodeHealth{
		NodeID:           health.NodeID,
		Status:           health.Status,
		LastCheck:        health.LastCheck,
		LastHealthy:      health.LastHealthy,
		ConsecutiveFails: health.ConsecutiveFails,
	}
}

// GetAllNodeHealth returns the health status of all monitored nodes.
// Returns a map where keys are node IDs and values are health records.
//
// Returns:
//   - map[string]*NodeHealth: Current health status of all nodes
//
// Example:
//
//	allHealth := monitor.GetAllNodeHealth()
//	for nodeID, health := range allHealth {
//	    log.Printf("Node %s: %s", nodeID, health.Status)
//	}
func (h *HealthMonitor) GetAllNodeHealth() map[string]*NodeHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Create a copy to prevent external modification
	result := make(map[string]*NodeHealth)
	for id, health := range h.nodes {
		result[id] = &NodeHealth{
			NodeID:           health.NodeID,
			Status:           health.Status,
			LastCheck:        health.LastCheck,
			LastHealthy:      health.LastHealthy,
			ConsecutiveFails: health.ConsecutiveFails,
		}
	}

	return result
}

// IsHealthy returns whether a specific node is currently healthy.
// Returns false if the node is not being monitored.
//
// Parameters:
//   - nodeID: ID of the node to check
//
// Returns:
//   - bool: true if the node is healthy, false otherwise
//
// Example:
//
//	if monitor.IsHealthy("node-1") {
//	    // Route traffic to this node
//	}
func (h *HealthMonitor) IsHealthy(nodeID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	health, exists := h.nodes[nodeID]
	if !exists {
		return false
	}

	return health.Status == "healthy"
}

// SetCheckFunction allows overriding the default health check function.
// This is useful for testing or custom health check implementations.
//
// Parameters:
//   - checkFunc: Function that takes an address and returns an error
//
// Example:
//
//	monitor.SetCheckFunction(func(addr string) error {
//	    // Custom health check logic
//	    return nil
//	})
func (h *HealthMonitor) SetCheckFunction(checkFunc func(addr string) error) {
	h.checkFunc = checkFunc
}
