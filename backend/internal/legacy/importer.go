// Package legacy: chuyển dữ liệu từ máy chủ Nghiệm thu bản Python cũ (SQLite nghiemthu.db + thư mục anh/, bienlai/)
// sang Postgres + DATA_DIR của backend Go. Giữ nguyên id (link cũ, mã hồ sơ CTV đã biết vẫn đúng).
// Mật khẩu cũ (scrypt) giữ nguyên: backend nhận được và tự nâng lên bcrypt ở lần đăng nhập đầu.
package legacy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite" // driver "sqlite", thuần Go

	"vuathau/backend/internal/pkg/imaging"
)

// Options cho 1 lần chuyển.
type Options struct {
	SQLitePath string // .../data/nghiemthu.db
	SourceDir  string // thư mục chứa anh/ và bienlai/ của bản cũ (mặc định: cạnh file .db)
	DataDir    string // DATA_DIR của backend Go
	Log        func(format string, args ...any)
}

// Stats: số dòng đã chuyển mỗi bảng.
type Stats map[string]int

var vn = func() *time.Location {
	if l, err := time.LoadLocation("Asia/Ho_Chi_Minh"); err == nil {
		return l
	}
	return time.FixedZone("ICT", 7*3600)
}()

// localTime: giờ SQLite "YYYY-MM-DD HH:MM:SS" (giờ Việt Nam) -> time.Time.
func localTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(s), vn); err == nil {
			return t
		}
	}
	return time.Now()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Import chuyển toàn bộ dữ liệu. Postgres đích phải đã chạy migration và còn trống (chưa có người dùng).
func Import(ctx context.Context, pg *pgxpool.Pool, opt Options) (Stats, error) {
	logf := opt.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if opt.SourceDir == "" {
		opt.SourceDir = filepath.Dir(opt.SQLitePath)
	}
	if _, err := os.Stat(opt.SQLitePath); err != nil {
		return nil, fmt.Errorf("không mở được %s: %w", opt.SQLitePath, err)
	}
	src, err := sql.Open("sqlite", "file:"+filepath.ToSlash(opt.SQLitePath)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer src.Close()

	var n int
	if err := pg.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return nil, fmt.Errorf("Postgres chưa chạy migration? %w", err)
	}
	if n > 0 {
		return nil, errors.New("Postgres đích đã có dữ liệu (bảng users không trống): chỉ chuyển vào CSDL mới")
	}

	tx, err := pg.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	st := Stats{}
	cols := func(table string) map[string]bool {
		out := map[string]bool{}
		rows, err := src.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt sql.NullString
			if rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk) == nil {
				out[name] = true
			}
		}
		return out
	}
	opt2 := func(have map[string]bool, col, fallback string) string {
		if have[col] {
			return col
		}
		return fallback
	}

	// ---- users ----
	uc := cols("users")
	rows, err := src.QueryContext(ctx, fmt.Sprintf(`SELECT id, phone, name, role, pw, bank, created_at, %s, %s, %s FROM users ORDER BY id`,
		opt2(uc, "google", "''"), opt2(uc, "email", "''"), opt2(uc, "locked", "0")))
	if err != nil {
		return nil, fmt.Errorf("đọc users: %w", err)
	}
	users := map[int64]bool{}
	for rows.Next() {
		var id int64
		var phone, name, role, pw, bank, created, google, email string
		var locked int
		if err := rows.Scan(&id, &phone, &name, &role, &pw, &bank, &created, &google, &email, &locked); err != nil {
			rows.Close()
			return nil, err
		}
		if role == "ceo" {
			role = "admin"
		}
		if len(phone) > 50 {
			phone = phone[:50]
		}
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, username, name, role, password_hash, bank, google_sub, email, locked, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)`,
			id, phone, name, role, pw, bank, nullStr(google), nullStr(email), locked, localTime(created)); err != nil {
			rows.Close()
			return nil, fmt.Errorf("users #%d: %w", id, err)
		}
		users[id] = true
		st["users"]++
	}
	rows.Close()
	userRef := func(id sql.NullInt64) any {
		if id.Valid && users[id.Int64] {
			return id.Int64
		}
		return nil
	}

	// ---- batches (phiếu chi) ----
	bc := cols("batches")
	rows, err = src.QueryContext(ctx, fmt.Sprintf(`SELECT id, status, total, note, created_by, created_at, %s, %s FROM batches ORDER BY id`,
		opt2(bc, "ctv_id", "NULL"), opt2(bc, "receipt", "''")))
	if err != nil {
		return nil, fmt.Errorf("đọc batches: %w", err)
	}
	batches := map[int64]bool{}
	for rows.Next() {
		var id, total int64
		var status, note, created, receipt string
		var by, ctv sql.NullInt64
		if err := rows.Scan(&id, &status, &total, &note, &by, &created, &ctv, &receipt); err != nil {
			rows.Close()
			return nil, err
		}
		if status != "huy" {
			status = "da_chi"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO batches (id, status, total, ref, receipt, ctv_id, created_by, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, status, total, note, receipt, userRef(ctv), userRef(by), localTime(created)); err != nil {
			rows.Close()
			return nil, fmt.Errorf("batches #%d: %w", id, err)
		}
		batches[id] = true
		st["batches"]++
	}
	rows.Close()
	batchRef := func(id sql.NullInt64) any {
		if id.Valid && batches[id.Int64] {
			return id.Int64
		}
		return nil
	}

	// ---- houses (hồ sơ) ----
	hc := cols("houses")
	rows, err = src.QueryContext(ctx, fmt.Sprintf(`SELECT id, ctv_id, status, address, lat, lng, contractor_name, contractor_phone,
		floors, stage, note, dup, amount, ai_state, %s, batch_id, created_at, %s FROM houses ORDER BY id`,
		opt2(hc, "ai_json", "''"), opt2(hc, "drive", "''")))
	if err != nil {
		return nil, fmt.Errorf("đọc houses: %w", err)
	}
	houses := map[int64]bool{}
	for rows.Next() {
		var id, ctv, amount int64
		var status, address, cname, cphone, stage, note, dup, aiState, aiJSON, created, drive string
		var lat, lng sql.NullFloat64
		var floors, batch sql.NullInt64
		if err := rows.Scan(&id, &ctv, &status, &address, &lat, &lng, &cname, &cphone, &floors, &stage, &note, &dup, &amount,
			&aiState, &aiJSON, &batch, &created, &drive); err != nil {
			rows.Close()
			return nil, err
		}
		switch status {
		case "cho_ceo", "cho_chi": // luồng cũ đã bỏ: về lại chờ thanh toán
			status, batch = "da_duyet", sql.NullInt64{}
		}
		var js any
		if aiJSON != "" && json.Valid([]byte(aiJSON)) {
			js = aiJSON
		}
		if aiState == "dang_doc" { // AI đang đọc dở lúc tắt máy chủ cũ
			aiState = "loi"
		}
		var fl any
		if floors.Valid {
			fl = floors.Int64
		}
		var la, ln any
		if lat.Valid && lng.Valid {
			la, ln = lat.Float64, lng.Float64
		}
		t := localTime(created)
		if _, err := tx.Exec(ctx, `INSERT INTO houses (id, ctv_id, status, address, lat, lng, contractor_name, contractor_phone, floors,
			stage, note, dup, amount, ai_state, ai_json, batch_id, drive, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16,$17,$18,$18)`,
			id, ctv, status, address, la, ln, cname, cphone, fl, stage, note, dup, amount, aiState, js, batchRef(batch), drive, t); err != nil {
			rows.Close()
			return nil, fmt.Errorf("houses #%d: %w", id, err)
		}
		houses[id] = true
		st["houses"]++
	}
	rows.Close()
	houseRef := func(id sql.NullInt64) any {
		if id.Valid && houses[id.Int64] {
			return id.Int64
		}
		return nil
	}

	// ---- photos: chép file, tính lại dHash bằng Go ----
	srcPhotos, dstPhotos := filepath.Join(opt.SourceDir, "anh"), filepath.Join(opt.DataDir, "anh")
	if err := os.MkdirAll(dstPhotos, 0o755); err != nil {
		return nil, err
	}
	rows, err = src.QueryContext(ctx, `SELECT id, house_id, sha256, dhash, exif_time, ai_time, lat, lng, address, is_site, dup_of
		FROM photos ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("đọc photos: %w", err)
	}
	missing := 0
	for rows.Next() {
		var id, house int64
		var sha, dhash, exifT, aiT, addr string
		var lat, lng sql.NullFloat64
		var isSite, dupOf sql.NullInt64
		if err := rows.Scan(&id, &house, &sha, &dhash, &exifT, &aiT, &lat, &lng, &addr, &isSite, &dupOf); err != nil {
			rows.Close()
			return nil, err
		}
		if !houses[house] {
			continue
		}
		mime := "image/jpeg"
		if data, err := os.ReadFile(filepath.Join(srcPhotos, sha)); err == nil {
			if img, _, format, err := imaging.Open(data); err == nil {
				dhash, mime = imaging.DHash(img), imaging.MimeOf(format)
			}
			for _, name := range []string{sha, sha + "_s.jpg"} {
				if err := copyFile(filepath.Join(srcPhotos, name), filepath.Join(dstPhotos, name)); err != nil && name == sha {
					rows.Close()
					return nil, err
				}
			}
		} else {
			missing++
		}
		var site any
		if isSite.Valid {
			site = isSite.Int64 != 0
		}
		var la, ln any
		if lat.Valid {
			la = lat.Float64
		}
		if lng.Valid {
			ln = lng.Float64
		}
		var dup any
		if dupOf.Valid {
			dup = dupOf.Int64
		}
		if len(dhash) != 16 {
			dhash = fmt.Sprintf("%016s", dhash)[:16]
		}
		if _, err := tx.Exec(ctx, `INSERT INTO photos (id, house_id, sha256, dhash, exif_time, ai_time, lat, lng, address, is_site, dup_of, mime)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			id, house, sha, dhash, exifT, aiT, la, ln, addr, site, dup, mime); err != nil {
			rows.Close()
			return nil, fmt.Errorf("photos #%d: %w", id, err)
		}
		st["photos"]++
	}
	rows.Close()
	if missing > 0 {
		logf("Cảnh báo: %d ảnh không còn file trong %s (vẫn giữ dòng dữ liệu)", missing, srcPhotos)
	}

	// ---- log (lịch sử) ----
	rows, err = src.QueryContext(ctx, `SELECT id, house_id, batch_id, user_id, action, note, at FROM log ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("đọc log: %w", err)
	}
	for rows.Next() {
		var id int64
		var house, batch, user sql.NullInt64
		var action, note, at string
		if err := rows.Scan(&id, &house, &batch, &user, &action, &note, &at); err != nil {
			rows.Close()
			return nil, err
		}
		if house.Valid && !houses[house.Int64] { // hồ sơ đã xoá
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO house_logs (id, house_id, batch_id, user_id, action, note, at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			id, houseRef(house), batchRef(batch), userRef(user), action, note, localTime(at)); err != nil {
			rows.Close()
			return nil, fmt.Errorf("log #%d: %w", id, err)
		}
		st["logs"]++
	}
	rows.Close()

	// ---- adjust (thưởng / trừ) ----
	if len(cols("adjust")) > 0 {
		rows, err = src.QueryContext(ctx, `SELECT id, ctv_id, amount, note, batch_id, created_by, created_at FROM adjust ORDER BY id`)
		if err != nil {
			return nil, fmt.Errorf("đọc adjust: %w", err)
		}
		for rows.Next() {
			var id, ctv, amount int64
			var note, created string
			var batch, by sql.NullInt64
			if err := rows.Scan(&id, &ctv, &amount, &note, &batch, &by, &created); err != nil {
				rows.Close()
				return nil, err
			}
			if amount == 0 || !users[ctv] {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO adjustments (id, ctv_id, amount, note, batch_id, created_by, created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, ctv, amount, note, batchRef(batch), userRef(by), localTime(created)); err != nil {
				rows.Close()
				return nil, fmt.Errorf("adjust #%d: %w", id, err)
			}
			st["adjustments"]++
		}
		rows.Close()
	}

	// ---- settings ----
	rows, err = src.QueryContext(ctx, `SELECT k, v FROM settings`)
	if err == nil {
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) != nil {
				continue
			}
			var iv int64
			if _, err := fmt.Sscan(v, &iv); err != nil {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO settings (k, v) VALUES ($1, $2) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`, k, iv); err != nil {
				rows.Close()
				return nil, err
			}
			st["settings"]++
		}
		rows.Close()
	}

	// ---- ai_cache ----
	rows, err = src.QueryContext(ctx, `SELECT sha256, json, at FROM ai_cache`)
	if err == nil {
		for rows.Next() {
			var sha, js, at string
			if rows.Scan(&sha, &js, &at) != nil || len(sha) != 64 || !json.Valid([]byte(js)) {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ai_cache (sha256, data, at) VALUES ($1, $2::jsonb, $3) ON CONFLICT DO NOTHING`,
				sha, js, localTime(at)); err != nil {
				rows.Close()
				return nil, err
			}
			st["ai_cache"]++
		}
		rows.Close()
	}

	// ---- đặt lại bộ đếm id sau khi chèn id có sẵn ----
	for _, t := range []string{"users", "batches", "houses", "photos", "house_logs", "adjustments", "messages"} {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE((SELECT MAX(id) FROM %s), 0) + 1, false)`,
			t, t)); err != nil {
			return nil, fmt.Errorf("setval %s: %w", t, err)
		}
	}

	// ---- biên lai ----
	srcR, dstR := filepath.Join(opt.SourceDir, "bienlai"), filepath.Join(opt.DataDir, "bienlai")
	if err := os.MkdirAll(dstR, 0o755); err != nil {
		return nil, err
	}
	if ents, err := os.ReadDir(srcR); err == nil {
		for _, e := range ents {
			if !e.IsDir() {
				if err := copyFile(filepath.Join(srcR, e.Name()), filepath.Join(dstR, e.Name())); err != nil {
					return nil, err
				}
				st["receipt_files"]++
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return st, nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
