package invite

import (
	"errors"
	"testing"
	"time"

	"qrchat/internal/apperr"
)

func TestCheck(t *testing.T) {
	now := time.Date(2026, 9, 26, 6, 0, 0, 0, time.UTC)
	intp := func(n int) *int { return &n }
	revoked := now.Add(-time.Minute)

	tests := []struct {
		name string
		inv  Invite
		want error
	}{
		{"valid unlimited", Invite{ExpiresAt: now.Add(time.Hour)}, nil},
		{"valid with uses left", Invite{ExpiresAt: now.Add(time.Hour), MaxUses: intp(3), UsedCount: 2}, nil},
		{"expired", Invite{ExpiresAt: now.Add(-time.Second)}, apperr.InviteExpired},
		{"expires exactly now", Invite{ExpiresAt: now}, apperr.InviteExpired},
		{"exhausted", Invite{ExpiresAt: now.Add(time.Hour), MaxUses: intp(3), UsedCount: 3}, apperr.InviteExhausted},
		{"revoked", Invite{ExpiresAt: now.Add(time.Hour), RevokedAt: &revoked}, apperr.InviteInvalid},
		{"revoked wins over expired", Invite{ExpiresAt: now.Add(-time.Hour), RevokedAt: &revoked}, apperr.InviteInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Check(tt.inv, now); !errors.Is(got, tt.want) {
				t.Fatalf("Check() = %v, want %v", got, tt.want)
			}
		})
	}
}
