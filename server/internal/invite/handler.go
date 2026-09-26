package invite

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"qrchat/internal/apperr"
	"qrchat/internal/platform/httpx"
	"qrchat/internal/room"
)

type Handler struct {
	svc         *Service
	rooms       *room.Handler
	joinLimiter *httpx.KeyedLimiter
}

func NewHandler(svc *Service, rooms *room.Handler, joinLimiter *httpx.KeyedLimiter) *Handler {
	return &Handler{svc: svc, rooms: rooms, joinLimiter: joinLimiter}
}

func (h *Handler) Register(r gin.IRouter) {
	r.POST("/join", h.join)

	owner := r.Group("/rooms/:id/invites", h.rooms.RequireMember(), room.RequireOwner())
	owner.POST("", h.create)
	owner.GET("", h.list)
	owner.DELETE("/:inviteId", h.revoke)
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		ExpiresInMinutes int  `json:"expires_in_minutes"`
		MaxUses          *int `json:"max_uses"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Error(c, apperr.Invalid("invalid JSON body"))
			return
		}
	}
	ttl := time.Duration(req.ExpiresInMinutes) * time.Minute
	created, err := h.svc.Create(c.Request.Context(), room.MemberFrom(c).RoomID, ttl, req.MaxUses)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *Handler) list(c *gin.Context) {
	invites, err := h.svc.List(c.Request.Context(), room.MemberFrom(c).RoomID)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"invites": invites})
}

func (h *Handler) revoke(c *gin.Context) {
	inviteID, err := uuid.Parse(c.Param("inviteId"))
	if err != nil {
		httpx.Error(c, apperr.NotFound)
		return
	}
	if err := h.svc.Revoke(c.Request.Context(), room.MemberFrom(c).RoomID, inviteID); err != nil {
		httpx.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) join(c *gin.Context) {
	if !h.joinLimiter.Allow(c.ClientIP()) {
		httpx.Error(c, apperr.RateLimited)
		return
	}
	var req struct {
		Token       string `json:"token"`
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperr.Invalid("invalid JSON body"))
		return
	}
	joined, err := h.svc.Join(c.Request.Context(), req.Token, req.DisplayName)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, joined)
}
