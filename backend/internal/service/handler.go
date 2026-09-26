package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"smalltalk/internal/apperr"
	"smalltalk/internal/auth"
	"smalltalk/internal/platform/httpx"
)

type Handler struct {
	svc *Service
	key string
}

func NewHandler(svc *Service, key string) *Handler {
	return &Handler{svc: svc, key: key}
}

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/service/v1/rooms/:ref", RequireKey(h.key))
	g.PUT("", h.ensureRoom)
	g.POST("/sessions", h.session)
	g.POST("/close", h.close)
}

// RequireKey checks the shared service key in constant time. An empty key
// turns the API off: every route answers 404 as if it did not exist.
func RequireKey(key string) gin.HandlerFunc {
	hash := auth.HashToken(key)
	return func(c *gin.Context) {
		if key == "" {
			httpx.Error(c, apperr.NotFound)
			return
		}
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || !auth.TokenMatches(token, hash) {
			httpx.Error(c, apperr.Unauthorized)
			return
		}
		c.Next()
	}
}

func (h *Handler) ensureRoom(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperr.Invalid("invalid JSON body"))
		return
	}
	st, err := h.svc.EnsureRoom(c.Request.Context(), c.Param("ref"), req.Name)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *Handler) session(c *gin.Context) {
	var req struct {
		ExternalUserID string `json:"external_user_id"`
		DisplayName    string `json:"display_name"`
		Label          string `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperr.Invalid("invalid JSON body"))
		return
	}
	sess, err := h.svc.Session(c.Request.Context(), c.Param("ref"), req.ExternalUserID, req.DisplayName, req.Label)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *Handler) close(c *gin.Context) {
	if err := h.svc.Close(c.Request.Context(), c.Param("ref")); err != nil {
		httpx.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
