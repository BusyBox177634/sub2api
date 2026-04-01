package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type chatRepository struct {
	sql *sql.DB
}

func NewChatRepository(sqlDB *sql.DB) service.ChatRepository {
	return &chatRepository{sql: sqlDB}
}

func (r *chatRepository) CreateConversation(ctx context.Context, conversation *service.ChatConversation) error {
	if conversation == nil {
		return nil
	}
	var lastMessageAt sql.NullTime
	err := r.sql.QueryRowContext(ctx, `
		INSERT INTO chat_conversations (
			user_id, api_key_id, title, model, last_message_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING id, created_at, updated_at, last_message_at
	`,
		conversation.UserID,
		conversation.APIKeyID,
		conversation.Title,
		conversation.Model,
		conversation.LastMessageAt,
	).Scan(&conversation.ID, &conversation.CreatedAt, &conversation.UpdatedAt, &lastMessageAt)
	if err != nil {
		return err
	}
	if lastMessageAt.Valid {
		conversation.LastMessageAt = &lastMessageAt.Time
	}
	return nil
}

func (r *chatRepository) GetConversationByID(ctx context.Context, id int64) (*service.ChatConversation, error) {
	conversation := &service.ChatConversation{}
	var lastMessageAt sql.NullTime
	err := r.sql.QueryRowContext(ctx, `
		SELECT id, user_id, api_key_id, title, model, last_message_at, created_at, updated_at
		FROM chat_conversations
		WHERE id = $1
	`, id).Scan(
		&conversation.ID,
		&conversation.UserID,
		&conversation.APIKeyID,
		&conversation.Title,
		&conversation.Model,
		&lastMessageAt,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrChatConversationNotFound
		}
		return nil, err
	}
	if lastMessageAt.Valid {
		conversation.LastMessageAt = &lastMessageAt.Time
	}
	return conversation, nil
}

func (r *chatRepository) ListConversationsByUser(ctx context.Context, userID int64) ([]service.ChatConversation, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, user_id, api_key_id, title, model, last_message_at, created_at, updated_at
		FROM chat_conversations
		WHERE user_id = $1
		ORDER BY updated_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ChatConversation, 0)
	for rows.Next() {
		var conversation service.ChatConversation
		var lastMessageAt sql.NullTime
		if err := rows.Scan(
			&conversation.ID,
			&conversation.UserID,
			&conversation.APIKeyID,
			&conversation.Title,
			&conversation.Model,
			&lastMessageAt,
			&conversation.CreatedAt,
			&conversation.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if lastMessageAt.Valid {
			conversation.LastMessageAt = &lastMessageAt.Time
		}
		out = append(out, conversation)
	}
	return out, rows.Err()
}

func (r *chatRepository) UpdateConversation(ctx context.Context, conversation *service.ChatConversation) error {
	if conversation == nil {
		return nil
	}
	var lastMessageAt sql.NullTime
	err := r.sql.QueryRowContext(ctx, `
		UPDATE chat_conversations
		SET api_key_id = $2,
			title = $3,
			model = $4,
			last_message_at = $5,
			updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at, last_message_at
	`,
		conversation.ID,
		conversation.APIKeyID,
		conversation.Title,
		conversation.Model,
		conversation.LastMessageAt,
	).Scan(&conversation.UpdatedAt, &lastMessageAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrChatConversationNotFound
		}
		return err
	}
	if lastMessageAt.Valid {
		conversation.LastMessageAt = &lastMessageAt.Time
	} else {
		conversation.LastMessageAt = nil
	}
	return nil
}

func (r *chatRepository) DeleteConversation(ctx context.Context, id int64) error {
	result, err := r.sql.ExecContext(ctx, `DELETE FROM chat_conversations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return service.ErrChatConversationNotFound
	}
	return nil
}

func (r *chatRepository) CreateMessage(ctx context.Context, message *service.ChatMessage) error {
	if message == nil {
		return nil
	}
	attachmentIDsJSON, _ := json.Marshal(message.AttachmentIDs)
	err := r.sql.QueryRowContext(ctx, `
		INSERT INTO chat_messages (
			conversation_id, user_id, role, status, text, model, attachment_ids, error_message, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`,
		message.ConversationID,
		message.UserID,
		message.Role,
		message.Status,
		message.Text,
		message.Model,
		attachmentIDsJSON,
		message.ErrorMessage,
	).Scan(&message.ID, &message.CreatedAt, &message.UpdatedAt)
	return err
}

func (r *chatRepository) GetMessageByID(ctx context.Context, id int64) (*service.ChatMessage, error) {
	message := &service.ChatMessage{}
	var attachmentIDsJSON []byte
	err := r.sql.QueryRowContext(ctx, `
		SELECT id, conversation_id, user_id, role, status, text, model, attachment_ids, error_message, created_at, updated_at
		FROM chat_messages
		WHERE id = $1
	`, id).Scan(
		&message.ID,
		&message.ConversationID,
		&message.UserID,
		&message.Role,
		&message.Status,
		&message.Text,
		&message.Model,
		&attachmentIDsJSON,
		&message.ErrorMessage,
		&message.CreatedAt,
		&message.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrChatMessageNotFound
		}
		return nil, err
	}
	if len(attachmentIDsJSON) > 0 {
		_ = json.Unmarshal(attachmentIDsJSON, &message.AttachmentIDs)
	}
	return message, nil
}

func (r *chatRepository) ListMessagesByConversation(ctx context.Context, conversationID int64) ([]service.ChatMessage, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, conversation_id, user_id, role, status, text, model, attachment_ids, error_message, created_at, updated_at
		FROM chat_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC, id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ChatMessage, 0)
	for rows.Next() {
		var message service.ChatMessage
		var attachmentIDsJSON []byte
		if err := rows.Scan(
			&message.ID,
			&message.ConversationID,
			&message.UserID,
			&message.Role,
			&message.Status,
			&message.Text,
			&message.Model,
			&attachmentIDsJSON,
			&message.ErrorMessage,
			&message.CreatedAt,
			&message.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(attachmentIDsJSON) > 0 {
			_ = json.Unmarshal(attachmentIDsJSON, &message.AttachmentIDs)
		}
		out = append(out, message)
	}
	return out, rows.Err()
}

func (r *chatRepository) UpdateMessage(ctx context.Context, message *service.ChatMessage) error {
	if message == nil {
		return nil
	}
	attachmentIDsJSON, _ := json.Marshal(message.AttachmentIDs)
	err := r.sql.QueryRowContext(ctx, `
		UPDATE chat_messages
		SET status = $2,
			text = $3,
			model = $4,
			attachment_ids = $5,
			error_message = $6,
			updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`,
		message.ID,
		message.Status,
		message.Text,
		message.Model,
		attachmentIDsJSON,
		message.ErrorMessage,
	).Scan(&message.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrChatMessageNotFound
		}
		return err
	}
	return nil
}

func (r *chatRepository) CreateAttachment(ctx context.Context, attachment *service.ChatAttachment) error {
	if attachment == nil {
		return nil
	}
	err := r.sql.QueryRowContext(ctx, `
		INSERT INTO chat_attachments (
			conversation_id, message_id, user_id, kind, mime_type, original_name, size_bytes,
			storage_type, storage_path, sha256, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`,
		attachment.ConversationID,
		attachment.MessageID,
		attachment.UserID,
		attachment.Kind,
		attachment.MimeType,
		attachment.OriginalName,
		attachment.SizeBytes,
		attachment.StorageType,
		attachment.StoragePath,
		attachment.SHA256,
	).Scan(&attachment.ID, &attachment.CreatedAt, &attachment.UpdatedAt)
	return err
}

func (r *chatRepository) GetAttachmentByID(ctx context.Context, id int64) (*service.ChatAttachment, error) {
	attachment := &service.ChatAttachment{}
	var messageID sql.NullInt64
	err := r.sql.QueryRowContext(ctx, `
		SELECT id, conversation_id, message_id, user_id, kind, mime_type, original_name, size_bytes,
			storage_type, storage_path, sha256, created_at, updated_at
		FROM chat_attachments
		WHERE id = $1
	`, id).Scan(
		&attachment.ID,
		&attachment.ConversationID,
		&messageID,
		&attachment.UserID,
		&attachment.Kind,
		&attachment.MimeType,
		&attachment.OriginalName,
		&attachment.SizeBytes,
		&attachment.StorageType,
		&attachment.StoragePath,
		&attachment.SHA256,
		&attachment.CreatedAt,
		&attachment.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrChatAttachmentNotFound
		}
		return nil, err
	}
	if messageID.Valid {
		value := messageID.Int64
		attachment.MessageID = &value
	}
	return attachment, nil
}

func (r *chatRepository) ListAttachmentsByConversation(ctx context.Context, conversationID int64) ([]service.ChatAttachment, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, conversation_id, message_id, user_id, kind, mime_type, original_name, size_bytes,
			storage_type, storage_path, sha256, created_at, updated_at
		FROM chat_attachments
		WHERE conversation_id = $1
		ORDER BY created_at ASC, id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.ChatAttachment, 0)
	for rows.Next() {
		attachment, err := scanChatAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *attachment)
	}
	return out, rows.Err()
}

func (r *chatRepository) ListAttachmentsByMessageIDs(ctx context.Context, messageIDs []int64) (map[int64][]service.ChatAttachment, error) {
	out := make(map[int64][]service.ChatAttachment)
	if len(messageIDs) == 0 {
		return out, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, conversation_id, message_id, user_id, kind, mime_type, original_name, size_bytes,
			storage_type, storage_path, sha256, created_at, updated_at
		FROM chat_attachments
		WHERE message_id = ANY($1)
		ORDER BY created_at ASC, id ASC
	`, pq.Array(messageIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		attachment, err := scanChatAttachment(rows)
		if err != nil {
			return nil, err
		}
		if attachment.MessageID == nil {
			continue
		}
		messageID := *attachment.MessageID
		out[messageID] = append(out[messageID], *attachment)
	}
	return out, rows.Err()
}

func (r *chatRepository) UpdateAttachmentMessageID(ctx context.Context, attachmentIDs []int64, messageID int64) error {
	if len(attachmentIDs) == 0 {
		return nil
	}
	_, err := r.sql.ExecContext(ctx, `
		UPDATE chat_attachments
		SET message_id = $1,
			updated_at = NOW()
		WHERE id = ANY($2)
	`, messageID, pq.Array(attachmentIDs))
	return err
}

func (r *chatRepository) DeleteAttachment(ctx context.Context, id int64) error {
	result, err := r.sql.ExecContext(ctx, `DELETE FROM chat_attachments WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return service.ErrChatAttachmentNotFound
	}
	return nil
}

func scanChatAttachment(scanner interface {
	Scan(dest ...any) error
}) (*service.ChatAttachment, error) {
	attachment := &service.ChatAttachment{}
	var messageID sql.NullInt64
	if err := scanner.Scan(
		&attachment.ID,
		&attachment.ConversationID,
		&messageID,
		&attachment.UserID,
		&attachment.Kind,
		&attachment.MimeType,
		&attachment.OriginalName,
		&attachment.SizeBytes,
		&attachment.StorageType,
		&attachment.StoragePath,
		&attachment.SHA256,
		&attachment.CreatedAt,
		&attachment.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if messageID.Valid {
		value := messageID.Int64
		attachment.MessageID = &value
	}
	return attachment, nil
}
