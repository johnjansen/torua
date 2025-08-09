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

# --- BDD Testing ---
.PHONY: bdd-install
bdd-install:
	python3 -m pip install -r requirements-test.txt
	echo "BDD test dependencies installed."

.PHONY: bdd-test
bdd-test: build
	python3 run_bdd_tests.py
	echo "BDD tests completed."

.PHONY: bdd-test-verbose
bdd-test-verbose: build
	python3 run_bdd_tests.py --verbose --debug
	echo "BDD tests completed (verbose)."

.PHONY: bdd-test-feature
bdd-test-feature: build
	python3 run_bdd_tests.py --feature $(FEATURE)
	echo "BDD feature tests completed."

.PHONY: bdd-test-coverage
bdd-test-coverage: build
	python3 run_bdd_tests.py --coverage
	echo "BDD tests with coverage completed."

.PHONY: bdd-dry-run
bdd-dry-run:
	python3 run_bdd_tests.py --dry-run
	echo "BDD dry run completed."

.PHONY: bdd-list
bdd-list:
	python3 run_bdd_tests.py --list-features
	python3 run_bdd_tests.py --list-scenarios

.PHONY: test-all
test-all: test bdd-test
	echo "All tests (unit and BDD) completed."

.PHONY: test-all-coverage
test-all-coverage: test-coverage bdd-test-coverage
	echo "All tests with coverage completed."

# --- Service Building ---
.PHONY: build-coordinator
build-coordinator:
	go build $(GOFLAGS) -o bin/coordinator ./cmd/coordinator
	echo "bin/coordinator built."

.PHONY: build-node
build-node:
	go build $(GOFLAGS) -o bin/node ./cmd/node
	echo "bin/node built."
