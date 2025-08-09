# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release of Torua distributed GraphRAG system
- Core distributed architecture with coordinator and node components
- Automatic node registration and discovery
- Health monitoring system with configurable check intervals
- Distributed key-value storage with sharding
- Broadcast messaging system for cluster-wide operations
- Comprehensive test suite with 97% coverage
- BDD test scenarios using Cucumber/Behave
- Docker support with multi-stage builds
- Docker Compose configuration for local development
- GitHub Actions CI/CD pipeline
- Professional open-source project structure
- MIT License
- Contributing guidelines
- Security policy
- Issue and PR templates

### Features
- **Coordinator Service**
  - Node registration and management
  - Health monitoring with automatic unhealthy node detection
  - Broadcast control messages to all nodes
  - Cluster topology management
  - RESTful API for cluster operations

- **Node Service**
  - Automatic registration with coordinator
  - Shard management and storage
  - Health endpoint for monitoring
  - Control message handling
  - In-memory key-value storage

- **Testing**
  - Unit tests with high coverage
  - BDD tests for user scenarios
  - Integration tests for distributed operations
  - Automated testing in CI/CD pipeline

### Infrastructure
- Makefile for common development tasks
- Procfile for multi-process management
- Docker support with optimized images
- Docker Compose for local cluster deployment
- GitHub Actions for automated testing and releases
- Dependabot for dependency updates

### Documentation
- Comprehensive README with quick start guide
- API reference documentation
- Architecture overview
- Contributing guidelines
- Security policy
- Issue and solution tracking
- Project status documentation

### Known Issues
- No shard assignment protocol (uses on-demand creation)
- First node gets all shards (race condition)
- No replication support yet
- No automatic recovery from node failures
- No retry logic for failed requests

## [0.1.0] - TBD

Initial public release (planned)

### Planned Features
- Kuzu graph database integration
- Distributed query planning
- Replication and fault tolerance
- Automatic shard rebalancing
- Vector embedding support
- RAG pipeline integration

---

## Version History

This project follows semantic versioning:
- **Major version** (X.0.0): Incompatible API changes
- **Minor version** (0.X.0): New functionality in a backwards compatible manner
- **Patch version** (0.0.X): Backwards compatible bug fixes

## Release Process

1. Update version in `go.mod` and source files
2. Update CHANGELOG.md with release notes
3. Commit changes: `git commit -m "chore: prepare release v0.X.Y"`
4. Tag release: `git tag -a v0.X.Y -m "Release v0.X.Y"`
5. Push changes: `git push origin main --tags`
6. GitHub Actions will automatically:
   - Run all tests
   - Build binaries for multiple platforms
   - Create GitHub release with artifacts
   - Build and push Docker images

## Support

- **Bug Reports**: [GitHub Issues](https://github.com/johnjansen/torua/issues)
- **Feature Requests**: [GitHub Discussions](https://github.com/johnjansen/torua/discussions)
- **Security Issues**: security@torua.dev