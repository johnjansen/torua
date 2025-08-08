package coordinator

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
)

// ShardAssignment represents the assignment of a shard to a node
type ShardAssignment struct {
	ShardID   int    // The shard identifier
	NodeID    string // The node that owns this shard
	IsPrimary bool   // Whether this is the primary or a replica
}

// ShardRegistry manages shard-to-node assignments in the cluster
// It tracks which node owns which shard and routes keys accordingly
type ShardRegistry struct {
	mu          sync.RWMutex             // Protects concurrent access
	assignments map[int]*ShardAssignment // shardID -> assignment
	numShards   int                      // Total number of shards in the cluster
}

// NewShardRegistry creates a new shard registry with the specified number of shards
func NewShardRegistry(numShards int) *ShardRegistry {
	return &ShardRegistry{
		assignments: make(map[int]*ShardAssignment),
		numShards:   numShards,
	}
}

// AssignShard assigns a shard to a node
// Returns error if shard ID is invalid or node ID is empty
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

// RemoveShard removes a shard assignment
// Returns error if shard ID is invalid
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

// GetAssignment returns the assignment for a specific shard
// Returns nil if shard is not assigned
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

// GetAllAssignments returns all current shard assignments
// Returns a copy to prevent external modification
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

// GetShardForKey determines which shard owns a given key
// Uses consistent hashing (FNV-1a) to map keys to shards
func (r *ShardRegistry) GetShardForKey(key string) int {
	// Use FNV-1a hash for consistency with shard implementation
	h := fnv.New32a()
	h.Write([]byte(key))

	// Map to shard using modulo
	return int(h.Sum32()) % r.numShards
}

// GetNodeForKey finds the node that owns the shard for a given key
// Returns error if the shard is not assigned to any node
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

// GetNodeShards returns all shard IDs assigned to a specific node
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

// NumShards returns the total number of shards in the cluster
func (r *ShardRegistry) NumShards() int {
	return r.numShards
}

// RebalanceShards redistributes shards evenly across the given nodes
// This is a simple round-robin rebalancing strategy
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
