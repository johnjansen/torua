package storage

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestMemoryStore tests the in-memory store implementation
func TestMemoryStore(t *testing.T) {
	t.Run("new store is empty", func(t *testing.T) {
		store := NewMemoryStore()

		// List should return empty slice
		keys := store.List()
		if len(keys) != 0 {
			t.Errorf("Expected empty store, got %d keys", len(keys))
		}

		// Get should return ErrKeyNotFound
		_, err := store.Get("nonexistent")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("put and get values", func(t *testing.T) {
		store := NewMemoryStore()

		// Put a value
		err := store.Put("key1", []byte("value1"))
		if err != nil {
			t.Fatalf("Failed to put value: %v", err)
		}

		// Get the value back
		value, err := store.Get("key1")
		if err != nil {
			t.Fatalf("Failed to get value: %v", err)
		}

		// Verify the value
		if !bytes.Equal(value, []byte("value1")) {
			t.Errorf("Expected 'value1', got %s", string(value))
		}
	})

	t.Run("overwrite existing key", func(t *testing.T) {
		store := NewMemoryStore()

		// Put initial value
		err := store.Put("key1", []byte("value1"))
		if err != nil {
			t.Fatalf("Failed to put initial value: %v", err)
		}

		// Overwrite with new value
		err = store.Put("key1", []byte("value2"))
		if err != nil {
			t.Fatalf("Failed to overwrite value: %v", err)
		}

		// Get should return new value
		value, err := store.Get("key1")
		if err != nil {
			t.Fatalf("Failed to get value: %v", err)
		}

		if !bytes.Equal(value, []byte("value2")) {
			t.Errorf("Expected 'value2', got %s", string(value))
		}
	})

	t.Run("delete values", func(t *testing.T) {
		store := NewMemoryStore()

		// Put a value
		err := store.Put("key1", []byte("value1"))
		if err != nil {
			t.Fatalf("Failed to put value: %v", err)
		}

		// Delete the value
		err = store.Delete("key1")
		if err != nil {
			t.Fatalf("Failed to delete value: %v", err)
		}

		// Get should return ErrKeyNotFound
		_, err = store.Get("key1")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound after delete, got %v", err)
		}

		// List should be empty
		keys := store.List()
		if len(keys) != 0 {
			t.Errorf("Expected empty store after delete, got %d keys", len(keys))
		}
	})

	t.Run("delete non-existent key", func(t *testing.T) {
		store := NewMemoryStore()

		// Delete non-existent key should not error
		err := store.Delete("nonexistent")
		if err != nil {
			t.Errorf("Delete of non-existent key should not error, got %v", err)
		}
	})

	t.Run("list keys", func(t *testing.T) {
		store := NewMemoryStore()

		// Put multiple values
		testData := map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key3": []byte("value3"),
		}

		for k, v := range testData {
			err := store.Put(k, v)
			if err != nil {
				t.Fatalf("Failed to put %s: %v", k, err)
			}
		}

		// List should return all keys
		keys := store.List()
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

	t.Run("empty and nil values", func(t *testing.T) {
		store := NewMemoryStore()

		// Put empty value
		err := store.Put("empty", []byte{})
		if err != nil {
			t.Fatalf("Failed to put empty value: %v", err)
		}

		// Get empty value back
		value, err := store.Get("empty")
		if err != nil {
			t.Fatalf("Failed to get empty value: %v", err)
		}

		if len(value) != 0 {
			t.Errorf("Expected empty value, got %d bytes", len(value))
		}

		// Put nil value should store empty byte slice
		err = store.Put("nil", nil)
		if err != nil {
			t.Fatalf("Failed to put nil value: %v", err)
		}

		value, err = store.Get("nil")
		if err != nil {
			t.Fatalf("Failed to get nil value: %v", err)
		}

		if value == nil || len(value) != 0 {
			t.Errorf("Expected empty byte slice for nil value, got %v", value)
		}
	})

	t.Run("empty key handling", func(t *testing.T) {
		store := NewMemoryStore()

		// Empty key should be valid
		err := store.Put("", []byte("empty-key-value"))
		if err != nil {
			t.Fatalf("Failed to put with empty key: %v", err)
		}

		// Should be able to get it back
		value, err := store.Get("")
		if err != nil {
			t.Fatalf("Failed to get empty key: %v", err)
		}

		if !bytes.Equal(value, []byte("empty-key-value")) {
			t.Errorf("Expected 'empty-key-value', got %s", string(value))
		}

		// Should appear in list
		keys := store.List()
		found := false
		for _, k := range keys {
			if k == "" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Empty key should appear in list")
		}

		// Should be able to delete it
		err = store.Delete("")
		if err != nil {
			t.Fatalf("Failed to delete empty key: %v", err)
		}
	})
}

// TestMemoryStoreConcurrency tests thread-safe concurrent access
func TestMemoryStoreConcurrency(t *testing.T) {
	t.Run("concurrent writes", func(t *testing.T) {
		store := NewMemoryStore()

		// Number of goroutines and operations
		numGoroutines := 100
		numOps := 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		// Each goroutine writes its own keys
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOps; j++ {
					key := fmt.Sprintf("goroutine-%d-key-%d", id, j)
					value := []byte(fmt.Sprintf("value-%d-%d", id, j))
					if err := store.Put(key, value); err != nil {
						t.Errorf("Failed to put: %v", err)
					}
				}
			}(i)
		}

		wg.Wait()

		// Verify all keys were written
		keys := store.List()
		expectedKeys := numGoroutines * numOps
		if len(keys) != expectedKeys {
			t.Errorf("Expected %d keys, got %d", expectedKeys, len(keys))
		}
	})

	t.Run("concurrent reads", func(t *testing.T) {
		store := NewMemoryStore()

		// Pre-populate store
		numKeys := 100
		for i := 0; i < numKeys; i++ {
			key := fmt.Sprintf("key-%d", i)
			value := []byte(fmt.Sprintf("value-%d", i))
			store.Put(key, value)
		}

		// Concurrent reads
		numReaders := 100
		numReads := 1000

		var wg sync.WaitGroup
		wg.Add(numReaders)

		for i := 0; i < numReaders; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numReads; j++ {
					key := fmt.Sprintf("key-%d", j%numKeys)
					expectedValue := []byte(fmt.Sprintf("value-%d", j%numKeys))

					value, err := store.Get(key)
					if err != nil {
						t.Errorf("Reader %d failed to get %s: %v", id, key, err)
						continue
					}

					if !bytes.Equal(value, expectedValue) {
						t.Errorf("Reader %d got wrong value for %s", id, key)
					}
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent mixed operations", func(t *testing.T) {
		store := NewMemoryStore()

		// Run mixed operations concurrently
		var wg sync.WaitGroup
		numGoroutines := 50
		wg.Add(numGoroutines * 4) // 4 types of operations

		// Writers
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					key := fmt.Sprintf("key-%d", j)
					value := []byte(fmt.Sprintf("writer-%d-value-%d", id, j))
					store.Put(key, value)
				}
			}(i)
		}

		// Readers
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					key := fmt.Sprintf("key-%d", j)
					store.Get(key) // May or may not exist
				}
			}(i)
		}

		// Deleters
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					if j%10 == 0 { // Delete every 10th key
						key := fmt.Sprintf("key-%d", j)
						store.Delete(key)
					}
				}
			}(i)
		}

		// Listers
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					store.List()
					time.Sleep(time.Microsecond) // Small delay to spread out list operations
				}
			}(i)
		}

		wg.Wait()

		// Store should still be functional
		err := store.Put("final-key", []byte("final-value"))
		if err != nil {
			t.Errorf("Store not functional after concurrent ops: %v", err)
		}

		value, err := store.Get("final-key")
		if err != nil {
			t.Errorf("Failed to get final key: %v", err)
		}

		if !bytes.Equal(value, []byte("final-value")) {
			t.Error("Final value incorrect after concurrent ops")
		}
	})

	t.Run("concurrent overwrites", func(t *testing.T) {
		store := NewMemoryStore()

		// Multiple goroutines writing to the same key
		key := "contested-key"
		numWriters := 100
		numWrites := 100

		var wg sync.WaitGroup
		wg.Add(numWriters)

		for i := 0; i < numWriters; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numWrites; j++ {
					value := []byte(fmt.Sprintf("writer-%d-iteration-%d", id, j))
					if err := store.Put(key, value); err != nil {
						t.Errorf("Writer %d failed: %v", id, err)
					}
				}
			}(i)
		}

		wg.Wait()

		// Key should exist with some value (we don't know which writer won)
		value, err := store.Get(key)
		if err != nil {
			t.Errorf("Key should exist after concurrent writes: %v", err)
		}

		if len(value) == 0 {
			t.Error("Value should not be empty after concurrent writes")
		}
	})
}

// TestStoreInterface verifies the Store interface contract
func TestStoreInterface(t *testing.T) {
	// This test ensures MemoryStore implements Store interface
	var _ Store = (*MemoryStore)(nil)

	// Test with actual instance
	var store Store = NewMemoryStore()

	// Verify all interface methods work
	err := store.Put("interface-key", []byte("interface-value"))
	if err != nil {
		t.Fatalf("Interface Put failed: %v", err)
	}

	value, err := store.Get("interface-key")
	if err != nil {
		t.Fatalf("Interface Get failed: %v", err)
	}

	if !bytes.Equal(value, []byte("interface-value")) {
		t.Error("Interface Get returned wrong value")
	}

	keys := store.List()
	if len(keys) != 1 {
		t.Errorf("Interface List returned wrong count: %d", len(keys))
	}

	err = store.Delete("interface-key")
	if err != nil {
		t.Fatalf("Interface Delete failed: %v", err)
	}
}

// TestMemoryStoreStats tests the statistics functionality
func TestMemoryStoreStats(t *testing.T) {
	t.Run("stats tracking", func(t *testing.T) {
		store := NewMemoryStore()

		// Initial stats should be zero
		stats := store.Stats()
		if stats.Keys != 0 || stats.Bytes != 0 {
			t.Errorf("Initial stats should be zero, got keys=%d bytes=%d", stats.Keys, stats.Bytes)
		}

		// Add some data
		testData := map[string][]byte{
			"key1": []byte("value1"),   // 6 bytes
			"key2": []byte("value22"),  // 7 bytes
			"key3": []byte("value333"), // 8 bytes
		}

		for k, v := range testData {
			store.Put(k, v)
		}

		// Check stats
		stats = store.Stats()
		if stats.Keys != 3 {
			t.Errorf("Expected 3 keys, got %d", stats.Keys)
		}

		expectedBytes := 6 + 7 + 8
		if stats.Bytes != expectedBytes {
			t.Errorf("Expected %d bytes, got %d", expectedBytes, stats.Bytes)
		}

		// Delete a key
		store.Delete("key2")

		stats = store.Stats()
		if stats.Keys != 2 {
			t.Errorf("Expected 2 keys after delete, got %d", stats.Keys)
		}

		expectedBytes = 6 + 8
		if stats.Bytes != expectedBytes {
			t.Errorf("Expected %d bytes after delete, got %d", expectedBytes, stats.Bytes)
		}
	})
}
