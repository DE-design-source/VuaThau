package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound  = errors.New("record not found")
	ErrDuplicate = errors.New("duplicate record")
)

// Postgres error codes: https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
)

func pgErrCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
