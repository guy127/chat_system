package room

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"qrchat/internal/apperr"
	"qrchat/internal/auth"
	"qrchat/internal/platform/httpx"
)

type Handler struct {
	svc    *Service
	issuer *auth.Issuer
}

func NewHandler(svc *Service, issuer *auth.Issuer) *Handler {
	return &Handler{svc: svc, issuer: issuer}
}

func (h *Handler) Register(r gin.IRouter) {
	r.POST("/rooms", h.create)
	r.POST("/rooms/:id/owner-session", h.ownerSession)

	member := r.Group("/rooms/:id", h.RequireMember())
	member.GET("", h.get)
	member.GET("/members", h.members)

	owner := r.Group("/rooms/:id", h.RequireMember(), RequireOwner())
	owner.POST("/members/:mid/kick", h.kick)
	owner.POST("/close", h.close)
}

const memberKey = "room_member"

// RequireMember authenticates the Bearer JWT, checks it belongs to the room in
// the path and that the member is still active.
func (h *Handler) RequireMember() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			httpx.Error(c, apperr.NotFound)
			return
		}
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok {
			httpx.Error(c, apperr.Unauthorized)
			return
		}
		claims, err := h.issuer.Parse(token)
		if err != nil || claims.RoomID != roomID {
			httpx.Error(c, apperr.Unauthorized)
			return
		}
		m, err := h.svc.ActiveMember(c.Request.Context(), roomID, claims.MemberID)
		if err != nil {
			httpx.Error(c, err)
			return
		}
		c.Set(memberKey, m)
		c.Set("member_id", m.ID.String())
		c.Next()
	}
}

// RequireOwner must run after RequireMember.
func RequireOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		if MemberFrom(c).Role != RoleOwner {
			httpx.Error(c, apperr.Forbidden)
			return
		}
		c.Next()
	}
}

// MemberFrom returns the member authenticated by RequireMember.
func MemberFrom(c *gin.Context) Member {
	m, _ := c.Get(memberKey)
	return m.(Member)
}

type roomView struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func toView(rm Room) roomView {
	return roomView{ID: rm.ID, Name: rm.Name, Status: rm.Status, CreatedAt: rm.CreatedAt.UTC()}
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperr.Invalid("invalid JSON body"))
		return
	}
	created, err := h.svc.Create(c.Request.Context(), req.Name, req.DisplayName)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *Handler) ownerSession(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, apperr.NotFound)
		return
	}
	var req struct {
		OwnerToken string `json:"owner_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.OwnerToken == "" {
		httpx.Error(c, apperr.Invalid("owner_token is required"))
		return
	}
	sess, err := h.svc.OwnerSession(c.Request.Context(), roomID, req.OwnerToken)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *Handler) get(c *gin.Context) {
	rm, err := h.svc.Get(c.Request.Context(), MemberFrom(c).RoomID)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, toView(rm))
}

func (h *Handler) members(c *gin.Context) {
	members, err := h.svc.Members(c.Request.Context(), MemberFrom(c).RoomID)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members})
}

func (h *Handler) kick(c *gin.Context) {
	memberID, err := uuid.Parse(c.Param("mid"))
	if err != nil {
		httpx.Error(c, apperr.NotFound)
		return
	}
	var req struct {
		Ban bool `json:"ban"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Error(c, apperr.Invalid("invalid JSON body"))
			return
		}
	}
	if err := h.svc.Remove(c.Request.Context(), MemberFrom(c).RoomID, memberID, req.Ban); err != nil {
		httpx.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) close(c *gin.Context) {
	if err := h.svc.Close(c.Request.Context(), MemberFrom(c).RoomID); err != nil {
		httpx.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
