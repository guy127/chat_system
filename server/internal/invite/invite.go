// Package invite issues and revokes QR invites and exchanges an invite token
// for a room membership.
package invite

import (
	"time"

	"github.com/google/uuid"

	"qrchat/internal/apperr"
)

const DefaultTTL = 24 * time.Hour

type Invite struct {
	ID        uuid.UUID
	RoomID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	MaxUses   *int
	UsedCount int
	RevokedAt *time.Time
	CreatedAt time.Time
}

// Check reports why an invite cannot be used right now, or nil if it can.
func Check(inv Invite, now time.Time) error {
	switch {
	case inv.RevokedAt != nil:
		return apperr.InviteInvalid
	case !now.Before(inv.ExpiresAt):
		return apperr.InviteExpired
	case inv.MaxUses != nil && inv.UsedCount >= *inv.MaxUses:
		return apperr.InviteExhausted
	}
	return nil
}
