package invite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"qrchat/internal/apperr"
	"qrchat/internal/auth"
	"qrchat/internal/room"
)

type Service struct {
	repo    *Repository
	rooms   *room.Service
	issuer  *auth.Issuer
	baseURL string
	now     func() time.Time
}

func NewService(repo *Repository, rooms *room.Service, issuer *auth.Issuer, baseURL string) *Service {
	return &Service{repo: repo, rooms: rooms, issuer: issuer, baseURL: baseURL, now: time.Now}
}

// Created is returned once; the raw token is never stored.
type Created struct {
	ID        uuid.UUID `json:"id"`
	Token     string    `json:"token"`
	InviteURL string    `json:"invite_url"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   *int      `json:"max_uses"`
}

const maxTTL = 30 * 24 * time.Hour

func (s *Service) Create(ctx context.Context, roomID uuid.UUID, ttl time.Duration, maxUses *int) (Created, error) {
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if ttl < time.Minute || ttl > maxTTL {
		return Created{}, apperr.Invalid("expires_in_minutes must be between 1 and 43200")
	}
	if maxUses != nil && (*maxUses < 1 || *maxUses > room.MaxMembers) {
		return Created{}, apperr.Invalid(fmt.Sprintf("max_uses must be between 1 and %d", room.MaxMembers))
	}
	rm, err := s.rooms.Get(ctx, roomID)
	if err != nil {
		return Created{}, fmt.Errorf("create invite: %w", err)
	}
	if rm.Status == room.StatusClosed {
		return Created{}, apperr.RoomClosed
	}
	token := auth.NewToken()
	inv := Invite{
		ID:        uuid.New(),
		RoomID:    roomID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: s.now().UTC().Add(ttl).Truncate(time.Second),
		MaxUses:   maxUses,
	}
	if err := s.repo.Create(ctx, inv); err != nil {
		return Created{}, fmt.Errorf("create invite: %w", err)
	}
	return Created{
		ID:        inv.ID,
		Token:     token,
		InviteURL: s.baseURL + "/j/" + token,
		ExpiresAt: inv.ExpiresAt,
		MaxUses:   maxUses,
	}, nil
}

type View struct {
	ID        uuid.UUID `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   *int      `json:"max_uses"`
	UsedCount int       `json:"used_count"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) List(ctx context.Context, roomID uuid.UUID) ([]View, error) {
	invites, err := s.repo.ListUsable(ctx, roomID, s.now())
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	out := make([]View, 0, len(invites))
	for _, inv := range invites {
		out = append(out, View{
			ID: inv.ID, ExpiresAt: inv.ExpiresAt.UTC(), MaxUses: inv.MaxUses,
			UsedCount: inv.UsedCount, CreatedAt: inv.CreatedAt.UTC(),
		})
	}
	return out, nil
}

func (s *Service) Revoke(ctx context.Context, roomID, inviteID uuid.UUID) error {
	if err := s.repo.Revoke(ctx, roomID, inviteID, s.now()); err != nil {
		return fmt.Errorf("revoke invite: %w", err)
	}
	return nil
}

type Joined struct {
	auth.Session
	RoomName string `json:"room_name"`
}

func (s *Service) Join(ctx context.Context, token, displayName string) (Joined, error) {
	if token == "" {
		return Joined{}, apperr.InviteInvalid
	}
	name, err := room.CleanName(displayName, 32, "display_name")
	if err != nil {
		return Joined{}, err
	}
	m := room.Member{ID: uuid.New(), DisplayName: name, Role: room.RoleMember}
	rm, err := s.repo.Join(ctx, auth.HashToken(token), s.now(), m)
	if err != nil {
		return Joined{}, fmt.Errorf("join room: %w", err)
	}
	sess, err := s.issuer.Issue(auth.Claims{RoomID: rm.ID, MemberID: m.ID, Role: room.RoleMember})
	if err != nil {
		return Joined{}, fmt.Errorf("join room: %w", err)
	}
	return Joined{Session: sess, RoomName: rm.Name}, nil
}
