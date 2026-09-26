package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"smalltalk/internal/apperr"
)

type Image struct {
	ID          uuid.UUID
	RoomID      uuid.UUID
	MemberID    uuid.UUID
	ContentType string
	SizeBytes   int
	Width       int
	Height      int
	StorageKey  string
	CreatedAt   time.Time
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Insert(ctx context.Context, img Image) error {
	if _, err := r.db.Exec(ctx,
		`INSERT INTO images (id, room_id, member_id, content_type, size_bytes, width, height, storage_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		img.ID, img.RoomID, img.MemberID, img.ContentType, img.SizeBytes, img.Width, img.Height, img.StorageKey); err != nil {
		return fmt.Errorf("insert image: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, roomID, id uuid.UUID) (Image, error) {
	var img Image
	err := r.db.QueryRow(ctx,
		`SELECT id, room_id, member_id, content_type, size_bytes, width, height, storage_key, created_at
		 FROM images WHERE room_id = $1 AND id = $2`, roomID, id).
		Scan(&img.ID, &img.RoomID, &img.MemberID, &img.ContentType, &img.SizeBytes,
			&img.Width, &img.Height, &img.StorageKey, &img.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Image{}, apperr.NotFound
	}
	if err != nil {
		return Image{}, fmt.Errorf("get image: %w", err)
	}
	return img, nil
}

// DeleteUnattached removes images uploaded before cutoff that no message
// references, returning their storage keys so the bytes can be deleted too.
func (r *Repository) DeleteUnattached(ctx context.Context, cutoff time.Time) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`DELETE FROM images i
		 WHERE i.created_at < $1 AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.image_id = i.id)
		 RETURNING storage_key`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("delete unattached images: %w", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("delete unattached images: %w", err)
	}
	return keys, nil
}
