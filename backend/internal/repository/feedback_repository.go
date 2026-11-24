package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Đánh giá app của CTV / nhà thầu (migration 029).

type Feedback struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	Role      string    `json:"role"`
	Stars     *int      `json:"stars"`
	Comment   string    `json:"comment"`
	Later     bool      `json:"later"`
	Photos    int       `json:"photos"`
	Version   string    `json:"version"`
	Platform  string    `json:"platform"`
	CreatedAt time.Time `json:"created_at"`
}

// FeedbackState: người dùng đã trả lời thế nào: số lần "Để sau", đã đánh giá chưa.
type FeedbackState struct {
	Laters int
	Rated  bool
}

type FeedbackRepository interface {
	// Photos: số ảnh người dùng đã tải lên (ảnh hồ sơ CTV + ảnh nhật ký / dự án nhà thầu)
	Photos(ctx context.Context, uid int64) (int, error)
	State(ctx context.Context, uid int64) (FeedbackState, error)
	Add(ctx context.Context, f *Feedback) error
	// List: mới nhất trước, kèm tên / SĐT / vai trò người gửi
	List(ctx context.Context, limit int) ([]Feedback, error)
}

type feedbackRepository struct{ db *pgxpool.Pool }

func NewFeedbackRepository(db *pgxpool.Pool) FeedbackRepository { return &feedbackRepository{db: db} }

func (r *feedbackRepository) Photos(ctx context.Context, uid int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM photos p JOIN houses h ON h.id = p.house_id WHERE h.ctv_id = $1) +
		(SELECT COUNT(*) FROM nt_media m JOIN nt_projects pr ON pr.id = m.project_id WHERE pr.user_id = $1 AND m.kind = 'photo')`, uid).Scan(&n)
	return n, err
}

func (r *feedbackRepository) State(ctx context.Context, uid int64) (FeedbackState, error) {
	var s FeedbackState
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE later), COALESCE(BOOL_OR(NOT later), false)
		FROM app_feedback WHERE user_id = $1`, uid).Scan(&s.Laters, &s.Rated)
	return s, err
}

func (r *feedbackRepository) Add(ctx context.Context, f *Feedback) error {
	return r.db.QueryRow(ctx, `INSERT INTO app_feedback (user_id, stars, comment, later, photos, version, platform)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		f.UserID, f.Stars, f.Comment, f.Later, f.Photos, f.Version, f.Platform).Scan(&f.ID, &f.CreatedAt)
}

func (r *feedbackRepository) List(ctx context.Context, limit int) ([]Feedback, error) {
	rows, err := r.db.Query(ctx, `SELECT f.id, f.user_id, u.name, u.username, u.role, f.stars, f.comment, f.later, f.photos,
			f.version, f.platform, f.created_at
		FROM app_feedback f JOIN users u ON u.id = f.user_id
		WHERE NOT f.later
		ORDER BY f.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Feedback{}
	for rows.Next() {
		var f Feedback
		if err := rows.Scan(&f.ID, &f.UserID, &f.Name, &f.Phone, &f.Role, &f.Stars, &f.Comment, &f.Later, &f.Photos,
			&f.Version, &f.Platform, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
