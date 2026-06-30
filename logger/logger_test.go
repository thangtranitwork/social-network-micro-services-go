package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"google.golang.org/grpc"
)

func TestJsonFieldLogging(t *testing.T) {
	// Redirect stdout to capture logs
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Set environment for console logging
	os.Setenv("LOG_LEVELS", "INFO,WARN,ERROR,FATAL")
	initialized = false // Force re-initialization
	initLogger()

	// Create and log an entry with normal fields and json fields
	jsonVal := map[string]interface{}{
		"user_id":  "12345",
		"username": "thangtran",
		"roles":    []string{"admin", "user"},
	}

	NewEntry().
		Field("request_id", "req-xyz").
		JsonField("custom_data", jsonVal).
		Info("Testing generic JsonField output")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Validate output
	t.Logf("Captured Output:\n%s", output)

	if !strings.Contains(output, "Testing generic JsonField output") {
		t.Error("Expected log message not found in stdout")
	}

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("Expected redirected stdout to contain JSON log entry: %v", err)
	}

	if entry.Fields["request_id"] != "req-xyz" {
		t.Errorf("Expected request_id field to be req-xyz, got %v", entry.Fields["request_id"])
	}

	customData, ok := entry.Fields["custom_data"].(string)
	if !ok {
		t.Fatalf("Expected custom_data field to be serialized as string, got %T", entry.Fields["custom_data"])
	}

	var parsedCustomData map[string]interface{}
	if err := json.Unmarshal([]byte(customData), &parsedCustomData); err != nil {
		t.Fatalf("Expected custom_data to contain valid JSON: %v", err)
	}

	if parsedCustomData["username"] != "thangtran" {
		t.Errorf("Expected custom_data username to be thangtran, got %v", parsedCustomData["username"])
	}
	roles, ok := parsedCustomData["roles"].([]interface{})
	if !ok || len(roles) != 2 || roles[0] != "admin" || roles[1] != "user" {
		t.Errorf("Expected custom_data roles to contain admin and user, got %v", parsedCustomData["roles"])
	}
}

func TestSerializeJSONVal(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected string
	}{
		{nil, ""},
		{"plain_string", "plain_string"},
		{[]byte("byte_slice"), "byte_slice"},
		{map[string]interface{}{"a": 1}, `{"a":1}`},
		{errors.New("some error"), "some error"},
	}

	for _, tc := range tests {
		got := serializeJSONVal(tc.input)
		// Map marshalling can have varying ordering, but for simple ones it is stable
		if tc.expected == `{"a":1}` {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(got), &m); err != nil || m["a"].(float64) != 1 {
				t.Errorf("serializeJSONVal(%v) = %q, expected %q", tc.input, got, tc.expected)
			}
		} else if got != tc.expected {
			t.Errorf("serializeJSONVal(%v) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestUnaryServerInterceptor(t *testing.T) {
	// Redirect stdout to capture logs
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Force re-initialization
	initialized = false
	initLogger()

	interceptor := UnaryServerInterceptor()
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return map[string]string{"status": "ok"}, nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/pb.AuthService/ValidateToken",
	}

	ctx := context.Background()
	_, _ = interceptor(ctx, map[string]string{"user_id": "123"}, info, handler)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "/pb.AuthService/ValidateToken") {
		t.Error("Expected gRPC method log in stdout")
	}
	if !strings.Contains(output, "grpc_method") {
		t.Error("Expected grpc_method key in stdout")
	}
	if !strings.Contains(output, "grpc_req") {
		t.Error("Expected grpc_req key in stdout")
	}
	if !strings.Contains(output, "grpc_resp") {
		t.Error("Expected grpc_resp key in stdout")
	}
}
