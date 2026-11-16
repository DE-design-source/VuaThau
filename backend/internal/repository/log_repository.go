package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
)

type LogRepository interface {
	Add(ctx context.Context, userID int64, action string, houseID, batchID *int64, note string) error
	// ForHouse: lịch sử 1 hồ sơ, kèm lịch sử phiếu chi của hồ sơ (nếu có), theo thời gian.
	ForHouse(ctx context.Context, houseID int64, batchID *int64) ([]model.Log, error)
	ForBatch(ctx context.Context, batchID int64) ([]model.Log, error)
	// Flow: số hồ sơ có thao tác trong khoảng thời gian, theo tên thao tác.
	Flow(ctx context.Context, actions []string, from, to time.Time) (map[string]int, error)
}

type logRepository struct {
	db *pgxpool.Pool
}

func NewLogRepository(db *pgxpool.Pool) LogRepository {
	return &logRepository{db: db}
}

func (r *logRepository) Add(ctx context.Context, userID int64, action string, houseID, batchID *int64, note string) error {
	_, err := r.db.Exec(ctx, `INSERT INTO house_logs (user_id, action, house_id, batch_id, note) VALUES ($1, $2, $3, $4, $5)`,
		userID, action, houseID, batchID, note)
	return err
}

func (r *logRepository) list(ctx context.Context, where string, args ...any) ([]model.Log, error) {
	rows, err := r.db.Query(ctx, `SELECT l.id, l.house_id, l.batch_id, l.user_id, COALESCE(u.name, ''), l.action, l.note, l.at
		FROM house_logs l LEFT JOIN users u ON u.id = l.user_id WHERE `+where+` ORDER BY l.at, l.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Log{}
	for rows.Next() {
		var l model.Log
		if err := rows.Scan(&l.ID, &l.HouseID, &l.BatchID, &l.UserID, &l.UserName, &l.Action, &l.Note, &l.At); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *logRepository) ForHouse(ctx context.Context, houseID int64, batchID *int64) ([]model.Log, error) {
	if batchID != nil {
		return r.list(ctx, `l.house_id = $1 OR (l.batch_id = $2 AND l.house_id IS NULL)`, houseID, *batchID)
	}
	return r.list(ctx, `l.house_id = $1`, houseID)
}

func (r *logRepository) ForBatch(ctx context.Context, batchID int64) ([]model.Log, error) {
	return r.list(ctx, `l.batch_id = $1 AND l.house_id IS NULL`, batchID)
}

func (r *logRepository) Flow(ctx context.Context, actions []string, from, to time.Time) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `SELECT action, COUNT(DISTINCT house_id) FROM house_logs
		WHERE at >= $1 AND at < $2 AND action = ANY($3) GROUP BY action`, from, to, actions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var a string
		var n int
		if err := rows.Scan(&a, &n); err != nil {
			return nil, err
		}
		out[a] = n
	}
	return out, rows.Err()
}
