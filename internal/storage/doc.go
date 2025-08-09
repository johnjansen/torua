// Package storage defines the abstract storage interfaces and provides concrete
// implementations for Torua's data persistence layer, enabling pluggable storage
// backends with consistent APIs for key-value and graph operations.
//
// # Overview
//
// The storage package is the foundation of Torua's data persistence, providing
// a clean abstraction over various storage engines. It defines interfaces that
// all storage implementations must satisfy, ensuring consistency across different
// backends while allowing for specialized optimizations.
//
// # Architecture
//
// The package follows a layered design:
//
//	┌─────────────────────────────────────┐
//	│         Application Layer           │
//	│         (Shards, Nodes)             │
//	└─────────────────────────────────────┘
//	                 │
//	                 ▼
//	┌─────────────────────────────────────┐
//	│        Storage Interface            │
//	│    (Store, GraphStore, TxStore)     │
//	└─────────────────────────────────────┘
//	                 │
//	    ┌────────────┼────────────┐
//	    ▼            ▼            ▼
//	┌────────┐  ┌────────┐  ┌────────┐
//	│ Memory │  │ RocksDB│  │  Kuzu  │
//	│ Store  │  │ Store  │  │ Store  │
//	└────────┘  └────────┘  └────────┘
//
// # Core Interfaces
//
// Store: Basic key-value storage operations
//   - Get(key) - Retrieve a value by key
//   - Put(key, value) - Store or update a key-value pair
//   - Delete(key) - Remove a key-value pair
//   - Keys() - List all keys in the store
//   - Clear() - Remove all entries
//
// GraphStore (Future): Graph database operations
//   - CreateNode(properties) - Add a graph node
//   - CreateEdge(from, to, type) - Add relationship
//   - Query(cypher) - Execute Cypher queries
//   - Traverse(start, pattern) - Graph traversal
//
// TransactionalStore (Future): ACID transaction support
//   - Begin() - Start a new transaction
//   - Commit() - Persist transaction changes
//   - Rollback() - Discard transaction changes
//   - Snapshot() - Create read snapshot
//
// # Implementations
//
// MemoryStore: In-memory storage with sync.RWMutex
//   - Fast operations (nanosecond latency)
//   - No persistence (data lost on restart)
//   - Suitable for caching and testing
//   - Thread-safe with fine-grained locking
//
// RocksDBStore (Future): LSM-tree based persistent storage
//   - Persistent, crash-safe storage
//   - High write throughput
//   - Compression support
//   - Suitable for large datasets
//
// KuzuStore (Future): Embedded graph database
//   - Native graph operations
//   - Cypher query support
//   - ACID transactions
//   - Suitable for connected data
//
// # Concurrency and Thread Safety
//
// All storage implementations guarantee thread safety:
//
// Locking Strategy:
//   - Read operations use shared locks (RLock)
//   - Write operations use exclusive locks (Lock)
//   - Operations are atomic and isolated
//   - No locks held during I/O operations
//
// Consistency Guarantees:
//   - Sequential consistency for single keys
//   - No guarantees across multiple keys (without transactions)
//   - Snapshot isolation for iterations (implementation-dependent)
//   - Linearizability for CAS operations (future)
//
// Performance Considerations:
//   - Minimize lock hold times
//   - Batch operations where possible
//   - Use appropriate storage backend for workload
//   - Consider sharding for parallelism
//
// # Memory Management
//
// Different backends have different memory characteristics:
//
// MemoryStore:
//   - All data in heap memory
//   - No automatic eviction
//   - GC pressure with large datasets
//   - ~50 bytes overhead per entry
//
// RocksDBStore (Future):
//   - Configurable memory for caches
//   - Block cache for hot data
//   - Write buffer for batching
//   - OS page cache utilization
//
// Best Practices:
//   - Set appropriate cache sizes
//   - Monitor memory usage
//   - Implement eviction policies
//   - Use compression for large values
//
// # Error Handling
//
// The package defines standard error types:
//
// ErrKeyNotFound: Key doesn't exist in store
//   - Returned by Get() and Delete()
//   - Safe to retry after Put()
//   - Check existence with Get() first
//
// ErrStoreClosed: Store has been shut down
//   - No operations allowed
//   - Must create new store instance
//   - Ensure proper cleanup in defer
//
// ErrInvalidKey: Key format is invalid
//   - Empty keys not allowed
//   - Key length limits (backend-specific)
//   - Character restrictions (backend-specific)
//
// ErrValueTooLarge: Value exceeds size limit
//   - Backend-specific limits
//   - Consider chunking large values
//   - Use compression if supported
//
// # Performance Optimization
//
// Tips for optimal performance:
//
// Batching:
//   - Group multiple operations
//   - Reduce lock acquisition overhead
//   - Improve I/O efficiency
//   - Use batch APIs when available
//
// Caching:
//   - Keep frequently accessed data in memory
//   - Use appropriate cache size
//   - Consider cache-aside pattern
//   - Implement cache warming
//
// Sharding:
//   - Distribute load across multiple stores
//   - Reduce lock contention
//   - Enable parallel operations
//   - Balance based on access patterns
//
// Compression:
//   - Reduce storage footprint
//   - Lower I/O bandwidth
//   - Trade CPU for space
//   - Choose algorithm based on data
//
// # Usage Examples
//
//	// Creating a memory store
//	store := storage.NewMemoryStore()
//	defer store.Close()
//
//	// Basic operations
//	err := store.Put("user:123", `{"name":"Alice"}`)
//	if err != nil {
//	    log.Fatalf("Failed to store: %v", err)
//	}
//
//	value, err := store.Get("user:123")
//	if err == storage.ErrKeyNotFound {
//	    log.Println("User not found")
//	} else if err != nil {
//	    log.Fatalf("Failed to retrieve: %v", err)
//	}
//
//	// Iteration
//	keys := store.Keys()
//	for _, key := range keys {
//	    value, _ := store.Get(key)
//	    fmt.Printf("%s: %s\n", key, value)
//	}
//
//	// Batch operations (future)
//	batch := store.NewBatch()
//	batch.Put("key1", "value1")
//	batch.Put("key2", "value2")
//	batch.Delete("key3")
//	err = batch.Commit()
//
//	// Transactions (future)
//	tx := store.Begin()
//	tx.Put("account:1", "100")
//	tx.Put("account:2", "200")
//	if err := tx.Commit(); err != nil {
//	    tx.Rollback()
//	}
//
// # Testing
//
// The package includes comprehensive test suites:
//
// Unit Tests:
//   - Interface compliance tests
//   - Concurrent operation safety
//   - Error condition handling
//   - Memory leak detection
//
// Integration Tests:
//   - Cross-backend compatibility
//   - Performance benchmarks
//   - Stress testing
//   - Crash recovery (persistent stores)
//
// Test Utilities:
//   - Mock store implementation
//   - Test data generators
//   - Assertion helpers
//   - Benchmark harnesses
//
// Running tests:
//
//	go test ./internal/storage/... -cover
//	go test -bench=. ./internal/storage/...
//	go test -race ./internal/storage/...
//
// # Metrics and Monitoring
//
// Storage metrics to track:
//
// Operation Metrics:
//   - storage_ops_total{op="get|put|delete"}
//   - storage_op_duration_seconds{op="..."}
//   - storage_op_errors_total{op="...",error="..."}
//
// Storage Metrics:
//   - storage_keys_total
//   - storage_bytes_total
//   - storage_compression_ratio
//
// Performance Metrics:
//   - storage_cache_hits_total
//   - storage_cache_misses_total
//   - storage_compactions_total
//
// # Migration and Compatibility
//
// Guidelines for storage migrations:
//
// Version Compatibility:
//   - Maintain backward compatibility
//   - Version storage formats
//   - Support gradual migrations
//   - Test upgrade paths
//
// Data Migration:
//   - Online migration support
//   - Batch processing tools
//   - Progress tracking
//   - Rollback capability
//
// Schema Evolution:
//   - Additive changes preferred
//   - Handle missing fields gracefully
//   - Version schemas explicitly
//   - Document breaking changes
//
// # Future Enhancements
//
// Planned improvements:
//
// Near-term:
//   - Batch operation API
//   - Prefix scanning
//   - TTL support
//   - Compression options
//
// Medium-term:
//   - Transaction support
//   - Secondary indexes
//   - Change data capture
//   - Replication hooks
//
// Long-term:
//   - Multi-version concurrency
//   - Distributed transactions
//   - SQL query support
//   - Time-travel queries
//
// # Best Practices
//
// Recommendations for storage usage:
//
// Key Design:
//   - Use hierarchical keys (user:123:profile)
//   - Keep keys reasonably short
//   - Avoid special characters
//   - Consider sort order
//
// Value Format:
//   - Use consistent serialization (JSON, Proto)
//   - Include version information
//   - Validate on write
//   - Handle parsing errors gracefully
//
// Error Handling:
//   - Always check error returns
//   - Handle ErrKeyNotFound explicitly
//   - Implement retry logic
//   - Log storage errors
//
// Resource Management:
//   - Always close stores when done
//   - Use defer for cleanup
//   - Monitor resource usage
//   - Implement graceful shutdown
//
// # See Also
//
// Related packages:
//   - internal/shard: Uses storage for data persistence
//   - internal/coordinator: Manages storage distribution
//   - pkg/client: Client-side storage abstractions
package storage
