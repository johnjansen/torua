.SILENT:

GOFLAGS ?=

# --- Defaults ---
NODE_ID      ?= n1
NODE_LISTEN  ?= :8081
NODE_ADDR    ?= http://127.0.0.1:8081
COORDINATOR_ADDR ?= http://127.0.0.1:8080

build:
	go build $(GOFLAGS) -o bin/coordinator ./cmd/coordinator
	go build $(GOFLAGS) -o bin/node ./cmd/node
	echo "bin/coordinator and bin/node built."

test:
	go test -v ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	echo "Coverage report generated: coverage.html"

test-coverage-text:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

clean:
	rm -f bin/coordinator bin/node
	rm -f coverage.out coverage.html
	echo "Cleaned build artifacts and coverage files."

run-coordinator:
	COORDINATOR_ADDR=:8080 go run ./cmd/coordinator

run-node:
	NODE_ID=$(NODE_ID) \
	NODE_LISTEN=$(NODE_LISTEN) \
	NODE_ADDR=$(NODE_ADDR) \
	COORDINATOR_ADDR=$(COORDINATOR_ADDR) \
	go run ./cmd/node
