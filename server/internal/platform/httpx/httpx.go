// Package httpx holds HTTP helpers shared by every handler: the error
// envelope, request IDs and access logging.
package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"smalltalk/internal/apperr"
)

var statusByCode = map[string]int{
	apperr.NotFound.Code:        http.StatusNotFound,
	apperr.Unauthorized.Code:    http.StatusUnauthorized,
	apperr.Forbidden.Code:       http.StatusForbidden,
	apperr.InviteInvalid.Code:   http.StatusNotFound,
	apperr.InviteExpired.Code:   http.StatusGone,
	apperr.InviteExhausted.Code: http.StatusGone,
	apperr.RoomClosed.Code:      http.StatusGone,
	apperr.RoomFull.Code:        http.StatusConflict,
	apperr.RateLimited.Code:     http.StatusTooManyRequests,
	apperr.Banned.Code:          http.StatusForbidden,
	apperr.Kicked.Code:          http.StatusForbidden,
	apperr.MessageTooLong.Code:  http.StatusBadRequest,
	apperr.ImageTooLarge.Code:   http.StatusRequestEntityTooLarge,
	apperr.ImageInvalid.Code:    http.StatusUnsupportedMediaType,
	"invalid_input":             http.StatusBadRequest,
}

// Error writes the standard {"error": {"code", "message"}} envelope and aborts.
func Error(c *gin.Context, err error) {
	if e, ok := apperr.As(err); ok {
		status, known := statusByCode[e.Code]
		if !known {
			status = http.StatusBadRequest
		}
		c.AbortWithStatusJSON(status, errorBody(e.Code, e.Message))
		return
	}
	slog.ErrorContext(c.Request.Context(), "request failed",
		"request_id", RequestID(c), "path", c.FullPath(), "err", err)
	c.AbortWithStatusJSON(http.StatusInternalServerError, errorBody("internal", "internal server error"))
}

func errorBody(code, msg string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": msg}}
}

const requestIDKey = "request_id"

func RequestID(c *gin.Context) string { return c.GetString(requestIDKey) }

// RequestLog assigns a request ID and logs one structured line per request.
// It never logs bodies, query strings or headers, so tokens stay out of logs.
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := uuid.NewString()
		c.Set(requestIDKey, id)
		c.Header("X-Request-Id", id)
		start := time.Now()
		c.Next()
		slog.InfoContext(c.Request.Context(), "http",
			"request_id", id,
			"method", c.Request.Method,
			"route", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"room_id", c.Param("id"),
			"member_id", c.GetString("member_id"),
		)
	}
}
