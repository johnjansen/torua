# Agent Plan - Torua CI/Test Coverage Improvements

## Current Status
- Fixed compilation errors in handlers_test.go
- Fixed panic in handleData function for edge cases
- Improved test coverage for cmd/coordinator from ~47% to 83.5%
- Fixed shard assignment issues in tests

## Completed Tasks
1. ✅ Fixed undefined constants (statusUnhealthy -> healthStatusUnhealthy)
2. ✅ Fixed forward function signatures in tests
3. ✅ Fixed handleData to handle "/data" path without panic
4. ✅ Fixed HTTP method checking order to return proper status codes
5. ✅ Fixed targetURL format to match node endpoints (/shards/%d/data/%s)
6. ✅ Fixed shard assignments in tests to match actual key hashing

## In Progress
1. Fixing remaining test failures in cmd/coordinator:
   - TestHandleShardAssign - status code mismatch (204 vs 200)
   - TestForward* tests - empty targetURL issues
   - TestForwardRequestBodyHandling - Content-Type not being forwarded

## Next Steps
1. Complete coordinator test fixes:
   - Fix handleShardAssign response status
   - Fix forward function tests to properly build URLs
   - Ensure Content-Type headers are forwarded in PUT requests

2. Improve test coverage for cmd/node (currently 35.3%):
   - Add tests for shard operations
   - Add tests for data storage operations
   - Add tests for error conditions

3. Fix BDD tests:
   - Address failing Gherkin scenarios
   - Update test expectations to match current implementation

4. Address remaining Dependabot PRs:
   - GitHub Actions updates (4 remaining)

5. Verify CI pipeline:
   - Ensure all tests pass
   - Confirm linting passes
   - Validate coverage meets requirements

## Technical Debt
- Consider adding integration tests between coordinator and nodes
- Document test utilities and helpers
- Add benchmarks for critical paths