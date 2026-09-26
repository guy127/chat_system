// Package service is the API for trusted backends, such as the medical claims
// system, that decide who may chat themselves and ask smalltalk for rooms and
// member sessions on their users' behalf. It is served under /service/v1 and
// must only be reachable from the internal network.
package service

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/google/uuid"

	"smalltalk/internal/apperr"
	"smalltalk/internal/auth"
	"smalltalk/internal/media"
	"smalltalk/internal/room"
)

var (
	refPattern    = regexp.MustCompile(`^[a-z0-9:_-]{1,128}$`)
	userIDPattern = regexp.MustCompile(`^[A-Za-z0-9:_-]{1,128}$`)
)

// ValidRef checks a caller's room reference, e.g. claims:ticket:123:public.
func ValidRef(ref string) error {
	if !refPattern.MatchString(ref) {
		return apperr.Invalid("ref must match ^[a-z0-9:_-]{1,128}$")
	}
	return nil
}

type Service struct {
	rooms  *room.Service
	images *media.Service
	issuer *auth.Issuer
	ttl    time.Duration
}

func NewService(rooms *room.Service, images *media.Service, issuer *auth.Issuer, ttl time.Duration) *Service {
	return &Service{rooms: rooms, images: images, issuer: issuer, ttl: ttl}
}

type RoomState struct {
	RoomID uuid.UUID `json:"room_id"`
	Status string    `json:"status"`
}

// EnsureRoom creates the room for ref on first use; later calls return it
// unchanged, keeping its original name.
func (s *Service) EnsureRoom(ctx context.Context, ref, name string) (RoomState, error) {
	if err := ValidRef(ref); err != nil {
		return RoomState{}, err
	}
	rm, err := s.rooms.EnsureByRef(ctx, ref, name)
	if err != nil {
		return RoomState{}, fmt.Errorf("ensure room: %w", err)
	}
	return RoomState{RoomID: rm.ID, Status: rm.Status}, nil
}

// Session issues a member JWT for an external user. It works on closed rooms
// too so history stays readable; posting is refused by room.CanPost.
func (s *Service) Session(ctx context.Context, ref, externalUserID, displayName, label string) (auth.Session, error) {
	if err := ValidRef(ref); err != nil {
		return auth.Session{}, err
	}
	if !userIDPattern.MatchString(externalUserID) {
		return auth.Session{}, apperr.Invalid("external_user_id must match ^[A-Za-z0-9:_-]{1,128}$")
	}
	rm, err := s.rooms.GetByRef(ctx, ref)
	if err != nil {
		return auth.Session{}, fmt.Errorf("service session: %w", err)
	}
	m, err := s.rooms.JoinExternal(ctx, rm.ID, externalUserID, displayName, label)
	if err != nil {
		return auth.Session{}, fmt.Errorf("service session: %w", err)
	}
	return s.issuer.IssueFor(auth.Claims{RoomID: rm.ID, MemberID: m.ID, Role: room.RoleMember}, s.ttl)
}

// Close closes the room and drops its live connections. Closing a closed room
// does nothing.
func (s *Service) Close(ctx context.Context, ref string) error {
	if err := ValidRef(ref); err != nil {
		return err
	}
	rm, err := s.rooms.GetByRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("close service room: %w", err)
	}
	if rm.Status == room.StatusClosed {
		return nil
	}
	return s.rooms.Close(ctx, rm.ID)
}

// SentImage opens an image sent in the room and reports who sent it, so the
// caller can apply its own rules (e.g. only the claimant's receipts count).
func (s *Service) SentImage(ctx context.Context, ref string, imageID uuid.UUID) (media.Image, io.ReadCloser, string, error) {
	if err := ValidRef(ref); err != nil {
		return media.Image{}, nil, "", err
	}
	rm, err := s.rooms.GetByRef(ctx, ref)
	if err != nil {
		return media.Image{}, nil, "", fmt.Errorf("sent image: %w", err)
	}
	img, rc, err := s.images.OpenSent(ctx, rm.ID, imageID)
	if err != nil {
		return media.Image{}, nil, "", fmt.Errorf("sent image: %w", err)
	}
	sender, err := s.rooms.Member(ctx, rm.ID, img.MemberID)
	if err != nil {
		_ = rc.Close()
		return media.Image{}, nil, "", fmt.Errorf("sent image: %w", err)
	}
	var senderID string
	if sender.ExternalUserID != nil {
		senderID = *sender.ExternalUserID
	}
	return img, rc, senderID, nil
}
