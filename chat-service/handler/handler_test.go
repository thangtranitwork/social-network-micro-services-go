package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"social-network-go/chat-service/model"
	"social-network-go/chat-service/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type MockChatService struct {
	Messages     []*model.Message
	MessageResps []*service.MessageResponse
	ChatRooms    []service.ChatRoom
	Err          error
}

func (m *MockChatService) RegisterClient(userID string, conn *websocket.Conn) {}
func (m *MockChatService) HandleIncomingMessages(userID string)               {}
func (m *MockChatService) GetChatHistory(userID, partnerID string) []*model.Message {
	return m.Messages
}
func (m *MockChatService) GetChatList(userID string) []service.ChatRoom {
	return m.ChatRooms
}
func (m *MockChatService) IsMemberOfChat(userID, chatID string) bool {
	return true
}
func (m *MockChatService) GetChatMessages(chatID string, skip, limit int64) []*service.MessageResponse {
	return m.MessageResps
}
func (m *MockChatService) MarkMessagesAsRead(chatID, userID string) {}
func (m *MockChatService) SearchChats(userID, searchQuery string) []service.ChatRoom {
	return m.ChatRooms
}
func (m *MockChatService) SendMessage(senderID, receiverUsername, text string) (*service.MessageResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if len(m.MessageResps) > 0 {
		return m.MessageResps[0], nil
	}
	return &service.MessageResponse{ID: "msg-123"}, nil
}
func (m *MockChatService) SendGif(senderID, receiverUsername, gifURL string) (*service.MessageResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if len(m.MessageResps) > 0 {
		return m.MessageResps[0], nil
	}
	return &service.MessageResponse{ID: "msg-123"}, nil
}
func (m *MockChatService) SendFile(senderID, receiverUsername, fileID string) (*service.MessageResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if len(m.MessageResps) > 0 {
		return m.MessageResps[0], nil
	}
	return &service.MessageResponse{ID: "msg-123"}, nil
}
func (m *MockChatService) SendVoice(senderID, receiverUsername, fileID string) (*service.MessageResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if len(m.MessageResps) > 0 {
		return m.MessageResps[0], nil
	}
	return &service.MessageResponse{ID: "msg-123"}, nil
}
func (m *MockChatService) DeleteMessage(messageID, userID string) error {
	return m.Err
}
func (m *MockChatService) EditMessage(messageID, newContent, userID string) error {
	return m.Err
}

func setupTestRouter(svc *MockChatService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewChatHandler(svc)

	r.GET("/v1/chat/ws", h.HandleWebSocket)
	r.GET("/v1/chat", h.GetChatList)
	r.GET("/v1/chat/search", h.SearchChats)
	r.GET("/v1/chat/history/:partnerId", h.GetChatHistory)
	r.GET("/v1/chat/messages/:chatId", h.GetChatMessages)
	r.POST("/v1/chat/send", h.SendMessage)
	r.POST("/v1/chat/send-file", h.SendFile)
	r.POST("/v1/chat/send-gif", h.SendGif)
	r.POST("/v1/chat/send-voice", h.SendVoice)
	r.PUT("/v1/chat/edit", h.EditMessage)
	r.DELETE("/v1/chat/:messageId", h.DeleteMessage)
	r.POST("/v1/stringee/create-token", h.CreateStringeeToken)

	return r
}

func TestGetChatHistory(t *testing.T) {
	mockSvc := &MockChatService{
		Messages: []*model.Message{
			{ID: "msg-1", Content: "Hello"},
		},
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/v1/chat/history/partner-1", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetChatList(t *testing.T) {
	mockSvc := &MockChatService{
		ChatRooms: []service.ChatRoom{
			{ChatID: "chat-1", Name: "Room 1"},
		},
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/v1/chat", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetChatMessages(t *testing.T) {
	mockSvc := &MockChatService{
		MessageResps: []*service.MessageResponse{
			{ID: "msg-1", Content: "Hi"},
		},
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/v1/chat/messages/chat-1", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestCreateStringeeToken(t *testing.T) {
	os.Setenv("STRINGEE_SID", "sid123")
	os.Setenv("STRINGEE_SECRET", "secret123")
	defer func() {
		os.Unsetenv("STRINGEE_SID")
		os.Unsetenv("STRINGEE_SECRET")
	}()

	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("POST", "/v1/stringee/create-token", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestSearchChats(t *testing.T) {
	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/v1/chat/search?query=alice", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestSendMessage(t *testing.T) {
	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	payload := struct {
		Username string `json:"username"`
		Text     string `json:"text"`
	}{
		Username: "alice",
		Text:     "hello",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/chat/send", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestSendGif(t *testing.T) {
	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	payload := struct {
		Username string `json:"username"`
		URL      string `json:"url"`
	}{
		Username: "alice",
		URL:      "http://tenor.com/gif-1",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/chat/send-gif", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestSendFile(t *testing.T) {
	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	payload := struct {
		Username string `json:"username"`
		FileId   string `json:"fileId"`
	}{
		Username: "alice",
		FileId:   "file-123",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/chat/send-file", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestSendVoice(t *testing.T) {
	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	payload := struct {
		Username string `json:"username"`
		FileId   string `json:"fileId"`
	}{
		Username: "alice",
		FileId:   "voice-123",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/chat/send-voice", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestEditMessage(t *testing.T) {
	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	payload := struct {
		MessageID string `json:"messagesId"`
		Text      string `json:"text"`
	}{
		MessageID: "msg-1",
		Text:      "edited content",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PUT", "/v1/chat/edit", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteMessage(t *testing.T) {
	mockSvc := &MockChatService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("DELETE", "/v1/chat/msg-1", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}
