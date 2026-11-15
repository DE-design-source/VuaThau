package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

type MessageRepository interface {
	Create(ctx context.Context, m *model.Message) error
	// ListConversation returns messages between two users, newest first.
	ListConversation(ctx context.Context, userA, userB int64, limit, offset int) ([]model.Message, error)
}

type messageRepository struct {
	db *pgxpool.Pool
}

func NewMessageRepository(db *pgxpool.Pool) MessageRepository {
	return &messageRepository{db: db}
}

func (r *messageRepository) Create(ctx context.Context, m *model.Message) error {
	const q = `
		INSERT INTO messages (sender_id, receiver_id, content)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`

	err := r.db.QueryRow(ctx, q, m.SenderID, m.ReceiverID, m.Content).Scan(&m.ID, &m.CreatedAt)
	if pgErrCode(err) == pgForeignKeyViolation {
		return ErrNotFound
	}
	return err
}

func (r *messageRepository) ListConversation(ctx context.Context, userA, userB int64, limit, offset int) ([]model.Message, error) {
	const q = `
		SELECT id, sender_id, receiver_id, content, created_at
		FROM messages
		WHERE (sender_id = $1 AND receiver_id = $2)
		   OR (sender_id = $2 AND receiver_id = $1)
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.db.Query(ctx, q, userA, userB, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs := make([]model.Message, 0, limit)
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.SenderID, &m.ReceiverID, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
