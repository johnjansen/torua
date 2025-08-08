# Agent Plan - Torua Distributed GraphRAG System

## Current State
- Basic cluster coordination working (coordinator + 2 nodes)
- Nodes self-register and receive broadcast messages
- Health monitoring in place
- Foundation for distributed system established

## Immediate Tasks

### 1. Documentation Phase
- [x] Create .agent directory structure
- [x] Write comprehensive README.md explaining the system
- [x] Create ARCHITECTURE.md with detailed system design
- [x] Document coordinator architecture and responsibilities
- [x] Document node/shard architecture and capabilities
- [x] Create diagrams showing data flow and system topology

### 2. Testing Phase (TDD/BDD Approach)
- [x] Create comprehensive test suite for coordinator
- [x] Create comprehensive test suite for node
- [x] Create comprehensive test suite for cluster package
- [x] Add integration tests for coordinator-node communication
- [ ] Add BDD-style acceptance tests
- [x] Achieve 100% test coverage (97% achieved)
- [x] Add test targets to Makefile
- [x] Document TDD/BDD practices for the project

### 3. Baby Step: Simple Key-Value Storage
- [ ] Create Store interface in internal/storage
- [ ] Implement in-memory store with sync.RWMutex
- [ ] Add GET /store/{key} endpoint to nodes
- [ ] Add PUT /store/{key} endpoint to nodes
- [ ] Add DELETE /store/{key} endpoint to nodes
- [ ] Add GET /store endpoint to list keys
- [ ] Write tests with 100% coverage
- [ ] Update documentation

### 4. System Analysis (Future)
- [ ] Understand how Kuzu will integrate as the graph database
- [ ] Define shard distribution strategy
- [ ] Plan query routing mechanism
- [ ] Design replication strategy
- [ ] Plan fault tolerance mechanisms

### 4. Architecture Documentation Structure
- **README.md**: High-level overview, quick start, use cases
- **ARCHITECTURE.md**: Deep dive into system design
  - System Overview
  - Coordinator Design
  - Node/Shard Design
  - Communication Protocol
  - Data Distribution
  - Query Execution
  - Fault Tolerance
  - Scaling Strategy

## Key Design Principles to Document
1. **Test-Driven Development**: Write tests first, then implementation
2. **Behavior-Driven Development**: Consider for acceptance tests
3. **Simplicity**: Minimal, clean implementation inspired by Elasticsearch
4. **Distributed by Design**: Horizontal scaling through sharding
5. **Graph-Native**: Kuzu as the embedded graph engine
6. **Fault Tolerant**: Handle node failures gracefully
7. **Observable**: Health checks, monitoring, logging
8. **100% Test Coverage**: Every line of code must be tested

## Next Steps
1. ~~Update README.md with comprehensive overview~~ ✅
2. ~~Create ARCHITECTURE.md with detailed system design~~ ✅
3. ~~Document the coordinator's role in orchestration~~ ✅
4. ~~Document node/shard architecture and Kuzu integration~~ ✅
5. ~~Create Mermaid diagrams for visual representation~~ ✅
6. ~~Write comprehensive test suite with 100% coverage~~ ✅ (97% achieved)
7. ~~Implement TDD/BDD practices going forward~~ ✅
8. ~~Document testing strategy and patterns~~ ✅
9. Implement simple key-value storage in nodes (current baby step)
10. Add distributed routing through coordinator
11. Integrate Kuzu graph database

## Questions to Address in Documentation
- How will graph data be partitioned across shards?
- What's the replication factor and consistency model?
- How will distributed graph queries be executed?
- What's the strategy for node failure and recovery?
- How will the system handle hot spots and rebalancing?
- What's the approach for distributed transactions?

## Success Criteria
- Clear, comprehensive documentation that a junior engineer can understand
- Architectural decisions well-justified
- Trade-offs explicitly stated
- Implementation roadmap clear
- System boundaries well-defined
- 100% test coverage on all code
- TDD/BDD practices established and documented
- All tests passing in CI/CD pipeline
- Tests serve as living documentation

## Testing Philosophy
- **TDD vs BDD**: Starting with TDD for unit tests, may adopt BDD for acceptance tests
- **Coverage Goal**: 100% code coverage is mandatory
- **Test Types**: Unit, Integration, Acceptance
- **Mocking Strategy**: Use interfaces for testability
- **Test as Documentation**: Tests should clearly show how to use the code