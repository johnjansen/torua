# Ready for Health Monitoring Implementation

## Current State: Documentation Phase Complete ✅

### What's Done
- ✅ 100% documentation coverage for all packages
- ✅ 100% documentation for all public functions (47/47)
- ✅ 100% documentation for all exported types (12/12)
- ✅ Architecture diagrams added throughout
- ✅ Thread safety documented for all concurrent operations
- ✅ 22/39 BDD scenarios passing

### Test Status
```
Total: 22/39 scenarios passing (56%)
- distributed-storage.feature: 21/21 ✅ (100%)
- cluster-management.feature: 1/18 ⚠️ (5.5%)
  - ✅ Initial cluster formation
  - ❌ 17 scenarios blocked by missing health monitoring
```

## Next Phase: Health Monitoring Implementation

### Why This Is The Next Priority
1. **Unblocks 17 BDD test scenarios** - Critical for test coverage
2. **Production requirement** - No failure detection currently exists
3. **Enables future features** - Replication, rebalancing need health data

### Entry Point: First File to Create
```go
// internal/coordinator/health_monitor.go
package coordinator

import (
    "context"
    "sync"
    "time"
    "github.com/dreamware/torua/internal/cluster"
)

type NodeHealth struct {
    NodeID          string
    Status          string    // "healthy", "unhealthy", "unknown"
    LastCheck       time.Time
    ConsecutiveFails int
}

type HealthMonitor struct {
    interval        time.Duration
    maxFailures     int
    nodes          map[string]*NodeHealth
    mu             sync.RWMutex
    checkFunc      func(addr string) error
}

func NewHealthMonitor(interval time.Duration) *HealthMonitor {
    return &HealthMonitor{
        interval:    interval,
        maxFailures: 3,
        nodes:      make(map[string]*NodeHealth),
        checkFunc:  defaultHealthCheck,
    }
}

func (h *HealthMonitor) Start(ctx context.Context, nodeProvider func() []cluster.NodeInfo) {
    ticker := time.NewTicker(h.interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            h.checkAllNodes(nodeProvider())
        case <-ctx.Done():
            return
        }
    }
}
```

### First Test to Run
```bash
# After creating health_monitor.go
cd torua

# Run the specific failing test
python run_bdd_tests.py --scenario "Node health monitoring" --fail-fast

# Expected: Should start checking health but still fail on redistribution
```

### Implementation Sequence
1. **Hour 1-2: Core Health Monitor**
   - Create health_monitor.go
   - Add Start() method with goroutine
   - Implement checkAllNodes()
   - Add defaultHealthCheck() using HTTP

2. **Hour 3-4: Integration with Coordinator**
   - Add HealthMonitor to server struct
   - Start monitor in main()
   - Update NodeInfo with Status field
   - Include status in /nodes response

3. **Hour 5-6: Shard Redistribution**
   - Detect unhealthy nodes
   - Trigger reassignment
   - Update routing tables
   - Test redistribution

### Success Criteria
- [ ] Health checks run every 5 seconds
- [ ] Node marked unhealthy after 3 failures
- [ ] Status visible in `/nodes` endpoint
- [ ] "Node health monitoring" scenario passes
- [ ] No goroutine leaks
- [ ] Graceful shutdown handling

### Commands to Start
```bash
# 1. Navigate to project
cd torua

# 2. Create the new file
touch internal/coordinator/health_monitor.go

# 3. Open in editor and paste initial structure
# (See "Entry Point" section above)

# 4. Create test file
touch internal/coordinator/health_monitor_test.go

# 5. Run tests to ensure nothing breaks
make test

# 6. Test specific BDD scenario
python run_bdd_tests.py --scenario "Node health monitoring"
```

### Expected Challenges
1. **Concurrent Access**: Must handle nodes list changing during health checks
2. **Network Timeouts**: Need reasonable timeout for health checks (2s recommended)
3. **State Transitions**: Must handle healthy → unhealthy → healthy gracefully
4. **Test Timing**: BDD tests expect detection within 10 seconds

### Definition of Done
- [ ] All health monitoring tests passing (17 additional scenarios)
- [ ] No race conditions (run with -race flag)
- [ ] Documented with same quality as existing code
- [ ] No performance regression in existing tests
- [ ] Health status visible in coordinator UI/API

## Quick Reference

### Files to Modify
1. `internal/coordinator/health_monitor.go` - NEW
2. `internal/cluster/types.go` - Add Status field to NodeInfo
3. `cmd/coordinator/main.go` - Integrate HealthMonitor
4. `internal/coordinator/shard_registry.go` - Add redistribution trigger

### Key Test Files
- `features/cluster-management.feature:17` - Node health monitoring
- `features/steps/cluster_management_steps.py:201` - Health check verification

### Dependencies
- No new external dependencies required
- Uses existing HTTP client from cluster package
- Leverages existing shard registry for redistribution

## Ready to Start?
Everything is prepared for the health monitoring implementation:
- Documentation phase complete ✅
- Test environment working ✅
- Clear implementation path ✅
- Success criteria defined ✅
- First file structure provided ✅

The next action is to create `internal/coordinator/health_monitor.go` and begin implementation.