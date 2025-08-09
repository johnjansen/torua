# Critical Issues and Solutions

## Issue Priority Matrix

| Priority | Issue | Impact | Effort | Status |
|----------|-------|--------|--------|--------|
| 🔴 P0 | No Shard Assignment Protocol | System barely functional | Medium | Workaround in place |
| 🔴 P0 | Shard Distribution Race Condition | All data on one node | Medium | Identified |
| 🔴 P0 | No Replication | Data loss on failure | High | Not started |
| 🟡 P1 | No Failure Detection | Requests to dead nodes | Medium | Not started |
| 🟡 P1 | No Retry Logic | Poor reliability | Low | Not started |
| 🟡 P1 | No Load Balancing | Performance bottlenecks | Medium | Not started |
| 🟢 P2 | No Metrics/Monitoring | No observability | Low | Not started |
| 🟢 P2 | No Security | Not production-ready | Medium | Not started |

## Detailed Issue Analysis

### 🔴 P0: No Shard Assignment Protocol

#### Current Behavior
```go
// Coordinator knows shard assignments
coordinator.registry.AssignShard(0, "node1", true)

// But nodes don't receive this information
// They create shards on-demand when requests arrive
if shard == nil {
    newShard := shard.NewShard(shardID, true)
    node.AddShard(newShard)
}
```

#### Problem Details
- **Root Cause**: No communication channel for shard assignments
- **Discovery**: BDD tests failed with "shard not found" errors
- **Workaround**: On-demand shard creation in nodes
- **Files Affected**: 
  - `cmd/coordinator/main.go:autoAssignShards()`
  - `cmd/node/main.go:handleShardRequest()`

#### Proposed Solution
```go
// Option 1: Include in registration response
type RegisterResponse struct {
    NodeID string `json:"node_id"`
    Shards []ShardAssignment `json:"shards"`
}

// Option 2: Separate assignment endpoint
POST /node/{nodeID}/shards
{
    "shards": [
        {"id": 0, "is_primary": true},
        {"id": 2, "is_replica": true}
    ]
}

// Option 3: Broadcast assignments to all nodes
POST /broadcast
{
    "type": "shard_assignment",
    "assignments": {...}
}
```

#### Implementation Steps
1. Define shard assignment protocol
2. Add assignment notification endpoint
3. Update node to receive and process assignments
4. Remove on-demand creation workaround
5. Add tests for assignment protocol

---

### 🔴 P0: Shard Distribution Race Condition

#### Current Behavior
```go
func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
    // Every registration triggers assignment
    s.autoAssignShards()
}

func (s *server) autoAssignShards() {
    // Assigns ALL unassigned shards immediately
    for shardID := 0; shardID < s.registry.NumShards(); shardID++ {
        if !assignedShards[shardID] {
            s.registry.AssignShard(shardID, nodeID, true)
        }
    }
}
```

#### Problem Timeline
1. Node1 registers → Gets shards 0,1,2,3
2. Node2 registers → Gets nothing (all assigned)
3. Node3 registers → Gets nothing

#### Proposed Solution
```go
type CoordinatorConfig struct {
    MinNodesBeforeAssignment int
    RebalanceOnJoin bool
    AssignmentDelay time.Duration
}

func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
    s.nodes = append(s.nodes, req.Node)
    
    if len(s.nodes) >= s.config.MinNodesBeforeAssignment {
        if !s.initialAssignmentDone {
            s.performInitialAssignment()
            s.initialAssignmentDone = true
        } else if s.config.RebalanceOnJoin {
            s.rebalanceShards()
        }
    }
}
```

#### Implementation Steps
1. Add configuration for minimum nodes
2. Implement delayed assignment strategy
3. Add rebalancing algorithm
4. Notify affected nodes of changes
5. Test with various node join sequences

---

### 🔴 P0: No Replication

#### Current State
- Each shard exists on exactly one node
- Complete data loss if node fails
- No read scaling possible

#### Proposed Architecture
```
Shard 0: Primary on Node1, Replicas on Node2, Node3
Shard 1: Primary on Node2, Replicas on Node1, Node3
Shard 2: Primary on Node3, Replicas on Node1, Node2
Shard 3: Primary on Node1, Replicas on Node2, Node3
```

#### Implementation Requirements
```go
type ShardAssignment struct {
    ShardID   int
    NodeID    string
    IsPrimary bool
    Version   int64  // For consistency
    State     string // "active", "syncing", "failed"
}

type ReplicationConfig struct {
    Factor int // Number of copies (including primary)
    Mode   string // "sync" or "async"
    Consistency string // "strong", "eventual"
}
```

#### Synchronization Protocol
```go
// Primary node broadcasts changes
type ReplicationMessage struct {
    ShardID   int
    Operation string // "put", "delete"
    Key       string
    Value     []byte
    Version   int64
    Timestamp time.Time
}

// Replica acknowledges
type ReplicationAck struct {
    ShardID int
    Version int64
    NodeID  string
    Success bool
}
```

---

### 🟡 P1: No Failure Detection

#### Required Components
```go
// Heartbeat mechanism
type Heartbeat struct {
    NodeID    string
    Timestamp time.Time
    Shards    []int
    Load      float64
}

// Node states
const (
    NodeHealthy   = "healthy"
    NodeUnhealthy = "unhealthy"
    NodeDead      = "dead"
)

// Health checker
type HealthChecker struct {
    interval    time.Duration
    timeout     time.Duration
    maxFailures int
}
```

#### Detection Flow
1. Nodes send heartbeat every 5 seconds
2. Coordinator tracks last heartbeat time
3. After 3 missed heartbeats → mark unhealthy
4. After 6 missed heartbeats → mark dead
5. Trigger shard reallocation for dead nodes

---

### 🟡 P1: No Retry Logic

#### Current Problem
```go
// Single attempt, immediate failure
resp, err := http.DefaultClient.Do(req)
if err != nil {
    http.Error(w, "failed", http.StatusBadGateway)
    return
}
```

#### Proposed Solution
```go
type RetryConfig struct {
    MaxAttempts int
    InitialDelay time.Duration
    MaxDelay time.Duration
    BackoffFactor float64
}

func (s *server) forwardWithRetry(req *http.Request, config RetryConfig) (*http.Response, error) {
    var lastErr error
    delay := config.InitialDelay
    
    for attempt := 0; attempt < config.MaxAttempts; attempt++ {
        resp, err := http.DefaultClient.Do(req)
        if err == nil && resp.StatusCode < 500 {
            return resp, nil
        }
        
        lastErr = err
        if attempt < config.MaxAttempts-1 {
            time.Sleep(delay)
            delay = time.Duration(float64(delay) * config.BackoffFactor)
            if delay > config.MaxDelay {
                delay = config.MaxDelay
            }
        }
    }
    
    return nil, lastErr
}
```

---

### 🟡 P1: No Load Balancing

#### Current Routing
```go
// Always routes to primary, no consideration of load
nodeID, err := s.registry.GetNodeForKey(key)
targetURL := fmt.Sprintf("%s/shard/%d/store/%s", nodeAddr, shardID, key)
```

#### Improved Routing
```go
type LoadBalancer struct {
    strategy string // "round-robin", "least-connections", "weighted"
}

func (s *server) selectNode(shardID int, operation string) string {
    assignments := s.registry.GetAssignmentsForShard(shardID)
    
    if operation == "GET" {
        // Can use any replica
        return s.loadBalancer.SelectNode(assignments)
    } else {
        // Must use primary for writes
        return s.registry.GetPrimaryForShard(shardID)
    }
}
```

## Testing Strategy for Fixes

### Unit Tests Required
- Shard assignment protocol
- Rebalancing algorithm
- Replication message handling
- Heartbeat processing
- Retry logic with backoff
- Load balancer selection

### Integration Tests Required
- Multi-node shard assignment
- Node failure and recovery
- Replication consistency
- Request retry scenarios
- Load distribution verification

### BDD Scenarios to Add
```gherkin
Feature: Shard Assignment Protocol
  Scenario: Nodes receive shard assignments
    Given a coordinator with 4 shards
    When node "n1" registers
    Then node "n1" should receive shard assignments
    And node "n1" should create the assigned shards

Feature: Replication
  Scenario: Data replicated across nodes
    Given replication factor is 2
    When I store "data" with key "test"
    Then the data should exist on 2 nodes
    When the primary node fails
    Then the data should still be retrievable

Feature: Failure Detection
  Scenario: Dead node detection
    Given node "n1" is healthy
    When node "n1" stops sending heartbeats
    Then coordinator should mark "n1" as unhealthy after 15 seconds
    And coordinator should mark "n1" as dead after 30 seconds
    And coordinator should reassign shards from "n1"
```

## Monitoring Requirements

### Metrics to Track
- Shard distribution balance
- Node health status
- Replication lag
- Request retry rate
- Failed request rate
- Response time percentiles
- Load distribution across nodes

### Alerts to Configure
- Node down > 1 minute
- Replication lag > 10 seconds
- Failed requests > 1%
- Unbalanced shard distribution
- No replicas for shard

## Rollout Plan

### Week 1-2: Foundation
- [ ] Implement shard assignment protocol
- [ ] Fix shard distribution race condition
- [ ] Add basic rebalancing

### Week 3-4: Reliability
- [ ] Add replication (factor 2)
- [ ] Implement failure detection
- [ ] Add retry logic

### Week 5-6: Performance
- [ ] Add load balancing
- [ ] Optimize replication protocol
- [ ] Add caching layer

### Week 7-8: Production Readiness
- [ ] Add monitoring/metrics
- [ ] Security implementation
- [ ] Documentation completion
- [ ] Performance testing

## Success Criteria

### Functional
- ✅ All shards have at least 2 copies
- ✅ System survives single node failure
- ✅ Automatic rebalancing on node join/leave
- ✅ All BDD tests passing

### Performance
- ✅ <10ms p50 latency
- ✅ <100ms p99 latency
- ✅ 10,000 ops/sec per node
- ✅ Linear scaling with nodes

### Reliability
- ✅ 99.9% availability
- ✅ Zero data loss with replication
- ✅ Automatic failure recovery
- ✅ Graceful degradation