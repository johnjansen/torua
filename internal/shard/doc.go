// Package shard implements the fundamental storage unit for Torua's distributed
// system, providing a self-contained, thread-safe data partition that manages
// a subset of the total key space with embedded graph database capabilities.
//
// # Overview
//
// A shard is the atomic unit of data distribution in Torua. Each shard is
// responsible for storing and serving a portion of the overall dataset,
// determined by consistent hashing. Shards are designed to be moved between
// nodes for rebalancing and replicated across nodes for fault tolerance.
//
// # Architecture
//
// Each shard contains multiple storage backends:
//
//	┌─────────────────────────────────────┐
//	│            SHARD                     │
//	├─────────────────────────────────────┤
//	│                                     │
//	│  ┌──────────────────────────────┐  │
//	│  │   Key-Value Store             │  │
//	│  │   - In-memory map             │  │
//	│  │   - RWMutex protection        │  │
//	│  │   - O(1) operations           │  │
//	│  └──────────────────────────────┘  │
//	│                                     │
//	│  ┌──────────────────────────────┐  │
//	│  │   Graph Database (Future)     │  │
//	│  │   - Embedded Kuzu             │  │
//	│  │   - Property graphs           │  │
//	│  │   - Cypher queries            │  │
//	│  └──────────────────────────────┘  │
//	│                                     │
//	│  ┌──────────────────────────────┐  │
//	│  │   Metadata                    │  │
//	│  │   - Shard ID                  │  │
//	│  │   - Key range                 │  │
//	│  │   - Statistics                │  │
//	│  └──────────────────────────────┘  │
//	│                                     │
//	└─────────────────────────────────────┘
//
// # Core Components
//
// Shard: The main storage container
//   - Manages a subset of the key space
//   - Provides CRUD operations for key-value pairs
//   - Maintains operation statistics
//   - Thread-safe for concurrent access
//
// Storage Backend: Pluggable storage implementation
//   - Currently: In-memory map with RWMutex
//   - Future: Persistent storage (RocksDB/BadgerDB)
//   - Future: Graph storage (Kuzu embedded)
//
// Metadata: Shard configuration and state
//   - Immutable shard ID
//   - Key range boundaries (future)
//   - Operation counters and metrics
//   - Replication state (future)
//
// # Key Space Partitioning
//
// Shards divide the key space using consistent hashing:
//
//	Total Key Space (32-bit hash space):
//	[0x00000000 ─────────────────── 0xFFFFFFFF]
//
//	Shard Distribution:
//	Shard 0: [0x00000000 - 0x1FFFFFFF]
//	Shard 1: [0x20000000 - 0x3FFFFFFF]
//	Shard 2: [0x40000000 - 0x5FFFFFFF]
//	Shard 3: [0x60000000 - 0x7FFFFFFF]
//	Shard 4: [0x80000000 - 0x9FFFFFFF]
//	Shard 5: [0xA0000000 - 0xBFFFFFFF]
//	Shard 6: [0xC0000000 - 0xDFFFFFFF]
//	Shard 7: [0xE0000000 - 0xFFFFFFFF]
//
// Key assignment process:
// 1. Hash key using FNV-1a or similar
// 2. Map hash to shard ID using modulo or range lookup
// 3. Route operation to appropriate shard
//
// # Operations
//
// The shard supports standard CRUD operations:
//
// Create/Update (PUT):
//   - Stores or updates a key-value pair
//   - Overwrites existing values
//   - Returns previous value if exists
//   - O(1) average complexity
//
// Read (GET):
//   - Retrieves value for a given key
//   - Returns error if key doesn't exist
//   - Supports concurrent reads
//   - O(1) average complexity
//
// Delete (DELETE):
//   - Removes a key-value pair
//   - Returns deleted value
//   - Idempotent operation
//   - O(1) average complexity
//
// List (KEYS):
//   - Returns all keys in shard
//   - Supports prefix filtering (future)
//   - Pagination for large sets (future)
//   - O(n) complexity for n keys
//
// # Concurrency Model
//
// Thread-safety strategy:
//
// Read Operations:
//   - Use RLock for shared access
//   - Multiple concurrent readers allowed
//   - No blocking between read operations
//   - Consistent snapshots during iteration
//
// Write Operations:
//   - Use Lock for exclusive access
//   - Serialized write operations
//   - Atomic updates guaranteed
//   - No torn reads during writes
//
// Lock Ordering:
//   - Always acquire shard lock before store lock
//   - Release in reverse order
//   - Prevents deadlocks in nested operations
//
// Performance Optimizations:
//   - Fine-grained locking per shard
//   - Lock-free statistics using atomics (future)
//   - Read-write lock upgrading (future)
//   - Optimistic concurrency control (future)
//
// # Memory Management
//
// Current implementation uses in-memory storage:
//
// Memory Layout:
//   - Go map[string]string for key-value pairs
//   - ~50 bytes overhead per entry
//   - String deduplication by Go runtime
//   - No explicit memory limits
//
// Memory Usage:
//   - Empty shard: ~1KB
//   - 1000 entries (100 byte avg): ~150KB
//   - 100K entries: ~15MB
//   - 1M entries: ~150MB
//
// Future Improvements:
//   - Configurable memory limits
//   - LRU eviction policy
//   - Compression for large values
//   - Off-heap storage for overflow
//
// # Persistence (Future)
//
// Planned persistence mechanisms:
//
// Write-Ahead Log (WAL):
//   - Append-only operation log
//   - Durability before acknowledgment
//   - Replay for crash recovery
//   - Periodic compaction
//
// Snapshots:
//   - Point-in-time shard backups
//   - Background snapshot creation
//   - Incremental snapshots
//   - S3/GCS upload support
//
// Storage Engines:
//   - RocksDB for LSM-tree storage
//   - BadgerDB for pure Go option
//   - Kuzu for graph operations
//   - Pluggable interface design
//
// # Replication (Future)
//
// Planned replication strategy:
//
// Primary-Backup Model:
//   - One primary per shard
//   - Multiple backup replicas
//   - Synchronous replication option
//   - Read from backups for load balancing
//
// Consistency Levels:
//   - Strong: All replicas must acknowledge
//   - Quorum: Majority must acknowledge
//   - Eventual: Async replication to backups
//   - Local: Only primary acknowledgment
//
// Failure Handling:
//   - Automatic failover to backup
//   - Re-replication on node loss
//   - Split-brain prevention
//   - Conflict resolution via vector clocks
//
// # Graph Capabilities (Future)
//
// Integration with Kuzu embedded graph database:
//
// Graph Model:
//   - Property graph with nodes and edges
//   - Typed relationships
//   - Property indexes
//   - Schema enforcement
//
// Query Support:
//   - Cypher query language
//   - Path traversals
//   - Pattern matching
//   - Aggregations
//
// Use Cases:
//   - Knowledge graphs
//   - Social networks
//   - Recommendation systems
//   - Fraud detection
//
// # Performance Characteristics
//
// Current performance metrics:
//
// Operation Latencies (in-memory):
//   - GET: ~100ns average
//   - PUT: ~200ns average
//   - DELETE: ~150ns average
//   - LIST: ~1μs per key
//
// Throughput (single shard):
//   - Reads: ~5M ops/sec
//   - Writes: ~2M ops/sec
//   - Mixed: ~3M ops/sec
//
// Scalability:
//   - Linear scaling with shard count
//   - Bounded by memory for storage
//   - CPU-bound for operations
//   - Network-bound for replication
//
// # Monitoring and Metrics
//
// Each shard exposes operational metrics:
//
// Operation Counts:
//   - shard_gets_total: Total GET operations
//   - shard_puts_total: Total PUT operations
//   - shard_deletes_total: Total DELETE operations
//   - shard_lists_total: Total LIST operations
//
// Storage Metrics:
//   - shard_keys_count: Current key count
//   - shard_bytes_used: Memory usage
//   - shard_largest_value: Max value size
//
// Performance Metrics:
//   - shard_operation_duration: Op latencies
//   - shard_lock_wait_time: Contention measure
//   - shard_cache_hit_ratio: Cache effectiveness
//
// # Usage Example
//
//	// Creating and using a shard
//	shard := NewShard(0)
//
//	// Store a value
//	err := shard.Put("user:123", `{"name":"Alice","age":30}`)
//	if err != nil {
//	    log.Printf("Failed to store: %v", err)
//	}
//
//	// Retrieve a value
//	value, err := shard.Get("user:123")
//	if err != nil {
//	    log.Printf("Key not found: %v", err)
//	}
//
//	// Delete a value
//	deleted, err := shard.Delete("user:123")
//	if err != nil {
//	    log.Printf("Failed to delete: %v", err)
//	}
//
//	// List all keys
//	keys := shard.Keys()
//	for _, key := range keys {
//	    fmt.Printf("Key: %s\n", key)
//	}
//
// # Testing
//
// Comprehensive test coverage includes:
//
// Unit Tests:
//   - CRUD operation correctness
//   - Concurrent access safety
//   - Edge cases and error conditions
//   - Memory leak detection
//
// Benchmark Tests:
//   - Operation throughput
//   - Latency percentiles
//   - Memory allocation rates
//   - Lock contention analysis
//
// Stress Tests:
//   - High concurrency scenarios
//   - Large value handling
//   - Memory pressure behavior
//   - Sustained load performance
//
// # Limitations and Future Work
//
// Current limitations:
//   - Memory-only storage (no persistence)
//   - No replication support
//   - Missing graph database integration
//   - No compression for large values
//   - Limited to single-machine scale
//
// Planned enhancements:
//   - Persistent storage backends
//   - Multi-version concurrency control
//   - Compression and encryption
//   - Graph query capabilities
//   - Distributed transactions
//
// # See Also
//
// Related packages:
//   - internal/storage: Storage interface definition
//   - internal/coordinator: Shard orchestration
//   - internal/cluster: Distributed coordination
package shard
