package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/dreamware/torua/internal/cluster"
)

// TestGetenv tests the getenv utility function
func TestGetenv(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		def      string
		expected string
	}{
		{
			name:     "environment variable set",
			key:      "TEST_ENV_VAR",
			value:    "test_value",
			def:      "default",
			expected: "test_value",
		},
		{
			name:     "environment variable not set",
			key:      "UNSET_ENV_VAR",
			value:    "",
			def:      "default_value",
			expected: "default_value",
		},
		{
			name:     "empty environment variable returns default",
			key:      "EMPTY_ENV_VAR",
			value:    "",
			def:      "fallback",
			expected: "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set or unset environment variable
			if tt.value != "" {
				os.Setenv(tt.key, tt.value)
				defer os.Unsetenv(tt.key)
			}

			// Test getenv
			result := getenv(tt.key, tt.def)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestMustGetenv tests the mustGetenv utility function
func TestMustGetenv(t *testing.T) {
	// Test with environment variable set
	t.Run("variable set", func(t *testing.T) {
		os.Setenv("MUST_HAVE_VAR", "required_value")
		defer os.Unsetenv("MUST_HAVE_VAR")

		result := mustGetenv("MUST_HAVE_VAR")
		if result != "required_value" {
			t.Errorf("Expected 'required_value', got %s", result)
		}
	})

	// Test with environment variable not set - should fatal
	t.Run("variable not set", func(t *testing.T) {
		// We need to catch the log.Fatal call
		// Save original log fatal function
		oldLogFatal := logFatal
		defer func() { logFatal = oldLogFatal }()

		fatalCalled := false
		logFatal = func(format string, v ...interface{}) {
			fatalCalled = true
		}

		// Call mustGetenv with unset variable
		_ = mustGetenv("UNSET_REQUIRED_VAR")

		if !fatalCalled {
			t.Error("Expected log.Fatal to be called but it wasn't")
		}
	})
}

// TestHandleControl tests the control message handler
func TestHandleControl(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectedLog    bool
	}{
		{
			name:           "valid control message",
			requestBody:    `{"op":"ping","timestamp":"2024-01-15T10:00:00Z"}`,
			expectedStatus: http.StatusNoContent,
			expectedLog:    true,
		},
		{
			name:           "empty control message",
			requestBody:    `{}`,
			expectedStatus: http.StatusNoContent,
			expectedLog:    true,
		},
		{
			name:           "complex nested control message",
			requestBody:    `{"op":"reindex","shards":[1,2,3],"params":{"force":true}}`,
			expectedStatus: http.StatusNoContent,
			expectedLog:    true,
		},
		{
			name:           "plain text message",
			requestBody:    `plain text control`,
			expectedStatus: http.StatusNoContent,
			expectedLog:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest(http.MethodPost, "/control", bytes.NewReader([]byte(tt.requestBody)))
			rec := httptest.NewRecorder()

			// Handle request
			handleControl(rec, req)

			// Check status code
			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}

// TestHandleControlReadError tests control handler with read error
func TestHandleControlReadError(t *testing.T) {
	// Create a reader that always returns an error
	errorReader := &errorReader{err: bytes.ErrTooLarge}
	req := httptest.NewRequest(http.MethodPost, "/control", errorReader)
	rec := httptest.NewRecorder()

	// Handle request
	handleControl(rec, req)

	// Should return bad request on read error
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

// errorReader is a reader that always returns an error
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}

// TestRegister tests the node registration function
func TestRegister(t *testing.T) {
	tests := []struct {
		name         string
		serverStatus int
		expectFatal  bool
		retries      int
	}{
		{
			name:         "successful registration on first try",
			serverStatus: http.StatusNoContent,
			expectFatal:  false,
			retries:      1,
		},
		{
			name:         "successful registration after retries",
			serverStatus: http.StatusNoContent,
			expectFatal:  false,
			retries:      3,
		},
		{
			name:         "registration fails after max retries",
			serverStatus: http.StatusInternalServerError,
			expectFatal:  true,
			retries:      11, // More than max retries
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track retry count
			retryCount := 0

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST method, got %s", r.Method)
				}
				if r.URL.Path != "/register" {
					t.Errorf("Expected /register path, got %s", r.URL.Path)
				}

				// Parse request body
				var req cluster.RegisterRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}

				// Verify request data
				if req.Node.ID != "test-node" {
					t.Errorf("Expected node ID 'test-node', got %s", req.Node.ID)
				}
				if req.Node.Addr != "http://localhost:8081" {
					t.Errorf("Expected node addr 'http://localhost:8081', got %s", req.Node.Addr)
				}

				retryCount++
				// Succeed after specified number of retries
				if retryCount >= tt.retries && tt.serverStatus == http.StatusNoContent {
					w.WriteHeader(http.StatusNoContent)
				} else {
					w.WriteHeader(tt.serverStatus)
				}
			}))
			defer server.Close()

			// Mock log.Fatal for testing
			oldLogFatal := logFatal
			defer func() { logFatal = oldLogFatal }()

			fatalCalled := false
			logFatal = func(format string, v ...interface{}) {
				fatalCalled = true
			}

			// Call register
			ctx := context.Background()
			register(ctx, server.URL, "test-node", "http://localhost:8081")

			// Check if fatal was called as expected
			if tt.expectFatal && !fatalCalled {
				t.Error("Expected log.Fatal to be called but it wasn't")
			}
			if !tt.expectFatal && fatalCalled {
				t.Error("Unexpected log.Fatal call")
			}
		})
	}
}

// TestRegisterWithUnreachableServer tests registration with unreachable server
func TestRegisterWithUnreachableServer(t *testing.T) {
	// Mock log.Fatal
	oldLogFatal := logFatal
	defer func() { logFatal = oldLogFatal }()

	fatalCalled := false
	logFatal = func(format string, v ...interface{}) {
		fatalCalled = true
	}

	// Try to register with unreachable server
	ctx := context.Background()
	register(ctx, "http://localhost:99999", "test-node", "http://localhost:8081")

	// Should call log.Fatal after retries
	if !fatalCalled {
		t.Error("Expected log.Fatal to be called for unreachable server")
	}
}

// TestMainFunction tests the main function with full lifecycle
func TestMainFunction(t *testing.T) {
	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set test args
	os.Args = []string{"node"}

	// Create a test coordinator server
	coordServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/register" {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer coordServer.Close()

	// Set required environment variables
	os.Setenv("NODE_ID", "test-node")
	os.Setenv("NODE_LISTEN", "127.0.0.1:0") // Use port 0 for automatic assignment
	os.Setenv("NODE_ADDR", "http://127.0.0.1:8081")
	os.Setenv("COORDINATOR_ADDR", coordServer.URL)
	defer func() {
		os.Unsetenv("NODE_ID")
		os.Unsetenv("NODE_LISTEN")
		os.Unsetenv("NODE_ADDR")
		os.Unsetenv("COORDINATOR_ADDR")
	}()

	// Mock log.Fatal to prevent actual exit
	oldLogFatal := logFatal
	defer func() { logFatal = oldLogFatal }()
	logFatal = func(format string, v ...interface{}) {
		// Don't actually exit in tests
	}

	// Run main in a goroutine
	done := make(chan bool)
	go func() {
		// Recover from any panic
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Main function panicked (expected during shutdown): %v", r)
			}
			done <- true
		}()
		main()
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Send interrupt signal to trigger shutdown
	process, _ := os.FindProcess(os.Getpid())
	process.Signal(syscall.SIGTERM)

	// Wait for main to finish with timeout
	select {
	case <-done:
		// Main exited successfully
	case <-time.After(10 * time.Second):
		t.Error("Main function did not shutdown within timeout")
	}
}

// TestHealthEndpoint tests the health check endpoint
func TestHealthEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

// TestNodeServerStartup tests the node server startup and shutdown
func TestNodeServerStartup(t *testing.T) {
	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/control", handleControl)

	s := &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start server
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	serverStarted := make(chan bool)
	go func() {
		serverStarted <- true
		s.Serve(listener)
	}()

	// Wait for server to start
	<-serverStarted
	time.Sleep(10 * time.Millisecond)

	// Get actual address
	addr := listener.Addr().String()

	// Test health endpoint
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Errorf("Failed to reach health endpoint: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	}

	// Test control endpoint
	controlBody := bytes.NewReader([]byte(`{"op":"test"}`))
	resp, err = http.Post("http://"+addr+"/control", "application/json", controlBody)
	if err != nil {
		t.Errorf("Failed to reach control endpoint: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status 204, got %d", resp.StatusCode)
		}
	}

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = s.Shutdown(ctx)
	if err != nil {
		t.Errorf("Failed to shutdown server: %v", err)
	}
}

// TestEnvironmentVariableDefaults tests default values for optional env vars
func TestEnvironmentVariableDefaults(t *testing.T) {
	// Ensure NODE_LISTEN is not set
	os.Unsetenv("NODE_LISTEN")

	// Test default value
	listen := getenv("NODE_LISTEN", ":8081")
	if listen != ":8081" {
		t.Errorf("Expected default ':8081', got %s", listen)
	}

	// Ensure NODE_ADDR is not set
	os.Unsetenv("NODE_ADDR")

	// Test default value
	addr := getenv("NODE_ADDR", "http://127.0.0.1:8081")
	if addr != "http://127.0.0.1:8081" {
		t.Errorf("Expected default 'http://127.0.0.1:8081', got %s", addr)
	}
}

// TestConcurrentControlMessages tests handling multiple concurrent control messages
func TestConcurrentControlMessages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/control", handleControl)

	server := httptest.NewServer(mux)
	defer server.Close()

	// Number of concurrent requests
	numRequests := 100
	done := make(chan bool, numRequests)

	// Send concurrent requests
	for i := 0; i < numRequests; i++ {
		go func(id int) {
			body := bytes.NewReader([]byte(fmt.Sprintf(`{"op":"test","id":%d}`, id)))
			resp, err := http.Post(server.URL+"/control", "application/json", body)
			if err != nil {
				t.Errorf("Request %d failed: %v", id, err)
			}
			if resp != nil {
				resp.Body.Close()
				if resp.StatusCode != http.StatusNoContent {
					t.Errorf("Request %d: expected status 204, got %d", id, resp.StatusCode)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		select {
		case <-done:
			// Request completed
		case <-time.After(5 * time.Second):
			t.Errorf("Timeout waiting for request %d", i)
		}
	}
}
