# Documentation Standards for Torua

## Purpose

Since Torua is primarily machine-generated code with AI assistance, comprehensive documentation is **critical** for human maintainability. Every function, type, and module must be thoroughly documented to enable developers to understand the system's composition and design without reading all implementation details.

## Core Principle

**Layer documentation so humans can progressively understand the system:**
1. Start with high-level purpose
2. Explain the mechanism/algorithm
3. Detail the implementation
4. Provide examples where helpful

## Documentation Requirements

### 1. Function Documentation

Every function MUST have comprehensive documentation that includes:

```go
// FunctionName performs [high-level purpose].
//
// Mechanism:
// [Explain HOW it works - the algorithm or approach]
//
// Details:
// - [Important detail 1]
// - [Important detail 2]
// - [Edge cases or special behavior]
//
// Parameters:
//   - param1: [purpose and constraints]
//   - param2: [purpose and constraints]
//
// Returns:
//   - [what it returns and when]
//   - error: [when errors occur]
//
// Example:
//   result, err := FunctionName(value1, value2)
//   if err != nil {
//       // handle error
//   }
//
// Thread Safety: [is it thread-safe?]
// Performance: [O(n) complexity, memory usage, etc.]
func FunctionName(param1 Type1, param2 Type2) (ReturnType, error) {
    // Implementation
}
```

### 2. Type Documentation

```go
// TypeName represents [what it represents in the system].
//
// Purpose:
// [Why this type exists and what problem it solves]
//
// Usage:
// [When and how to use this type]
//
// Invariants:
// - [Constraint 1 that must always be true]
// - [Constraint 2 that must always be true]
//
// Example:
//   thing := &TypeName{
//       Field1: value1,
//       Field2: value2,
//   }
//
// Thread Safety: [describe thread safety guarantees]
type TypeName struct {
    // Field1 controls [what it controls]
    // Valid values: [constraints]
    Field1 Type1
    
    // Field2 represents [what it represents]
    // This is used by [what uses it] to [do what]
    Field2 Type2
    
    // mu protects concurrent access to [which fields]
    mu sync.RWMutex
}
```

### 3. Package Documentation

Each package must have a doc.go file:

```go
// Package packagename provides [high-level purpose].
//
// Overview:
//
// This package implements [what it implements] for the Torua distributed
// storage system. It handles [main responsibilities].
//
// Architecture:
//
// The package is structured around [core concepts]:
//   - Concept1: [explanation]
//   - Concept2: [explanation]
//
// Key Types:
//   - TypeA: [one-line purpose]
//   - TypeB: [one-line purpose]
//
// Usage Example:
//
//   // Create a new instance
//   thing := packagename.New(config)
//
//   // Perform operation
//   result, err := thing.DoSomething(input)
//
// Thread Safety:
//
// [Describe thread safety model of the package]
//
// Performance Considerations:
//
// [Describe performance characteristics and trade-offs]
//
package packagename
```

### 4. Interface Documentation

```go
// InterfaceName defines the contract for [what it abstracts].
//
// Purpose:
// [Why this abstraction exists]
//
// Implementations:
//   - Implementation1: [when to use]
//   - Implementation2: [when to use]
//
// Usage:
// Typically used by [who uses it] to [achieve what]
//
// Contract:
// Implementations MUST:
//   - [Requirement 1]
//   - [Requirement 2]
// Implementations SHOULD:
//   - [Recommendation 1]
//
type InterfaceName interface {
    // MethodName does [what it does].
    // It MUST [invariant that must hold].
    // Returns error when [conditions that cause errors].
    MethodName(param Type) error
}
```

### 5. Complex Algorithm Documentation

For complex algorithms, add a separate comment block:

```go
// Algorithm: Shard Distribution
// ================================
// 
// This implements consistent hashing for shard distribution.
//
// Step 1: Hash the key using FNV-1a
//   - Chosen for speed and good distribution
//   - Produces 32-bit hash
//
// Step 2: Map to shard using modulo
//   - shardID = hash % numShards
//   - Ensures even distribution statistically
//
// Step 3: Look up node assignment
//   - Check primary assignment first
//   - Fall back to replicas if primary unavailable
//
// Trade-offs:
//   - Simple modulo means resharding requires full rebalance
//   - Consider jump consistent hashing for future
//
// Example Distribution (4 shards):
//   "user:123" -> hash: 0x1234ABCD -> shard: 1
//   "post:456" -> hash: 0x5678CDEF -> shard: 3
//
func (r *Registry) GetShardForKey(key string) int {
    // Implementation
}
```

### 6. Error Documentation

```go
// Common Errors:
//
//   - ErrNodeNotFound: The specified node ID doesn't exist in the registry
//     Caller should: Retry with discovery or handle as fatal
//
//   - ErrShardNotAssigned: No node owns this shard
//     Caller should: Trigger rebalancing or return 503
//
//   - ErrTimeout: Operation exceeded timeout
//     Caller should: Retry with backoff
//
var (
    ErrNodeNotFound     = errors.New("node not found")
    ErrShardNotAssigned = errors.New("shard not assigned")
    ErrTimeout          = errors.New("operation timeout")
)
```

## Documentation Patterns

### Pattern 1: Progressive Detail

Start broad, then narrow:
1. One-line summary
2. Paragraph explanation
3. Technical details
4. Examples
5. Edge cases

### Pattern 2: Why Before What

Always explain:
1. WHY this exists (problem it solves)
2. WHAT it does (functionality)
3. HOW it works (mechanism)
4. WHEN to use it (context)

### Pattern 3: Relationship Documentation

For interconnected components:

```go
// CoordinatorRegistry manages the global view of shard assignments.
//
// Relationships:
//   - Used by: Coordinator.handleData() for routing decisions
//   - Depends on: ShardRegistry for assignment storage
//   - Notifies: Nodes via broadcast when assignments change
//   - Updated by: Admin API and rebalancing operations
//
// Lifecycle:
//   1. Created during Coordinator initialization
//   2. Populated when nodes register
//   3. Modified during rebalancing
//   4. Cleaned when nodes leave
```

### Pattern 4: State Machine Documentation

For stateful components:

```go
// NodeState represents the health state of a node.
//
// State Transitions:
//   
//   [New] ──register──> [Healthy]
//     │                    │
//     │                    ├──missed heartbeat──> [Unhealthy]
//     │                    │                          │
//     │                    │<──heartbeat received─────┘
//     │                    │
//     └────────────────────├──timeout──> [Dead]
//                          │                │
//                          └──leave─────────┘
//
// Timing:
//   - Healthy -> Unhealthy: After 3 missed heartbeats (15s)
//   - Unhealthy -> Dead: After 6 missed heartbeats (30s)
//   - Unhealthy -> Healthy: Immediate on heartbeat
```

## API Documentation

### REST Endpoints

```go
// handleData routes storage operations to appropriate shards.
//
// Endpoint: /data/{key}
//
// Methods:
//   - GET: Retrieve value for key
//   - PUT: Store value for key
//   - DELETE: Remove key
//
// Request (PUT):
//   Body: Raw value to store
//   Content-Type: text/plain or application/json
//
// Response:
//   - 200: Success (GET)
//   - 204: Success (PUT/DELETE)
//   - 404: Key not found
//   - 503: No node available for shard
//
// Example:
//   curl -X PUT -d "value" http://coordinator/data/mykey
//
// Internal Flow:
//   1. Extract key from path
//   2. Calculate shard: hash(key) % numShards
//   3. Find node owning shard
//   4. Forward request to node
//   5. Return node's response
//
func (s *server) handleData(w http.ResponseWriter, r *http.Request) {
```

## Testing Documentation

### Test Documentation

```go
// TestShardDistribution verifies even distribution of keys across shards.
//
// Test Strategy:
//   1. Generate 10,000 random keys
//   2. Assign each to a shard
//   3. Verify distribution is within 10% of ideal
//
// Why This Matters:
//   Uneven distribution causes hot spots and performance degradation
//
// Related Tests:
//   - TestShardRebalancing: Verifies redistribution
//   - TestConsistentRouting: Ensures same key always routes same
//
func TestShardDistribution(t *testing.T) {
```

## File Headers

Every file should start with:

```go
// File: coordinator/registry.go
// Purpose: Manages global shard-to-node assignments for the cluster
// 
// This file implements the core routing logic that determines which node
// handles requests for a given key. It maintains the authoritative mapping
// of shards to nodes and handles rebalancing when nodes join or leave.
//
// Key Components:
//   - ShardRegistry: Core assignment storage
//   - Rebalancer: Handles redistribution
//   - HealthTracker: Monitors node availability
//
// Thread Safety: All public methods are thread-safe via internal locking
// Performance: O(1) lookups, O(n) rebalancing where n = number of shards
```

## Documentation Review Checklist

Before committing code, verify:

- [ ] Every public function has complete documentation
- [ ] Every type explains its purpose and usage
- [ ] Complex algorithms have step-by-step explanations
- [ ] Relationships between components are documented
- [ ] Error conditions and handling are specified
- [ ] Thread safety is explicitly stated
- [ ] Performance characteristics are noted
- [ ] Examples are provided for non-trivial usage
- [ ] Package doc.go file exists and is comprehensive
- [ ] File headers explain file purpose and contents

## Tools and Automation

### Recommended Tools

1. **golint**: Checks for missing comments
2. **godoc**: Preview documentation locally
3. **go-doc-check**: Custom script to verify documentation completeness

### Documentation Generation

```bash
# Generate HTML documentation
godoc -http=:6060

# Check documentation coverage
go list -f '{{.Doc}}' ./...

# Verify all exported symbols are documented
golint ./... | grep "should have comment"
```

## Living Documentation

Documentation must evolve with code:

1. **Update docs when changing functionality**
2. **Add examples when bugs reveal unclear usage**
3. **Enhance explanations when questions arise**
4. **Document lessons learned from production issues**
5. **Keep examples working and tested**

## Summary

Comprehensive documentation is not optional for machine-generated code. It's the bridge that allows human developers to:

1. Understand system architecture without reading all code
2. Modify components safely with full context
3. Debug issues by understanding intended behavior
4. Onboard new developers efficiently
5. Maintain code quality over time

Remember: **The code might be machine-generated, but it will be human-maintained.**