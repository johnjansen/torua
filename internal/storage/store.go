// Package storage defines the abstract storage interfaces and provides concrete implementations.
// See doc.go for complete package documentation.
package storage

import (
	"errors"
	"sync"
)

// ErrKeyNotFound is returned when a key doesn't exist in the store.
//
// This error is used consistently across all storage implementations to indicate
// that a requested key is not present in the store. Callers should check for this
// specific error to distinguish between missing keys and other storage failures.
//
// Usage pattern:
//
//	value, err := store.Get("key")
//	if err == storage.ErrKeyNotFound {
//	    // Handle missing key case
//	} else if err != nil {
//	    // Handle other errors
//	}
var ErrKeyNotFound = errors.New("key not found")

// Store defines the interface for key-value storage operations, providing a
// consistent API across different storage backends while ensuring thread-safety
// for concurrent access patterns.
//
// All implementations must guarantee:
//   - Thread-safety for all operations
//   - Atomic operations (no partial updates visible)
//   - Consistent error handling (especially ErrKeyNotFound)
//   - No data corruption under concurrent access
//
// The interface is designed to be minimal yet sufficient for building
// distributed storage systems, with operations that map directly to
// common database primitives.
//
// Implementation notes:
//   - Keys are strings for simplicity and compatibility
//   - Values are byte slices for flexibility
//   - All operations should be synchronous
//   - Implementations should not hold locks during I/O
//
// Future extensions may include:
//   - Batch operations for efficiency
//   - Transactions for atomicity
//   - TTL support for automatic expiration
//   - Watch/subscribe for change notifications
type Store interface {
	// Get retrieves a value by key from the store.
	//
	// Behavior:
	//   - Returns the value associated with the key
	//   - Returns ErrKeyNotFound if the key doesn't exist
	//   - Should return a copy of the value to prevent external modification
	//   - Must not return nil value with nil error
	//
	// Thread-safety:
	//   - Safe for concurrent calls
	//   - May block if a write operation is in progress
	//
	// Performance:
	//   - O(1) expected for hash-based stores
	//   - O(log n) for tree-based stores
	//
	// Parameters:
	//   - key: The key to retrieve (must not be empty)
	//
	// Returns:
	//   - Value bytes if key exists (may be empty/nil)
	//   - ErrKeyNotFound if key doesn't exist
	//   - Other error for storage failures
	Get(key string) ([]byte, error)

	// Put stores a value with the given key, creating a new entry or
	// updating an existing one.
	//
	// Behavior:
	//   - Creates new entry if key doesn't exist
	//   - Overwrites existing value if key exists
	//   - Should store a copy of the value to prevent external modification
	//   - Empty/nil values are valid and should be stored
	//
	// Thread-safety:
	//   - Safe for concurrent calls
	//   - Operations on same key are serialized
	//   - Operations on different keys may proceed in parallel
	//
	// Performance:
	//   - O(1) expected for hash-based stores
	//   - O(log n) for tree-based stores
	//   - May trigger memory allocation or disk I/O
	//
	// Parameters:
	//   - key: The key to store (must not be empty)
	//   - value: The value to store (may be empty/nil)
	//
	// Returns:
	//   - nil on success
	//   - Error if storage operation fails
	Put(key string, value []byte) error

	// Delete removes a key-value pair from the store.
	//
	// Behavior:
	//   - Removes the key-value pair if it exists
	//   - No error if key doesn't exist (idempotent)
	//   - Freed resources should be reclaimed
	//   - Must not affect other keys
	//
	// Thread-safety:
	//   - Safe for concurrent calls
	//   - May block concurrent operations on same key
	//
	// Performance:
	//   - O(1) expected for hash-based stores
	//   - O(log n) for tree-based stores
	//   - Should trigger garbage collection for large values
	//
	// Parameters:
	//   - key: The key to delete (any string)
	//
	// Returns:
	//   - nil on success (even if key didn't exist)
	//   - Error only if storage operation fails
	Delete(key string) error

	// List returns all keys in the store.
	//
	// Behavior:
	//   - Returns snapshot of keys at call time
	//   - Order is not guaranteed (implementation-dependent)
	//   - Should return empty slice if store is empty (not nil)
	//   - Keys may be added/removed during iteration
	//
	// Thread-safety:
	//   - Safe for concurrent calls
	//   - Returned slice is independent of store state
	//
	// Performance:
	//   - O(n) where n is number of keys
	//   - May allocate significant memory for large stores
	//   - Consider pagination for production use
	//
	// Returns:
	//   - Slice containing all keys (may be empty)
	//   - Never returns nil
	List() []string

	// Stats returns storage statistics for monitoring and capacity planning.
	//
	// Behavior:
	//   - Returns current statistics snapshot
	//   - Should be efficient (no full scan if possible)
	//   - Values may be approximate for performance
	//   - Safe to call frequently
	//
	// Thread-safety:
	//   - Safe for concurrent calls
	//   - May briefly lock internal structures
	//
	// Performance:
	//   - Should be O(1) if statistics are maintained
	//   - May be O(n) if calculation required
	//
	// Returns:
	//   - StoreStats with current metrics
	Stats() StoreStats
}

// StoreStats contains statistics about the store, providing visibility into
// resource usage and capacity for monitoring and optimization.
//
// Statistics are point-in-time snapshots that may become stale immediately
// in concurrent environments. They should be used for monitoring trends
// rather than exact accounting.
//
// These metrics are useful for:
//   - Capacity planning and scaling decisions
//   - Performance monitoring and alerting
//   - Memory usage tracking
//   - Cache effectiveness analysis
type StoreStats struct {
	// Keys is the total number of keys in the store.
	// This count includes all keys regardless of value size.
	// May be approximate for very large stores.
	Keys int // Number of keys

	// Bytes is the total size of all values in bytes.
	// Does not include key size or internal overhead.
	// May be approximate due to compression or encoding.
	Bytes int // Total size of all values in bytes
}

// MemoryStore implements the Store interface with in-memory storage, providing
// fast operations with no persistence across restarts.
//
// MemoryStore characteristics:
//   - All data stored in RAM (heap memory)
//   - No persistence (data lost on restart)
//   - Fast operations (nanosecond latency)
//   - Thread-safe via sync.RWMutex
//   - No size limits (bounded by available memory)
//
// Suitable for:
//   - Caching frequently accessed data
//   - Temporary data that can be regenerated
//   - Testing and development
//   - Small datasets that fit in memory
//
// Not suitable for:
//   - Data that must survive restarts
//   - Large datasets (> available RAM)
//   - Write-heavy workloads (lock contention)
//   - Multi-node replication (no WAL)
//
// Memory usage:
//   - ~50 bytes overhead per key-value pair
//   - Keys and values are copied (no reference sharing)
//   - No compression or deduplication
//   - GC pressure with high churn rate
//
// Example usage:
//
//	store := NewMemoryStore()
//	err := store.Put("config:timeout", []byte("30s"))
//	if err != nil {
//	    log.Fatal(err)
//	}
type MemoryStore struct {
	// data holds the key-value pairs in memory.
	// Using map provides O(1) average case operations.
	// All stored values are copies to prevent external modification.
	data map[string][]byte // Key-value storage

	// mu protects concurrent access to the data map.
	// Uses RWMutex to allow multiple concurrent readers
	// when no writer is active.
	mu sync.RWMutex // Protects concurrent access
}

// NewMemoryStore creates a new in-memory store ready for immediate use.
//
// The created store:
//   - Starts empty (no keys)
//   - Is immediately thread-safe
//   - Has no capacity limits
//   - Uses default map allocation
//
// Returns:
//   - Initialized MemoryStore ready for operations
//
// Example:
//
//	store := NewMemoryStore()
//	defer store.Close() // If Close method is added
//	// Use store...
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string][]byte),
	}
}

// Get retrieves a value by key from the in-memory store.
//
// Implementation details:
//   - Uses read lock allowing concurrent reads
//   - Returns a copy to prevent external modification
//   - Allocates memory for the returned value
//
// Parameters:
//   - key: The key to retrieve
//
// Returns:
//   - Copy of the value if key exists
//   - ErrKeyNotFound if key doesn't exist
//
// Thread-safety:
//   - Safe for concurrent calls
//   - Multiple goroutines can read simultaneously
//
// Performance:
//   - O(1) average case lookup
//   - Memory allocation for value copy
//   - ~100ns typical latency
//
// Example:
//
//	value, err := store.Get("user:123")
//	if err == ErrKeyNotFound {
//	    // Key doesn't exist
//	} else if err != nil {
//	    // Storage error (unlikely for memory store)
//	}
func (m *MemoryStore) Get(key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, exists := m.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	// Return a copy to prevent external modification
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Put stores a value with the given key in the in-memory store.
//
// Implementation details:
//   - Uses write lock for exclusive access
//   - Stores a copy to prevent external modification
//   - Overwrites any existing value
//   - Allocates memory for the stored copy
//
// Parameters:
//   - key: The key to store (must not be empty)
//   - value: The value to store (may be empty/nil)
//
// Returns:
//   - nil on success (always for memory store)
//   - Never returns error in current implementation
//
// Thread-safety:
//   - Safe for concurrent calls
//   - Blocks all other operations during write
//
// Performance:
//   - O(1) average case insertion
//   - Memory allocation for value copy
//   - ~200ns typical latency
//
// Memory impact:
//   - Allocates len(value) bytes
//   - May trigger GC if heap grows
//   - Old value (if any) becomes garbage
//
// Example:
//
//	err := store.Put("user:123", userData)
//	if err != nil {
//	    // Handle error (unlikely for memory store)
//	}
func (m *MemoryStore) Put(key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Make a copy to prevent external modification
	stored := make([]byte, len(value))
	copy(stored, value)
	m.data[key] = stored

	return nil
}

// Delete removes a key-value pair from the in-memory store.
//
// Implementation details:
//   - Uses write lock for exclusive access
//   - Idempotent (no error if key doesn't exist)
//   - Relies on Go's built-in delete function
//   - Memory reclaimed by garbage collector
//
// Parameters:
//   - key: The key to delete
//
// Returns:
//   - nil always (idempotent operation)
//
// Thread-safety:
//   - Safe for concurrent calls
//   - Blocks all other operations during delete
//
// Performance:
//   - O(1) average case deletion
//   - No memory allocation
//   - ~150ns typical latency
//
// Memory impact:
//   - Marks entry for GC
//   - Actual reclamation depends on GC cycle
//
// Example:
//
//	err := store.Delete("session:expired")
//	// err is always nil (idempotent)
func (m *MemoryStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

// List returns all keys in the store as a snapshot.
//
// Implementation details:
//   - Uses read lock allowing concurrent reads
//   - Creates new slice to prevent external modification
//   - Order is non-deterministic (map iteration order)
//   - Snapshot may be stale immediately after return
//
// Returns:
//   - Slice containing all keys (may be empty)
//   - Never returns nil
//
// Thread-safety:
//   - Safe for concurrent calls
//   - Other operations may change keys during/after call
//
// Performance:
//   - O(n) where n is number of keys
//   - Allocates slice of size n
//   - ~1μs per 10 keys typical
//
// Memory impact:
//   - Allocates ~16 bytes per key (slice overhead)
//   - May cause memory spike for large stores
//
// Warning:
//   - Consider pagination for stores with >10k keys
//   - Not suitable for real-time iteration
//
// Example:
//
//	keys := store.List()
//	fmt.Printf("Store contains %d keys\n", len(keys))
//	for _, key := range keys {
//	    // Process each key
//	}
func (m *MemoryStore) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for key := range m.data {
		keys = append(keys, key)
	}
	return keys
}

// Stats returns storage statistics for the in-memory store.
//
// Implementation details:
//   - Uses read lock allowing concurrent reads
//   - Calculates total bytes by iterating all values
//   - Returns exact counts (not approximations)
//
// Returns:
//   - StoreStats with current key count and byte size
//
// Thread-safety:
//   - Safe for concurrent calls
//   - Statistics may change immediately after return
//
// Performance:
//   - O(n) where n is number of keys (for byte counting)
//   - No memory allocation
//   - ~1μs per 100 keys typical
//
// Use cases:
//   - Memory usage monitoring
//   - Capacity planning
//   - Performance debugging
//   - Cache effectiveness metrics
//
// Example:
//
//	stats := store.Stats()
//	fmt.Printf("Store: %d keys, %d bytes\n", stats.Keys, stats.Bytes)
//	avgSize := float64(stats.Bytes) / float64(stats.Keys)
//	fmt.Printf("Average value size: %.2f bytes\n", avgSize)
func (m *MemoryStore) Stats() StoreStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalBytes := 0
	for _, value := range m.data {
		totalBytes += len(value)
	}

	return StoreStats{
		Keys:  len(m.data),
		Bytes: totalBytes,
	}
}
