# Torua - Distributed GraphRAG System

[![CI](https://github.com/johnjansen/torua/actions/workflows/ci.yml/badge.svg)](https://github.com/johnjansen/torua/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/johnjansen/torua)](https://goreportcard.com/report/github.com/johnjansen/torua)
[![GoDoc](https://pkg.go.dev/badge/github.com/johnjansen/torua)](https://pkg.go.dev/github.com/johnjansen/torua)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.23%2B-blue.svg)](https://go.dev/)
[![Coverage](https://img.shields.io/badge/Coverage-97%25-brightgreen.svg)](https://codecov.io/gh/johnjansen/torua)

A lightweight, distributed Graph Retrieval-Augmented Generation (GraphRAG) system built on simplified Elasticsearch-like architecture with Kuzu as the embedded graph database engine.

## Overview

Torua implements a horizontally-scalable, fault-tolerant distributed system for GraphRAG workloads. It combines the architectural elegance of Elasticsearch's clustering model with the power of Kuzu's embedded graph database, creating a system optimized for distributed graph operations and RAG pipelines.

### Key Features

- **Distributed by Design**: Automatic sharding and replication across nodes
- **Graph-Native**: Kuzu embedded graph database at each node
- **Self-Organizing**: Nodes auto-register and discover each other
- **Fault Tolerant**: Handles node failures gracefully with automatic failover
- **Horizontally Scalable**: Add nodes dynamically to increase capacity
- **Simple Architecture**: Minimal dependencies, clear separation of concerns
- **Test-Driven Development**: 97% test coverage with comprehensive unit and integration tests

## Quick Start

### Prerequisites

- Go 1.23+ 
- Make
- curl (for testing)
- jq (optional, for pretty JSON output)
- Docker & Docker Compose (optional, for containerized deployment)

### Running a Local Cluster

1. **Start the Coordinator** (Terminal 1):
```bash
make run-coordinator
```
The coordinator starts on port 8080 and manages cluster topology.

2. **Start Node 1** (Terminal 2):
```bash
NODE_ID=n1 NODE_LISTEN=:8081 NODE_ADDR=http://127.0.0.1:8081 \
COORDINATOR_ADDR=http://127.0.0.1:8080 make run-node
```

3. **Start Node 2** (Terminal 3):
```bash
NODE_ID=n2 NODE_LISTEN=:8082 NODE_ADDR=http://127.0.0.1:8082 \
COORDINATOR_ADDR=http://127.0.0.1:8080 make run-node
```

4. **Verify Cluster Status**:
```bash
# List all registered nodes
curl -s http://127.0.0.1:8080/nodes | jq

# Check coordinator health
curl -s http://127.0.0.1:8080/health

# Check node health
curl -s http://127.0.0.1:8081/health
```

### Using Procfile (Alternative)

For convenience, use a process manager like `goreman` or `foreman`:

```bash
# Install goreman
go install github.com/mattn/goreman@latest

# Start all components
goreman start
```

This starts the coordinator and two nodes automatically.

### Using Docker Compose

```bash
# Build and start all services
docker-compose up -d

# Scale to more nodes
docker-compose --profile scale up -d

# View logs
docker-compose logs -f

# Stop all services
docker-compose down
```

## System Architecture

### Components

#### Coordinator
- **Role**: Cluster orchestrator and metadata manager
- **Responsibilities**:
  - Node registration and discovery
  - Cluster topology management
  - Query routing and coordination
  - Broadcast control messages
  - Health monitoring

#### Nodes (Shards)
- **Role**: Data storage and query execution
- **Responsibilities**:
  - Store graph data shards
  - Execute local graph queries
  - Participate in distributed queries
  - Report health status
  - Handle replication

### Data Model

Torua distributes graph data across nodes using consistent hashing:

- **Vertices**: Distributed by ID across shards
- **Edges**: Co-located with source vertices for locality
- **Properties**: Stored with their associated vertices/edges
- **Indexes**: Local to each shard with global coordination

## API Reference

### Coordinator Endpoints

#### `GET /nodes`
Returns list of all registered nodes in the cluster.

**Response**:
```json
{
  "nodes": [
    {
      "id": "n1",
      "addr": "http://127.0.0.1:8081"
    },
    {
      "id": "n2", 
      "addr": "http://127.0.0.1:8082"
    }
  ]
}
```

#### `POST /register`
Register a new node with the coordinator (called automatically by nodes).

**Request**:
```json
{
  "node": {
    "id": "n3",
    "addr": "http://127.0.0.1:8083"
  }
}
```

#### `POST /broadcast`
Send a control message to all nodes in the cluster.

**Request**:
```json
{
  "path": "/control",
  "payload": {
    "op": "reindex",
    "params": {...}
  }
}
```

**Response**:
```json
{
  "sent_to": 2,
  "results": [
    {"node_id": "n1"},
    {"node_id": "n2"}
  ]
}
```

#### `GET /health`
Health check endpoint.

### Node Endpoints

#### `GET /health`
Node health check endpoint.

#### `POST /control`
Receive control messages from coordinator.

## Use Cases

### 1. Knowledge Graph RAG
- Store organizational knowledge as a graph
- Distribute graph across multiple nodes
- Execute graph traversals for context retrieval
- Generate responses using retrieved context

### 2. Multi-Modal RAG
- Store relationships between text, images, and other media
- Query across modalities using graph relationships
- Scale storage horizontally as data grows

### 3. Temporal Graph Analysis
- Store time-series graph data across shards
- Execute temporal queries in parallel
- Aggregate results at coordinator level

### 4. Recommendation Systems
- Distribute user-item interaction graphs
- Execute collaborative filtering queries
- Scale with user base growth

## Development

### Building from Source

```bash
# Build both coordinator and node binaries
make build

# Binaries will be in ./bin/
./bin/coordinator
./bin/node
```

### Testing

This is a **TDD (Test-Driven Development)** project with comprehensive test coverage:

```bash
# Run all tests
make test

# Run unit tests with coverage
make test-coverage

# Run BDD tests
make test-bdd

# View coverage in terminal
make test-coverage-text

# Clean build artifacts and coverage files
make clean
```

**Current Coverage**: 97% overall
- `internal/cluster`: 100% coverage
- `cmd/coordinator`: 96.9% coverage  
- `cmd/node`: 95.6% coverage

We follow TDD principles:
1. Write tests first, then implementation
2. Tests serve as living documentation
3. Every bug fix starts with a failing test
4. Refactoring only with green tests

### Project Structure

```
torua/
├── cmd/
│   ├── coordinator/    # Coordinator entry point
│   └── node/           # Node entry point
├── internal/
│   └── cluster/        # Shared cluster types and utilities
├── bin/                # Built binaries (git-ignored)
├── Procfile           # Process definitions for goreman/foreman
├── Makefile           # Build and run targets
└── README.md          # This file
```

### Environment Variables

#### Coordinator
- `COORDINATOR_ADDR`: Listen address (default: `:8080`)

#### Node
- `NODE_ID`: Unique node identifier (required)
- `NODE_LISTEN`: Listen address (required)
- `NODE_ADDR`: Public address for coordinator to reach this node (required)
- `COORDINATOR_ADDR`: Coordinator URL (required)

## Roadmap

### Phase 1: Foundation (Current)
- [x] Basic cluster coordination
- [x] Node registration and discovery
- [x] Health monitoring
- [x] Broadcast messaging

### Phase 2: Graph Integration
- [ ] Integrate Kuzu embedded database
- [ ] Implement graph sharding strategy
- [ ] Add basic graph operations API
- [ ] Implement shard routing

### Phase 3: Distributed Queries
- [ ] Query planning and optimization
- [ ] Distributed graph traversals
- [ ] Result aggregation
- [ ] Caching layer

### Phase 4: RAG Pipeline
- [ ] Vector embedding support
- [ ] Semantic search integration
- [ ] Context retrieval optimization
- [ ] LLM integration points

### Phase 5: Production Readiness
- [ ] Replication and fault tolerance
- [ ] Monitoring and metrics
- [ ] Performance optimization
- [ ] Administration tools

## Design Philosophy

1. **Test-Driven Development**: Write tests first, ensure 100% coverage goal
2. **Simplicity First**: Clear, understandable code over clever optimizations
3. **Distributed by Default**: Every decision assumes multiple nodes
4. **Fail Gracefully**: Partial results better than no results
5. **Observable**: Know what the system is doing at all times
6. **Developer Friendly**: Easy to understand, modify, and extend

## Contributing

We welcome contributions! Key areas where help is needed:

- Kuzu integration implementation
- Query planning and optimization
- Testing and benchmarking
- Documentation improvements
- Client libraries (Python, JavaScript, etc.)

## Project Status

### Current State
- ✅ **Core Functionality**: Working distributed key-value storage with sharding
- ✅ **Test Coverage**: 97% unit tests, 100% BDD tests passing (21/21 scenarios)
- ✅ **Performance**: <50ms response times for basic operations
- ⚠️ **Production Readiness**: Not yet - critical issues need addressing

### Known Issues
1. **No Shard Assignment Protocol**: Nodes use on-demand shard creation (workaround in place)
2. **Shard Distribution Race**: First node gets all shards
3. **No Replication**: Single point of failure for each shard
4. **No Failure Detection**: Coordinator doesn't detect dead nodes
5. **No Retry Logic**: Failed requests aren't retried

See [PROJECT_STATUS.md](PROJECT_STATUS.md) for detailed status and roadmap.
See [ISSUES_AND_SOLUTIONS.md](ISSUES_AND_SOLUTIONS.md) for critical issues and proposed fixes.

## Community

### Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details on how to get started.

### Support

- **Issues**: [GitHub Issues](https://github.com/johnjansen/torua/issues)
- **Discussions**: [GitHub Discussions](https://github.com/johnjansen/torua/discussions)
- **Security**: Please report security vulnerabilities to [security@torua.dev](mailto:security@torua.dev)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by Elasticsearch's clustering architecture
- Built with [Kuzu](https://kuzudb.com/) embedded graph database
- GraphRAG concepts from Microsoft Research

---

**Note**: Torua is under active development. APIs and architecture may change as we iterate toward production readiness.