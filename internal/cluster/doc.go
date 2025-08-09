// Package cluster provides the core distributed system functionality for Torua,
// implementing cluster membership, health monitoring, and inter-node communication
// protocols that enable reliable distributed storage operations.
//
// # Overview
//
// The cluster package is the foundation of Torua's distributed architecture,
// managing how nodes discover each other, maintain cluster membership, and
// communicate reliably. It implements a coordinator-based topology where a
// central coordinator orchestrates multiple storage nodes.
//
// # Architecture
//
// The package follows a hub-and-spoke model:
//
//	              ┌──────────────┐
//	              │ Coordinator  │
//	              │              │
//	              │ - Registry   │
//	              │ - Health Mon │
//	              │ - Broadcasts │
//	              └──────┬───────┘
//	                     │
//	      ┌──────────────┼──────────────┐
//	      │              │              │
//	┌─────▼─────┐ ┌─────▼─────┐ ┌─────▼─────┐
//	│  Node 1   │ │  Node 2   │ │  Node 3   │
//	│           │ │           │ │           │
//	│ Shards:   │ │ Shards:   │ │ Shards:   │
//	│ [0,1,2]   │ │ [3,4,5]   │ │ [6,7,8]   │
//	└───────────┘ └───────────┘ └───────────┘
//
// # Core Components
//
// NodeInfo: Represents a storage node in the cluster
//   - Tracks node identity, address, and health status
//   - Maintains list of shards assigned to the node
//   - Thread-safe for concurrent access
//
// ClusterState: Captures the complete cluster topology
//   - Enumerates all active nodes
//   - Maps shards to their assigned nodes
//   - Provides snapshot consistency for distributed operations
//
// HealthStatus: Monitors node availability
//   - Tracks last successful health check
//   - Determines node liveness based on configurable thresholds
//   - Enables automatic failure detection
//
// # Communication Protocol
//
// The package uses HTTP/JSON for all inter-node communication:
//
// Node Registration (POST /cluster/register):
//   - Nodes announce themselves to the coordinator
//   - Includes node capabilities and initial configuration
//   - Returns assigned node ID and shard allocations
//
// Health Checking (GET /health):
//   - Periodic liveness probes from coordinator to nodes
//   - Timeout-based failure detection
//   - Automatic node removal on repeated failures
//
// State Broadcasting (POST /cluster/broadcast):
//   - Coordinator pushes cluster state changes to all nodes
//   - Eventually consistent state propagation
//   - Enables nodes to route requests appropriately
//
// # Concurrency Model
//
// The package is designed for high concurrency:
//   - All types are thread-safe using sync.RWMutex
//   - Read operations use RLock for parallel access
//   - Write operations use Lock for exclusive access
//   - No operations hold locks during network I/O
//
// # Failure Handling
//
// The package implements several failure detection and recovery mechanisms:
//
// Network Failures:
//   - HTTP requests have configurable timeouts (default 5s)
//   - Failed requests trigger immediate health checks
//   - Exponential backoff for retries (not yet implemented)
//
// Node Failures:
//   - Health checks every 10 seconds (configurable)
//   - Nodes marked unhealthy after 3 failed checks
//   - Unhealthy nodes removed from routing tables
//   - Shard reassignment triggered (not yet implemented)
//
// Coordinator Failures:
//   - Currently a single point of failure
//   - Future: leader election via Raft consensus
//   - Future: standby coordinators for failover
//
// # Performance Characteristics
//
// Registration: O(1) - Simple map insertion
// Health Check: O(n) - Checks all n nodes in parallel
// Broadcast: O(n) - Sends to all n nodes in parallel
// State Lookup: O(1) - Hash map lookups
//
// Memory usage scales with number of nodes and shards:
//   - ~1KB per node metadata
//   - ~100 bytes per shard mapping
//   - Typical 100-node cluster: ~150KB total
//
// # Usage Example
//
//	// Creating a node and registering with coordinator
//	node := &NodeInfo{
//	    ID:      "node-1",
//	    Address: "localhost:8081",
//	    Shards:  []int{0, 1, 2},
//	}
//
//	// Register with coordinator
//	err := node.Register("http://localhost:8080")
//	if err != nil {
//	    log.Fatalf("Failed to register: %v", err)
//	}
//
//	// Health check implementation
//	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
//	    json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
//	})
//
// # Limitations and Future Work
//
// Current limitations that will be addressed:
//   - No automatic shard rebalancing on node join/leave
//   - No replication for fault tolerance
//   - Coordinator is a single point of failure
//   - No support for multi-datacenter deployments
//   - No encryption for inter-node communication
//
// Planned improvements:
//   - Implement Raft consensus for coordinator HA
//   - Add automatic shard rebalancing
//   - Support configurable replication factors
//   - Implement vector clocks for conflict resolution
//   - Add TLS support for secure communication
//
// # Testing
//
// The package includes comprehensive tests:
//   - Unit tests with 100% code coverage
//   - Integration tests for cluster operations
//   - BDD tests for end-to-end scenarios
//   - Chaos testing for failure conditions (planned)
//
// Run tests with:
//
//	go test ./internal/cluster/... -cover
//
// # See Also
//
// Related packages:
//   - internal/coordinator: Cluster orchestration logic
//   - internal/shard: Individual shard management
//   - internal/storage: Key-value storage implementation
package cluster
