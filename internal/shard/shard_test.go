package shard

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"testing"

	"github.com/dreamware/torua/internal/storage"
)

// TestNewShard tests shard creation
func TestNewShard(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		primary bool
	}{
		{
			name:    "create primary shard",
			id:      0,
			primary: true,
		},
		{
			name:    "create replica shard",
			id:      1,
			primary: false,
		},
		{
			name:    "create shard with large ID",
			id:      999999,
			primary: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shard := NewShard(tt.id, tt.primary)

			// Verify shard is created
			if shard == nil {
				t.Fatal("Expected shard instance, got nil")
			}

			// Verify ID is set correctly
			if shard.ID != tt.id {
				t.Errorf("Expected shard ID %d, got %d", tt.id, shard.ID)
			}

			// Verify primary flag is set correctly
			if shard.Primary != tt.primary {
				t.Errorf("Expected primary=%v, got %v", tt.primary, shard.Primary)
			}

			// Verify store is initialized
			if shard.Store == nil {
				t.Error("Expected store to be initialized")
			}

			// Verify stats are initialized
			if shard.Stats == nil {
				t.Error("Expected stats to be initialized")
			}
		})
	}
}

// TestShardKeyOperations tests key-value operations on a shard
func TestShardKeyOperations(t *testing.T) {
	t.Run("get and put operations", func(t *testing.T) {
		shard := NewShard(0, true)

		// Put a value
		err := shard.Put("key1", []byte("value1"))
		if err != nil {
			t.Fatalf("Failed to put value: %v", err)
		}

		// Get the value back
		value, err := shard.Get("key1")
		if err != nil {
			t.Fatalf("Failed to get value: %v", err)
		}

		// Verify the value
		if !bytes.Equal(value, []byte("value1")) {
			t.Errorf("Expected 'value1', got %s", string(value))
		}
	})

	t.Run("delete operation", func(t *testing.T) {
		shard := NewShard(0, true)

		// Put a value
		err := shard.Put("key1", []byte("value1"))
		if err != nil {
			t.Fatalf("Failed to put value: %v", err)
		}

		// Delete the value
		err = shard.Delete("key1")
		if err != nil {
			t.Fatalf("Failed to delete value: %v", err)
		}

		// Get should return error
		_, err = shard.Get("key1")
		if err != storage.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound after delete, got %v", err)
		}
	})

	t.Run("list keys", func(t *testing.T) {
		shard := NewShard(0, true)

		// Put multiple values
		testData := map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key3": []byte("value3"),
		}

		for k, v := range testData {
			err := shard.Put(k, v)
			if err != nil {
				t.Fatalf("Failed to put %s: %v", k, err)
			}
		}

		// List should return all keys
		keys := shard.ListKeys()
		if len(keys) != len(testData) {
			t.Errorf("Expected %d keys, got %d", len(testData), len(keys))
		}

		// Verify all keys are present
		keyMap := make(map[string]bool)
		for _, k := range keys {
			keyMap[k] = true
		}

		for k := range testData {
			if !keyMap[k] {
				t.Errorf("Expected key %s in list", k)
			}
		}
	})
}

// TestShardOwnership tests key ownership determination
func TestShardOwnership(t *testing.T) {
	// First, find a key that hashes to shard 0 with 4 shards
	var keyForShard0 string
	for i := 0; i < 1000; i++ {
		testKey := fmt.Sprintf("test-key-%d", i)
		h := fnv.New32a()
		h.Write([]byte(testKey))
		if int(h.Sum32())%4 == 0 {
			keyForShard0 = testKey
			break
		}
	}

	tests := []struct {
		name      string
		shardID   int
		key       string
		numShards int
		shouldOwn bool
	}{
		{
			name:      "shard 0 owns key that hashes to 0",
			shardID:   0,
			key:       keyForShard0,
			numShards: 4,
			shouldOwn: true,
		},
		{
			name:      "shard 1 doesn't own key for shard 0",
			shardID:   1,
			key:       keyForShard0,
			numShards: 4,
			shouldOwn: false,
		},
		{
			name:      "single shard owns all keys",
			shardID:   0,
			key:       "any-key",
			numShards: 1,
			shouldOwn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shard := NewShard(tt.shardID, true)

			// Check ownership
			owns := shard.OwnsKey(tt.key, tt.numShards)
			if owns != tt.shouldOwn {
				t.Errorf("Expected OwnsKey=%v, got %v", tt.shouldOwn, owns)
			}
		})
	}
}

// TestShardStats tests statistics tracking
func TestShardStats(t *testing.T) {
	t.Run("track operations", func(t *testing.T) {
		shard := NewShard(0, true)

		// Initial stats should be zero
		stats := shard.GetStats()
		if stats.Ops.Gets != 0 || stats.Ops.Puts != 0 || stats.Ops.Deletes != 0 {
			t.Error("Initial operation stats should be zero")
		}

		// Perform operations
		shard.Put("key1", []byte("value1"))
		shard.Put("key2", []byte("value2"))
		shard.Get("key1")
		shard.Get("key1") // Get same key twice
		shard.Delete("key2")

		// Check operation counts
		stats = shard.GetStats()
		if stats.Ops.Puts != 2 {
			t.Errorf("Expected 2 puts, got %d", stats.Ops.Puts)
		}
		if stats.Ops.Gets != 2 {
			t.Errorf("Expected 2 gets, got %d", stats.Ops.Gets)
		}
		if stats.Ops.Deletes != 1 {
			t.Errorf("Expected 1 delete, got %d", stats.Ops.Deletes)
		}
	})

	t.Run("track storage size", func(t *testing.T) {
		shard := NewShard(0, true)

		// Add data
		shard.Put("key1", []byte("value1"))   // 6 bytes
		shard.Put("key2", []byte("value22"))  // 7 bytes
		shard.Put("key3", []byte("value333")) // 8 bytes

		// Check storage stats
		stats := shard.GetStats()
		if stats.Storage.Keys != 3 {
			t.Errorf("Expected 3 keys, got %d", stats.Storage.Keys)
		}

		expectedBytes := 6 + 7 + 8
		if stats.Storage.Bytes != expectedBytes {
			t.Errorf("Expected %d bytes, got %d", expectedBytes, stats.Storage.Bytes)
		}
	})
}

// TestShardInfo tests shard metadata
func TestShardInfo(t *testing.T) {
	t.Run("get shard info", func(t *testing.T) {
		shard := NewShard(42, true)

		// Add some data
		shard.Put("key1", []byte("value1"))
		shard.Put("key2", []byte("value2"))

		// Get info
		info := shard.Info()

		// Verify info
		if info.ID != 42 {
			t.Errorf("Expected shard ID 42, got %d", info.ID)
		}

		if !info.Primary {
			t.Error("Expected primary=true")
		}

		if info.State != ShardStateActive {
			t.Errorf("Expected active state, got %s", info.State)
		}

		if info.KeyCount != 2 {
			t.Errorf("Expected 2 keys, got %d", info.KeyCount)
		}

		if info.ByteSize == 0 {
			t.Error("Expected non-zero byte size")
		}
	})

	t.Run("shard states", func(t *testing.T) {
		shard := NewShard(0, true)

		// Initial state should be active
		if shard.State != ShardStateActive {
			t.Errorf("Expected initial state to be active, got %s", shard.State)
		}

		// Set to migrating
		shard.SetState(ShardStateMigrating)
		if shard.State != ShardStateMigrating {
			t.Errorf("Expected state to be migrating, got %s", shard.State)
		}

		// Set to deleted
		shard.SetState(ShardStateDeleted)
		if shard.State != ShardStateDeleted {
			t.Errorf("Expected state to be deleted, got %s", shard.State)
		}
	})
}

// TestShardRangeOperations tests operations on key ranges
func TestShardRangeOperations(t *testing.T) {
	t.Run("get keys in range", func(t *testing.T) {
		shard := NewShard(0, true)

		// Add keys with numeric suffixes for easy range testing
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("key_%02d", i)
			value := []byte(fmt.Sprintf("value_%d", i))
			shard.Put(key, value)
		}

		// Get keys in range
		keys := shard.ListKeysInRange("key_03", "key_07")

		// Should include key_03, key_04, key_05, key_06 (not key_07, exclusive end)
		expectedCount := 4
		if len(keys) != expectedCount {
			t.Errorf("Expected %d keys in range, got %d", expectedCount, len(keys))
		}

		// Verify the keys are in the expected range
		for _, key := range keys {
			if key < "key_03" || key >= "key_07" {
				t.Errorf("Key %s is outside expected range [key_03, key_07)", key)
			}
		}
	})

	t.Run("delete keys in range", func(t *testing.T) {
		shard := NewShard(0, true)

		// Add keys
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("key_%02d", i)
			value := []byte(fmt.Sprintf("value_%d", i))
			shard.Put(key, value)
		}

		// Delete keys in range [key_03, key_07)
		deleted := shard.DeleteRange("key_03", "key_07")

		// Should have deleted 4 keys
		if deleted != 4 {
			t.Errorf("Expected to delete 4 keys, deleted %d", deleted)
		}

		// Verify keys are deleted
		for i := 3; i < 7; i++ {
			key := fmt.Sprintf("key_%02d", i)
			_, err := shard.Get(key)
			if err != storage.ErrKeyNotFound {
				t.Errorf("Expected key %s to be deleted", key)
			}
		}

		// Verify other keys still exist
		for i := 0; i < 3; i++ {
			key := fmt.Sprintf("key_%02d", i)
			_, err := shard.Get(key)
			if err != nil {
				t.Errorf("Expected key %s to still exist", key)
			}
		}
	})
}

// TestShardConcurrency tests concurrent operations on a shard
func TestShardConcurrency(t *testing.T) {
	t.Run("concurrent operations", func(t *testing.T) {
		shard := NewShard(0, true)

		// Number of goroutines and operations
		numGoroutines := 50
		numOps := 100

		// Channel to collect errors
		errors := make(chan error, numGoroutines*3)
		done := make(chan bool)

		// Writers
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				for j := 0; j < numOps; j++ {
					key := fmt.Sprintf("writer-%d-key-%d", id, j)
					value := []byte(fmt.Sprintf("value-%d-%d", id, j))
					if err := shard.Put(key, value); err != nil {
						errors <- err
					}
				}
				done <- true
			}(i)
		}

		// Readers
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				for j := 0; j < numOps; j++ {
					key := fmt.Sprintf("writer-%d-key-%d", id%numGoroutines, j)
					shard.Get(key) // May or may not exist yet
				}
				done <- true
			}(i)
		}

		// Listers
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				for j := 0; j < 10; j++ {
					shard.ListKeys()
					shard.GetStats()
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < numGoroutines*3; i++ {
			<-done
		}

		// Check for errors
		select {
		case err := <-errors:
			t.Fatalf("Concurrent operation failed: %v", err)
		default:
			// No errors
		}

		// Verify shard is still functional
		err := shard.Put("final-key", []byte("final-value"))
		if err != nil {
			t.Errorf("Shard not functional after concurrent ops: %v", err)
		}

		value, err := shard.Get("final-key")
		if err != nil {
			t.Errorf("Failed to get final key: %v", err)
		}

		if !bytes.Equal(value, []byte("final-value")) {
			t.Error("Final value incorrect after concurrent ops")
		}

		// Check stats are reasonable
		stats := shard.GetStats()
		if stats.Storage.Keys == 0 {
			t.Error("Expected non-zero keys after concurrent operations")
		}
	})
}
