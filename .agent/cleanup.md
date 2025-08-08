# Cleanup Tasks

## Documentation
- [ ] Review README.md for clarity and completeness after initial draft
- [ ] Ensure all code examples in documentation are tested and working
- [ ] Check for consistency in terminology across all documentation

## Code Organization
- [ ] Consider if any helper functions in main.go files should move to internal packages
- [ ] Review error handling patterns for consistency

## Future Improvements
- [ ] Add proper logging levels (debug, info, warn, error)
- [ ] Consider adding metrics/observability endpoints
- [ ] Standardize HTTP response formats across all endpoints

## Technical Debt
- [ ] Hard-coded timeout values could be configurable
- [ ] HTTP client in cluster/types.go is a package-level variable
- [ ] Missing proper context propagation in some areas

## Testing
- [ ] Need unit tests for cluster package
- [ ] Need integration tests for coordinator-node communication
- [ ] Need load testing for broadcast mechanism

## Deployment
- [ ] Verify Procfile works with common process managers
- [ ] Document deployment strategies for production