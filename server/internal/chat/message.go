// Package chat stores messages and delivers them over WebSocket.
package chat

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"qrchat/internal/apperr"
)

const MaxBodyLen = 2000

type Message struct {
	ID          string    `json:"id"`
	RoomID      uuid.UUID `json:"-"`
	MemberID    uuid.UUID `json:"member_id"`
	DisplayName string    `json:"display_name"`
	ClientMsgID uuid.UUID `json:"client_msg_id"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
}

// ValidateBody enforces the message rules shared by every sender.
func ValidateBody(body string) error {
	if !utf8.ValidString(body) {
		return apperr.Invalid("message must be valid UTF-8")
	}
	if strings.TrimSpace(body) == "" {
		return apperr.Invalid("message is empty")
	}
	if utf8.RuneCountInString(body) > MaxBodyLen {
		return apperr.MessageTooLong
	}
	return nil
}
