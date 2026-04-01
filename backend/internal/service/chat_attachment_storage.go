package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultChatAttachmentRoot = "/app/data/chat-attachments"
	defaultChatAttachmentMax  = 10 << 20
)

type ChatAttachmentStorage struct {
	root         string
	maxBytes     int64
	supportedMIMEs map[string]struct{}
}

func NewChatAttachmentStorage() *ChatAttachmentStorage {
	return &ChatAttachmentStorage{
		root:     resolveChatAttachmentRoot(),
		maxBytes: defaultChatAttachmentMax,
		supportedMIMEs: map[string]struct{}{
			"image/png":  {},
			"image/jpeg": {},
			"image/jpg":  {},
			"image/gif":  {},
			"image/webp": {},
		},
	}
}

func resolveChatAttachmentRoot() string {
	root := strings.TrimSpace(os.Getenv("CHAT_ATTACHMENT_ROOT"))
	if root == "" {
		if dataDir := strings.TrimSpace(os.Getenv("DATA_DIR")); dataDir != "" {
			root = filepath.Join(dataDir, "chat-attachments")
		} else {
			root = defaultChatAttachmentRoot
		}
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		if absRoot, err := filepath.Abs(root); err == nil {
			root = absRoot
		}
	}
	return root
}

func (s *ChatAttachmentStorage) SaveImage(userID int64, originalName string, payload []byte, contentType string) (*ChatAttachment, error) {
	if s == nil {
		return nil, fmt.Errorf("chat attachment storage is nil")
	}
	if len(payload) == 0 {
		return nil, ErrChatAttachmentInvalid
	}
	if int64(len(payload)) > s.maxBytes {
		return nil, ErrChatAttachmentTooLarge
	}

	mimeType := strings.ToLower(strings.TrimSpace(contentType))
	if mimeType == "" {
		mimeType = strings.ToLower(http.DetectContentType(payload))
	}
	if semicolon := strings.IndexByte(mimeType, ';'); semicolon >= 0 {
		mimeType = strings.TrimSpace(mimeType[:semicolon])
	}
	if _, ok := s.supportedMIMEs[mimeType]; !ok {
		return nil, ErrChatAttachmentInvalid
	}

	now := time.Now().UTC()
	dir := filepath.Join(s.root, fmt.Sprintf("%d", userID), now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create chat attachment dir: %w", err)
	}

	filename := uuid.NewString() + detectImageExtension(mimeType, originalName)
	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, payload, 0o644); err != nil {
		return nil, fmt.Errorf("write chat attachment: %w", err)
	}

	sum := sha256.Sum256(payload)
	return &ChatAttachment{
		UserID:       userID,
		Kind:         ChatAttachmentKindImage,
		MimeType:     mimeType,
		OriginalName: safeAttachmentName(originalName),
		SizeBytes:    int64(len(payload)),
		StorageType:  ChatAttachmentStorageTypeLocal,
		StoragePath:  fullPath,
		SHA256:       hex.EncodeToString(sum[:]),
		DataURL:      buildDataURL(mimeType, payload),
	}, nil
}

func (s *ChatAttachmentStorage) DeleteFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *ChatAttachmentStorage) BuildDataURL(path string, mimeType string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(payload)
	}
	return buildDataURL(mimeType, payload), nil
}

func buildDataURL(mimeType string, payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(payload)
}

func detectImageExtension(mimeType, originalName string) string {
	if ext := strings.TrimSpace(filepath.Ext(originalName)); ext != "" {
		return strings.ToLower(ext)
	}
	if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	switch mimeType {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func safeAttachmentName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "image"
	}
	return name
}
