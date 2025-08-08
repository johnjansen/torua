package shard

import (
	"hash/fnv"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/dreamware/torua/internal/storage"
)

// ShardState represents the current state of a shard
type ShardState string

const (
	// ShardStateActive means the shard is serving requests
	ShardStateActive ShardState = "active"
	// ShardStateMigrating means the shard is being moved
	ShardStateMigrating ShardState = "migrating"
	// ShardStateDeleted means the shard is marked for deletion
	ShardStateDeleted ShardState = "deleted"
)

// Shard represents a data partition in the distributed system
// Each shard owns a portion of the keyspace and manages its own storage
type Shard struct {
	ID      int           // Unique shard identifier
	Primary bool          // Is this the primary or a replica?
	Store   storage.Store // The storage backend for this shard
	State   ShardState    // Current shard state
	Stats   *ShardStats   // Operation statistics
	mu      sync.RWMutex  // Protects state changes
}

// ShardStats tracks operational statistics for a shard
type ShardStats struct {
	Ops     OperationStats     // Operation counts
	Storage storage.StoreStats // Storage statistics
}

// OperationStats tracks operation counts
type OperationStats struct {
	Gets    uint64 // Number of get operations
	Puts    uint64 // Number of put operations
	Deletes uint64 // Number of delete operations
}

// ShardInfo contains metadata about a shard
type ShardInfo struct {
	ID       int        // Shard identifier
	Primary  bool       // Primary or replica
	State    ShardState // Current state
	KeyCount int        // Number of keys
	ByteSize int        // Total size in bytes
}

// NewShard creates a new shard with in-memory storage
func NewShard(id int, primary bool) *Shard {
	return &Shard{
		ID:      id,
		Primary: primary,
		Store:   storage.NewMemoryStore(),
		State:   ShardStateActive,
		Stats:   &ShardStats{},
	}
}

// Get retrieves a value from the shard
// Increments get counter for statistics
func (s *Shard) Get(key string) ([]byte, error) {
	atomic.AddUint64(&s.Stats.Ops.Gets, 1)
	return s.Store.Get(key)
}

// Put stores a value in the shard
// Increments put counter for statistics
func (s *Shard) Put(key string, value []byte) error {
	atomic.AddUint64(&s.Stats.Ops.Puts, 1)
	return s.Store.Put(key, value)
}

// Delete removes a key from the shard
// Increments delete counter for statistics
func (s *Shard) Delete(key string) error {
	atomic.AddUint64(&s.Stats.Ops.Deletes, 1)
	return s.Store.Delete(key)
}

// ListKeys returns all keys in the shard
func (s *Shard) ListKeys() []string {
	return s.Store.List()
}

// OwnsKey determines if this shard owns a given key
// Uses consistent hashing to determine ownership
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

// GetStats returns current shard statistics
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

// Info returns metadata about the shard
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

// SetState updates the shard state
func (s *Shard) SetState(state ShardState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
}

// ListKeysInRange returns all keys in the lexicographic range [start, end)
// The end key is exclusive
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

// DeleteRange deletes all keys in the lexicographic range [start, end)
// Returns the number of keys deleted
func (s *Shard) DeleteRange(start, end string) int {
	keysToDelete := s.ListKeysInRange(start, end)

	for _, key := range keysToDelete {
		s.Delete(key)
	}

	return len(keysToDelete)
}
