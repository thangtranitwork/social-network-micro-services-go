package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestE2EHandlers_Interface(t *testing.T) {
	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)

	// Test SendEncryptedMessage via /v1/chat/send
	reqBody := map[string]interface{}{
		"chatId":      "chat-e2e-1",
		"text":        "encrypted-base64-ciphertext",
		"isEncrypted": true,
		"e2eHeader": map[string]interface{}{
			"version":        1,
			"algorithm":      "X3DH_DR_V1",
			"senderDeviceId": "dev-1",
			"iv":             "random-iv-base64",
		},
	}
	payload, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/send", bytes.NewBuffer(payload))
	req.Header.Set("X-User-ID", "user-alice")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ApiResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.Code)

	bodyMap, ok := resp.Body.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "e2e-msg-123", bodyMap["id"])
	assert.Equal(t, true, bodyMap["isEncrypted"])
}

func TestE2EHandlers_InvalidEncryptedPayload(t *testing.T) {
	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)

	// Missing e2eHeader when isEncrypted is true
	reqBody := map[string]interface{}{
		"chatId":      "chat-e2e-1",
		"text":        "ciphertext-without-header",
		"isEncrypted": true,
	}
	payload, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/send", bytes.NewBuffer(payload))
	req.Header.Set("X-User-ID", "user-alice")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
