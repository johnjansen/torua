package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNodeInfo tests the NodeInfo struct serialization
func TestNodeInfo(t *testing.T) {
	// Test NodeInfo JSON marshaling and unmarshaling
	node := NodeInfo{
		ID:   "test-node-1",
		Addr: "http://localhost:8080",
	}

	// Marshal to JSON
	data, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("Failed to marshal NodeInfo: %v", err)
	}

	// Verify JSON structure contains required fields
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(data, &jsonMap); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if jsonMap["id"] != "test-node-1" {
		t.Errorf("Expected id 'test-node-1', got %v", jsonMap["id"])
	}
	if jsonMap["addr"] != "http://localhost:8080" {
		t.Errorf("Expected addr 'http://localhost:8080', got %v", jsonMap["addr"])
	}
	// Verify health fields exist
	if _, ok := jsonMap["health_status"]; !ok {
		t.Error("Missing health_status field")
	}
	if _, ok := jsonMap["last_health_check"]; !ok {
		t.Error("Missing last_health_check field")
	}

	// Unmarshal back
	var decoded NodeInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NodeInfo: %v", err)
	}

	// Verify fields
	if decoded.ID != node.ID {
		t.Errorf("Expected ID %s, got %s", node.ID, decoded.ID)
	}
	if decoded.Addr != node.Addr {
		t.Errorf("Expected Addr %s, got %s", node.Addr, decoded.Addr)
	}
}

// TestRegisterRequest tests the RegisterRequest struct
func TestRegisterRequest(t *testing.T) {
	// Create a register request
	req := RegisterRequest{
		Node: NodeInfo{
			ID:   "node-2",
			Addr: "http://localhost:8081",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal RegisterRequest: %v", err)
	}

	// Unmarshal and verify
	var decoded RegisterRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal RegisterRequest: %v", err)
	}

	// Verify nested struct
	if decoded.Node.ID != req.Node.ID {
		t.Errorf("Expected Node.ID %s, got %s", req.Node.ID, decoded.Node.ID)
	}
	if decoded.Node.Addr != req.Node.Addr {
		t.Errorf("Expected Node.Addr %s, got %s", req.Node.Addr, decoded.Node.Addr)
	}
}

// TestBroadcastRequest tests the BroadcastRequest struct
func TestBroadcastRequest(t *testing.T) {
	// Create broadcast request with raw JSON payload
	payload := json.RawMessage(`{"op":"ping","timestamp":"2024-01-15T10:00:00Z"}`)
	req := BroadcastRequest{
		Path:    "/control",
		Payload: payload,
	}

	// Marshal to JSON
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal BroadcastRequest: %v", err)
	}

	// Unmarshal and verify
	var decoded BroadcastRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal BroadcastRequest: %v", err)
	}

	// Verify fields
	if decoded.Path != req.Path {
		t.Errorf("Expected Path %s, got %s", req.Path, decoded.Path)
	}

	// Verify payload (raw JSON should be preserved)
	if !bytes.Equal(decoded.Payload, req.Payload) {
		t.Errorf("Payload mismatch: expected %s, got %s", req.Payload, decoded.Payload)
	}
}

// TestPostJSON tests the PostJSON function with various scenarios
func TestPostJSON(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse int
		serverBody     string
		requestBody    interface{}
		responseBody   interface{}
		expectError    bool
		contextTimeout bool
	}{
		{
			name:           "successful POST with response",
			serverResponse: http.StatusOK,
			serverBody:     `{"status":"ok"}`,
			requestBody:    map[string]string{"test": "data"},
			responseBody:   &map[string]string{},
			expectError:    false,
		},
		{
			name:           "successful POST without response body",
			serverResponse: http.StatusNoContent,
			serverBody:     "",
			requestBody:    map[string]string{"test": "data"},
			responseBody:   nil,
			expectError:    false,
		},
		{
			name:           "server error response",
			serverResponse: http.StatusInternalServerError,
			serverBody:     `{"error":"internal error"}`,
			requestBody:    map[string]string{"test": "data"},
			responseBody:   nil,
			expectError:    true,
		},
		{
			name:           "bad request",
			serverResponse: http.StatusBadRequest,
			serverBody:     `{"error":"bad request"}`,
			requestBody:    map[string]string{"test": "data"},
			responseBody:   nil,
			expectError:    true,
		},
		{
			name:           "context timeout",
			serverResponse: http.StatusOK,
			serverBody:     `{"status":"ok"}`,
			requestBody:    map[string]string{"test": "data"},
			responseBody:   nil,
			expectError:    true,
			contextTimeout: true,
		},
		{
			name:           "unmarshalable request body",
			serverResponse: http.StatusOK,
			serverBody:     `{"status":"ok"}`,
			requestBody:    make(chan int), // channels can't be marshaled
			responseBody:   nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify method
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST method, got %s", r.Method)
				}

				// Verify content-type
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("Expected Content-Type application/json, got %s", ct)
				}

				// Simulate delay for timeout test
				if tt.contextTimeout {
					time.Sleep(100 * time.Millisecond)
				}

				// Send response
				w.WriteHeader(tt.serverResponse)
				if tt.serverBody != "" {
					w.Write([]byte(tt.serverBody))
				}
			}))
			defer server.Close()

			// Create context
			ctx := context.Background()
			if tt.contextTimeout {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 1*time.Millisecond)
				defer cancel()
			}

			// Call PostJSON
			err := PostJSON(ctx, server.URL, tt.requestBody, tt.responseBody)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify response body if applicable
			if !tt.expectError && tt.responseBody != nil {
				respMap := tt.responseBody.(*map[string]string)
				if (*respMap)["status"] != "ok" {
					t.Errorf("Expected response status 'ok', got %v", *respMap)
				}
			}
		})
	}
}

// TestPostJSONInvalidURL tests PostJSON with invalid URL
func TestPostJSONInvalidURL(t *testing.T) {
	ctx := context.Background()

	// Test with invalid URL
	err := PostJSON(ctx, "://invalid-url", map[string]string{"test": "data"}, nil)
	if err == nil {
		t.Error("Expected error for invalid URL, got none")
	}

	// Test with unreachable server
	err = PostJSON(ctx, "http://localhost:99999", map[string]string{"test": "data"}, nil)
	if err == nil {
		t.Error("Expected error for unreachable server, got none")
	}
}

// TestGetJSON tests the GetJSON function with various scenarios
func TestGetJSON(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse int
		serverBody     string
		responseBody   interface{}
		expectError    bool
		contextTimeout bool
	}{
		{
			name:           "successful GET",
			serverResponse: http.StatusOK,
			serverBody:     `{"data":"test","value":123}`,
			responseBody:   &map[string]interface{}{},
			expectError:    false,
		},
		{
			name:           "not found error",
			serverResponse: http.StatusNotFound,
			serverBody:     `{"error":"not found"}`,
			responseBody:   &map[string]interface{}{},
			expectError:    true,
		},
		{
			name:           "server error",
			serverResponse: http.StatusInternalServerError,
			serverBody:     `{"error":"internal server error"}`,
			responseBody:   &map[string]interface{}{},
			expectError:    true,
		},
		{
			name:           "context timeout",
			serverResponse: http.StatusOK,
			serverBody:     `{"data":"test"}`,
			responseBody:   &map[string]interface{}{},
			expectError:    true,
			contextTimeout: true,
		},
		{
			name:           "invalid JSON response",
			serverResponse: http.StatusOK,
			serverBody:     `{invalid json}`,
			responseBody:   &map[string]interface{}{},
			expectError:    true,
		},
		{
			name:           "redirect response",
			serverResponse: http.StatusMovedPermanently,
			serverBody:     "",
			responseBody:   &map[string]interface{}{},
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify method
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET method, got %s", r.Method)
				}

				// Simulate delay for timeout test
				if tt.contextTimeout {
					time.Sleep(100 * time.Millisecond)
				}

				// Send response
				w.WriteHeader(tt.serverResponse)
				if tt.serverBody != "" {
					w.Write([]byte(tt.serverBody))
				}
			}))
			defer server.Close()

			// Create context
			ctx := context.Background()
			if tt.contextTimeout {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 1*time.Millisecond)
				defer cancel()
			}

			// Call GetJSON
			err := GetJSON(ctx, server.URL, tt.responseBody)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify response body if successful
			if !tt.expectError && tt.responseBody != nil {
				respMap := tt.responseBody.(*map[string]interface{})
				if (*respMap)["data"] != "test" {
					t.Errorf("Expected data 'test', got %v", (*respMap)["data"])
				}
				if (*respMap)["value"] != float64(123) { // JSON numbers decode as float64
					t.Errorf("Expected value 123, got %v", (*respMap)["value"])
				}
			}
		})
	}
}

// TestGetJSONInvalidURL tests GetJSON with invalid URL
func TestGetJSONInvalidURL(t *testing.T) {
	ctx := context.Background()
	var result map[string]interface{}

	// Test with invalid URL
	err := GetJSON(ctx, "://invalid-url", &result)
	if err == nil {
		t.Error("Expected error for invalid URL, got none")
	}

	// Test with unreachable server
	err = GetJSON(ctx, "http://localhost:99999", &result)
	if err == nil {
		t.Error("Expected error for unreachable server, got none")
	}
}

// TestHTTPClient tests that the HTTP client has proper timeout
func TestHTTPClient(t *testing.T) {
	// Verify the global httpClient is configured correctly
	if httpClient.Timeout != 5*time.Second {
		t.Errorf("Expected HTTP client timeout of 5s, got %v", httpClient.Timeout)
	}
}

// TestJSONRawMessage tests json.RawMessage handling in BroadcastRequest
func TestJSONRawMessage(t *testing.T) {
	// Test with various payload types
	testCases := []struct {
		name    string
		payload string
	}{
		{"object payload", `{"op":"test","value":123}`},
		{"array payload", `[1,2,3]`},
		{"string payload", `"simple string"`},
		{"number payload", `42`},
		{"boolean payload", `true`},
		{"null payload", `null`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := BroadcastRequest{
				Path:    "/test",
				Payload: json.RawMessage(tc.payload),
			}

			// Marshal and unmarshal
			data, err := json.Marshal(req)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var decoded BroadcastRequest
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			// Verify payload is preserved exactly
			if string(decoded.Payload) != tc.payload {
				t.Errorf("Payload not preserved: expected %s, got %s", tc.payload, string(decoded.Payload))
			}
		})
	}
}
