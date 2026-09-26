// Package room owns rooms and their members: creating rooms, host sessions,
// membership checks, kicking/banning and closing.
package room

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"qrchat/internal/apperr"
	"qrchat/internal/auth"
)

// Notifier pushes moderation events to live connections (implemented by chat.Hub).
type Notifier interface {
	Kick(ctx context.Context, roomID, memberID uuid.UUID, reason string) error
	CloseRoom(ctx context.Context, roomID uuid.UUID) error
}

// Presence reports which members currently hold a live connection.
type Presence interface {
	Online(roomID uuid.UUID) map[uuid.UUID]bool
}

type Service struct {
	repo     *Repository
	issuer   *auth.Issuer
	notifier Notifier
	presence Presence
}

func NewService(repo *Repository, issuer *auth.Issuer, notifier Notifier, presence Presence) *Service {
	return &Service{repo: repo, issuer: issuer, notifier: notifier, presence: presence}
}

type Created struct {
	auth.Session
	OwnerToken string `json:"owner_token"`
}

func (s *Service) Create(ctx context.Context, name, hostName string) (Created, error) {
	name, err := CleanName(name, 80, "room name")
	if err != nil {
		return Created{}, err
	}
	hostName, err = CleanName(hostName, 32, "display_name")
	if err != nil {
		return Created{}, err
	}
	ownerToken := auth.NewToken()
	rm := Room{ID: uuid.New(), Name: name, OwnerTokenHash: auth.HashToken(ownerToken)}
	owner := Member{ID: uuid.New(), RoomID: rm.ID, DisplayName: hostName, Role: RoleOwner}
	if err := s.repo.CreateWithOwner(ctx, rm, owner); err != nil {
		return Created{}, fmt.Errorf("create room: %w", err)
	}
	sess, err := s.issuer.Issue(auth.Claims{RoomID: rm.ID, MemberID: owner.ID, Role: RoleOwner})
	if err != nil {
		return Created{}, fmt.Errorf("create room: %w", err)
	}
	return Created{Session: sess, OwnerToken: ownerToken}, nil
}

// OwnerSession lets a host come back with the owner_token after their JWT expires.
func (s *Service) OwnerSession(ctx context.Context, roomID uuid.UUID, ownerToken string) (auth.Session, error) {
	rm, err := s.repo.Get(ctx, roomID)
	if err != nil {
		return auth.Session{}, fmt.Errorf("owner session: %w", err)
	}
	if !auth.TokenMatches(ownerToken, rm.OwnerTokenHash) {
		return auth.Session{}, apperr.Unauthorized
	}
	owner, err := s.repo.OwnerMember(ctx, roomID)
	if err != nil {
		return auth.Session{}, fmt.Errorf("owner session: %w", err)
	}
	return s.issuer.Issue(auth.Claims{RoomID: roomID, MemberID: owner.ID, Role: RoleOwner})
}

func (s *Service) Get(ctx context.Context, roomID uuid.UUID) (Room, error) {
	return s.repo.Get(ctx, roomID)
}

// ActiveMember loads a member and fails if they were kicked or banned.
// It is called on every request and every sent message, not only at join.
func (s *Service) ActiveMember(ctx context.Context, roomID, memberID uuid.UUID) (Member, error) {
	m, err := s.repo.GetMember(ctx, roomID, memberID)
	if err != nil {
		return Member{}, fmt.Errorf("active member: %w", err)
	}
	switch {
	case m.BannedAt != nil:
		return Member{}, apperr.Banned
	case m.KickedAt != nil:
		return Member{}, apperr.Kicked
	}
	return m, nil
}

// CanPost checks the member is active and the room is still open.
func (s *Service) CanPost(ctx context.Context, roomID, memberID uuid.UUID) (Member, error) {
	m, err := s.ActiveMember(ctx, roomID, memberID)
	if err != nil {
		return Member{}, err
	}
	rm, err := s.repo.Get(ctx, roomID)
	if err != nil {
		return Member{}, fmt.Errorf("can post: %w", err)
	}
	if rm.Status == StatusClosed {
		return Member{}, apperr.RoomClosed
	}
	return m, nil
}

type MemberView struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
	Online      bool      `json:"online"`
}

func (s *Service) Members(ctx context.Context, roomID uuid.UUID) ([]MemberView, error) {
	members, err := s.repo.ListActiveMembers(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("members: %w", err)
	}
	online := s.presence.Online(roomID)
	out := make([]MemberView, 0, len(members))
	for _, m := range members {
		out = append(out, MemberView{
			ID: m.ID, DisplayName: m.DisplayName, Role: m.Role,
			JoinedAt: m.JoinedAt.UTC(), Online: online[m.ID],
		})
	}
	return out, nil
}

// Remove kicks (or bans) a member and drops their live connections at once.
func (s *Service) Remove(ctx context.Context, roomID, memberID uuid.UUID, ban bool) error {
	m, err := s.repo.GetMember(ctx, roomID, memberID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if m.Role == RoleOwner {
		return apperr.Invalid("the host cannot be removed")
	}
	if err := s.repo.RemoveMember(ctx, roomID, memberID, ban); err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	reason := apperr.Kicked.Code
	if ban {
		reason = apperr.Banned.Code
	}
	if err := s.notifier.Kick(ctx, roomID, memberID, reason); err != nil {
		return fmt.Errorf("remove member: notify: %w", err)
	}
	return nil
}

func (s *Service) Close(ctx context.Context, roomID uuid.UUID) error {
	if err := s.repo.Close(ctx, roomID); err != nil {
		return fmt.Errorf("close room: %w", err)
	}
	if err := s.notifier.CloseRoom(ctx, roomID); err != nil {
		return fmt.Errorf("close room: notify: %w", err)
	}
	return nil
}

// CleanName trims a user-supplied name and enforces a length in characters.
func CleanName(s string, maxLen int, field string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(s)
	if n == 0 {
		return "", apperr.Invalid(field + " is required")
	}
	if n > maxLen {
		return "", apperr.Invalid(fmt.Sprintf("%s must be at most %d characters", field, maxLen))
	}
	return s, nil
}
