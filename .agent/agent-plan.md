# Agent Plan - Torua Distributed GraphRAG System

## Current State
- Basic cluster coordination working (coordinator + 2 nodes)
- Nodes self-register and receive broadcast messages
- Health monitoring in place
- Foundation for distributed system established

## Immediate Tasks

### 1. Documentation Phase
- [x] Create .agent directory structure
- [ ] Write comprehensive README.md explaining the system
- [ ] Create ARCHITECTURE.md with detailed system design
- [ ] Document coordinator architecture and responsibilities
- [ ] Document node/shard architecture and capabilities
- [ ] Create diagrams showing data flow and system topology

### 2. System Analysis
- [ ] Understand how Kuzu will integrate as the graph database
- [ ] Define shard distribution strategy
- [ ] Plan query routing mechanism
- [ ] Design replication strategy
- [ ] Plan fault tolerance mechanisms

### 3. Architecture Documentation Structure
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
1. **Simplicity**: Minimal, clean implementation inspired by Elasticsearch
2. **Distributed by Design**: Horizontal scaling through sharding
3. **Graph-Native**: Kuzu as the embedded graph engine
4. **Fault Tolerant**: Handle node failures gracefully
5. **Observable**: Health checks, monitoring, logging

## Next Steps
1. Update README.md with comprehensive overview
2. Create ARCHITECTURE.md with detailed system design
3. Document the coordinator's role in orchestration
4. Document node/shard architecture and Kuzu integration
5. Create Mermaid diagrams for visual representation

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