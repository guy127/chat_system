package room

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
	ExternalRef    *string // set for rooms created through /service/v1
	CreatedAt      time.Time
}

type Member struct {
	ID             uuid.UUID
	RoomID         uuid.UUID
	DisplayName    string
	Role           string
	ExternalUserID *string
	Label          string
	JoinedAt       time.Time
	KickedAt       *time.Time
	BannedAt       *time.Time
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

const roomCols = `id, name, owner_token_hash, status, external_ref, created_at`

func scanRoom(row pgx.Row) (Room, error) {
	var rm Room
	err := row.Scan(&rm.ID, &rm.Name, &rm.OwnerTokenHash, &rm.Status, &rm.ExternalRef, &rm.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, apperr.NotFound
	}
	if err != nil {
		return Room{}, fmt.Errorf("get room: %w", err)
	}
	return rm, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Room, error) {
	return scanRoom(r.db.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE id = $1`, id))
}

func (r *Repository) GetByRef(ctx context.Context, ref string) (Room, error) {
	return scanRoom(r.db.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE external_ref = $1`, ref))
}

// EnsureByRef creates the room for ref unless it exists, then returns it.
// Concurrent callers with the same ref all get the same room.
func (r *Repository) EnsureByRef(ctx context.Context, rm Room, ref string) (Room, error) {
	if _, err := r.db.Exec(ctx,
		`INSERT INTO rooms (id, name, owner_token_hash, status, external_ref) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (external_ref) DO NOTHING`,
		rm.ID, rm.Name, rm.OwnerTokenHash, StatusActive, ref); err != nil {
		return Room{}, fmt.Errorf("ensure room: %w", err)
	}
	return r.GetByRef(ctx, ref)
}

func (r *Repository) Close(ctx context.Context, id uuid.UUID) error {
	if _, err := r.db.Exec(ctx, `UPDATE rooms SET status = $2 WHERE id = $1`, id, StatusClosed); err != nil {
		return fmt.Errorf("close room: %w", err)
	}
	return nil
}

const memberCols = `id, room_id, display_name, role, joined_at, kicked_at, banned_at, external_user_id, label`

func scanMember(row pgx.Row) (Member, error) {
	var m Member
	err := row.Scan(&m.ID, &m.RoomID, &m.DisplayName, &m.Role, &m.JoinedAt, &m.KickedAt, &m.BannedAt,
		&m.ExternalUserID, &m.Label)
	return m, err
}

// UpsertExternalMember adds an external user to the room, or refreshes the
// name and label of the member they already are. It never clears kicked_at or
// banned_at, so the caller must check the returned member.
func (r *Repository) UpsertExternalMember(ctx context.Context, m Member) (Member, error) {
	got, err := scanMember(r.db.QueryRow(ctx,
		`INSERT INTO room_members (id, room_id, display_name, role, external_user_id, label)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (room_id, external_user_id) DO UPDATE
		   SET display_name = EXCLUDED.display_name, label = EXCLUDED.label
		 RETURNING `+memberCols,
		m.ID, m.RoomID, m.DisplayName, m.Role, m.ExternalUserID, m.Label))
	if err != nil {
		return Member{}, fmt.Errorf("upsert member: %w", err)
	}
	return got, nil
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
