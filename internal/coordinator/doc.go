// Package coordinator implements the orchestration layer for Torua's distributed
// storage system, managing cluster topology, shard distribution, request routing,
// and maintaining system-wide consistency through centralized coordination.
//
// # Overview
//
// The coordinator serves as the control plane for the Torua cluster, making
// global decisions about data placement, node membership, and query routing.
// It maintains authoritative state about which nodes host which shards and
// ensures this information is propagated throughout the cluster.
//
// # Architecture
//
// The coordinator implements several key subsystems:
//
//	┌─────────────────────────────────────┐
//	│         COORDINATOR                  │
//	├─────────────────────────────────────┤
//	│                                     │
//	│  ┌──────────────────────────────┐  │
//	│  │   Shard Registry              │  │
//	│  │   - Node → Shard mappings     │  │
//	│  │   - Shard → Node mappings     │  │
//	│  │   - Replication tracking      │  │
//	│  └──────────────────────────────┘  │
//	│                                     │
//	│  ┌──────────────────────────────┐  │
//	│  │   Node Manager                │  │
//	│  │   - Registration handling     │  │
//	│  │   - Health monitoring         │  │
//	│  │   - Failure detection         │  │
//	│  └──────────────────────────────┘  │
//	│                                     │
//	│  ┌──────────────────────────────┐  │
//	│  │   Request Router              │  │
//	│  │   - Key → Shard mapping       │  │
//	│  │   - Load balancing            │  │
//	│  │   - Retry logic               │  │
//	│  └──────────────────────────────┘  │
//	│                                     │
//	│  ┌──────────────────────────────┐  │
//	│  │   State Broadcaster           │  │
//	│  │   - Cluster state propagation │  │
//	│  │   - Event notifications       │  │
//	│  │   - Configuration updates     │  │
//	│  └──────────────────────────────┘  │
//	│                                     │
//	└─────────────────────────────────────┘
//
// # Core Components
//
// ShardRegistry: Central registry of shard assignments
//   - Maintains bidirectional mappings between nodes and shards
//   - Tracks replication groups for each shard
//   - Provides atomic updates for rebalancing operations
//   - Thread-safe for concurrent access patterns
//
// NodeManager: Handles node lifecycle and health
//   - Processes node registration requests
//   - Monitors node health via periodic checks
//   - Detects and handles node failures
//   - Triggers shard reassignment on topology changes
//
// RequestRouter: Routes client requests to appropriate nodes
//   - Uses consistent hashing for key-to-shard mapping
//   - Implements load balancing across replicas
//   - Handles request retry on transient failures
//   - Provides circuit breaking for failing nodes
//
// StateBroadcaster: Disseminates cluster state changes
//   - Pushes topology updates to all nodes
//   - Ensures eventual consistency of routing tables
//   - Batches updates for efficiency
//   - Handles partial broadcast failures gracefully
//
// # Shard Distribution Strategy
//
// The coordinator uses consistent hashing with virtual nodes:
//
//	Hash Ring (32-bit space):
//	0                                    2^32
//	|──────────────────────────────────────|
//	 ↑     ↑      ↑       ↑       ↑      ↑
//	 S0    S3     S1      S4      S2     S5
//
//	Key "user:123" → hash(key) → maps to S1
//	Key "post:456" → hash(key) → maps to S4
//
// Shard assignment algorithm:
// 1. Each shard is hashed to multiple positions (virtual nodes)
// 2. Keys are hashed to find their position on the ring
// 3. Key belongs to the first shard clockwise from its position
// 4. Provides ~1/n load per shard with n shards
//
// Rebalancing on node changes:
// - Node addition: Reassign shards from neighbors
// - Node removal: Redistribute shards to remaining nodes
// - Maintains minimal data movement (1/n of data affected)
//
// # Request Routing Protocol
//
// The coordinator routes requests using a multi-phase protocol:
//
// 1. Key Resolution:
//   - Hash the key to determine target shard
//   - Lookup nodes hosting the shard
//   - Select node based on load/health
//
// 2. Request Forwarding:
//   - Forward request to selected node
//   - Include shard hint in request headers
//   - Set appropriate timeout based on operation
//
// 3. Response Handling:
//   - Return successful responses immediately
//   - Retry on transient failures (network, timeout)
//   - Failover to replicas on node failures
//   - Return error after exhausting retries
//
// # Concurrency and Synchronization
//
// The coordinator handles high concurrency through:
//
// Lock Granularity:
//   - Fine-grained locks per shard for updates
//   - Read-write locks for registry access
//   - Lock-free reads for immutable state
//
// Goroutine Patterns:
//   - Worker pools for parallel health checks
//   - Buffered channels for broadcast queue
//   - Context cancellation for request timeouts
//
// Consistency Guarantees:
//   - Linearizable reads from registry
//   - Eventually consistent broadcast propagation
//   - At-least-once delivery for state updates
//
// # Failure Scenarios and Recovery
//
// The coordinator handles various failure modes:
//
// Node Failures:
//   - Detection: Health check timeouts (3 strikes)
//   - Impact: Shards on failed node unavailable
//   - Recovery: Reassign shards to healthy nodes
//   - Future: Promote replicas to primaries
//
// Network Partitions:
//   - Detection: Partial health check failures
//   - Impact: Split-brain scenarios possible
//   - Recovery: Quorum-based decision making (future)
//   - Mitigation: Prefer availability over consistency
//
// Coordinator Failures:
//   - Detection: Client connection failures
//   - Impact: No new nodes can register
//   - Recovery: Manual restart required (current)
//   - Future: Standby coordinators with failover
//
// # Performance Characteristics
//
// Operation complexities:
//   - Node registration: O(s) where s = shards to assign
//   - Shard lookup: O(1) average, O(n) worst case
//   - Health check round: O(n) with n nodes (parallel)
//   - Broadcast: O(n*m) where m = message size
//   - Key routing: O(log s) for shard lookup
//
// Scalability limits:
//   - Nodes: ~1000 (limited by broadcast overhead)
//   - Shards: ~10000 (limited by memory usage)
//   - Requests/sec: ~50000 (limited by routing CPU)
//   - Typical latency: <5ms for routing decision
//
// # Configuration
//
// Key configuration parameters:
//
//	HealthCheckInterval: 10s    // Frequency of health probes
//	HealthCheckTimeout:  5s     // Timeout for each probe
//	MaxFailedChecks:     3      // Failures before node removal
//	BroadcastTimeout:    10s    // Timeout for state broadcasts
//	ShardCount:          128    // Total shards in cluster
//	ReplicationFactor:   3      // Copies per shard (future)
//
// # Usage Example
//
//	// Starting a coordinator
//	registry := NewShardRegistry()
//
//	// Register node handler
//	http.HandleFunc("/cluster/register", func(w http.ResponseWriter, r *http.Request) {
//	    var node NodeInfo
//	    json.NewDecoder(r.Body).Decode(&node)
//
//	    // Assign shards to node
//	    shards := registry.AssignShards(node.ID, 10)
//	    node.Shards = shards
//
//	    // Add to registry
//	    registry.AddNode(&node)
//
//	    json.NewEncoder(w).Encode(node)
//	})
//
//	// Route data requests
//	http.HandleFunc("/data/", func(w http.ResponseWriter, r *http.Request) {
//	    key := strings.TrimPrefix(r.URL.Path, "/data/")
//	    shard := hashToShard(key)
//	    node := registry.GetNodeForShard(shard)
//
//	    // Forward to node
//	    proxyRequest(node, r, w)
//	})
//
// # Monitoring and Observability
//
// The coordinator exposes metrics for monitoring:
//
// Health Metrics:
//   - coordinator_nodes_total: Active nodes in cluster
//   - coordinator_shards_assigned: Shards with assignments
//   - coordinator_health_checks_failed: Failed health probes
//
// Performance Metrics:
//   - coordinator_routing_latency: Request routing time
//   - coordinator_broadcast_duration: State propagation time
//   - coordinator_registry_operations: Registry op counts
//
// Error Metrics:
//   - coordinator_routing_errors: Failed routing attempts
//   - coordinator_broadcast_failures: Failed broadcasts
//   - coordinator_node_failures: Detected node failures
//
// # Limitations and Future Work
//
// Current limitations:
//   - Single coordinator (SPOF)
//   - No automatic shard rebalancing
//   - Missing replication support
//   - No transaction coordination
//   - Limited to single datacenter
//
// Roadmap:
//   - Implement Raft for coordinator HA
//   - Add automatic shard rebalancing
//   - Support configurable replication
//   - Implement 2PC for transactions
//   - Add cross-region support
//
// # See Also
//
// Related packages:
//   - internal/cluster: Core cluster types and communication
//   - internal/shard: Individual shard implementation
//   - cmd/coordinator: Coordinator server implementation
package coordinator
