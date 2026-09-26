// Package apperr defines domain errors that carry a stable, client-facing code.
// Services wrap these with fmt.Errorf("...: %w", err); only the handler layer
// turns them into HTTP statuses or WebSocket error frames.
package apperr

import "errors"

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

var (
	NotFound        = &Error{"not_found", "not found"}
	Unauthorized    = &Error{"unauthorized", "missing or invalid credentials"}
	Forbidden       = &Error{"forbidden", "not allowed"}
	InviteInvalid   = &Error{"invite_invalid", "invite is invalid or was revoked"}
	InviteExpired   = &Error{"invite_expired", "invite has expired"}
	InviteExhausted = &Error{"invite_exhausted", "invite has reached its usage limit"}
	RoomClosed      = &Error{"room_closed", "room is closed"}
	RoomFull        = &Error{"room_full", "room is full"}
	RateLimited     = &Error{"rate_limited", "too many requests"}
	Banned          = &Error{"banned", "you are banned from this room"}
	Kicked          = &Error{"kicked", "you were removed from this room"}
	MessageTooLong  = &Error{"message_too_long", "message exceeds 2000 characters"}
)

// Invalid reports a malformed request with a specific explanation.
func Invalid(msg string) *Error { return &Error{"invalid_input", msg} }

// As extracts the domain error from err, if any.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
