// Package coordinator provides the cluster coordination server functionality.
// This file contains tests for the health monitoring functionality.
package coordinator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dreamware/torua/internal/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHealthMonitor verifies that NewHealthMonitor creates a properly configured instance.
// It checks that all default values are set correctly and the monitor is ready to use.
func TestNewHealthMonitor(t *testing.T) {
	// Create a new health monitor with 5 second interval
	monitor := NewHealthMonitor(5 * time.Second)
	defer monitor.Stop() // Ensure cleanup

	// Verify the monitor is properly initialized
	assert.NotNil(t, monitor)
	assert.Equal(t, 5*time.Second, monitor.interval)
	assert.Equal(t, 2*time.Second, monitor.timeout)
	assert.Equal(t, 3, monitor.maxFailures)
	assert.NotNil(t, monitor.nodes)
	assert.NotNil(t, monitor.httpClient)
	assert.NotNil(t, monitor.ctx)
	assert.NotNil(t, monitor.cancel)

	// Verify the nodes map is empty initially
	assert.Len(t, monitor.nodes, 0)
}

// TestHealthMonitorStart verifies that the health monitor starts and performs health checks.
// It uses a mock health check function to verify the monitoring behavior.
func TestHealthMonitorStart(t *testing.T) {
	// Create monitor with short interval for testing
	monitor := NewHealthMonitor(100 * time.Millisecond)
	defer monitor.Stop()

	// Track health check calls
	checkCalls := 0
	var mu sync.Mutex

	// Set up mock health check function
	monitor.SetCheckFunction(func(addr string) error {
		mu.Lock()
		checkCalls++
		mu.Unlock()
		return nil // Always healthy
	})

	// Mock node provider with URL format addresses (as nodes register)
	nodeProvider := func() []cluster.NodeInfo {
		return []cluster.NodeInfo{
			{ID: "node-1", Addr: "http://localhost:8081"},
			{ID: "node-2", Addr: "http://localhost:8082"},
		}
	}

	// Start monitor in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Start(ctx, nodeProvider)

	// Wait for multiple check cycles
	time.Sleep(350 * time.Millisecond)

	// Verify health checks were performed
	mu.Lock()
	calls := checkCalls
	mu.Unlock()

	// Should have performed at least 3 checks per node (initial + 2 intervals)
	// Total minimum: 3 checks * 2 nodes = 6
	assert.GreaterOrEqual(t, calls, 6, "Expected at least 6 health checks")

	// Verify nodes are tracked
	allHealth := monitor.GetAllNodeHealth()
	assert.Len(t, allHealth, 2)
	assert.Contains(t, allHealth, "node-1")
	assert.Contains(t, allHealth, "node-2")

	// Verify nodes are healthy
	assert.True(t, monitor.IsHealthy("node-1"))
	assert.True(t, monitor.IsHealthy("node-2"))
}

// TestHealthMonitorNodeFailure verifies that nodes are marked unhealthy after failures.
// It simulates health check failures and verifies the state transitions.
func TestHealthMonitorNodeFailure(t *testing.T) {
	// Create monitor with short interval
	monitor := NewHealthMonitor(50 * time.Millisecond)
	defer monitor.Stop()

	// Track which nodes should fail
	failingNodes := make(map[string]bool)
	var mu sync.Mutex

	// Set up mock health check that can fail
	monitor.SetCheckFunction(func(addr string) error {
		mu.Lock()
		defer mu.Unlock()
		// Handle both URL and host:port formats
		if (addr == "http://localhost:8081" || addr == "localhost:8081") && failingNodes["node-1"] {
			return fmt.Errorf("node is down")
		}
		return nil
	})

	// Track unhealthy callbacks
	unhealthyCalls := []string{}
	monitor.SetOnUnhealthy(func(nodeID string) {
		mu.Lock()
		unhealthyCalls = append(unhealthyCalls, nodeID)
		mu.Unlock()
	})

	// Node provider with URL format
	nodeProvider := func() []cluster.NodeInfo {
		return []cluster.NodeInfo{
			{ID: "node-1", Addr: "http://localhost:8081"},
			{ID: "node-2", Addr: "http://localhost:8082"},
		}
	}

	// Start monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Start(ctx, nodeProvider)

	// Wait for initial health check
	time.Sleep(100 * time.Millisecond)

	// Verify both nodes are initially healthy
	assert.True(t, monitor.IsHealthy("node-1"))
	assert.True(t, monitor.IsHealthy("node-2"))

	// Make node-1 fail
	mu.Lock()
	failingNodes["node-1"] = true
	mu.Unlock()

	// Wait for 3 failed checks (50ms * 3 = 150ms) plus buffer
	time.Sleep(250 * time.Millisecond)

	// Verify node-1 is now unhealthy
	assert.False(t, monitor.IsHealthy("node-1"))
	assert.True(t, monitor.IsHealthy("node-2"))

	// Verify callback was triggered
	mu.Lock()
	assert.Contains(t, unhealthyCalls, "node-1")
	mu.Unlock()

	// Get node health details
	health := monitor.GetNodeHealth("node-1")
	require.NotNil(t, health)
	assert.Equal(t, "unhealthy", health.Status)
	assert.GreaterOrEqual(t, health.ConsecutiveFails, 3)
}

// TestHealthMonitorNodeRecovery verifies that unhealthy nodes can recover.
// It simulates a node failure followed by recovery.
func TestHealthMonitorNodeRecovery(t *testing.T) {
	// Create monitor with short interval
	monitor := NewHealthMonitor(50 * time.Millisecond)
	defer monitor.Stop()

	// Control node health
	nodeHealthy := true
	var mu sync.Mutex

	// Mock health check
	monitor.SetCheckFunction(func(addr string) error {
		mu.Lock()
		defer mu.Unlock()
		// Handle both URL and host:port formats
		if (addr == "http://localhost:8081" || addr == "localhost:8081") && !nodeHealthy {
			return fmt.Errorf("node is down")
		}
		return nil
	})

	// Node provider with URL format
	nodeProvider := func() []cluster.NodeInfo {
		return []cluster.NodeInfo{
			{ID: "node-1", Addr: "http://localhost:8081"},
		}
	}

	// Start monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Start(ctx, nodeProvider)

	// Wait for initial check
	time.Sleep(100 * time.Millisecond)
	assert.True(t, monitor.IsHealthy("node-1"))

	// Make node unhealthy
	mu.Lock()
	nodeHealthy = false
	mu.Unlock()

	// Wait for node to be marked unhealthy
	time.Sleep(250 * time.Millisecond)
	assert.False(t, monitor.IsHealthy("node-1"))

	// Recover the node
	mu.Lock()
	nodeHealthy = true
	mu.Unlock()

	// Wait for recovery
	time.Sleep(100 * time.Millisecond)

	// Verify node is healthy again
	assert.True(t, monitor.IsHealthy("node-1"))

	// Verify failure count is reset
	health := monitor.GetNodeHealth("node-1")
	require.NotNil(t, health)
	assert.Equal(t, "healthy", health.Status)
	assert.Equal(t, 0, health.ConsecutiveFails)
}

// TestHealthMonitorNodeRemoval verifies that removed nodes are cleaned up.
// It tests that nodes no longer in the cluster are removed from monitoring.
func TestHealthMonitorNodeRemoval(t *testing.T) {
	// Create monitor
	monitor := NewHealthMonitor(50 * time.Millisecond)
	defer monitor.Stop()

	// Always healthy
	monitor.SetCheckFunction(func(addr string) error {
		return nil
	})

	// Dynamic node list
	var nodes []cluster.NodeInfo
	var mu sync.Mutex

	nodeProvider := func() []cluster.NodeInfo {
		mu.Lock()
		defer mu.Unlock()
		return nodes
	}

	// Start with two nodes using URL format
	mu.Lock()
	nodes = []cluster.NodeInfo{
		{ID: "node-1", Addr: "http://localhost:8081"},
		{ID: "node-2", Addr: "http://localhost:8082"},
	}
	mu.Unlock()

	// Start monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Start(ctx, nodeProvider)

	// Wait for initial checks
	time.Sleep(100 * time.Millisecond)

	// Verify both nodes are monitored
	allHealth := monitor.GetAllNodeHealth()
	assert.Len(t, allHealth, 2)

	// Remove node-2
	mu.Lock()
	nodes = []cluster.NodeInfo{
		{ID: "node-1", Addr: "http://localhost:8081"},
	}
	mu.Unlock()

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify only node-1 remains
	allHealth = monitor.GetAllNodeHealth()
	assert.Len(t, allHealth, 1)
	assert.Contains(t, allHealth, "node-1")
	assert.NotContains(t, allHealth, "node-2")
}

// TestHealthMonitorStop verifies graceful shutdown of the health monitor.
// It ensures that the monitor stops cleanly without goroutine leaks.
func TestHealthMonitorStop(t *testing.T) {
	// Create and start monitor
	monitor := NewHealthMonitor(50 * time.Millisecond)

	// Track if monitor is running
	running := true
	checkCount := 0
	var mu sync.Mutex

	monitor.SetCheckFunction(func(addr string) error {
		mu.Lock()
		defer mu.Unlock()
		checkCount++
		return nil
	})

	nodeProvider := func() []cluster.NodeInfo {
		mu.Lock()
		defer mu.Unlock()
		if running {
			return []cluster.NodeInfo{
				{ID: "node-1", Addr: "http://localhost:8081"},
			}
		}
		return nil
	}

	// Start monitor
	go monitor.Start(nil, nodeProvider) // Use internal context

	// Let it run for a bit
	time.Sleep(150 * time.Millisecond)

	// Get check count before stopping
	mu.Lock()
	checksBeforeStop := checkCount
	mu.Unlock()

	// Stop the monitor
	mu.Lock()
	running = false
	mu.Unlock()
	monitor.Stop()

	// Wait a bit more
	time.Sleep(150 * time.Millisecond)

	// Verify no more checks after stop
	mu.Lock()
	checksAfterStop := checkCount
	mu.Unlock()

	// Should have performed checks before stop
	assert.Greater(t, checksBeforeStop, 0)
	// No new checks should occur after stop
	assert.Equal(t, checksBeforeStop, checksAfterStop)
}

// TestHealthMonitorConcurrency verifies thread safety of the health monitor.
// It performs concurrent operations to ensure there are no race conditions.
func TestHealthMonitorConcurrency(t *testing.T) {
	// Create monitor
	monitor := NewHealthMonitor(10 * time.Millisecond)
	defer monitor.Stop()

	// Always healthy
	monitor.SetCheckFunction(func(addr string) error {
		return nil
	})

	// Node provider with changing nodes
	nodeCount := 5
	nodeProvider := func() []cluster.NodeInfo {
		nodes := make([]cluster.NodeInfo, nodeCount)
		for i := 0; i < nodeCount; i++ {
			nodes[i] = cluster.NodeInfo{
				ID:   fmt.Sprintf("node-%d", i),
				Addr: fmt.Sprintf("http://localhost:808%d", i),
			}
		}
		return nodes
	}

	// Start monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Start(ctx, nodeProvider)

	// Perform concurrent operations
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// Concurrent reads
				monitor.IsHealthy(fmt.Sprintf("node-%d", id%nodeCount))
				monitor.GetNodeHealth(fmt.Sprintf("node-%d", id%nodeCount))
				monitor.GetAllNodeHealth()

				// Small delay to interleave operations
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Wait for all goroutines
	wg.Wait()

	// Verify monitor is still functioning
	allHealth := monitor.GetAllNodeHealth()
	assert.Len(t, allHealth, nodeCount)
}

// TestHealthMonitorGetNodeHealth verifies GetNodeHealth returns correct information.
// It tests both existing and non-existing nodes.
func TestHealthMonitorGetNodeHealth(t *testing.T) {
	// Create monitor
	monitor := NewHealthMonitor(50 * time.Millisecond)
	defer monitor.Stop()

	// Always healthy
	monitor.SetCheckFunction(func(addr string) error {
		return nil
	})

	// Single node with URL format
	nodeProvider := func() []cluster.NodeInfo {
		return []cluster.NodeInfo{
			{ID: "node-1", Addr: "http://localhost:8081"},
		}
	}

	// Start monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Start(ctx, nodeProvider)

	// Wait for initial check
	time.Sleep(100 * time.Millisecond)

	// Get health of existing node
	health := monitor.GetNodeHealth("node-1")
	require.NotNil(t, health)
	assert.Equal(t, "node-1", health.NodeID)
	assert.Equal(t, "healthy", health.Status)
	assert.Equal(t, 0, health.ConsecutiveFails)
	assert.False(t, health.LastCheck.IsZero())
	assert.False(t, health.LastHealthy.IsZero())

	// Get health of non-existing node
	health = monitor.GetNodeHealth("node-999")
	assert.Nil(t, health)
}

// TestHealthMonitorUnhealthyCallback verifies the unhealthy callback is triggered correctly.
// It ensures the callback is only called once per state transition.
func TestHealthMonitorUnhealthyCallback(t *testing.T) {
	// Create monitor
	monitor := NewHealthMonitor(50 * time.Millisecond)
	defer monitor.Stop()

	// Control health
	failCount := 0
	var mu sync.Mutex

	monitor.SetCheckFunction(func(addr string) error {
		mu.Lock()
		defer mu.Unlock()
		if failCount < 3 {
			failCount++
			return fmt.Errorf("failing")
		}
		return nil
	})

	// Track callbacks
	callbackCount := 0
	var callbackMu sync.Mutex
	monitor.SetOnUnhealthy(func(nodeID string) {
		callbackMu.Lock()
		callbackCount++
		callbackMu.Unlock()
	})

	// Single node with URL format
	nodeProvider := func() []cluster.NodeInfo {
		return []cluster.NodeInfo{
			{ID: "node-1", Addr: "http://localhost:8081"},
		}
	}

	// Start monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Start(ctx, nodeProvider)

	// Wait for node to become unhealthy (3 failures)
	time.Sleep(250 * time.Millisecond)

	// Callback should be called exactly once
	callbackMu.Lock()
	assert.Equal(t, 1, callbackCount)
	callbackMu.Unlock()

	// Let it check a few more times while unhealthy
	time.Sleep(150 * time.Millisecond)

	// Callback should still only be called once
	callbackMu.Lock()
	assert.Equal(t, 1, callbackCount)
	callbackMu.Unlock()
}
