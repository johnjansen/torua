# Learning: Shard Architecture Clarification

## Date: 2024-01-15
## Feature: Clarifying Where Storage Lives - Node vs Shard

## The Critical Question

"Is the in-memory store in a shard within a node or somewhere else?"

This question reveals an important architectural decision we need to make explicit.

## The Correct Architecture

```
Coordinator (Orchestration Layer)
    ↓
Nodes (Physical/Process Layer)
    ↓
Shards (Logical Data Partitions)
    ↓
Store (Storage Implementation - Memory/Kuzu)
```

## Where Storage Should Live

**Answer: The Store belongs at the SHARD level, not the Node level**

### Why Shard-Level Storage?

1. **Shards are the unit of data distribution**
   - Data is partitioned into shards
   - Shards are distributed across nodes
   - Each shard owns a subset of the keyspace/graph

2. **Nodes are shard containers**
   - A node can host multiple shards
   - Nodes provide compute and memory resources
   - Nodes handle network communication

3. **This matches Elasticsearch's model**
   - ES Nodes host multiple shards
   - Each shard is an independent Lucene index
   - Shards can move between nodes

## Revised Architecture

### Current (Incorrect) Thinking
```
Node 1
  └── Store (all data for this node)

Node 2
  └── Store (all data for this node)
```

### Correct Architecture
```
Node 1
  ├── Shard 0
  │   └── Store (data for keys 0-99)
  └── Shard 2
      └── Store (data for keys 200-299)

Node 2
  ├── Shard 1
  │   └── Store (data for keys 100-199)
  └── Shard 3
      └── Store (data for keys 300-399)
```

## Implementation Implications

### 1. Need Shard Abstraction
```go
type Shard struct {
    ID     int
    Store  Store  // The storage for this shard
    Primary bool   // Is this the primary or replica?
}

type Node struct {
    ID     string
    Shards map[int]*Shard  // Multiple shards per node
}
```

### 2. Routing Becomes Two-Level
```go
// Coordinator determines:
// 1. Which shard owns this key?
shardID := getShardForKey(key)

// 2. Which node hosts this shard?
nodeID := getNodeForShard(shardID)

// 3. Route to that node/shard
node.GetShard(shardID).Store.Get(key)
```

### 3. Shard Management Operations
- Create shard
- Move shard between nodes
- Replicate shard
- Split/merge shards
- Rebalance shards across nodes

## Baby Step Adjustment

Given this clarification, our baby step should be:

### Step 1: Add Shard abstraction (even with just 1 shard per node initially)
```go
// internal/shard/shard.go
type Shard struct {
    ID    int
    Store storage.Store
}

// Each node starts with one shard
node.AddShard(0, storage.NewMemoryStore())
```

### Step 2: Route through shard
```go
// GET /shard/{shardID}/store/{key}
func (n *Node) handleShardGet(shardID int, key string) {
    shard := n.shards[shardID]
    if shard == nil {
        return 404 // Don't have this shard
    }
    return shard.Store.Get(key)
}
```

### Step 3: Coordinator tracks shard assignments
```go
type ShardMapping struct {
    ShardID int
    NodeID  string
    Primary bool
}

coordinator.shardMap = []ShardMapping{
    {ShardID: 0, NodeID: "n1", Primary: true},
    {ShardID: 1, NodeID: "n2", Primary: true},
}
```

## Benefits of Getting This Right Now

1. **Clean separation of concerns**
   - Nodes = compute/resources
   - Shards = data ownership
   - Store = storage implementation

2. **Future flexibility**
   - Can move shards between nodes
   - Can have multiple shards per node
   - Can replicate shards easily

3. **Scalability path**
   - Add nodes → redistribute shards
   - Hot shard → split into multiple shards
   - Cold shards → merge together

4. **Easier Kuzu integration**
   - Each shard gets its own Kuzu instance
   - No shared state between shards
   - Clean transaction boundaries

## Example Flow

1. **Client**: PUT /data/user123 {"name": "Alice"}
2. **Coordinator**: 
   - Calculates: shard = hash("user123") % 4 = 2
   - Looks up: shard 2 is on node "n1"
   - Routes: PUT n1/shard/2/store/user123
3. **Node n1**:
   - Finds shard 2 in its shard map
   - Calls: shard[2].Store.Put("user123", data)
4. **Shard 2's Store**:
   - Stores the data (in memory for now)

## Decision

✅ **Implement Shard abstraction from the beginning**

Even if we start with just one shard per node, having the abstraction in place:
- Makes the architecture cleaner
- Avoids refactoring later
- Matches industry patterns (Elasticsearch, MongoDB)
- Enables proper scaling

The slight additional complexity is worth the architectural correctness.

## Updated Baby Step Plan

1. Create Shard abstraction with Store
2. Modify Node to manage Shards
3. Update Coordinator to track shard assignments
4. Implement shard-aware routing
5. Test with single shard per node
6. Later: multiple shards, replication, rebalancing

This is the right foundation for a distributed system.