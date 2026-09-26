package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"

	"smalltalk/internal/apperr"
	"smalltalk/internal/media"
	"smalltalk/internal/platform/httpx"
	"smalltalk/internal/room"
)

// Images looks up uploaded images (implemented by media.Service).
type Images interface {
	Get(ctx context.Context, roomID, id uuid.UUID) (media.Image, error)
}

type Service struct {
	repo    *Repository
	rooms   *room.Service
	images  Images
	broker  Broker
	limiter *httpx.KeyedLimiter
	// roomLocks serialise ID generation, insert and publish per room, so live
	// delivery order matches ULID order and a resume never skips a message.
	roomLocks [256]sync.Mutex
}

func NewService(repo *Repository, rooms *room.Service, images Images, broker Broker, limiter *httpx.KeyedLimiter) *Service {
	return &Service{repo: repo, rooms: rooms, images: images, broker: broker, limiter: limiter}
}

// Send stores a message and then broadcasts it. Re-sending the same
// client_msg_id returns the stored message instead of creating a duplicate.
// imageID, when set, must be an image this member uploaded to this room.
func (s *Service) Send(ctx context.Context, roomID, memberID, clientMsgID uuid.UUID, body string, imageID *uuid.UUID) (Message, error) {
	if err := ValidateBody(body, imageID != nil); err != nil {
		return Message{}, err
	}
	if !s.limiter.Allow(memberID.String()) {
		return Message{}, apperr.RateLimited
	}
	member, err := s.rooms.CanPost(ctx, roomID, memberID)
	if err != nil {
		return Message{}, fmt.Errorf("send message: %w", err)
	}
	var image *ImageRef
	if imageID != nil {
		img, err := s.images.Get(ctx, roomID, *imageID)
		if err != nil {
			return Message{}, fmt.Errorf("send message: %w", err)
		}
		if img.MemberID != memberID {
			return Message{}, apperr.Forbidden
		}
		image = &ImageRef{ID: img.ID, ContentType: img.ContentType, Width: img.Width, Height: img.Height}
	}

	mu := &s.roomLocks[roomID[0]]
	mu.Lock()
	defer mu.Unlock()

	stored, inserted, err := s.repo.Insert(ctx, Message{
		ID:          ulid.Make().String(),
		RoomID:      roomID,
		MemberID:    memberID,
		DisplayName: member.DisplayName,
		ClientMsgID: clientMsgID,
		Body:        body,
		Image:       image,
	})
	if err != nil {
		return Message{}, fmt.Errorf("send message: %w", err)
	}
	if !inserted && stored.MemberID != memberID {
		return Message{}, apperr.Invalid("client_msg_id already used")
	}
	// A retried send is published again: clients de-duplicate by id, and this
	// covers a first attempt that was stored but never broadcast.
	if err := s.broker.Publish(ctx, Event{Kind: EventMessage, RoomID: roomID, Frame: messageFrame(stored)}); err != nil {
		return Message{}, fmt.Errorf("send message: publish: %w", err)
	}
	return stored, nil
}

type Page struct {
	Messages   []Message `json:"messages"`    // oldest first
	NextCursor *string   `json:"next_cursor"` // pass as ?before= to load older; null when done
}

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

// History loads one page of older messages for infinite scroll.
func (s *Service) History(ctx context.Context, roomID uuid.UUID, before string, limit int) (Page, error) {
	if err := validCursor(before); err != nil {
		return Page{}, err
	}
	msgs, err := s.repo.Before(ctx, roomID, before, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("history: %w", err)
	}
	var page Page
	if len(msgs) > limit {
		msgs = msgs[:limit]
		next := msgs[len(msgs)-1].ID
		page.NextCursor = &next
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	page.Messages = msgs
	return page, nil
}

const resumeBatch = 200

// Missed streams every message after lastID to fn, oldest first.
func (s *Service) Missed(ctx context.Context, roomID uuid.UUID, lastID string, fn func(Message)) error {
	if lastID == "" {
		return nil
	}
	if err := validCursor(lastID); err != nil {
		return err
	}
	for {
		msgs, err := s.repo.After(ctx, roomID, lastID, resumeBatch)
		if err != nil {
			return fmt.Errorf("missed messages: %w", err)
		}
		for _, m := range msgs {
			fn(m)
		}
		if len(msgs) < resumeBatch {
			return nil
		}
		lastID = msgs[len(msgs)-1].ID
	}
}

func validCursor(id string) error {
	if id == "" {
		return nil
	}
	if _, err := ulid.ParseStrict(id); err != nil {
		return apperr.Invalid("invalid message id cursor")
	}
	return nil
}

func messageFrame(m Message) []byte {
	b, _ := json.Marshal(struct {
		Type string `json:"type"`
		Message
	}{"message", m})
	return b
}

// SummaryQuery asks about one room the client has a session for.
type SummaryQuery struct {
	RoomID     uuid.UUID
	MemberID   uuid.UUID
	LastReadID string
}

type Summary struct {
	RoomID      uuid.UUID `json:"room_id"`
	State       string    `json:"state"` // active | kicked | banned | unauthorized | not_found
	Name        string    `json:"name,omitempty"`
	Status      string    `json:"status,omitempty"`
	LastMessage *Message  `json:"last_message"`
	Unread      int       `json:"unread"` // capped at MaxUnread
}

const MaxUnread = 99

// Summarize powers the room list: name, status, newest message and how many
// messages from others arrived after the client's last read message.
func (s *Service) Summarize(ctx context.Context, q SummaryQuery) (Summary, error) {
	sum := Summary{RoomID: q.RoomID}
	if _, err := s.rooms.ActiveMember(ctx, q.RoomID, q.MemberID); err != nil {
		e, ok := apperr.As(err)
		if !ok {
			return Summary{}, err
		}
		sum.State = e.Code
		return sum, nil
	}
	rm, err := s.rooms.Get(ctx, q.RoomID)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize: %w", err)
	}
	sum.State, sum.Name, sum.Status = "active", rm.Name, rm.Status

	last, err := s.repo.Before(ctx, q.RoomID, "", 1)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize: %w", err)
	}
	if len(last) == 1 {
		sum.LastMessage = &last[0]
	}
	cursor := q.LastReadID
	if validCursor(cursor) != nil {
		cursor = ""
	}
	sum.Unread, err = s.repo.UnreadCount(ctx, q.RoomID, q.MemberID, cursor, MaxUnread)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize: %w", err)
	}
	return sum, nil
}
