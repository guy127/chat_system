// Package chat stores messages and delivers them over WebSocket.
package chat

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"smalltalk/internal/apperr"
)

const MaxBodyLen = 2000

type Message struct {
	ID          string    `json:"id"`
	RoomID      uuid.UUID `json:"-"`
	MemberID    uuid.UUID `json:"member_id"`
	DisplayName string    `json:"display_name"`
	ClientMsgID uuid.UUID `json:"client_msg_id"`
	Body        string    `json:"body"`
	Image       *ImageRef `json:"image"`
	CreatedAt   time.Time `json:"created_at"`
}

// ImageRef is what clients need to lay out an image before loading it from
// GET /rooms/{id}/images/{image_id}.
type ImageRef struct {
	ID          uuid.UUID `json:"id"`
	ContentType string    `json:"content_type"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
}

// ValidateBody enforces the message rules shared by every sender. The body
// may be empty only when the message carries an image.
func ValidateBody(body string, hasImage bool) error {
	if !utf8.ValidString(body) {
		return apperr.Invalid("message must be valid UTF-8")
	}
	if strings.TrimSpace(body) == "" && !hasImage {
		return apperr.Invalid("message is empty")
	}
	if utf8.RuneCountInString(body) > MaxBodyLen {
		return apperr.MessageTooLong
	}
	return nil
}
