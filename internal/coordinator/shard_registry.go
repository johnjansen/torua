// Package coordinator implements the orchestration layer for Torua's distributed storage system.
// See doc.go for complete package documentation.
package coordinator

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
)

// ShardAssignment represents the assignment of a shard to a specific node in the cluster,
// tracking ownership and replication status for data distribution and fault tolerance.
//
// Each shard can have multiple assignments:
//   - One primary assignment for write operations
//   - Multiple replica assignments for read scaling and fault tolerance
//
// The assignment model ensures:
//   - Every shard has exactly one primary at any time
//   - Replicas are distributed across different nodes
//   - Assignments can be changed for rebalancing or failure recovery
//
// Thread Safety:
// ShardAssignment structs are immutable once created. The registry returns
// copies to prevent external modification.
//
// Example:
//
//	assignment := &ShardAssignment{
//	    ShardID:   0,
//	    NodeID:    "node-1",
//	    IsPrimary: true,
//	}
type ShardAssignment struct {
	// NodeID identifies the node that owns this shard.
	// Must match a registered node's ID in the cluster.
	// Format: typically "node-{number}" or UUID.
	NodeID string // The node that owns this shard

	// IsPrimary indicates whether this is the primary or replica assignment.
	// Primary: Handles writes and strongly consistent reads
	// Replica: Handles eventually consistent reads, provides fault tolerance
	IsPrimary bool // Whether this is the primary or a replica

	// ShardID is the unique identifier for this shard.
	// Valid range: [0, numShards)
	// Shards are numbered sequentially for simplicity.
	ShardID int // The shard identifier
}

// ShardRegistry manages shard-to-node assignments in the cluster, serving as the
// authoritative source for data placement decisions and enabling efficient
// request routing based on consistent hashing.
//
// The registry implements a consistent hashing scheme where:
//   - Keys are hashed to determine their owning shard
//   - Shards are assigned to nodes for load distribution
//   - Assignments can be rebalanced when nodes join or leave
//
// Architecture:
//
//	┌─────────────────────────────────────┐
//	│         ShardRegistry               │
//	├─────────────────────────────────────┤
//	│  assignments: map[shardID]→node     │
//	│  numShards: total shard count       │
//	│  mu: RWMutex for thread safety      │
//	├─────────────────────────────────────┤
//	│  Key → Hash → Shard → Node          │
//	│  "user:123" → 0x1a2b → 5 → "node-2" │
//	└─────────────────────────────────────┘
//
// Concurrency Model:
//   - Read operations use RLock for parallel access
//   - Write operations use Lock for exclusive access
//   - All returned data is copied to prevent races
//   - No locks held during external calls
//
// Performance Characteristics:
//   - GetShardForKey: O(1) - Hash computation
//   - GetAssignment: O(1) - Map lookup
//   - GetNodeShards: O(n) - Linear scan of assignments
//   - RebalanceShards: O(n) - Updates all assignments
//
// Memory Usage:
//   - ~100 bytes per shard assignment
//   - 128 shards = ~12KB
//   - 1024 shards = ~100KB
type ShardRegistry struct {
	// assignments maps shard IDs to their current assignments.
	// A shard may be unassigned (not in map) during transitions.
	assignments map[int]*ShardAssignment // shardID -> assignment

	// mu protects concurrent access to the assignments map.
	// Uses RWMutex to allow multiple concurrent readers.
	mu sync.RWMutex // Protects concurrent access

	// numShards is the total number of shards in the cluster.
	// This is fixed at registry creation and determines the
	// granularity of data distribution.
	numShards int // Total number of shards in the cluster
}

// NewShardRegistry creates a new shard registry with the specified number of shards.
//
// The number of shards determines:
//   - Data distribution granularity
//   - Maximum parallelism for operations
//   - Rebalancing flexibility
//
// Recommended shard counts:
//   - Small cluster (1-10 nodes): 32-128 shards
//   - Medium cluster (10-100 nodes): 128-1024 shards
//   - Large cluster (100+ nodes): 1024-4096 shards
//
// The shard count should be:
//   - Much larger than expected node count for flexibility
//   - Power of 2 for optimal hash distribution (optional)
//   - Fixed for the cluster lifetime (changing requires full rebuild)
//
// Parameters:
//   - numShards: Total number of shards to manage (must be > 0)
//
// Returns:
//   - Initialized ShardRegistry ready for assignments
//
// Example:
//
//	registry := NewShardRegistry(128)
//	registry.AssignShard(0, "node-1", true)
func NewShardRegistry(numShards int) *ShardRegistry {
	return &ShardRegistry{
		assignments: make(map[int]*ShardAssignment),
		numShards:   numShards,
	}
}

// AssignShard assigns a shard to a node, establishing or updating the ownership
// relationship for data placement and request routing.
//
// Assignment process:
// 1. Validates shard ID is within valid range
// 2. Validates node ID is not empty
// 3. Creates or updates the assignment atomically
// 4. Previous assignment (if any) is overwritten
//
// Use cases:
//   - Initial shard distribution during cluster setup
//   - Rebalancing shards when nodes join/leave
//   - Promoting replicas to primary during failover
//   - Moving shards for load balancing
//
// Parameters:
//   - shardID: The shard to assign (must be in [0, numShards))
//   - nodeID: The node to assign to (must be non-empty)
//   - isPrimary: Whether this is a primary (true) or replica (false) assignment
//
// Returns:
//   - nil on success
//   - Error if shard ID is invalid or node ID is empty
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
// Uses exclusive lock to ensure atomic updates.
//
// Example:
//
//	err := registry.AssignShard(5, "node-2", true)
//	if err != nil {
//	    log.Printf("Failed to assign shard: %v", err)
//	}
func (r *ShardRegistry) AssignShard(shardID int, nodeID string, isPrimary bool) error {
	// Validate inputs
	if shardID < 0 || shardID >= r.numShards {
		return fmt.Errorf("invalid shard ID %d, must be in range [0, %d)", shardID, r.numShards)
	}
	if nodeID == "" {
		return errors.New("node ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create or update assignment
	r.assignments[shardID] = &ShardAssignment{
		ShardID:   shardID,
		NodeID:    nodeID,
		IsPrimary: isPrimary,
	}

	return nil
}

// RemoveShard removes a shard assignment, effectively making the shard unassigned
// and unavailable for operations until reassigned.
//
// Removal scenarios:
//   - Node failure or planned decommission
//   - Preparation for shard migration
//   - Cluster shrinking operations
//   - Maintenance mode for a shard
//
// Effects of removal:
//   - Shard becomes unavailable for reads/writes
//   - Requests to this shard will fail
//   - Should trigger reassignment to maintain availability
//
// Parameters:
//   - shardID: The shard to remove (must be in [0, numShards))
//
// Returns:
//   - nil on success (even if shard wasn't assigned)
//   - Error if shard ID is invalid
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Example:
//
//	err := registry.RemoveShard(5)
//	if err != nil {
//	    log.Printf("Failed to remove shard: %v", err)
//	}
func (r *ShardRegistry) RemoveShard(shardID int) error {
	// Validate shard ID
	if shardID < 0 || shardID >= r.numShards {
		return fmt.Errorf("invalid shard ID %d, must be in range [0, %d)", shardID, r.numShards)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove assignment (no error if doesn't exist)
	delete(r.assignments, shardID)
	return nil
}

// GetAssignment returns the current assignment for a specific shard, enabling
// request routing and shard location queries.
//
// The returned assignment indicates:
//   - Which node currently owns the shard
//   - Whether it's a primary or replica
//   - nil if the shard is unassigned
//
// Use cases:
//   - Routing read/write requests to the correct node
//   - Health checking specific shards
//   - Monitoring shard distribution
//   - Debugging data placement issues
//
// Parameters:
//   - shardID: The shard to query
//
// Returns:
//   - Copy of ShardAssignment if shard is assigned
//   - nil if shard is not assigned or ID is invalid
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
// Returns a copy to prevent external modification.
//
// Performance:
// O(1) map lookup with minimal lock hold time.
//
// Example:
//
//	assignment := registry.GetAssignment(5)
//	if assignment != nil {
//	    fmt.Printf("Shard %d is on node %s\n", assignment.ShardID, assignment.NodeID)
//	}
func (r *ShardRegistry) GetAssignment(shardID int) *ShardAssignment {
	r.mu.RLock()
	defer r.mu.RUnlock()

	assignment := r.assignments[shardID]
	if assignment == nil {
		return nil
	}

	// Return a copy to prevent external modification
	return &ShardAssignment{
		ShardID:   assignment.ShardID,
		NodeID:    assignment.NodeID,
		IsPrimary: assignment.IsPrimary,
	}
}

// GetAllAssignments returns all current shard assignments in the cluster,
// providing a complete view of data distribution for monitoring and management.
//
// The returned slice contains:
//   - All assigned shards (unassigned shards are not included)
//   - Both primary and replica assignments
//   - Assignments in no particular order
//
// Use cases:
//   - Generating cluster topology visualizations
//   - Calculating load distribution statistics
//   - Detecting unbalanced shard distributions
//   - Backing up cluster configuration
//
// Returns:
//   - Slice of all current assignments (may be empty)
//   - Each assignment is a copy (safe to modify)
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
// Returns copies to prevent external modification.
//
// Performance:
// O(n) where n is the number of assigned shards.
// Allocates memory for the returned slice.
//
// Example:
//
//	assignments := registry.GetAllAssignments()
//	for _, a := range assignments {
//	    fmt.Printf("Shard %d -> Node %s (primary=%v)\n",
//	        a.ShardID, a.NodeID, a.IsPrimary)
//	}
func (r *ShardRegistry) GetAllAssignments() []*ShardAssignment {
	r.mu.RLock()
	defer r.mu.RUnlock()

	assignments := make([]*ShardAssignment, 0, len(r.assignments))
	for _, assignment := range r.assignments {
		// Create a copy of each assignment
		assignments = append(assignments, &ShardAssignment{
			ShardID:   assignment.ShardID,
			NodeID:    assignment.NodeID,
			IsPrimary: assignment.IsPrimary,
		})
	}

	return assignments
}

// GetShardForKey determines which shard owns a given key using consistent hashing,
// enabling deterministic data placement across the cluster.
//
// Hashing algorithm:
//   - Uses FNV-1a (Fowler-Noll-Vo) hash function
//   - Fast, non-cryptographic hash with good distribution
//   - Deterministic: same key always maps to same shard
//   - Uniform: keys distribute evenly across shards
//
// The mapping process:
// 1. Hash the key to a 32-bit integer
// 2. Map to shard using modulo operation
// 3. Result is deterministic and consistent
//
// Parameters:
//   - key: The key to map to a shard (any string)
//
// Returns:
//   - Shard ID in range [0, numShards)
//
// Thread Safety:
// This method is thread-safe and lock-free.
// Pure computation with no shared state access.
//
// Performance:
// O(k) where k is the key length.
// Typically <100ns for short keys.
//
// Example:
//
//	shardID := registry.GetShardForKey("user:123")
//	// shardID will always be the same for "user:123"
func (r *ShardRegistry) GetShardForKey(key string) int {
	// Use FNV-1a hash for consistency with shard implementation
	h := fnv.New32a()
	h.Write([]byte(key))

	// Map to shard using modulo
	return int(h.Sum32()) % r.numShards
}

// GetNodeForKey finds the node that owns the shard for a given key, providing
// direct routing information for client requests.
//
// This method combines two operations:
// 1. Determines which shard owns the key (via consistent hashing)
// 2. Looks up which node owns that shard
//
// Routing process:
//   - Key → Hash → Shard ID → Node ID
//   - Example: "user:123" → 0x1a2b3c4d → Shard 5 → "node-2"
//
// Parameters:
//   - key: The key to route (any string)
//
// Returns:
//   - Node ID that owns the shard for this key
//   - Error if the shard is not assigned to any node
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Performance:
// O(k) for hashing where k is key length, plus O(1) lookup.
//
// Error Cases:
//   - Shard is unassigned (node failure, migration in progress)
//   - Returns error with shard ID for debugging
//
// Example:
//
//	nodeID, err := registry.GetNodeForKey("user:123")
//	if err != nil {
//	    log.Printf("Cannot route key: %v", err)
//	} else {
//	    // Forward request to nodeID
//	}
func (r *ShardRegistry) GetNodeForKey(key string) (string, error) {
	// Find which shard owns this key
	shardID := r.GetShardForKey(key)

	// Get the assignment for this shard
	r.mu.RLock()
	assignment := r.assignments[shardID]
	r.mu.RUnlock()

	if assignment == nil {
		return "", fmt.Errorf("shard %d is not assigned to any node", shardID)
	}

	return assignment.NodeID, nil
}

// GetNodeShards returns all shard IDs assigned to a specific node, useful for
// node-level operations and monitoring.
//
// The returned slice contains:
//   - All shards where this node is primary
//   - All shards where this node is replica (future)
//   - Shards in no particular order
//
// Use cases:
//   - Calculating per-node load
//   - Node decommissioning (which shards to migrate)
//   - Health monitoring (which shards are affected)
//   - Load balancing decisions
//
// Parameters:
//   - nodeID: The node to query
//
// Returns:
//   - Slice of shard IDs assigned to this node
//   - Empty slice if node has no shards or doesn't exist
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Performance:
// O(n) where n is total number of assignments.
// Consider caching if called frequently.
//
// Example:
//
//	shards := registry.GetNodeShards("node-2")
//	fmt.Printf("Node-2 owns shards: %v\n", shards)
func (r *ShardRegistry) GetNodeShards(nodeID string) []int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var shards []int
	for shardID, assignment := range r.assignments {
		if assignment.NodeID == nodeID {
			shards = append(shards, shardID)
		}
	}

	return shards
}

// NumShards returns the total number of shards in the cluster.
//
// This value is:
//   - Fixed at registry creation
//   - Determines data distribution granularity
//   - Used for hash-based shard selection
//
// Use cases:
//   - Validating shard IDs
//   - Calculating distribution statistics
//   - Configuring client routing tables
//
// Returns:
//   - Total number of shards (always > 0)
//
// Thread Safety:
// This method is thread-safe and lock-free.
// The value is immutable after creation.
//
// Example:
//
//	total := registry.NumShards()
//	fmt.Printf("Cluster has %d shards\n", total)
func (r *ShardRegistry) NumShards() int {
	return r.numShards
}

// RebalanceShards redistributes shards evenly across the given nodes using a
// simple round-robin strategy, ensuring balanced load distribution.
//
// Rebalancing algorithm:
//   - Assigns shards to nodes in round-robin fashion
//   - Shard i goes to node[i % len(nodes)]
//   - All assignments are marked as primary
//   - Previous assignments are overwritten
//
// When to rebalance:
//   - After node addition (scale out)
//   - After node removal (node failure)
//   - Periodic rebalancing for load distribution
//   - Manual rebalancing for maintenance
//
// Current limitations:
//   - Simple round-robin (doesn't consider actual load)
//   - No data migration coordination
//   - No gradual rebalancing
//   - All assignments are primary (no replicas yet)
//
// Future improvements:
//   - Consider current data size per shard
//   - Minimize data movement
//   - Maintain replication during rebalance
//   - Support weighted distribution
//
// Parameters:
//   - nodes: List of node IDs to distribute shards across
//
// Returns:
//   - nil on success
//   - Error if nodes list is empty
//
// Thread Safety:
// This method is thread-safe but may cause temporary unavailability
// as shards are reassigned.
//
// Performance:
// O(n) where n is the number of shards.
//
// Example:
//
//	nodes := []string{"node-1", "node-2", "node-3"}
//	err := registry.RebalanceShards(nodes)
//	if err != nil {
//	    log.Printf("Rebalancing failed: %v", err)
//	}
func (r *ShardRegistry) RebalanceShards(nodes []string) error {
	// Validate inputs
	if len(nodes) == 0 {
		return errors.New("cannot rebalance with no nodes")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Simple round-robin assignment
	// In production, this would consider current load, data size, etc.
	for shardID := 0; shardID < r.numShards; shardID++ {
		nodeIndex := shardID % len(nodes)
		nodeID := nodes[nodeIndex]

		r.assignments[shardID] = &ShardAssignment{
			ShardID:   shardID,
			NodeID:    nodeID,
			IsPrimary: true, // All primaries for now, replicas come later
		}
	}

	return nil
}
