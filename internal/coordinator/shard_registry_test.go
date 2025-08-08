package coordinator

import (
	"fmt"
	"sync"
	"testing"
)

// TestNewShardRegistry tests creation of shard registry
func TestNewShardRegistry(t *testing.T) {
	tests := []struct {
		name      string
		numShards int
	}{
		{
			name:      "create with 1 shard",
			numShards: 1,
		},
		{
			name:      "create with 4 shards",
			numShards: 4,
		},
		{
			name:      "create with 100 shards",
			numShards: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewShardRegistry(tt.numShards)

			// Verify registry is created
			if registry == nil {
				t.Fatal("Expected registry instance, got nil")
			}

			// Verify number of shards is set
			if registry.NumShards() != tt.numShards {
				t.Errorf("Expected %d shards, got %d", tt.numShards, registry.NumShards())
			}

			// Verify assignments map is initialized
			if registry.GetAllAssignments() == nil {
				t.Error("Expected assignments to be initialized")
			}

			// Should start with no assignments
			if len(registry.GetAllAssignments()) != 0 {
				t.Errorf("Expected 0 assignments initially, got %d", len(registry.GetAllAssignments()))
			}
		})
	}
}

// TestShardAssignment tests assigning shards to nodes
func TestShardAssignment(t *testing.T) {
	t.Run("assign shard to node", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Assign shard 0 to node1
		err := registry.AssignShard(0, "node1", true)
		if err != nil {
			t.Fatalf("Failed to assign shard: %v", err)
		}

		// Verify assignment
		assignment := registry.GetAssignment(0)
		if assignment == nil {
			t.Fatal("Expected assignment, got nil")
		}

		if assignment.ShardID != 0 {
			t.Errorf("Expected shard ID 0, got %d", assignment.ShardID)
		}
		if assignment.NodeID != "node1" {
			t.Errorf("Expected node ID 'node1', got %s", assignment.NodeID)
		}
		if !assignment.IsPrimary {
			t.Error("Expected primary assignment")
		}
	})

	t.Run("reassign shard to different node", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Initial assignment
		registry.AssignShard(0, "node1", true)

		// Reassign to node2
		err := registry.AssignShard(0, "node2", true)
		if err != nil {
			t.Fatalf("Failed to reassign shard: %v", err)
		}

		// Verify new assignment
		assignment := registry.GetAssignment(0)
		if assignment.NodeID != "node2" {
			t.Errorf("Expected node ID 'node2' after reassignment, got %s", assignment.NodeID)
		}
	})

	t.Run("assign invalid shard ID", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Try to assign shard outside range
		err := registry.AssignShard(5, "node1", true)
		if err == nil {
			t.Error("Expected error for invalid shard ID, got nil")
		}

		// Try negative shard ID
		err = registry.AssignShard(-1, "node1", true)
		if err == nil {
			t.Error("Expected error for negative shard ID, got nil")
		}
	})

	t.Run("assign with empty node ID", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Try to assign with empty node ID
		err := registry.AssignShard(0, "", true)
		if err == nil {
			t.Error("Expected error for empty node ID, got nil")
		}
	})
}

// TestGetShardForKey tests key-to-shard mapping
func TestGetShardForKey(t *testing.T) {
	tests := []struct {
		name      string
		numShards int
		key       string
	}{
		{
			name:      "single shard gets all keys",
			numShards: 1,
			key:       "any-key",
		},
		{
			name:      "key distribution with 4 shards",
			numShards: 4,
			key:       "test-key",
		},
		{
			name:      "empty key",
			numShards: 4,
			key:       "",
		},
		{
			name:      "very long key",
			numShards: 8,
			key:       "this-is-a-very-long-key-that-should-still-hash-correctly-even-though-it-is-much-longer-than-typical-keys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewShardRegistry(tt.numShards)

			// Get shard for key
			shardID := registry.GetShardForKey(tt.key)

			// Verify shard is in valid range
			if shardID < 0 || shardID >= tt.numShards {
				t.Errorf("Shard ID %d out of range [0, %d)", shardID, tt.numShards)
			}

			// Verify consistency - same key should always map to same shard
			for i := 0; i < 10; i++ {
				consistentShardID := registry.GetShardForKey(tt.key)
				if consistentShardID != shardID {
					t.Errorf("Inconsistent shard mapping: got %d, expected %d", consistentShardID, shardID)
				}
			}
		})
	}

	t.Run("key distribution", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Count keys per shard
		shardCounts := make(map[int]int)
		numKeys := 1000

		for i := 0; i < numKeys; i++ {
			key := fmt.Sprintf("key-%d", i)
			shardID := registry.GetShardForKey(key)
			shardCounts[shardID]++
		}

		// Verify all shards get some keys (basic distribution check)
		for shardID := 0; shardID < 4; shardID++ {
			count := shardCounts[shardID]
			if count == 0 {
				t.Errorf("Shard %d got no keys", shardID)
			}
			// Each shard should get roughly 25% of keys (with some variance)
			if count < numKeys/8 || count > numKeys*3/8 {
				t.Errorf("Shard %d has poor distribution: %d keys (expected ~%d)", shardID, count, numKeys/4)
			}
		}
	})
}

// TestGetNodeForKey tests finding the node that owns a key
func TestGetNodeForKey(t *testing.T) {
	t.Run("get node for assigned shard", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Assign shards to nodes
		registry.AssignShard(0, "node1", true)
		registry.AssignShard(1, "node2", true)
		registry.AssignShard(2, "node1", true)
		registry.AssignShard(3, "node2", true)

		// Find a key that maps to shard 0
		var keyForShard0 string
		for i := 0; i < 1000; i++ {
			key := fmt.Sprintf("test-key-%d", i)
			if registry.GetShardForKey(key) == 0 {
				keyForShard0 = key
				break
			}
		}

		// Get node for key
		nodeID, err := registry.GetNodeForKey(keyForShard0)
		if err != nil {
			t.Fatalf("Failed to get node for key: %v", err)
		}

		if nodeID != "node1" {
			t.Errorf("Expected node1 for key in shard 0, got %s", nodeID)
		}
	})

	t.Run("get node for unassigned shard", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Don't assign any shards
		_, err := registry.GetNodeForKey("some-key")
		if err == nil {
			t.Error("Expected error for unassigned shard, got nil")
		}
	})
}

// TestGetAllAssignments tests retrieving all shard assignments
func TestGetAllAssignments(t *testing.T) {
	t.Run("get all assignments", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Assign multiple shards
		registry.AssignShard(0, "node1", true)
		registry.AssignShard(1, "node2", true)
		registry.AssignShard(2, "node1", false) // replica

		// Get all assignments
		assignments := registry.GetAllAssignments()

		// Verify count
		if len(assignments) != 3 {
			t.Errorf("Expected 3 assignments, got %d", len(assignments))
		}

		// Verify each assignment is present
		found := make(map[int]bool)
		for _, assignment := range assignments {
			found[assignment.ShardID] = true
		}

		for _, shardID := range []int{0, 1, 2} {
			if !found[shardID] {
				t.Errorf("Shard %d not found in assignments", shardID)
			}
		}
	})
}

// TestGetNodeShards tests getting all shards for a specific node
func TestGetNodeShards(t *testing.T) {
	t.Run("get shards for node", func(t *testing.T) {
		registry := NewShardRegistry(6)

		// Assign shards to nodes
		registry.AssignShard(0, "node1", true)
		registry.AssignShard(1, "node2", true)
		registry.AssignShard(2, "node1", true)
		registry.AssignShard(3, "node2", true)
		registry.AssignShard(4, "node1", false) // replica
		registry.AssignShard(5, "node3", true)

		// Get shards for node1
		shards := registry.GetNodeShards("node1")

		// Should have 3 shards (0, 2, 4)
		if len(shards) != 3 {
			t.Errorf("Expected 3 shards for node1, got %d", len(shards))
		}

		// Verify shard IDs
		expectedShards := map[int]bool{0: true, 2: true, 4: true}
		for _, shard := range shards {
			if !expectedShards[shard] {
				t.Errorf("Unexpected shard %d for node1", shard)
			}
		}

		// Get shards for node with no assignments
		shards = registry.GetNodeShards("node4")
		if len(shards) != 0 {
			t.Errorf("Expected 0 shards for unassigned node, got %d", len(shards))
		}
	})
}

// TestRemoveShard tests removing shard assignments
func TestRemoveShard(t *testing.T) {
	t.Run("remove assigned shard", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Assign and then remove
		registry.AssignShard(0, "node1", true)
		err := registry.RemoveShard(0)
		if err != nil {
			t.Fatalf("Failed to remove shard: %v", err)
		}

		// Verify shard is removed
		assignment := registry.GetAssignment(0)
		if assignment != nil {
			t.Error("Expected nil assignment after removal")
		}
	})

	t.Run("remove unassigned shard", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Try to remove unassigned shard
		err := registry.RemoveShard(0)
		if err != nil {
			t.Error("Removing unassigned shard should not error")
		}
	})

	t.Run("remove invalid shard ID", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Try to remove invalid shard
		err := registry.RemoveShard(5)
		if err == nil {
			t.Error("Expected error for invalid shard ID")
		}
	})
}

// TestConcurrentOperations tests thread safety of registry
func TestConcurrentOperations(t *testing.T) {
	t.Run("concurrent assignments", func(t *testing.T) {
		registry := NewShardRegistry(100)

		// Concurrent assignments
		var wg sync.WaitGroup
		numGoroutines := 50

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				shardID := id % 100
				nodeID := fmt.Sprintf("node%d", id%10)
				registry.AssignShard(shardID, nodeID, true)
			}(i)
		}

		wg.Wait()

		// Verify some assignments were made
		assignments := registry.GetAllAssignments()
		if len(assignments) == 0 {
			t.Error("Expected some assignments after concurrent operations")
		}
	})

	t.Run("concurrent reads", func(t *testing.T) {
		registry := NewShardRegistry(10)

		// Pre-assign some shards
		for i := 0; i < 10; i++ {
			registry.AssignShard(i, fmt.Sprintf("node%d", i%3), true)
		}

		// Concurrent reads
		var wg sync.WaitGroup
		numReaders := 100

		wg.Add(numReaders)
		for i := 0; i < numReaders; i++ {
			go func(id int) {
				defer wg.Done()
				// Perform various read operations
				key := fmt.Sprintf("key-%d", id)
				registry.GetShardForKey(key)
				registry.GetNodeForKey(key)
				registry.GetAllAssignments()
				registry.GetAssignment(id % 10)
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent mixed operations", func(t *testing.T) {
		registry := NewShardRegistry(20)

		var wg sync.WaitGroup
		numOps := 100

		// Writers
		wg.Add(numOps)
		for i := 0; i < numOps; i++ {
			go func(id int) {
				defer wg.Done()
				shardID := id % 20
				nodeID := fmt.Sprintf("node%d", id%5)
				registry.AssignShard(shardID, nodeID, id%2 == 0)
			}(i)
		}

		// Readers
		wg.Add(numOps)
		for i := 0; i < numOps; i++ {
			go func(id int) {
				defer wg.Done()
				key := fmt.Sprintf("key-%d", id)
				registry.GetShardForKey(key)
				registry.GetNodeForKey(key)
			}(i)
		}

		// Removers
		wg.Add(numOps / 2)
		for i := 0; i < numOps/2; i++ {
			go func(id int) {
				defer wg.Done()
				registry.RemoveShard(id % 20)
			}(i)
		}

		wg.Wait()

		// Registry should still be functional
		err := registry.AssignShard(0, "final-node", true)
		if err != nil {
			t.Errorf("Registry not functional after concurrent ops: %v", err)
		}
	})
}

// TestRebalancing tests shard rebalancing operations
func TestRebalancing(t *testing.T) {
	t.Run("rebalance shards across nodes", func(t *testing.T) {
		registry := NewShardRegistry(12)

		// Initial unbalanced assignment - all to node1
		for i := 0; i < 12; i++ {
			registry.AssignShard(i, "node1", true)
		}

		// Rebalance across 3 nodes
		nodes := []string{"node1", "node2", "node3"}
		err := registry.RebalanceShards(nodes)
		if err != nil {
			t.Fatalf("Failed to rebalance: %v", err)
		}

		// Check distribution
		for _, nodeID := range nodes {
			shards := registry.GetNodeShards(nodeID)
			// Each node should have roughly 4 shards
			if len(shards) < 3 || len(shards) > 5 {
				t.Errorf("Node %s has unbalanced shard count: %d", nodeID, len(shards))
			}
		}

		// Verify all shards are still assigned
		assignments := registry.GetAllAssignments()
		if len(assignments) != 12 {
			t.Errorf("Expected 12 assignments after rebalance, got %d", len(assignments))
		}
	})

	t.Run("rebalance with no nodes", func(t *testing.T) {
		registry := NewShardRegistry(4)

		// Try to rebalance with empty node list
		err := registry.RebalanceShards([]string{})
		if err == nil {
			t.Error("Expected error when rebalancing with no nodes")
		}
	})
}
