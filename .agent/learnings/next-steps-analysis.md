# Learning: Next Steps Analysis for Torua

## Date: 2024-01-15
## Feature: Determining the Next Baby Step

## Current System State

We have a solid foundation:
- ✅ Coordinator-node communication working
- ✅ Node registration and discovery 
- ✅ Health monitoring active
- ✅ Broadcast control messages functional
- ✅ 97% test coverage with TDD practices
- ✅ Clean architecture documented

## The Next Baby Step: Simple Key-Value Storage

After analyzing our options, the best next baby step is to **add a simple in-memory key-value store to each node**. This is the right choice because:

### Why This First (Before Kuzu)

1. **Proves the Distribution Model**: We can test distributed storage without graph complexity
2. **Establishes API Patterns**: Create the REST endpoints we'll use for graph operations later
3. **Simple to Test**: Key-value operations are easy to unit test with 100% coverage
4. **Incremental Progress**: We can build on this to add persistence, then graphs
5. **Immediate Value**: The system becomes useful even before graph integration

### What This Baby Step Includes

```
Phase 1: Local Storage (Current Baby Step)
├── In-memory map at each node
├── GET /store/{key} - Retrieve value
├── PUT /store/{key} - Store value  
├── DELETE /store/{key} - Remove value
└── Tests with 100% coverage

Phase 2: Distributed Operations (Next Baby Step)
├── Coordinator routes requests to correct node
├── Simple hash-based sharding (key % node_count)
├── GET /data/{key} at coordinator level
└── Broadcast updates for replication

Phase 3: Persistence (Following Step)
├── Write to disk for durability
├── WAL (Write-Ahead Logging)
└── Recovery on restart
```

## Implementation Plan for Baby Step #1

### 1. Add Storage to Node (`internal/storage/store.go`)
```go
type Store interface {
    Get(key string) ([]byte, error)
    Put(key string, value []byte) error
    Delete(key string) error
    List() []string
}

type MemoryStore struct {
    mu   sync.RWMutex
    data map[string][]byte
}
```

### 2. Add HTTP Handlers to Node
- `GET /store/{key}` - Return value or 404
- `PUT /store/{key}` - Store value, return 204
- `DELETE /store/{key}` - Delete value, return 204
- `GET /store` - List all keys

### 3. Write Tests First (TDD)
- Test concurrent access
- Test missing keys
- Test overwrites
- Test deletion
- Test list operation

### 4. Add Coordinator Routing (Baby Step #2)
Once local storage works, add:
- Shard calculation: `shard = hash(key) % len(nodes)`
- Route to appropriate node
- Handle node failures gracefully

## Why Not Jump Straight to Kuzu?

1. **Too Big a Leap**: Kuzu integration involves:
   - C++ bindings
   - Schema design  
   - Cypher query parsing
   - Graph algorithms

2. **Untested Distribution**: We haven't proven our distribution model works for data operations

3. **No Baseline**: Hard to measure if Kuzu is working well without a simpler reference

4. **Risk Mitigation**: If distribution doesn't work with simple KV, it won't work with graphs

## Success Criteria for This Baby Step

- [ ] Nodes can store and retrieve data locally
- [ ] 100% test coverage maintained
- [ ] Concurrent access is thread-safe
- [ ] API is RESTful and clear
- [ ] Response times < 10ms for local operations
- [ ] Documentation updated
- [ ] Integration test shows full workflow

## What Comes After

Once we have working KV storage:

1. **Add Sharding**: Distribute keys across nodes
2. **Add Replication**: Store copies for fault tolerance  
3. **Add Persistence**: Survive restarts
4. **Replace with Kuzu**: Swap KV store for graph store
5. **Add Graph Operations**: Traversals, patterns, etc.

## Code Patterns to Follow

Based on our established patterns:

1. **Interface First**: Define `Store` interface before implementation
2. **Test First**: Write failing tests before implementing
3. **Keep It Simple**: Just memory map, no optimization yet
4. **Thread Safe**: Use RWMutex for concurrent access
5. **Error Handling**: Return errors, don't panic
6. **RESTful API**: Standard HTTP verbs and status codes

## Estimated Effort

- Storage interface & memory implementation: 1 hour
- HTTP handlers: 30 minutes  
- Tests with 100% coverage: 1 hour
- Documentation updates: 30 minutes
- **Total: ~3 hours for a working, tested feature**

## Risk Assessment

**Risks**:
- Might over-engineer the storage interface
- Could get distracted by optimization
- Might add features we don't need yet

**Mitigations**:
- Stick to simplest possible implementation
- No optimization until we measure
- Only add what's needed for KV operations

## Decision

✅ **Proceed with simple in-memory key-value storage as the next baby step**

This gives us:
- Immediate progress
- Testable functionality
- Foundation for distribution
- Clear path forward
- Low risk implementation

The motto for this step: **"Make it work, make it right, make it fast"** - and we're firmly in the "make it work" phase.