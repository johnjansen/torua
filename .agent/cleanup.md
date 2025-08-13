# Cleanup Tasks

## Completed
- ✅ Fixed compilation errors in handlers_test.go
- ✅ Fixed panic in handleData function
- ✅ Fixed HTTP method checking order
- ✅ Fixed targetURL format in coordinator
- ✅ Fixed shard assignment issues in tests
- ✅ Improved coordinator test coverage (47% -> 87%)

## In Progress
- [ ] Remove coord.html file (accidentally committed)
- [ ] Improve cmd/node test coverage (currently 35.3%)
- [ ] Fix remaining BDD test failures (36 scenarios failing)

## Todo
- [ ] Add missing tests for cmd/node:
  - [ ] Shard creation and management
  - [ ] Data storage operations (GET/PUT/DELETE)
  - [ ] Error handling scenarios
  - [ ] Concurrent operations
- [ ] Fix BDD test environment issues:
  - [ ] Connection/registration problems
  - [ ] Response format mismatches
  - [ ] Undefined step definitions
- [ ] Address remaining GitHub Actions Dependabot PRs
- [ ] Verify CI pipeline passes with 90% coverage requirement
- [ ] Clean up test output files
- [ ] Review and consolidate test utilities

## Technical Debt
- [ ] Consider extracting common test helpers into shared package
- [ ] Add benchmarks for critical paths
- [ ] Document test patterns and best practices
- [ ] Add integration tests between coordinator and nodes
- [ ] Improve error messages in tests for better debugging