package room

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"qrchat/internal/apperr"
)

const (
	StatusActive = "active"
	StatusClosed = "closed"
	RoleOwner    = "owner"
	RoleMember   = "member"
	// MaxMembers caps how many active members a room may hold.
	MaxMembers = 200
)

type Room struct {
	ID             uuid.UUID
	Name           string
	OwnerTokenHash string
	Status         string
	CreatedAt      time.Time
}

type Member struct {
	ID          uuid.UUID
	RoomID      uuid.UUID
	DisplayName string
	Role        string
	JoinedAt    time.Time
	KickedAt    *time.Time
	BannedAt    *time.Time
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) CreateWithOwner(ctx context.Context, rm Room, owner Member) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO rooms (id, name, owner_token_hash, status) VALUES ($1, $2, $3, $4)`,
		rm.ID, rm.Name, rm.OwnerTokenHash, StatusActive); err != nil {
		return fmt.Errorf("insert room: %w", err)
	}
	if err := InsertMember(ctx, tx, owner); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// InsertMember is shared with the invite join transaction.
func InsertMember(ctx context.Context, tx pgx.Tx, m Member) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO room_members (id, room_id, display_name, role) VALUES ($1, $2, $3, $4)`,
		m.ID, m.RoomID, m.DisplayName, m.Role); err != nil {
		return fmt.Errorf("insert member: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Room, error) {
	var rm Room
	err := r.db.QueryRow(ctx,
		`SELECT id, name, owner_token_hash, status, created_at FROM rooms WHERE id = $1`, id).
		Scan(&rm.ID, &rm.Name, &rm.OwnerTokenHash, &rm.Status, &rm.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, apperr.NotFound
	}
	if err != nil {
		return Room{}, fmt.Errorf("get room: %w", err)
	}
	return rm, nil
}

func (r *Repository) Close(ctx context.Context, id uuid.UUID) error {
	if _, err := r.db.Exec(ctx, `UPDATE rooms SET status = $2 WHERE id = $1`, id, StatusClosed); err != nil {
		return fmt.Errorf("close room: %w", err)
	}
	return nil
}

const memberCols = `id, room_id, display_name, role, joined_at, kicked_at, banned_at`

func scanMember(row pgx.Row) (Member, error) {
	var m Member
	err := row.Scan(&m.ID, &m.RoomID, &m.DisplayName, &m.Role, &m.JoinedAt, &m.KickedAt, &m.BannedAt)
	return m, err
}

func (r *Repository) GetMember(ctx context.Context, roomID, memberID uuid.UUID) (Member, error) {
	m, err := scanMember(r.db.QueryRow(ctx,
		`SELECT `+memberCols+` FROM room_members WHERE room_id = $1 AND id = $2`, roomID, memberID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, apperr.NotFound
	}
	if err != nil {
		return Member{}, fmt.Errorf("get member: %w", err)
	}
	return m, nil
}

func (r *Repository) OwnerMember(ctx context.Context, roomID uuid.UUID) (Member, error) {
	m, err := scanMember(r.db.QueryRow(ctx,
		`SELECT `+memberCols+` FROM room_members WHERE room_id = $1 AND role = $2 ORDER BY joined_at LIMIT 1`,
		roomID, RoleOwner))
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, apperr.NotFound
	}
	if err != nil {
		return Member{}, fmt.Errorf("get owner: %w", err)
	}
	return m, nil
}

// ListActiveMembers returns members who are neither kicked nor banned, oldest first.
func (r *Repository) ListActiveMembers(ctx context.Context, roomID uuid.UUID) ([]Member, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+memberCols+` FROM room_members
		 WHERE room_id = $1 AND kicked_at IS NULL AND banned_at IS NULL
		 ORDER BY joined_at, id`, roomID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) { return scanMember(row) })
	if err != nil {
		return nil, fmt.Errorf("scan members: %w", err)
	}
	return members, nil
}

func (r *Repository) RemoveMember(ctx context.Context, roomID, memberID uuid.UUID, ban bool) error {
	col := "kicked_at"
	if ban {
		col = "banned_at"
	}
	tag, err := r.db.Exec(ctx,
		`UPDATE room_members SET `+col+` = now() WHERE room_id = $1 AND id = $2`, roomID, memberID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound
	}
	return nil
}
