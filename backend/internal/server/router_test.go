//go:build unit

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newRegisterRoutesTestRouter(chatHandler *handler.ChatHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(gin.Recovery())

	registerRoutes(
		router,
		&handler.Handlers{
			Chat:  chatHandler,
			Admin: &handler.AdminHandlers{},
		},
		middleware2.JWTAuthMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		middleware2.AdminAuthMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		middleware2.APIKeyAuthMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
		nil,
	)

	return router
}

func TestRegisterRoutesRegistersChatRoutesWhenHandlerPresent(t *testing.T) {
	router := newRegisterRoutesTestRouter(&handler.ChatHandler{})

	for _, path := range []string{
		"/api/v1/chat/api-keys",
		"/api/v1/chat/conversations",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should be registered", path)
	}
}

func TestRegisterRoutesLeavesChatRoutesUnregisteredWhenHandlerMissing(t *testing.T) {
	router := newRegisterRoutesTestRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/api-keys", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}
