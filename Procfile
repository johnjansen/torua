coordinator: COORDINATOR_ADDR=:8080 go run ./cmd/coordinator
node1: NODE_ID=n1 NODE_LISTEN=:8081 NODE_ADDR=http://127.0.0.1:8081 COORDINATOR_ADDR=http://127.0.0.1:8080 go run ./cmd/node
node2: NODE_ID=n2 NODE_LISTEN=:8082 NODE_ADDR=http://127.0.0.1:8082 COORDINATOR_ADDR=http://127.0.0.1:8080 go run ./cmd/node
