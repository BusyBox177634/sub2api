package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type ChatHandler struct {
	chatService    *service.ChatService
	gateway        *GatewayHandler
	openAIGateway  *OpenAIGatewayHandler
}

func NewChatHandler(
	chatService *service.ChatService,
	gateway *GatewayHandler,
	openAIGateway *OpenAIGatewayHandler,
) *ChatHandler {
	return &ChatHandler{
		chatService:   chatService,
		gateway:       gateway,
		openAIGateway: openAIGateway,
	}
}

type createChatConversationRequest struct {
	APIKeyID int64  `json:"api_key_id" binding:"required"`
	Model    string `json:"model" binding:"required"`
	Title    string `json:"title"`
}

type updateChatConversationRequest struct {
	APIKeyID *int64  `json:"api_key_id"`
	Model    *string `json:"model"`
	Title    *string `json:"title"`
}

type sendChatMessageRequest struct {
	Text          string  `json:"text"`
	AttachmentIDs []int64 `json:"attachment_ids"`
}

type chatAPIKeyOption struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	GroupID      int64   `json:"group_id"`
	GroupName    string  `json:"group_name"`
	GroupPlatform string `json:"group_platform"`
	Model        string  `json:"model,omitempty"`
}

type chatModelResponse struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"display_name"`
	SupportsImageInput bool   `json:"supports_image_input"`
	SupportsStream     bool   `json:"supports_stream"`
}

func (h *ChatHandler) ListAPIKeys(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	keys, err := h.chatService.ListAvailableAPIKeys(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]chatAPIKeyOption, 0, len(keys))
	for i := range keys {
		if keys[i].Group == nil {
			continue
		}
		out = append(out, chatAPIKeyOption{
			ID:            keys[i].ID,
			Name:          keys[i].Name,
			GroupID:       keys[i].Group.ID,
			GroupName:     keys[i].Group.Name,
			GroupPlatform: keys[i].Group.Platform,
		})
	}
	response.Success(c, out)
}

func (h *ChatHandler) ListModels(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	apiKeyID, err := strconv.ParseInt(strings.TrimSpace(c.Query("api_key_id")), 10, 64)
	if err != nil || apiKeyID <= 0 {
		response.BadRequest(c, "Invalid api_key_id")
		return
	}

	apiKey, err := h.chatService.GetAPIKeyForChat(c.Request.Context(), subject.UserID, apiKeyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	models, err := h.loadChatModels(c.Request.Context(), apiKey)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, models)
}

func (h *ChatHandler) ListConversations(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	items, err := h.chatService.ListConversations(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *ChatHandler) CreateConversation(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req createChatConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	conversation, err := h.chatService.CreateConversation(
		c.Request.Context(),
		subject.UserID,
		req.APIKeyID,
		req.Model,
		req.Title,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, conversation)
}

func (h *ChatHandler) UpdateConversation(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}

	var req updateChatConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	conversation, err := h.chatService.UpdateConversation(
		c.Request.Context(),
		subject.UserID,
		conversationID,
		req.Title,
		req.APIKeyID,
		req.Model,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, conversation)
}

func (h *ChatHandler) DeleteConversation(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}
	if err := h.chatService.DeleteConversation(c.Request.Context(), subject.UserID, conversationID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "conversation deleted"})
}

func (h *ChatHandler) ListMessages(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}
	items, err := h.chatService.ListMessages(c.Request.Context(), subject.UserID, conversationID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *ChatHandler) UploadAttachment(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.BadRequest(c, "failed to read file")
		return
	}
	defer func() { _ = file.Close() }()

	payload, err := io.ReadAll(file)
	if err != nil {
		response.BadRequest(c, "failed to read file")
		return
	}

	contentType := fileHeader.Header.Get("Content-Type")
	attachment, err := h.chatService.CreateAttachment(
		c.Request.Context(),
		subject.UserID,
		conversationID,
		fileHeader.Filename,
		payload,
		contentType,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, attachment)
}

func (h *ChatHandler) DeleteAttachment(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	attachmentID, err := strconv.ParseInt(c.Param("attachmentID"), 10, 64)
	if err != nil || attachmentID <= 0 {
		response.BadRequest(c, "Invalid attachment ID")
		return
	}
	if err := h.chatService.DeleteAttachment(c.Request.Context(), subject.UserID, attachmentID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "attachment deleted"})
}

func (h *ChatHandler) StreamMessage(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}

	var req sendChatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	turn, err := h.chatService.PrepareResponsesTurn(
		c.Request.Context(),
		subject.UserID,
		conversationID,
		req.Text,
		req.AttachmentIDs,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	captured, statusCode, contentType, bridgeErr := h.bridgeResponsesStream(c, turn.APIKey, turn.Subscription, turn.RequestBody)
	finalStatus := service.ChatMessageStatusCompleted
	finalText := ""
	finalError := ""

	switch {
	case bridgeErr != nil:
		finalStatus = service.ChatMessageStatusFailed
		finalError = bridgeErr.Error()
	case c.Request.Context().Err() != nil:
		finalStatus = service.ChatMessageStatusStopped
		finalText, finalError, _ = service.ParseAssistantTextFromResponsesSSE(captured)
	case strings.Contains(strings.ToLower(contentType), "text/event-stream"):
		finalText, finalError, _ = service.ParseAssistantTextFromResponsesSSE(captured)
		if strings.TrimSpace(finalError) != "" {
			finalStatus = service.ChatMessageStatusFailed
		}
	case statusCode >= http.StatusBadRequest:
		finalStatus = service.ChatMessageStatusFailed
		finalError = extractChatBridgeError(captured)
	default:
		finalStatus = service.ChatMessageStatusStopped
		finalText = strings.TrimSpace(captured)
	}

	if _, err := h.chatService.FinalizeAssistantMessage(
		context.WithoutCancel(c.Request.Context()),
		subject.UserID,
		turn.AssistantMessage.ID,
		finalStatus,
		finalText,
		finalError,
	); err != nil {
		slog.Warn("finalize chat assistant message failed", "conversation_id", conversationID, "message_id", turn.AssistantMessage.ID, "error", err)
	}
}

func (h *ChatHandler) loadChatModels(ctx context.Context, apiKey *service.APIKey) ([]chatModelResponse, error) {
	recorder := httptest.NewRecorder()
	innerCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(ctx)
	innerCtx.Request = req
	seedChatGatewayContext(innerCtx, apiKey, nil)
	h.gateway.Models(innerCtx)
	if recorder.Code >= http.StatusBadRequest {
		return nil, fmt.Errorf("load models failed: %s", extractChatBridgeError(recorder.Body.String()))
	}

	modelItems := gjson.GetBytes(recorder.Body.Bytes(), "data").Array()
	out := make([]chatModelResponse, 0, len(modelItems))
	for _, item := range modelItems {
		id := strings.TrimSpace(item.Get("id").String())
		if id == "" {
			continue
		}
		displayName := strings.TrimSpace(item.Get("display_name").String())
		if displayName == "" {
			displayName = id
		}
		out = append(out, chatModelResponse{
			ID:                 id,
			DisplayName:        displayName,
			SupportsImageInput: modelSupportsImageInput(apiKey.Group.Platform, id),
			SupportsStream:     true,
		})
	}
	return out, nil
}

func (h *ChatHandler) bridgeResponsesStream(
	c *gin.Context,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	requestBody []byte,
) (captured string, statusCode int, contentType string, err error) {
	writer := newChatBridgeWriter(c.Writer)
	innerCtx, _ := gin.CreateTestContext(writer)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody)).WithContext(c.Request.Context())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	innerCtx.Request = req
	seedChatGatewayContext(innerCtx, apiKey, subscription)

	if apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform == service.PlatformOpenAI {
		h.openAIGateway.Responses(innerCtx)
	} else {
		h.gateway.Responses(innerCtx)
	}

	return writer.BufferString(), writer.Status(), c.Writer.Header().Get("Content-Type"), nil
}

func seedChatGatewayContext(c *gin.Context, apiKey *service.APIKey, subscription *service.UserSubscription) {
	if c == nil || apiKey == nil || apiKey.User == nil {
		return
	}
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
		UserID:      apiKey.User.ID,
		Concurrency: apiKey.User.Concurrency,
	})
	c.Set(string(middleware2.ContextKeyUserRole), apiKey.User.Role)
	if subscription != nil {
		c.Set(string(middleware2.ContextKeySubscription), subscription)
	}
	if apiKey.Group != nil {
		reqCtx := context.WithValue(c.Request.Context(), ctxkey.Group, apiKey.Group)
		c.Request = c.Request.WithContext(reqCtx)
	}
}

type chatBridgeWriter struct {
	target gin.ResponseWriter
	buffer bytes.Buffer
	status int
}

func newChatBridgeWriter(target gin.ResponseWriter) *chatBridgeWriter {
	return &chatBridgeWriter{target: target}
}

func (w *chatBridgeWriter) Header() http.Header {
	return w.target.Header()
}

func (w *chatBridgeWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.target.WriteHeader(statusCode)
}

func (w *chatBridgeWriter) Write(payload []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	_, _ = w.buffer.Write(payload)
	return w.target.Write(payload)
}

func (w *chatBridgeWriter) Flush() {
	if flusher, ok := any(w.target).(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *chatBridgeWriter) BufferString() string {
	return w.buffer.String()
}

func (w *chatBridgeWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func extractChatBridgeError(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "chat request failed"
	}
	if gjson.Valid(raw) {
		return firstNonEmptyChatString(
			gjson.Get(raw, "error.message").String(),
			gjson.Get(raw, "message").String(),
			gjson.Get(raw, "reason").String(),
		)
	}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if !gjson.Valid(payload) {
			continue
		}
		if message := firstNonEmptyChatString(
			gjson.Get(payload, "error.message").String(),
			gjson.Get(payload, "message").String(),
		); message != "" {
			return message
		}
	}
	return raw
}

func modelSupportsImageInput(platform, modelID string) bool {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	switch platform {
	case service.PlatformAnthropic:
		return strings.HasPrefix(modelID, "claude")
	case service.PlatformOpenAI:
		if strings.Contains(modelID, "codex") {
			return false
		}
		return strings.HasPrefix(modelID, "gpt-") ||
			strings.HasPrefix(modelID, "chatgpt-") ||
			strings.HasPrefix(modelID, "o1") ||
			strings.HasPrefix(modelID, "o3") ||
			strings.HasPrefix(modelID, "o4")
	default:
		return false
	}
}

func firstNonEmptyChatString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
