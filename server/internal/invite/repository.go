package invite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"qrchat/internal/apperr"
	"qrchat/internal/room"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const inviteCols = `id, room_id, token_hash, expires_at, max_uses, used_count, revoked_at, created_at`

func scanInvite(row pgx.Row) (Invite, error) {
	var inv Invite
	err := row.Scan(&inv.ID, &inv.RoomID, &inv.TokenHash, &inv.ExpiresAt, &inv.MaxUses,
		&inv.UsedCount, &inv.RevokedAt, &inv.CreatedAt)
	return inv, err
}

func (r *Repository) Create(ctx context.Context, inv Invite) error {
	if _, err := r.db.Exec(ctx,
		`INSERT INTO room_invites (id, room_id, token_hash, expires_at, max_uses) VALUES ($1, $2, $3, $4, $5)`,
		inv.ID, inv.RoomID, inv.TokenHash, inv.ExpiresAt, inv.MaxUses); err != nil {
		return fmt.Errorf("insert invite: %w", err)
	}
	return nil
}

// ListUsable returns invites of a room that are not revoked, expired or used up.
func (r *Repository) ListUsable(ctx context.Context, roomID uuid.UUID, now time.Time) ([]Invite, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+inviteCols+` FROM room_invites
		 WHERE room_id = $1 AND revoked_at IS NULL AND expires_at > $2
		   AND (max_uses IS NULL OR used_count < max_uses)
		 ORDER BY created_at DESC`, roomID, now)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	invites, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Invite, error) { return scanInvite(row) })
	if err != nil {
		return nil, fmt.Errorf("scan invites: %w", err)
	}
	return invites, nil
}

func (r *Repository) Revoke(ctx context.Context, roomID, inviteID uuid.UUID, now time.Time) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE room_invites SET revoked_at = COALESCE(revoked_at, $3) WHERE room_id = $1 AND id = $2`,
		roomID, inviteID, now)
	if err != nil {
		return fmt.Errorf("revoke invite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound
	}
	return nil
}

// Join redeems an invite and creates the member in one transaction. The room
// row is locked so the member cap and closed status cannot race, and the use
// counter is bumped atomically so max_uses is never exceeded.
func (r *Repository) Join(ctx context.Context, tokenHash string, now time.Time, m room.Member) (room.Room, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return room.Room{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	inv, err := scanInvite(tx.QueryRow(ctx,
		`SELECT `+inviteCols+` FROM room_invites WHERE token_hash = $1`, tokenHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return room.Room{}, apperr.InviteInvalid
	}
	if err != nil {
		return room.Room{}, fmt.Errorf("get invite: %w", err)
	}

	var rm room.Room
	if err := tx.QueryRow(ctx,
		`SELECT id, name, status, created_at FROM rooms WHERE id = $1 FOR UPDATE`, inv.RoomID).
		Scan(&rm.ID, &rm.Name, &rm.Status, &rm.CreatedAt); err != nil {
		return room.Room{}, fmt.Errorf("lock room: %w", err)
	}
	if rm.Status == room.StatusClosed {
		return room.Room{}, apperr.RoomClosed
	}
	if err := Check(inv, now); err != nil {
		return room.Room{}, err
	}

	var active int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM room_members WHERE room_id = $1 AND kicked_at IS NULL AND banned_at IS NULL`,
		rm.ID).Scan(&active); err != nil {
		return room.Room{}, fmt.Errorf("count members: %w", err)
	}
	if active >= room.MaxMembers {
		return room.Room{}, apperr.RoomFull
	}

	var redeemed uuid.UUID
	err = tx.QueryRow(ctx,
		`UPDATE room_invites SET used_count = used_count + 1
		 WHERE id = $1 AND revoked_at IS NULL AND expires_at > $2
		   AND (max_uses IS NULL OR used_count < max_uses)
		 RETURNING id`, inv.ID, now).Scan(&redeemed)
	if errors.Is(err, pgx.ErrNoRows) {
		return room.Room{}, apperr.InviteExhausted
	}
	if err != nil {
		return room.Room{}, fmt.Errorf("redeem invite: %w", err)
	}

	m.RoomID = rm.ID
	if err := room.InsertMember(ctx, tx, m); err != nil {
		return room.Room{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return room.Room{}, fmt.Errorf("commit: %w", err)
	}
	return rm, nil
}
