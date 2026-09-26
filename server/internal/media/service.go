package media

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"smalltalk/internal/apperr"
	"smalltalk/internal/platform/httpx"
	"smalltalk/internal/room"
)

type Service struct {
	repo    *Repository
	store   Storage
	rooms   *room.Service
	limiter *httpx.KeyedLimiter
}

func NewService(repo *Repository, store Storage, rooms *room.Service, limiter *httpx.KeyedLimiter) *Service {
	return &Service{repo: repo, store: store, rooms: rooms, limiter: limiter}
}

// Upload validates and stores an image. It is not visible to anyone until a
// message references it; unattached uploads are removed by Cleanup.
func (s *Service) Upload(ctx context.Context, roomID, memberID uuid.UUID, data []byte) (Image, error) {
	if !s.limiter.Allow(memberID.String()) {
		return Image{}, apperr.RateLimited
	}
	if _, err := s.rooms.CanPost(ctx, roomID, memberID); err != nil {
		return Image{}, fmt.Errorf("upload image: %w", err)
	}
	info, clean, err := Inspect(data)
	if err != nil {
		return Image{}, err
	}
	img := Image{
		ID:          uuid.New(),
		RoomID:      roomID,
		MemberID:    memberID,
		ContentType: info.ContentType,
		SizeBytes:   len(clean),
		Width:       info.Width,
		Height:      info.Height,
	}
	img.StorageKey = roomID.String() + "/" + img.ID.String()
	if err := s.store.Put(ctx, img.StorageKey, clean); err != nil {
		return Image{}, fmt.Errorf("upload image: %w", err)
	}
	if err := s.repo.Insert(ctx, img); err != nil {
		_ = s.store.Delete(context.WithoutCancel(ctx), img.StorageKey)
		return Image{}, fmt.Errorf("upload image: %w", err)
	}
	return img, nil
}

// Get returns image metadata; used by chat to check an attachment before sending.
func (s *Service) Get(ctx context.Context, roomID, id uuid.UUID) (Image, error) {
	return s.repo.Get(ctx, roomID, id)
}

func (s *Service) Open(ctx context.Context, roomID, id uuid.UUID) (Image, io.ReadCloser, error) {
	img, err := s.repo.Get(ctx, roomID, id)
	if err != nil {
		return Image{}, nil, fmt.Errorf("open image: %w", err)
	}
	rc, err := s.store.Open(ctx, img.StorageKey)
	if err != nil {
		return Image{}, nil, fmt.Errorf("open image: %w", err)
	}
	return img, rc, nil
}

// unattachedTTL is how long an upload may wait for its message.
const unattachedTTL = time.Hour

// RunCleanup deletes unattached uploads every interval until ctx is done.
func (s *Service) RunCleanup(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if err := s.Cleanup(ctx, now.Add(-unattachedTTL)); err != nil {
				slog.ErrorContext(ctx, "image cleanup", "err", err)
			}
		}
	}
}

func (s *Service) Cleanup(ctx context.Context, cutoff time.Time) error {
	keys, err := s.repo.DeleteUnattached(ctx, cutoff)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := s.store.Delete(ctx, k); err != nil {
			slog.ErrorContext(ctx, "delete image file", "key", k, "err", err)
		}
	}
	return nil
}
