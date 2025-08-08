# Learning: TDD Implementation for Torua

## Date: 2024-01-15
## Feature: Comprehensive Test Suite Implementation

## The TDD vs BDD Decision

After implementing tests for the Torua codebase, I've concluded that **TDD (Test-Driven Development)** is the right approach for this project, with potential for BDD (Behavior-Driven Development) at the acceptance test level later.

### Why TDD Over BDD

1. **Technical Nature**: Torua is infrastructure software with clear technical boundaries
2. **Unit Focus**: Most testing needs are at the unit and integration level
3. **Simplicity**: TDD's red-green-refactor cycle is simpler for this codebase
4. **Coverage Goals**: TDD naturally drives toward 100% coverage targets

### Where BDD Might Help

Future areas where BDD could add value:
- End-to-end cluster scenarios
- Client-facing API behavior
- Distributed query execution flows
- Failure recovery scenarios

## Test Coverage Achievement

### Final Coverage Stats
- **Overall**: 97.0% coverage
- `internal/cluster`: 100.0% ✅
- `cmd/coordinator`: 96.9% ✅
- `cmd/node`: 95.6% ✅

### Why Not 100%?

The remaining 3-5% uncovered code consists of:
1. **Main function setup**: Server initialization that runs until interrupted
2. **Signal handling**: OS signal capture for graceful shutdown
3. **Fatal error paths**: Log.Fatal calls that terminate the process

These are acceptable gaps because:
- They're tested manually during development
- They're mostly boilerplate code
- Full coverage would require complex process management in tests

## Testing Patterns Implemented

### 1. Table-Driven Tests
Used extensively for comprehensive scenario coverage:
```go
tests := []struct {
    name           string
    input          interface{}
    expectedStatus int
    expectError    bool
}{
    // Test cases...
}
```

### 2. Mock HTTP Servers
Using `httptest` package for network testing without real connections:
```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // Mock behavior
}))
```

### 3. Mockable Dependencies
Made `log.Fatal` mockable to test fatal error paths:
```go
var logFatal = log.Fatalf // In production code

// In tests:
logFatal = func(format string, v ...interface{}) {
    fatalCalled = true
}
```

### 4. Concurrent Testing
Verified thread safety with concurrent operations:
```go
for i := 0; i < numOps; i++ {
    go func(id int) {
        // Concurrent operation
    }(i)
}
```

### 5. Context Testing
Proper timeout and cancellation testing:
```go
ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
defer cancel()
```

## Challenges Overcome

### 1. Testing Main Functions
**Challenge**: Main functions run forever with signal handling
**Solution**: Run in goroutine, send interrupt signal, verify shutdown

### 2. Testing Fatal Errors
**Challenge**: log.Fatal exits the process
**Solution**: Make it a variable that can be mocked in tests

### 3. Testing Network Failures
**Challenge**: Can't rely on actual network failures
**Solution**: Use unreachable ports and mock servers returning errors

### 4. Testing Concurrent Access
**Challenge**: Race conditions are hard to reproduce
**Solution**: Run 100+ concurrent operations to stress test locks

### 5. Testing Registration Retries
**Challenge**: Time-based retry logic is slow to test
**Solution**: Accept the time cost (4 seconds) for thorough testing

## Best Practices Established

### 1. Test Organization
- One test file per source file
- Test functions mirror source functions
- Table-driven tests for multiple scenarios
- Descriptive test names explaining the scenario

### 2. Coverage Guidelines
- Aim for 100% coverage where possible
- Accept 95%+ for files with main functions
- Document why uncovered code is acceptable
- Use coverage reports to find gaps

### 3. Test Independence
- Each test is self-contained
- No shared state between tests
- Clean up resources in defer blocks
- Use unique ports/addresses per test

### 4. Error Testing
- Test both success and failure paths
- Verify error messages when relevant
- Test timeout scenarios
- Test invalid input handling

### 5. Mock Strategy
- Mock external dependencies (HTTP, time, OS)
- Keep mocks simple and focused
- Verify mock interactions when important
- Use real implementations when possible

## TDD Workflow for Future Development

Going forward, all new features should follow:

1. **Write failing test first**
   - Define expected behavior
   - Run test to see it fail
   - Confirms test is actually testing something

2. **Write minimal code to pass**
   - Just enough to make test green
   - Don't add features not required by test
   - Keep it simple

3. **Refactor with confidence**
   - Clean up code while tests are green
   - Tests ensure behavior doesn't change
   - Improve design iteratively

4. **Document through tests**
   - Tests show how to use the code
   - Test names explain what's being tested
   - Comments in tests explain why

5. **Maintain coverage**
   - New code must have tests
   - Coverage should never decrease
   - Use make test-coverage regularly

## Lessons for Kuzu Integration

When adding Kuzu graph database:

1. **Interface First**: Define graph operation interfaces before implementation
2. **Mock Kuzu**: Create mock graph store for unit tests
3. **Integration Tests**: Separate tests that need real Kuzu instance
4. **Test Data**: Use consistent test graphs across tests
5. **Performance Tests**: Add benchmarks for graph operations

## Metrics to Track

- **Coverage Percentage**: Must stay above 95%
- **Test Execution Time**: Should stay under 30 seconds
- **Test Flakiness**: Zero tolerance for flaky tests
- **Test/Code Ratio**: Aim for 2:1 (twice as much test code)

## Conclusion

TDD has proven to be the right choice for Torua. The comprehensive test suite provides:
- Confidence in the codebase
- Documentation through examples
- Safe refactoring ability
- Quality gates for new features

The 97% coverage achieved is excellent for a distributed system, and the testing patterns established will serve the project well as it grows.