package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ChatRoleUser      = "user"
	ChatRoleAssistant = "assistant"
)

const (
	ChatMessageStatusPending   = "pending"
	ChatMessageStatusStreaming = "streaming"
	ChatMessageStatusCompleted = "completed"
	ChatMessageStatusFailed    = "failed"
	ChatMessageStatusStopped   = "stopped"
)

const (
	ChatAttachmentKindImage = "image"
)

const (
	ChatAttachmentStorageTypeLocal = "local"
)

var (
	ErrChatConversationNotFound = infraerrors.NotFound("CHAT_CONVERSATION_NOT_FOUND", "chat conversation not found")
	ErrChatMessageNotFound      = infraerrors.NotFound("CHAT_MESSAGE_NOT_FOUND", "chat message not found")
	ErrChatAttachmentNotFound   = infraerrors.NotFound("CHAT_ATTACHMENT_NOT_FOUND", "chat attachment not found")
	ErrChatEmptyInput           = infraerrors.BadRequest("CHAT_EMPTY_INPUT", "message text or attachment is required")
	ErrChatUnsupportedPlatform  = infraerrors.BadRequest("CHAT_UNSUPPORTED_PLATFORM", "selected api key platform is not supported for web chat")
	ErrChatAPIKeyUnavailable    = infraerrors.Forbidden("CHAT_APIKEY_UNAVAILABLE", "selected api key is unavailable for web chat")
	ErrChatAttachmentTooLarge   = infraerrors.BadRequest("CHAT_ATTACHMENT_TOO_LARGE", "attachment exceeds the upload limit")
	ErrChatAttachmentInvalid    = infraerrors.BadRequest("CHAT_ATTACHMENT_INVALID", "attachment must be an image")
	ErrChatAttachmentAssigned   = infraerrors.Forbidden("CHAT_ATTACHMENT_ASSIGNED", "attachment is already bound to a sent message")
)

type ChatConversation struct {
	ID            int64      `json:"id"`
	UserID        int64      `json:"user_id"`
	APIKeyID      int64      `json:"api_key_id"`
	Title         string     `json:"title"`
	Model         string     `json:"model"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type ChatMessage struct {
	ID             int64            `json:"id"`
	ConversationID int64            `json:"conversation_id"`
	UserID         int64            `json:"user_id"`
	Role           string           `json:"role"`
	Status         string           `json:"status"`
	Text           string           `json:"text"`
	Model          string           `json:"model"`
	AttachmentIDs  []int64          `json:"attachment_ids"`
	ErrorMessage   string           `json:"error_message,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	Attachments    []ChatAttachment `json:"attachments,omitempty"`
}

type ChatAttachment struct {
	ID             int64      `json:"id"`
	ConversationID int64      `json:"conversation_id"`
	MessageID      *int64     `json:"message_id,omitempty"`
	UserID         int64      `json:"user_id"`
	Kind           string     `json:"kind"`
	MimeType       string     `json:"mime_type"`
	OriginalName   string     `json:"original_name"`
	SizeBytes      int64      `json:"size_bytes"`
	StorageType    string     `json:"storage_type"`
	StoragePath    string     `json:"storage_path"`
	SHA256         string     `json:"sha256"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DataURL        string     `json:"data_url,omitempty"`
}

type ChatPreparedTurn struct {
	Conversation    *ChatConversation
	APIKey          *APIKey
	Subscription    *UserSubscription
	UserMessage     *ChatMessage
	AssistantMessage *ChatMessage
	RequestBody     []byte
}

type ChatRepository interface {
	CreateConversation(ctx context.Context, conversation *ChatConversation) error
	GetConversationByID(ctx context.Context, id int64) (*ChatConversation, error)
	ListConversationsByUser(ctx context.Context, userID int64) ([]ChatConversation, error)
	UpdateConversation(ctx context.Context, conversation *ChatConversation) error
	DeleteConversation(ctx context.Context, id int64) error

	CreateMessage(ctx context.Context, message *ChatMessage) error
	GetMessageByID(ctx context.Context, id int64) (*ChatMessage, error)
	ListMessagesByConversation(ctx context.Context, conversationID int64) ([]ChatMessage, error)
	UpdateMessage(ctx context.Context, message *ChatMessage) error

	CreateAttachment(ctx context.Context, attachment *ChatAttachment) error
	GetAttachmentByID(ctx context.Context, id int64) (*ChatAttachment, error)
	ListAttachmentsByConversation(ctx context.Context, conversationID int64) ([]ChatAttachment, error)
	ListAttachmentsByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64][]ChatAttachment, error)
	UpdateAttachmentMessageID(ctx context.Context, attachmentIDs []int64, messageID int64) error
	DeleteAttachment(ctx context.Context, id int64) error
}
