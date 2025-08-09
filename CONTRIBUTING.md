# Contributing to Torua

Thank you for your interest in contributing to Torua! This document provides guidelines and instructions for contributing to the project.

## Code of Conduct

We are committed to providing a welcoming and inclusive environment. Please be respectful and considerate in all interactions.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone git@github.com:YOUR_USERNAME/torua.git
   cd torua
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream git@github.com:johnjansen/torua.git
   ```
4. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Workflow

### Prerequisites

- Go 1.23+
- Python 3.11+ (for BDD tests)
- Make
- goreman (optional, for running multi-process setup)

### Setting Up Development Environment

```bash
# Install Go dependencies
go mod download

# Install Python test dependencies
pip install -r requirements-test.txt

# Run all tests to verify setup
make test
make test-bdd
```

### Code Standards

We follow **Test-Driven Development (TDD)** principles:

1. **Write tests first** - Every feature starts with a failing test
2. **Make tests pass** - Write minimal code to make tests green
3. **Refactor** - Clean up code while keeping tests green
4. **Document** - Add clear comments explaining what, how, and why

#### Go Code Style

- Follow standard Go conventions and idioms
- Run `go fmt` before committing
- Run `go vet` to catch common issues
- Keep functions under 20 lines when possible
- Aim for cyclomatic complexity under 8
- Use meaningful variable and function names
- Add comments for exported functions and types

#### Comments and Documentation

```go
// Good comment example - explains the why
// We use exponential backoff here to avoid overwhelming
// the coordinator during network partitions
func retryWithBackoff(fn func() error) error {
    // implementation
}

// Bad comment example - states the obvious
// This function retries
func retry(fn func() error) error {
    // implementation
}
```

### Testing Requirements

All contributions must include appropriate tests:

1. **Unit Tests** - Test individual functions and methods
2. **Integration Tests** - Test component interactions
3. **BDD Tests** - Test user-facing scenarios

#### Running Tests

```bash
# Run all Go tests with coverage
make test-coverage

# Run BDD tests
make test-bdd

# Run specific test
go test -v ./internal/coordinator -run TestHealthMonitor

# Check test coverage
make test-coverage-text
```

#### Test Coverage Goals

- New code should have **minimum 90% coverage**
- Critical paths should have **100% coverage**
- Every bug fix must include a regression test

### Making Changes

1. **Follow TDD**:
   - Write a failing test for your feature
   - Write code to make the test pass
   - Refactor if needed

2. **Keep commits atomic**:
   - Each commit should represent one logical change
   - Write clear, descriptive commit messages

3. **Commit Message Format**:
   ```
   type(scope): brief description
   
   Longer explanation if needed. Explain what changed and why,
   not how (the code shows how).
   
   Fixes #123
   ```
   
   Types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `chore`

4. **Update documentation**:
   - Update README if adding features
   - Update API docs for new endpoints
   - Add inline comments for complex logic

### Submitting Pull Requests

1. **Update your branch**:
   ```bash
   git fetch upstream
   git rebase upstream/master
   ```

2. **Run all tests**:
   ```bash
   make test
   make test-bdd
   make test-coverage
   ```

3. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

4. **Create Pull Request**:
   - Use a clear, descriptive title
   - Reference any related issues
   - Include test results and coverage
   - Describe what changed and why

5. **PR Checklist**:
   - [ ] Tests pass locally
   - [ ] Code follows project style
   - [ ] Comments explain complex logic
   - [ ] Documentation updated if needed
   - [ ] No unnecessary files committed
   - [ ] Commit messages are clear

## Areas Needing Contribution

### High Priority

1. **Kuzu Integration** - Implement graph database layer
2. **Query Planning** - Distributed query optimization
3. **Replication** - Add data redundancy
4. **Recovery** - Implement node failure recovery

### Medium Priority

1. **Client Libraries** - Python, JavaScript, Rust clients
2. **Monitoring** - Metrics and observability
3. **Benchmarks** - Performance testing suite
4. **Documentation** - Tutorials and examples

### Good First Issues

Look for issues labeled `good first issue` in the GitHub issue tracker.

## Project Structure

```
torua/
├── cmd/                    # Entry points
│   ├── coordinator/        # Coordinator service
│   └── node/              # Node service
├── internal/              # Internal packages
│   ├── cluster/           # Cluster types and utilities
│   ├── coordinator/       # Coordinator logic
│   ├── shard/            # Shard management
│   └── storage/          # Storage layer
├── features/              # BDD test scenarios
├── test/                  # Integration tests
└── .agent/               # Development notes and plans
```

## Development Tips

### Running Local Cluster

```bash
# Terminal 1 - Coordinator
make run-coordinator

# Terminal 2 - Node 1
NODE_ID=n1 NODE_LISTEN=:8081 NODE_ADDR=http://127.0.0.1:8081 \
COORDINATOR_ADDR=http://127.0.0.1:8080 make run-node

# Terminal 3 - Node 2
NODE_ID=n2 NODE_LISTEN=:8082 NODE_ADDR=http://127.0.0.1:8082 \
COORDINATOR_ADDR=http://127.0.0.1:8080 make run-node

# Or use goreman
goreman start
```

### Debugging

1. **Enable verbose logging**:
   ```go
   log.Printf("[DEBUG] Processing request: %+v", req)
   ```

2. **Use test helpers**:
   ```go
   // In tests
   t.Logf("State: %+v", state)
   ```

3. **Check health endpoints**:
   ```bash
   curl http://localhost:8080/health
   curl http://localhost:8081/health
   ```

## Questions?

If you have questions:

1. Check existing issues and discussions
2. Read the documentation thoroughly
3. Open a new issue with the `question` label
4. Join our discussions on GitHub

## Recognition

Contributors are recognized in:
- The project README
- Release notes
- GitHub contributors page

Thank you for contributing to Torua! Your efforts help make distributed GraphRAG accessible to everyone.