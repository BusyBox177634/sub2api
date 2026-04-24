package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/tidwall/gjson"
)

const (
	defaultChatConversationTitle  = "新对话"
	maxChatConversationTitleRunes = 48
	maxChatListAPIKeys            = 200
)

type ChatService struct {
	repo                ChatRepository
	apiKeyService       *APIKeyService
	subscriptionService *SubscriptionService
	attachmentStorage   *ChatAttachmentStorage
}

func NewChatService(
	repo ChatRepository,
	apiKeyService *APIKeyService,
	subscriptionService *SubscriptionService,
	attachmentStorage *ChatAttachmentStorage,
) *ChatService {
	return &ChatService{
		repo:                repo,
		apiKeyService:       apiKeyService,
		subscriptionService: subscriptionService,
		attachmentStorage:   attachmentStorage,
	}
}

func (s *ChatService) ListAvailableAPIKeys(ctx context.Context, userID int64) ([]APIKey, error) {
	if s == nil || s.apiKeyService == nil {
		return nil, infraerrors.InternalServer("CHAT_SERVICE_UNAVAILABLE", "chat service is unavailable")
	}
	keys, _, err := s.apiKeyService.List(ctx, userID, pagination.PaginationParams{
		Page:     1,
		PageSize: maxChatListAPIKeys,
	}, APIKeyListFilters{})
	if err != nil {
		return nil, err
	}

	out := make([]APIKey, 0, len(keys))
	for i := range keys {
		key := keys[i]
		if !s.isAPIKeyAllowedForChat(&key) {
			continue
		}
		out = append(out, key)
	}
	return out, nil
}

func (s *ChatService) GetAPIKeyForChat(ctx context.Context, userID, apiKeyID int64) (*APIKey, error) {
	apiKey, _, err := s.loadAPIKeyForConversation(ctx, userID, apiKeyID)
	if err != nil {
		return nil, err
	}
	return apiKey, nil
}

func (s *ChatService) ListConversations(ctx context.Context, userID int64) ([]ChatConversation, error) {
	return s.repo.ListConversationsByUser(ctx, userID)
}

func (s *ChatService) GetConversation(ctx context.Context, userID, conversationID int64) (*ChatConversation, error) {
	conversation, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conversation.UserID != userID {
		return nil, ErrChatConversationNotFound
	}
	return conversation, nil
}

func (s *ChatService) CreateConversation(ctx context.Context, userID, apiKeyID int64, model, title string) (*ChatConversation, error) {
	apiKey, _, err := s.loadAPIKeyForConversation(ctx, userID, apiKeyID)
	if err != nil {
		return nil, err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, infraerrors.BadRequest("CHAT_MODEL_REQUIRED", "model is required")
	}

	conversation := &ChatConversation{
		UserID:    userID,
		APIKeyID:  apiKey.ID,
		Title:     normalizeConversationTitle(title),
		Model:     model,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if conversation.Title == "" {
		conversation.Title = defaultChatConversationTitle
	}
	if err := s.repo.CreateConversation(ctx, conversation); err != nil {
		return nil, err
	}
	return conversation, nil
}

func (s *ChatService) UpdateConversation(
	ctx context.Context,
	userID, conversationID int64,
	title *string,
	apiKeyID *int64,
	model *string,
) (*ChatConversation, error) {
	conversation, err := s.GetConversation(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}

	if title != nil {
		nextTitle := normalizeConversationTitle(*title)
		if nextTitle == "" {
			nextTitle = defaultChatConversationTitle
		}
		conversation.Title = nextTitle
	}
	if apiKeyID != nil {
		apiKey, _, err := s.loadAPIKeyForConversation(ctx, userID, *apiKeyID)
		if err != nil {
			return nil, err
		}
		conversation.APIKeyID = apiKey.ID
	}
	if model != nil {
		nextModel := strings.TrimSpace(*model)
		if nextModel == "" {
			return nil, infraerrors.BadRequest("CHAT_MODEL_REQUIRED", "model is required")
		}
		conversation.Model = nextModel
	}
	conversation.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateConversation(ctx, conversation); err != nil {
		return nil, err
	}
	return conversation, nil
}

func (s *ChatService) DeleteConversation(ctx context.Context, userID, conversationID int64) error {
	conversation, err := s.GetConversation(ctx, userID, conversationID)
	if err != nil {
		return err
	}
	attachments, err := s.repo.ListAttachmentsByConversation(ctx, conversation.ID)
	if err != nil {
		return err
	}
	for i := range attachments {
		if err := s.attachmentStorage.DeleteFile(attachments[i].StoragePath); err != nil {
			return err
		}
	}
	return s.repo.DeleteConversation(ctx, conversation.ID)
}

func (s *ChatService) ListMessages(ctx context.Context, userID, conversationID int64) ([]ChatMessage, error) {
	conversation, err := s.GetConversation(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	messages, err := s.repo.ListMessagesByConversation(ctx, conversation.ID)
	if err != nil {
		return nil, err
	}
	return s.hydrateMessages(ctx, messages)
}

func (s *ChatService) CreateAttachment(
	ctx context.Context,
	userID, conversationID int64,
	originalName string,
	payload []byte,
	contentType string,
) (*ChatAttachment, error) {
	conversation, err := s.GetConversation(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}

	attachment, err := s.attachmentStorage.SaveImage(userID, originalName, payload, contentType)
	if err != nil {
		return nil, err
	}
	attachment.ConversationID = conversation.ID
	attachment.CreatedAt = time.Now().UTC()
	attachment.UpdatedAt = attachment.CreatedAt

	if err := s.repo.CreateAttachment(ctx, attachment); err != nil {
		_ = s.attachmentStorage.DeleteFile(attachment.StoragePath)
		return nil, err
	}
	return attachment, nil
}

func (s *ChatService) DeleteAttachment(ctx context.Context, userID, attachmentID int64) error {
	attachment, err := s.repo.GetAttachmentByID(ctx, attachmentID)
	if err != nil {
		return err
	}
	if attachment.UserID != userID {
		return ErrChatAttachmentNotFound
	}
	if attachment.MessageID != nil {
		return ErrChatAttachmentAssigned
	}
	if err := s.attachmentStorage.DeleteFile(attachment.StoragePath); err != nil {
		return err
	}
	return s.repo.DeleteAttachment(ctx, attachmentID)
}

func (s *ChatService) PrepareResponsesTurn(
	ctx context.Context,
	userID, conversationID int64,
	text string,
	attachmentIDs []int64,
) (*ChatPreparedTurn, error) {
	text = strings.TrimSpace(text)
	if text == "" && len(attachmentIDs) == 0 {
		return nil, ErrChatEmptyInput
	}

	conversation, err := s.GetConversation(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	apiKey, subscription, err := s.loadAPIKeyForConversation(ctx, userID, conversation.APIKeyID)
	if err != nil {
		return nil, err
	}

	attachments, err := s.loadPendingAttachments(ctx, userID, conversation.ID, attachmentIDs)
	if err != nil {
		return nil, err
	}

	userMessage := &ChatMessage{
		ConversationID: conversation.ID,
		UserID:         userID,
		Role:           ChatRoleUser,
		Status:         ChatMessageStatusCompleted,
		Text:           text,
		Model:          conversation.Model,
		AttachmentIDs:  cloneInt64Slice(attachmentIDs),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := s.repo.CreateMessage(ctx, userMessage); err != nil {
		return nil, err
	}
	if len(attachmentIDs) > 0 {
		if err := s.repo.UpdateAttachmentMessageID(ctx, attachmentIDs, userMessage.ID); err != nil {
			return nil, err
		}
	}
	userMessage.Attachments = attachments

	assistantMessage := &ChatMessage{
		ConversationID: conversation.ID,
		UserID:         userID,
		Role:           ChatRoleAssistant,
		Status:         ChatMessageStatusPending,
		Text:           "",
		Model:          conversation.Model,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := s.repo.CreateMessage(ctx, assistantMessage); err != nil {
		return nil, err
	}

	if conversation.Title == "" || conversation.Title == defaultChatConversationTitle {
		conversation.Title = autoConversationTitle(text, len(attachmentIDs) > 0)
	}
	now := time.Now().UTC()
	conversation.LastMessageAt = &now
	conversation.UpdatedAt = now
	if err := s.repo.UpdateConversation(ctx, conversation); err != nil {
		return nil, err
	}

	requestBody, err := s.buildResponsesRequestBody(ctx, conversation.ID, conversation.Model)
	if err != nil {
		return nil, err
	}

	return &ChatPreparedTurn{
		Conversation:     conversation,
		APIKey:           apiKey,
		Subscription:     subscription,
		UserMessage:      userMessage,
		AssistantMessage: assistantMessage,
		RequestBody:      requestBody,
	}, nil
}

func (s *ChatService) FinalizeAssistantMessage(
	ctx context.Context,
	userID, assistantMessageID int64,
	status string,
	text string,
	errorMessage string,
) (*ChatMessage, error) {
	message, err := s.repo.GetMessageByID(ctx, assistantMessageID)
	if err != nil {
		return nil, err
	}
	if message.UserID != userID {
		return nil, ErrChatMessageNotFound
	}
	message.Status = status
	message.Text = strings.TrimSpace(text)
	message.ErrorMessage = strings.TrimSpace(errorMessage)
	message.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateMessage(ctx, message); err != nil {
		return nil, err
	}
	return message, nil
}

func (s *ChatService) loadAPIKeyForConversation(ctx context.Context, userID, apiKeyID int64) (*APIKey, *UserSubscription, error) {
	if s.apiKeyService == nil {
		return nil, nil, infraerrors.InternalServer("CHAT_SERVICE_UNAVAILABLE", "chat service is unavailable")
	}
	apiKey, err := s.apiKeyService.GetByID(ctx, apiKeyID)
	if err != nil {
		return nil, nil, err
	}
	if apiKey.UserID != userID {
		return nil, nil, ErrChatAPIKeyUnavailable
	}
	if !s.isAPIKeyAllowedForChat(apiKey) {
		return nil, nil, ErrChatAPIKeyUnavailable
	}

	var subscription *UserSubscription
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() && s.subscriptionService != nil {
		subscription, err = s.subscriptionService.GetActiveSubscription(ctx, userID, apiKey.Group.ID)
		if err != nil {
			return nil, nil, err
		}
	}
	return apiKey, subscription, nil
}

func (s *ChatService) isAPIKeyAllowedForChat(apiKey *APIKey) bool {
	if apiKey == nil || apiKey.Group == nil {
		return false
	}
	if apiKey.User != nil {
		if apiKey.User.Status != StatusActive {
			return false
		}
		if !apiKey.User.IsActive() {
			return false
		}
	}
	if apiKey.Status != StatusAPIKeyActive {
		return false
	}
	if apiKey.IsExpired() || apiKey.IsQuotaExhausted() {
		return false
	}
	switch apiKey.Group.Platform {
	case PlatformOpenAI, PlatformAnthropic:
		return true
	default:
		return false
	}
}

func (s *ChatService) loadPendingAttachments(
	ctx context.Context,
	userID, conversationID int64,
	attachmentIDs []int64,
) ([]ChatAttachment, error) {
	if len(attachmentIDs) == 0 {
		return nil, nil
	}
	out := make([]ChatAttachment, 0, len(attachmentIDs))
	for _, attachmentID := range attachmentIDs {
		attachment, err := s.repo.GetAttachmentByID(ctx, attachmentID)
		if err != nil {
			return nil, err
		}
		if attachment.UserID != userID || attachment.ConversationID != conversationID {
			return nil, ErrChatAttachmentNotFound
		}
		if attachment.MessageID != nil {
			return nil, ErrChatAttachmentAssigned
		}
		dataURL, err := s.attachmentStorage.BuildDataURL(attachment.StoragePath, attachment.MimeType)
		if err != nil {
			return nil, err
		}
		attachment.DataURL = dataURL
		out = append(out, *attachment)
	}
	return out, nil
}

func (s *ChatService) buildResponsesRequestBody(ctx context.Context, conversationID int64, model string) ([]byte, error) {
	messages, err := s.repo.ListMessagesByConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	messages, err = s.hydrateMessages(ctx, messages)
	if err != nil {
		return nil, err
	}

	input := make([]apicompat.ResponsesInputItem, 0, len(messages))
	for i := range messages {
		message := messages[i]
		if message.Role != ChatRoleUser && message.Role != ChatRoleAssistant {
			continue
		}
		if message.Role == ChatRoleAssistant && message.Status != ChatMessageStatusCompleted {
			continue
		}
		content, err := buildResponsesContentForChatMessage(&message)
		if err != nil {
			return nil, err
		}
		input = append(input, apicompat.ResponsesInputItem{
			Role:    message.Role,
			Content: content,
		})
	}

	request := apicompat.ResponsesRequest{
		Model:  model,
		Stream: true,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal chat input: %w", err)
	}
	request.Input = inputJSON
	return json.Marshal(request)
}

func buildResponsesContentForChatMessage(message *ChatMessage) (json.RawMessage, error) {
	if message == nil {
		return json.RawMessage(`[]`), nil
	}
	parts := make([]apicompat.ResponsesContentPart, 0, len(message.Attachments)+1)
	if text := strings.TrimSpace(message.Text); text != "" {
		partType := "input_text"
		if message.Role == ChatRoleAssistant {
			partType = "output_text"
		}
		parts = append(parts, apicompat.ResponsesContentPart{
			Type: partType,
			Text: text,
		})
	}
	if message.Role == ChatRoleUser {
		for i := range message.Attachments {
			if message.Attachments[i].Kind != ChatAttachmentKindImage || message.Attachments[i].DataURL == "" {
				continue
			}
			parts = append(parts, apicompat.ResponsesContentPart{
				Type:     "input_image",
				ImageURL: message.Attachments[i].DataURL,
			})
		}
	}
	if len(parts) == 0 {
		return json.RawMessage(`[]`), nil
	}
	return json.Marshal(parts)
}

func (s *ChatService) hydrateMessages(ctx context.Context, messages []ChatMessage) ([]ChatMessage, error) {
	if len(messages) == 0 {
		return messages, nil
	}
	messageIDs := make([]int64, 0, len(messages))
	for i := range messages {
		messageIDs = append(messageIDs, messages[i].ID)
	}
	attachmentsByMessageID, err := s.repo.ListAttachmentsByMessageIDs(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for i := range messages {
		attachments := attachmentsByMessageID[messages[i].ID]
		for j := range attachments {
			dataURL, err := s.attachmentStorage.BuildDataURL(attachments[j].StoragePath, attachments[j].MimeType)
			if err != nil {
				return nil, err
			}
			attachments[j].DataURL = dataURL
		}
		messages[i].Attachments = attachments
	}
	return messages, nil
}

func normalizeConversationTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if utf8.RuneCountInString(title) <= maxChatConversationTitleRunes {
		return title
	}
	runes := []rune(title)
	return strings.TrimSpace(string(runes[:maxChatConversationTitleRunes]))
}

func autoConversationTitle(text string, hasAttachment bool) string {
	text = strings.TrimSpace(text)
	if text == "" {
		if hasAttachment {
			return "图片对话"
		}
		return defaultChatConversationTitle
	}
	return normalizeConversationTitle(text)
}

func cloneInt64Slice(src []int64) []int64 {
	if len(src) == 0 {
		return nil
	}
	out := make([]int64, len(src))
	copy(out, src)
	return out
}

func ParseAssistantTextFromResponsesSSE(raw string) (text string, errMessage string, done bool) {
	lines := strings.Split(raw, "\n")
	var builder strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		switch gjson.Get(payload, "type").String() {
		case "response.output_text.delta":
			builder.WriteString(gjson.Get(payload, "delta").String())
		case "response.completed", "response.done":
			done = true
			if builder.Len() == 0 {
				builder.WriteString(extractCompletedResponseText(payload))
			}
		case "error", "response.failed", "response.incomplete":
			errMessage = strings.TrimSpace(
				firstNonEmptyString(
					gjson.Get(payload, "error.message").String(),
					gjson.Get(payload, "response.error.message").String(),
					gjson.Get(payload, "message").String(),
				),
			)
		}
	}
	return strings.TrimSpace(builder.String()), errMessage, done
}

func extractCompletedResponseText(payload string) string {
	output := gjson.Get(payload, "response.output")
	if !output.Exists() || !output.IsArray() {
		return ""
	}
	var parts []string
	for _, item := range output.Array() {
		if item.Get("role").String() != ChatRoleAssistant {
			continue
		}
		content := item.Get("content")
		if !content.Exists() || !content.IsArray() {
			continue
		}
		for _, block := range content.Array() {
			if block.Get("type").String() == "output_text" {
				if text := strings.TrimSpace(block.Get("text").String()); text != "" {
					parts = append(parts, text)
				}
			}
		}
	}
	return strings.Join(parts, "\n\n")
}
