package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterChatRoutes registers authenticated chat routes for the user web client.
func RegisterChatRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	settingService *service.SettingService,
) {
	if h.Chat == nil {
		return
	}

	authenticated := v1.Group("/chat")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	{
		authenticated.GET("/api-keys", h.Chat.ListAPIKeys)
		authenticated.GET("/models", h.Chat.ListModels)
		authenticated.GET("/conversations", h.Chat.ListConversations)
		authenticated.POST("/conversations", h.Chat.CreateConversation)
		authenticated.PATCH("/conversations/:id", h.Chat.UpdateConversation)
		authenticated.DELETE("/conversations/:id", h.Chat.DeleteConversation)
		authenticated.GET("/conversations/:id/messages", h.Chat.ListMessages)
		authenticated.POST("/conversations/:id/messages/stream", h.Chat.StreamMessage)
		authenticated.POST("/conversations/:id/attachments", h.Chat.UploadAttachment)
		authenticated.DELETE("/attachments/:attachmentID", h.Chat.DeleteAttachment)
	}
}
