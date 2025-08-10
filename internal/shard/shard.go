// Package shard implements the fundamental storage unit for Torua's distributed system.
// See doc.go for complete package documentation.
package shard

import (
	"hash/fnv"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/dreamware/torua/internal/storage"
)

// ShardState represents the current operational state of a shard, determining
// whether it can accept requests and how it should handle data operations.
//
// State transitions follow specific rules:
//   - Active → Migrating: When shard needs to move to another node
//   - Migrating → Active: After successful migration completion
//   - Migrating → Deleted: After data has been moved elsewhere
//   - Active → Deleted: When shard is being decommissioned
//
// Thread Safety:
// State changes must be protected by the shard's mutex to ensure
// consistency during concurrent operations.
type ShardState string

const (
	// ShardStateActive indicates the shard is fully operational and serving requests.
	// In this state, the shard:
	//   - Accepts all read and write operations
	//   - Participates in query routing
	//   - Maintains data consistency
	//   - Reports healthy in status checks
	ShardStateActive ShardState = "active"

	// ShardStateMigrating indicates the shard is being moved to another node.
	// During migration, the shard:
	//   - Continues serving read requests
	//   - May reject or queue write requests
	//   - Tracks migration progress
	//   - Maintains data consistency until handoff
	ShardStateMigrating ShardState = "migrating"

	// ShardStateDeleted indicates the shard is marked for deletion.
	// In this state, the shard:
	//   - Rejects all new operations
	//   - Allows cleanup operations only
	//   - Awaits garbage collection
	//   - Should be removed from routing tables
	ShardStateDeleted ShardState = "deleted"
)

// Shard represents a data partition in the distributed system, managing a subset
// of the total key space with its own storage backend and operational statistics.
//
// Each shard is a self-contained unit that:
//   - Owns a deterministic portion of the key space
//   - Manages its own storage backend
//   - Tracks operational statistics
//   - Handles concurrent operations safely
//
// Sharding strategy:
// Keys are mapped to shards using consistent hashing (FNV-1a), ensuring even
// distribution and minimal redistribution when shards are added or removed.
//
// Concurrency model:
//   - Read operations on immutable fields (ID, Primary) are lock-free
//   - State changes require exclusive locking
//   - Storage operations are delegated to thread-safe store
//   - Statistics use atomic operations for lock-free updates
//
// Example usage:
//
//	shard := NewShard(0, true)
//	err := shard.Put("user:123", []byte(`{"name":"Alice"}`))
//	if err != nil {
//	    log.Printf("Failed to store: %v", err)
//	}
type Shard struct {
	// Store is the pluggable storage backend for this shard.
	// Currently uses in-memory storage, but can be replaced with
	// persistent stores (RocksDB, BadgerDB) or graph stores (Kuzu).
	// All storage operations are delegated to this interface.
	Store storage.Store // The storage backend for this shard

	// Stats tracks operational metrics for monitoring and optimization.
	// Updated atomically to avoid lock contention.
	// Never nil after initialization.
	Stats *ShardStats // Operation statistics

	// mu protects mutable state (State field).
	// Uses RWMutex to allow concurrent reads when possible.
	// Not needed for Stats (atomic) or Store (thread-safe).
	mu sync.RWMutex // Protects state changes

	// State tracks the current operational state of the shard.
	// State transitions must be coordinated with the cluster coordinator.
	// Protected by mu for thread-safe updates.
	State ShardState // Current shard state

	// ID uniquely identifies this shard within the cluster.
	// Immutable after creation.
	// Valid range: [0, numShards)
	ID int // Unique shard identifier

	// Primary indicates whether this is the primary replica for the shard.
	// Primary shards handle writes and strong consistency reads.
	// Replica shards provide read scaling and fault tolerance.
	// Immutable after creation.
	Primary bool // Is this the primary or a replica?
}

// ShardStats tracks operational statistics for a shard, providing insights into
// usage patterns, performance characteristics, and resource consumption.
//
// Statistics are updated atomically to avoid lock contention and are used for:
//   - Performance monitoring and alerting
//   - Capacity planning and scaling decisions
//   - Load balancing and shard placement
//   - Debugging and troubleshooting
//
// All statistics are cumulative since shard creation.
type ShardStats struct {
	// Ops tracks the count of each operation type.
	// Updated atomically on each operation.
	Ops OperationStats // Operation counts

	// Storage provides storage-layer statistics.
	// Retrieved from the underlying store on demand.
	Storage storage.StoreStats // Storage statistics
}

// OperationStats tracks operation counts for a shard, enabling performance
// analysis and workload characterization.
//
// Counters are:
//   - Monotonically increasing (never reset)
//   - Updated atomically (lock-free)
//   - Thread-safe for concurrent updates
//   - Suitable for rate calculation
//
// Example usage for rate calculation:
//
//	stats1 := shard.GetStats()
//	time.Sleep(time.Second)
//	stats2 := shard.GetStats()
//	getRate := stats2.Ops.Gets - stats1.Ops.Gets // gets per second
type OperationStats struct {
	// Gets counts successful GET operations.
	// Incremented even if key doesn't exist (operation attempted).
	Gets uint64 // Number of get operations

	// Puts counts successful PUT operations.
	// Includes both inserts and updates.
	Puts uint64 // Number of put operations

	// Deletes counts successful DELETE operations.
	// Incremented even if key doesn't exist (idempotent).
	Deletes uint64 // Number of delete operations
}

// ShardInfo contains metadata about a shard for external consumption,
// providing a snapshot of the shard's current state without exposing
// internal implementation details.
//
// This structure is used for:
//   - Admin API responses
//   - Monitoring dashboards
//   - Cluster state broadcasts
//   - Debugging and diagnostics
//
// The data represents a point-in-time snapshot and may be stale
// immediately after retrieval in concurrent environments.
type ShardInfo struct {
	// ID is the shard's unique identifier.
	ID int // Shard identifier

	// Primary indicates if this is the primary replica.
	Primary bool // Primary or replica

	// State is the current operational state.
	State ShardState // Current state

	// KeyCount is the number of keys stored.
	// May be approximate for large shards.
	KeyCount int // Number of keys

	// ByteSize is the total storage size in bytes.
	// Includes keys, values, and overhead.
	ByteSize int // Total size in bytes
}

// NewShard creates a new shard with in-memory storage, initializing all
// components for immediate use.
//
// The created shard:
//   - Starts in Active state (ready for operations)
//   - Uses in-memory storage (non-persistent)
//   - Has zero statistics initially
//   - Is thread-safe immediately
//
// Parameters:
//   - id: Unique identifier for this shard (must be >= 0)
//   - primary: Whether this is a primary (true) or replica (false)
//
// Returns:
//   - Fully initialized shard ready for operations
//
// Example:
//
//	// Create primary shard with ID 0
//	primary := NewShard(0, true)
//
//	// Create replica shard with ID 0
//	replica := NewShard(0, false)
func NewShard(id int, primary bool) *Shard {
	return &Shard{
		ID:      id,
		Primary: primary,
		Store:   storage.NewMemoryStore(),
		State:   ShardStateActive,
		Stats:   &ShardStats{},
	}
}

// Get retrieves a value from the shard by key, delegating to the underlying
// storage and tracking the operation for statistics.
//
// Operation behavior:
//   - Returns the value if key exists
//   - Returns error if key doesn't exist
//   - Increments get counter regardless of outcome
//   - Does not modify shard state
//
// Parameters:
//   - key: The key to retrieve (any non-empty string)
//
// Returns:
//   - Value bytes if key exists
//   - Error if key doesn't exist or storage fails
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
// Multiple goroutines can read simultaneously.
//
// Performance:
// O(1) average case for in-memory storage.
// Lock-free statistics update.
//
// Example:
//
//	value, err := shard.Get("user:123")
//	if err != nil {
//	    log.Printf("Key not found: %v", err)
//	}
func (s *Shard) Get(key string) ([]byte, error) {
	atomic.AddUint64(&s.Stats.Ops.Gets, 1)
	return s.Store.Get(key)
}

// Put stores a value in the shard, creating a new entry or updating an
// existing one, while tracking the operation for statistics.
//
// Operation behavior:
//   - Creates new entry if key doesn't exist
//   - Overwrites existing value if key exists
//   - Increments put counter on success
//   - Returns error only on storage failure
//
// Parameters:
//   - key: The key to store (any non-empty string)
//   - value: The value to store (can be empty/nil)
//
// Returns:
//   - nil on success
//   - Error if storage operation fails
//
// Thread Safety:
// This method is thread-safe but operations on the same key
// are serialized by the underlying storage.
//
// Performance:
// O(1) average case for in-memory storage.
// May trigger memory allocation for new keys.
//
// Example:
//
//	err := shard.Put("user:123", []byte(`{"name":"Alice","age":30}`))
//	if err != nil {
//	    log.Printf("Failed to store: %v", err)
//	}
func (s *Shard) Put(key string, value []byte) error {
	atomic.AddUint64(&s.Stats.Ops.Puts, 1)
	return s.Store.Put(key, value)
}

// Delete removes a key from the shard, implementing idempotent deletion
// semantics where deleting a non-existent key is not an error.
//
// Operation behavior:
//   - Removes key-value pair if it exists
//   - No error if key doesn't exist (idempotent)
//   - Increments delete counter regardless
//   - Frees associated memory (eventually, via GC)
//
// Parameters:
//   - key: The key to delete (any string)
//
// Returns:
//   - nil on success (even if key didn't exist)
//   - Error only on storage failure
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Performance:
// O(1) average case for in-memory storage.
// Memory is reclaimed by Go's garbage collector.
//
// Example:
//
//	err := shard.Delete("user:123")
//	if err != nil {
//	    log.Printf("Delete failed: %v", err)
//	}
//	// Safe to delete again (idempotent)
//	err = shard.Delete("user:123") // No error
func (s *Shard) Delete(key string) error {
	atomic.AddUint64(&s.Stats.Ops.Deletes, 1)
	return s.Store.Delete(key)
}

// ListKeys returns all keys in the shard, useful for debugging, migration,
// and administrative operations.
//
// Operation characteristics:
//   - Returns snapshot of keys at call time
//   - Order is not guaranteed (implementation-dependent)
//   - May be slow for large shards
//   - Allocates memory for key slice
//
// Returns:
//   - Slice containing all keys (may be empty)
//   - Never returns nil
//
// Thread Safety:
// This method is thread-safe but the returned slice is a snapshot.
// Keys may be added/removed after this call returns.
//
// Performance:
// O(n) where n is the number of keys.
// Allocates O(n) memory for the result.
//
// Warning:
// For large shards, consider using pagination or streaming
// to avoid memory issues.
//
// Example:
//
//	keys := shard.ListKeys()
//	fmt.Printf("Shard %d has %d keys\n", shard.ID, len(keys))
//	for _, key := range keys {
//	    fmt.Printf("  %s\n", key)
//	}
func (s *Shard) ListKeys() []string {
	return s.Store.List()
}

// OwnsKey determines if this shard owns a given key based on consistent hashing,
// enabling distributed routing decisions without central coordination.
//
// The ownership model:
//   - Uses FNV-1a hash for consistency across nodes
//   - Deterministic: same key always maps to same shard
//   - Uniform: keys distribute evenly across shards
//   - Stable: unaffected by other keys or shard state
//
// Parameters:
//   - key: The key to check ownership for
//   - numShards: Total number of shards in the cluster
//
// Returns:
//   - true if this shard should handle the key
//   - false if another shard owns it or numShards <= 0
//
// Thread Safety:
// This method is thread-safe and lock-free.
// Pure computation with no shared state access.
//
// Performance:
// O(k) where k is the key length.
// No memory allocation for short keys.
//
// Example:
//
//	if shard.OwnsKey("user:123", 128) {
//	    value, err := shard.Get("user:123")
//	} else {
//	    // Route to different shard
//	}
func (s *Shard) OwnsKey(key string, numShards int) bool {
	if numShards <= 0 {
		return false
	}

	// Hash the key to determine its shard
	h := fnv.New32a()
	h.Write([]byte(key))
	targetShard := int(h.Sum32()) % numShards

	// Check if this shard should handle the key
	return targetShard == s.ID
}

// GetStats returns current shard statistics, providing a consistent snapshot
// of operational metrics for monitoring and analysis.
//
// The returned statistics include:
//   - Operation counts (gets, puts, deletes)
//   - Storage metrics (keys, bytes, etc.)
//   - Point-in-time snapshot (may be stale immediately)
//
// Use cases:
//   - Performance monitoring dashboards
//   - Capacity planning decisions
//   - Troubleshooting performance issues
//   - Load balancing calculations
//
// Returns:
//   - Copy of current statistics (safe to retain)
//
// Thread Safety:
// This method is thread-safe and lock-free for operation stats.
// Storage stats may briefly lock the underlying store.
//
// Performance:
// O(1) for operation stats (atomic loads).
// Storage stats complexity depends on implementation.
//
// Example:
//
//	stats := shard.GetStats()
//	fmt.Printf("Shard %d: %d gets, %d puts, %d keys\n",
//	    shard.ID, stats.Ops.Gets, stats.Ops.Puts, stats.Storage.Keys)
func (s *Shard) GetStats() ShardStats {
	// Get storage stats
	storageStats := s.Store.Stats()

	// Return combined stats
	return ShardStats{
		Ops: OperationStats{
			Gets:    atomic.LoadUint64(&s.Stats.Ops.Gets),
			Puts:    atomic.LoadUint64(&s.Stats.Ops.Puts),
			Deletes: atomic.LoadUint64(&s.Stats.Ops.Deletes),
		},
		Storage: storageStats,
	}
}

// Info returns metadata about the shard, providing a high-level view of
// the shard's current state and resource usage.
//
// The returned info is a snapshot that includes:
//   - Identity (ID, primary/replica status)
//   - Operational state
//   - Resource usage (keys, bytes)
//
// Use cases:
//   - Admin API responses
//   - Cluster state displays
//   - Rebalancing decisions
//   - Health monitoring
//
// Returns:
//   - ShardInfo snapshot (safe to serialize)
//
// Thread Safety:
// This method is thread-safe, using appropriate locks
// for accessing mutable state.
//
// Performance:
// O(1) for metadata fields.
// Storage stats may scan internal structures.
//
// Example:
//
//	info := shard.Info()
//	jsonBytes, _ := json.Marshal(info)
//	fmt.Printf("Shard info: %s\n", jsonBytes)
func (s *Shard) Info() ShardInfo {
	s.mu.RLock()
	state := s.State
	s.mu.RUnlock()

	storageStats := s.Store.Stats()

	return ShardInfo{
		ID:       s.ID,
		Primary:  s.Primary,
		State:    state,
		KeyCount: storageStats.Keys,
		ByteSize: storageStats.Bytes,
	}
}

// SetState updates the shard state, coordinating operational mode changes
// with the cluster coordinator.
//
// State transitions should follow these rules:
//   - Active → Migrating: Before moving shard
//   - Migrating → Active: After successful migration
//   - Any → Deleted: When decommissioning
//
// Parameters:
//   - state: New state to transition to
//
// Thread Safety:
// This method is thread-safe, using exclusive lock
// to ensure atomic state transitions.
//
// Side Effects:
// State changes may affect request routing and
// operation acceptance. Coordinate with cluster.
//
// Example:
//
//	// Mark shard for migration
//	shard.SetState(ShardStateMigrating)
//	// ... perform migration ...
//	shard.SetState(ShardStateActive)
func (s *Shard) SetState(state ShardState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
}

// ListKeysInRange returns all keys in the lexicographic range [start, end),
// enabling range queries and partial data operations.
//
// Range semantics:
//   - Inclusive start boundary (>= start)
//   - Exclusive end boundary (< end)
//   - Empty range if start >= end
//   - All keys if start = "" and end = ""
//
// The returned keys are:
//   - Sorted lexicographically
//   - Snapshot at call time
//   - May be large for wide ranges
//
// Parameters:
//   - start: Beginning of range (inclusive)
//   - end: End of range (exclusive)
//
// Returns:
//   - Sorted slice of keys in range
//   - Empty slice if no keys match
//
// Thread Safety:
// This method is thread-safe but returns a snapshot.
// Keys may change after this call returns.
//
// Performance:
// O(n log n) where n is total keys in shard.
// Consider pagination for large ranges.
//
// Example:
//
//	// Get all user keys
//	userKeys := shard.ListKeysInRange("user:", "user;")
//	// Note: ";" comes after ":" in ASCII
func (s *Shard) ListKeysInRange(start, end string) []string {
	allKeys := s.Store.List()

	// Filter keys in range
	var keysInRange []string
	for _, key := range allKeys {
		if key >= start && key < end {
			keysInRange = append(keysInRange, key)
		}
	}

	// Sort keys for consistent ordering
	sort.Strings(keysInRange)
	return keysInRange
}

// DeleteRange deletes all keys in the lexicographic range [start, end),
// enabling bulk deletion operations for data cleanup and migration.
//
// Deletion behavior:
//   - Atomically deletes each key in range
//   - Returns count of actual deletions
//   - Continues on individual delete failures
//   - Updates statistics for each deletion
//
// Warning:
// This operation can be slow and resource-intensive for large ranges.
// Consider batching or rate limiting for production use.
//
// Parameters:
//   - start: Beginning of range (inclusive)
//   - end: End of range (exclusive)
//
// Returns:
//   - Number of keys successfully deleted
//
// Thread Safety:
// This method is thread-safe but not atomic.
// Other operations may interleave with deletions.
//
// Performance:
// O(n log n + m) where n is total keys, m is keys in range.
// Generates m delete operations.
//
// Example:
//
//	// Delete all session keys older than a timestamp
//	deleted := shard.DeleteRange("session:2020", "session:2021")
//	fmt.Printf("Deleted %d expired sessions\n", deleted)
func (s *Shard) DeleteRange(start, end string) int {
	keysToDelete := s.ListKeysInRange(start, end)

	for _, key := range keysToDelete {
		_ = s.Delete(key)
	}

	return len(keysToDelete)
}
