# Learning: Coordinator Shard Routing Implementation Plan

## Date: 2024-01-15
## Feature: Next Baby Step - Distributed Key Routing

## Current State

We now have:
- ✅ Nodes with shard storage capability
- ✅ In-memory key-value store at shard level
- ✅ REST API for shard operations
- ✅ 100% test coverage on storage components

## Next Baby Step: Coordinator-Level Routing

### What This Step Adds

The coordinator needs to:
1. Track shard assignments (which node has which shard)
2. Route key operations to the correct shard/node
3. Provide a unified API that hides sharding complexity

### Implementation Tasks

#### 1. Shard Registry in Coordinator
```go
type ShardAssignment struct {
    ShardID   int
    NodeID    string
    IsPrimary bool
}

type ShardRegistry struct {
    mu          sync.RWMutex
    assignments map[int]*ShardAssignment  // shardID -> assignment
    numShards   int                       // total number of shards
}
```

#### 2. Key-to-Shard Mapping
```go
func (r *ShardRegistry) GetShardForKey(key string) int {
    h := fnv.New32a()
    h.Write([]byte(key))
    return int(h.Sum32()) % r.numShards
}
```

#### 3. Coordinator Endpoints
- `GET /data/{key}` - Get value (routes to appropriate shard)
- `PUT /data/{key}` - Store value (routes to appropriate shard)
- `DELETE /data/{key}` - Delete value (routes to appropriate shard)
- `GET /shards` - List shard assignments
- `POST /shards/assign` - Assign shard to node (admin)

#### 4. Request Flow Example
```
Client: PUT /data/user123 {"name": "Alice"}
    ↓
Coordinator:
    1. Calculate shard = hash("user123") % 4 = 2
    2. Look up node for shard 2 → "node1"
    3. Forward: PUT node1/shard/2/store/user123
    ↓
Node1:
    1. Shard 2 stores the data
    2. Return success
    ↓
Coordinator:
    Return success to client
```

### Test Coverage Plan

1. **Unit Tests**:
   - Shard calculation consistency
   - Registry operations (add, remove, lookup)
   - Concurrent access to registry

2. **Integration Tests**:
   - End-to-end key operations through coordinator
   - Node failure handling
   - Shard reassignment

### Success Criteria

- [ ] Coordinator tracks shard assignments
- [ ] Keys consistently route to correct shards
- [ ] Client doesn't need to know about sharding
- [ ] 100% test coverage maintained
- [ ] Response times < 50ms for routed operations

### Implementation Order

1. **ShardRegistry** (30 mins)
   - Basic CRUD for shard assignments
   - Thread-safe operations
   - Tests first (TDD)

2. **Key Routing Logic** (30 mins)
   - Hash-based shard calculation
   - Node lookup for shard
   - Error handling for missing shards

3. **HTTP Handlers** (45 mins)
   - Data operations (GET/PUT/DELETE)
   - Shard management endpoints
   - Request forwarding logic

4. **Integration Tests** (45 mins)
   - Full request flow
   - Error scenarios
   - Concurrent operations

5. **Documentation** (30 mins)
   - Update API docs
   - Add examples
   - Update architecture diagram

**Total Estimated Time: 3 hours**

### What This Enables

After this step, we'll have:
- True distributed storage (even if simple)
- Foundation for replication (multiple nodes per shard)
- Basis for failover (reassign shards)
- Clear path to Kuzu integration

### Risks and Mitigations

**Risk**: Shard assignments hardcoded initially
**Mitigation**: That's OK for baby step, dynamic assignment comes later

**Risk**: No persistence of shard assignments
**Mitigation**: Nodes re-register on coordinator restart

**Risk**: No replication yet
**Mitigation**: Single point of failure is acceptable for now

### Future Steps After This

1. **Dynamic shard assignment** - Coordinator assigns shards automatically
2. **Replication** - Multiple nodes per shard
3. **Rebalancing** - Move shards between nodes
4. **Persistence** - Save shard assignments
5. **Kuzu integration** - Replace memory store with graph store

### Key Design Decisions

1. **Start with fixed number of shards** (e.g., 4)
   - Simplifies initial implementation
   - Can add shard splitting later

2. **Use same hashing as nodes**
   - Ensures consistency
   - FNV-1a hash with modulo

3. **Synchronous forwarding initially**
   - Simpler error handling
   - Can optimize with async later

4. **No caching at coordinator**
   - Keep coordinator stateless for data
   - Only track metadata

### Code Patterns to Follow

- Test first (TDD)
- Interface-driven design
- Thread-safe with RWMutex
- Clear error messages
- RESTful conventions
- Aggressive commenting

### Definition of Done

- [x] All tests passing
- [x] Coverage > 95%
- [x] Can store and retrieve keys through coordinator
- [x] Sharding is transparent to client
- [x] Documentation updated
- [x] Clean git commit

This baby step transforms our system from standalone nodes to a truly distributed storage system, setting the stage for all future enhancements.