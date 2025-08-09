# Documentation Example: Before and After

This document shows how existing Torua code should be documented according to our standards.

## Example 1: Coordinator's handleData Function

### Before (Current)
```go
// handleData routes data operations to the appropriate shard/node
func (s *server) handleData(w http.ResponseWriter, r *http.Request) {
    // Extract key from path: /data/{key}
    key := r.URL.Path[len("/data/"):]
    if key == "" {
        http.Error(w, "key required", http.StatusBadRequest)
        return
    }
    // ... rest of implementation
}
```

### After (Properly Documented)
```go
// handleData routes data operations to the appropriate shard/node.
//
// Purpose:
// This is the main entry point for all distributed storage operations. It implements
// the coordinator's routing logic to ensure client requests reach the correct node
// without clients needing to know the cluster topology.
//
// Mechanism:
// 1. Extracts the key from the URL path
// 2. Calculates shard ID using consistent hashing (FNV-1a hash % numShards)
// 3. Looks up which node owns that shard
// 4. Forwards the request to the owning node
// 5. Returns the node's response to the client
//
// Request Path: /data/{key}
//   - {key} can contain any characters including slashes (e.g., "path/to/resource")
//   - Keys are extracted as everything after "/data/"
//
// Supported Methods:
//   - GET: Retrieve value for key
//   - PUT: Store value for key (body contains value)
//   - DELETE: Remove key from storage
//
// Response Codes:
//   - 200 OK: Successful GET with value in body
//   - 204 No Content: Successful PUT/DELETE
//   - 400 Bad Request: Missing or invalid key
//   - 404 Not Found: Key doesn't exist (GET only)
//   - 503 Service Unavailable: No node available for shard
//   - 502 Bad Gateway: Node request failed
//
// Example Flow:
//   Client: PUT /data/user:123 (body: {"name": "Alice"})
//   Coordinator: hash("user:123") % 4 = shard 2
//   Coordinator: shard 2 owned by node "n1" at localhost:8081
//   Coordinator: forwards to http://localhost:8081/shard/2/store/user:123
//   Node: stores value, returns 204
//   Coordinator: returns 204 to client
//
// Thread Safety: Safe for concurrent requests (uses read lock for node lookup)
// Performance: O(1) shard calculation, single network hop to node
//
// Known Issues:
//   - No retry logic if node request fails
//   - No failover to replica nodes
//   - No request timeout handling
func (s *server) handleData(w http.ResponseWriter, r *http.Request) {
    // Extract key from path: /data/{key}
    key := r.URL.Path[len("/data/"):]
    if key == "" {
        http.Error(w, "key required", http.StatusBadRequest)
        return
    }
    // ... rest of implementation
}
```

## Example 2: Node Structure

### Before (Current)
```go
type Node struct {
    ID     string
    shards map[int]*shard.Shard
    mu     sync.RWMutex
}
```

### After (Properly Documented)
```go
// Node represents a storage node in the Torua cluster.
//
// Purpose:
// A Node is the fundamental storage unit that manages one or more shards.
// Each node runs independently, handles client requests forwarded by the
// coordinator, and manages its assigned shards' data.
//
// Architecture:
// Nodes are designed to be stateless regarding cluster topology - they
// don't know about other nodes. The coordinator handles all routing
// decisions and shard assignments.
//
// Lifecycle:
//   1. Created: Node starts with empty shard map
//   2. Registered: Node registers with coordinator
//   3. Active: Receives shard assignments (currently on-demand)
//   4. Serving: Handles storage requests for assigned shards
//   5. Shutdown: Gracefully stops, data persists on disk
//
// Invariants:
//   - Node ID must be unique across the cluster
//   - Each shard can only be primary on one node
//   - Shard IDs are immutable once created
//
// Current Limitations:
//   - No replication support (each shard exists once)
//   - Shards created on-demand when requests arrive (workaround)
//   - No persistence between restarts
//
// Thread Safety:
// All public methods are thread-safe. The mu mutex protects:
//   - Read access to shards map (RLock)
//   - Write access to shards map (Lock)
//   - Adding/removing shards is synchronized
//
// Example Usage:
//   node := NewNode("n1")
//   node.AddShard(shard.NewShard(0, true))  // Add primary shard 0
//   s := node.GetShard(0)                    // Retrieve shard
//
type Node struct {
    // ID uniquely identifies this node in the cluster.
    // Format: alphanumeric string (e.g., "n1", "node-west-1")
    // Set via NODE_ID environment variable at startup.
    ID string
    
    // shards maps shard IDs to their local shard instances.
    // Key: shard ID (0 to numShards-1)
    // Value: pointer to shard instance managing that shard's data
    // Modified when: shard assignments change or on-demand creation
    shards map[int]*shard.Shard
    
    // mu protects concurrent access to the shards map.
    // Use RLock() for read operations (GetShard)
    // Use Lock() for write operations (AddShard, RemoveShard)
    mu sync.RWMutex
}
```

## Example 3: Complex Algorithm

### Before (Current)
```go
func (r *ShardRegistry) GetShardForKey(key string) int {
    h := fnv.New32a()
    h.Write([]byte(key))
    return int(h.Sum32()) % r.numShards
}
```

### After (Properly Documented)
```go
// GetShardForKey determines which shard should store a given key.
//
// Algorithm: Consistent Hashing using FNV-1a
// ==========================================
//
// This implements a simple consistent hashing scheme for distributing
// keys across shards. While not as sophisticated as jump consistent
// hashing or rendezvous hashing, it provides good distribution with
// minimal computational overhead.
//
// Step-by-Step Process:
//
// 1. Hash Generation (FNV-1a):
//    - Initialize FNV-1a 32-bit hash with offset basis
//    - For each byte in key: hash = (hash XOR byte) × FNV_prime
//    - Produces uniform distribution across 32-bit space
//    - Chosen for: speed (faster than SHA), good distribution
//
// 2. Shard Mapping (Modulo):
//    - shardID = hash % numShards
//    - Maps 32-bit space to shard range [0, numShards)
//    - Ensures statistically even distribution
//
// Why FNV-1a?
//   - Non-cryptographic (faster than SHA/MD5)
//   - Good avalanche properties (small changes = different hash)
//   - Simple implementation, no external dependencies
//   - Battle-tested in distributed systems
//
// Distribution Characteristics:
//   - Expected: ~equal keys per shard (±5% for 10K+ keys)
//   - Actual: May vary with key patterns
//   - Hot spots: Possible with sequential/pattern keys
//
// Limitations:
//   - Modulo approach means adding shards requires full redistribution
//   - No virtual nodes, so single shard = single physical location
//   - Key patterns can cause uneven distribution
//
// Future Improvements:
//   - Consider jump consistent hashing (minimal redistribution)
//   - Add virtual nodes for better balance
//   - Monitor actual distribution and alert on skew
//
// Example Distributions:
//   4 shards, 10000 keys: [2487, 2501, 2506, 2506] (±1% deviation)
//   Key "user:123" -> 0xA3B2C1D4 -> shard 0
//   Key "user:124" -> 0x5E7F8A9B -> shard 3
//
// Performance: O(1) time, O(1) space
// Thread Safety: Read-only operation, inherently thread-safe
//
// References:
//   - FNV Hash: http://www.isthe.com/chongo/tech/comp/fnv/
//   - Consistent Hashing: https://en.wikipedia.org/wiki/Consistent_hashing
//
func (r *ShardRegistry) GetShardForKey(key string) int {
    // Use FNV-1a hash for its speed and good distribution properties
    h := fnv.New32a()
    h.Write([]byte(key))
    
    // Map hash to shard range using modulo
    // This ensures keys are distributed across all available shards
    return int(h.Sum32()) % r.numShards
}
```

## Example 4: Package Documentation

### Create new file: `internal/coordinator/doc.go`

```go
// Package coordinator implements the cluster coordination layer for Torua.
//
// Overview
//
// The coordinator is the brain of the Torua distributed storage system. It maintains
// the global view of the cluster topology, routes client requests to appropriate nodes,
// and manages shard assignments. Think of it as a combination of Elasticsearch's
// master node and a smart HTTP router.
//
// Architecture
//
// The coordinator is designed to be:
//   - Stateless: All state can be reconstructed from node registrations
//   - Lightweight: Minimal memory footprint, no data storage
//   - Resilient: Handles node failures gracefully
//   - Simple: Clear separation of concerns, minimal dependencies
//
// Core Components:
//
//   ShardRegistry: Maintains shard-to-node assignments
//     - Tracks which node owns each shard
//     - Calculates shard for a given key
//     - Handles rebalancing (not yet implemented)
//
//   NodeRegistry: Tracks active nodes in the cluster  
//     - Maintains node health status
//     - Handles node registration/deregistration
//     - Provides node discovery
//
//   RequestRouter: Routes client requests to nodes
//     - Calculates target shard for keys
//     - Forwards requests to appropriate nodes
//     - Handles failures and retries (planned)
//
// Request Flow:
//
//   1. Client sends request to coordinator
//   2. Coordinator extracts key from request
//   3. ShardRegistry calculates shard ID for key
//   4. NodeRegistry finds node owning that shard
//   5. RequestRouter forwards request to node
//   6. Node response returned to client
//
// Thread Safety:
//
// All types in this package are thread-safe unless explicitly noted.
// The coordinator handles concurrent requests using read-write locks
// to protect shared state while maximizing parallelism.
//
// Performance Characteristics:
//
//   - Shard calculation: O(1) using FNV hash
//   - Node lookup: O(1) using map lookup
//   - Request forwarding: Single network hop
//   - Memory usage: O(nodes + shards), typically <10MB
//
// Known Limitations:
//
//   - Single coordinator is a single point of failure
//   - No coordinator clustering/HA support yet
//   - Shard assignments not persisted
//   - No request retry or circuit breaking
//
// Future Enhancements:
//
//   - Multi-coordinator support with consensus
//   - Persistent shard assignments
//   - Smart request routing with load awareness
//   - Request retry with exponential backoff
//   - Circuit breakers for failing nodes
//
// Example Usage:
//
//   import "github.com/dreamware/torua/internal/coordinator"
//
//   // Create coordinator with 16 shards
//   coord := coordinator.New(16)
//
//   // Register a node
//   coord.RegisterNode("node1", "http://localhost:8081")
//
//   // Route a request
//   nodeURL := coord.GetNodeForKey("user:123")
//
package coordinator
```

## Documentation Priority Order

When adding documentation to existing code, prioritize in this order:

1. **Public API functions** - These are the primary interface
2. **Complex algorithms** - GetShardForKey, rebalancing logic, etc.
3. **Core types** - Node, ShardRegistry, Coordinator
4. **Package documentation** - doc.go files for each package
5. **Internal helpers** - Lower priority but still needed
6. **Test documentation** - Explain test strategies and why they matter

## Key Principles to Remember

1. **Assume the reader knows nothing about the implementation**
2. **Explain WHY before WHAT** - Context is crucial
3. **Use examples liberally** - Show, don't just tell
4. **Document limitations and trade-offs** - Be honest about shortcomings
5. **Cross-reference related components** - Show the bigger picture
6. **Keep documentation next to code** - It's more likely to be updated
7. **Write for future you** - You'll forget the details in 6 months

## Measuring Documentation Quality

Good documentation answers these questions without reading the code:
- What problem does this solve?
- How does it solve that problem?
- When should I use this?
- What are the limitations?
- How does this relate to other components?
- What happens when things go wrong?
- How can I verify it's working correctly?

If documentation doesn't answer these, it needs improvement.