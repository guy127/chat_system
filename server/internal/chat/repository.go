package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const messageSelect = `SELECT m.id, m.room_id, m.member_id, rm.display_name, m.client_msg_id, m.body, m.created_at
	FROM messages m JOIN room_members rm ON rm.id = m.member_id `

func scanMessage(row pgx.Row) (Message, error) {
	var m Message
	err := row.Scan(&m.ID, &m.RoomID, &m.MemberID, &m.DisplayName, &m.ClientMsgID, &m.Body, &m.CreatedAt)
	m.CreatedAt = m.CreatedAt.UTC()
	return m, err
}

// Insert stores msg unless (room_id, client_msg_id) already exists, in which
// case it returns the stored message and inserted=false.
func (r *Repository) Insert(ctx context.Context, msg Message) (stored Message, inserted bool, err error) {
	err = r.db.QueryRow(ctx,
		`INSERT INTO messages (id, room_id, member_id, client_msg_id, body) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (room_id, client_msg_id) DO NOTHING
		 RETURNING created_at`,
		msg.ID, msg.RoomID, msg.MemberID, msg.ClientMsgID, msg.Body).Scan(&msg.CreatedAt)
	if err == nil {
		msg.CreatedAt = msg.CreatedAt.UTC()
		return msg, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Message{}, false, fmt.Errorf("insert message: %w", err)
	}
	existing, err := scanMessage(r.db.QueryRow(ctx,
		messageSelect+`WHERE m.room_id = $1 AND m.client_msg_id = $2`, msg.RoomID, msg.ClientMsgID))
	if err != nil {
		return Message{}, false, fmt.Errorf("load duplicate message: %w", err)
	}
	return existing, false, nil
}

// Before returns up to limit messages older than cursor, newest first.
// An empty cursor starts from the newest message.
func (r *Repository) Before(ctx context.Context, roomID uuid.UUID, cursor string, limit int) ([]Message, error) {
	return r.list(ctx,
		messageSelect+`WHERE m.room_id = $1 AND ($2 = '' OR m.id < $2) ORDER BY m.id DESC LIMIT $3`,
		roomID, cursor, limit)
}

// After returns up to limit messages newer than cursor, oldest first.
func (r *Repository) After(ctx context.Context, roomID uuid.UUID, cursor string, limit int) ([]Message, error) {
	return r.list(ctx,
		messageSelect+`WHERE m.room_id = $1 AND m.id > $2 ORDER BY m.id ASC LIMIT $3`,
		roomID, cursor, limit)
}

func (r *Repository) list(ctx context.Context, sql string, args ...any) ([]Message, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	msgs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Message, error) { return scanMessage(row) })
	if err != nil {
		return nil, fmt.Errorf("scan messages: %w", err)
	}
	return msgs, nil
}
