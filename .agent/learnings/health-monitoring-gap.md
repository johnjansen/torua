# Learning: Health Monitoring Gap

## Discovery Date
2024-01-09

## Context
While working on fixing the cluster-management.feature BDD tests, discovered that the Torua coordinator lacks essential health monitoring capabilities that the tests expect.

## What Was Expected
The cluster-management.feature tests assume the coordinator has the following capabilities:
1. Periodic health checks to registered nodes
2. Node status tracking (healthy/unhealthy/unknown)
3. Automatic detection of node failures
4. Shard redistribution when nodes become unhealthy
5. Health status reporting in the `/nodes` endpoint

## What Actually Exists
The current implementation only has:
1. Basic node registration (`/register`)
2. Static node list storage
3. Simple `/health` endpoint on each component (always returns 200 OK)
4. No background health checking
5. No node status tracking
6. No failure detection or recovery

## Impact
- **Test Coverage**: 18 cluster management scenarios cannot pass without health monitoring
- **Production Readiness**: System cannot handle node failures gracefully
- **Data Availability**: No automatic recovery when nodes fail
- **Operational Burden**: Manual intervention required for any node issues

## Technical Requirements for Implementation

### 1. Health Check Loop
```go
// In coordinator/main.go
func (s *server) startHealthMonitoring(interval time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            s.checkAllNodeHealth()
        }
    }()
}
```

### 2. Node Status Tracking
```go
type NodeStatus struct {
    ID              string
    Addr            string
    Status          string    // "healthy", "unhealthy", "unknown"
    LastHealthCheck time.Time
    FailureCount    int
}
```

### 3. Health Check Implementation
- HTTP GET to each node's `/health` endpoint
- Configurable timeout (e.g., 2 seconds)
- Mark unhealthy after N consecutive failures
- Mark healthy after successful check

### 4. Shard Redistribution on Failure
- Detect unhealthy nodes
- Identify shards on failed nodes
- Reassign to healthy nodes
- Update routing tables

## Lessons Learned

### 1. Test-Driven Development Benefits
The BDD tests revealed a critical gap that might not have been obvious from unit tests alone. The cluster-management scenarios describe real-world operational requirements.

### 2. Feature Completeness vs MVP
The initial implementation focused on basic distributed storage but missed critical operational features. Health monitoring is essential for any distributed system.

### 3. Documentation vs Reality
The tests serve as living documentation of expected behavior. When tests fail, it often reveals gaps between intended and actual functionality.

## Recommended Approach

### Phase 1: Basic Health Monitoring
1. Add health check loop in coordinator
2. Track node status in memory
3. Include status in `/nodes` response
4. Log health state changes

### Phase 2: Failure Detection
1. Configurable failure thresholds
2. Exponential backoff for failed nodes
3. Event system for status changes
4. Admin notifications

### Phase 3: Automatic Recovery
1. Shard redistribution on node failure
2. Rebalancing when nodes recover
3. Data replication for fault tolerance
4. Graceful degradation

## Code Locations Affected
- `cmd/coordinator/main.go` - Add health monitoring goroutine
- `internal/cluster/types.go` - Add NodeStatus type
- `internal/coordinator/shard_registry.go` - Add redistribution logic
- `features/steps/cluster_management_steps.py` - Tests waiting for implementation

## Similar Patterns in Other Systems

### Elasticsearch
- Periodic pings to all nodes
- Master node tracks cluster state
- Automatic shard reallocation
- Red/yellow/green cluster status

### Consul
- Gossip protocol for failure detection
- Serf for membership management
- Automatic leader election
- Health check definitions

### Kubernetes
- Liveness and readiness probes
- Node conditions and taints
- Pod eviction on node failure
- Automatic rescheduling

## Future Considerations
1. **Pluggable Health Checks**: Beyond simple HTTP, support custom health criteria
2. **Cascading Failures**: Prevent redistribution storms during network partitions
3. **Quorum Requirements**: Ensure majority agreement before marking nodes failed
4. **Health Check Delegation**: Let nodes check each other, not just coordinator
5. **Metrics Integration**: Export health status to Prometheus/Grafana

## References
- [Failure Detection in Distributed Systems](https://www.cs.yale.edu/homes/aspnes/pinewiki/FailureDetectors.html)
- [The Phi Accrual Failure Detector](https://github.com/akka/akka/blob/main/akka-remote/src/main/scala/akka/remote/PhiAccrualFailureDetector.scala)
- [Consul's Gossip Protocol](https://www.consul.io/docs/architecture/gossip)