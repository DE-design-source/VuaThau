package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"vuathau/backend/internal/model"
)

// Ghi vào hồ sơ CTV đang soạn (nháp / bị trả lại). Mỗi thao tác khoá dòng hồ sơ (SELECT ... FOR UPDATE) rồi kiểm trạng
// thái trong cùng giao dịch, nên:
//   - admin vừa duyệt / kế toán vừa trả tiền thì CTV không sửa, thêm, xoá ảnh / video được nữa (trước đây kiểm ở service
//     rồi mới ghi, ghi đè được hồ sơ đã duyệt);
//   - tải 2 đợt ảnh cùng lúc không vượt số ảnh tối đa;
//   - gửi duyệt chỉ thành công khi bộ ảnh / video đúng là bộ đã dùng để tính tiền.

var (
	// ErrNotEditable: hồ sơ không còn ở nháp / bị trả lại.
	ErrNotEditable = errors.New("house not editable")
	// ErrFull: hồ sơ đã đủ số ảnh / video tối đa.
	ErrFull = errors.New("house full")
	// ErrChanged: bộ ảnh / video đổi trong lúc gửi duyệt.
	ErrChanged = errors.New("house media changed")
)

// MediaSig: dấu của bộ ảnh / video (số lượng + tổng id): thêm hoặc xoá là đổi.
type MediaSig struct {
	Photos, Videos     int
	PhotoSum, VideoSum int64
}

// SigOf: dấu của bộ ảnh / video service đã đọc.
func SigOf(ps []model.Photo, vs []model.Video) MediaSig {
	s := MediaSig{Photos: len(ps), Videos: len(vs)}
	for _, p := range ps {
		s.PhotoSum += p.ID
	}
	for _, v := range vs {
		s.VideoSum += v.ID
	}
	return s
}

func lockDraft(ctx context.Context, tx pgx.Tx, houseID int64) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM houses WHERE id = $1 FOR UPDATE`, houseID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !model.Editable(status) {
		return ErrNotEditable
	}
	return nil
}

func inTx(ctx context.Context, begin func(context.Context) (pgx.Tx, error), fn func(pgx.Tx) error) error {
	tx, err := begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// InsertDraft: thêm ảnh vào hồ sơ đang soạn, tối đa max ảnh. ErrNotEditable / ErrFull / ErrDuplicate.
func (r *photoRepository) InsertDraft(ctx context.Context, p *model.Photo, max int) error {
	return inTx(ctx, r.db.Begin, func(tx pgx.Tx) error {
		if err := lockDraft(ctx, tx, p.HouseID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM photos WHERE house_id = $1`, p.HouseID).Scan(&n); err != nil {
			return err
		}
		if n >= max {
			return ErrFull
		}
		err := tx.QueryRow(ctx, `INSERT INTO photos (house_id, sha256, dhash, exif_time, lat, lng, address, dup_of, mime)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
			p.HouseID, p.SHA256, p.DHash, p.ExifTime, p.Lat, p.Lng, p.Address, p.DupOf, p.Mime).Scan(&p.ID)
		if pgErrCode(err) == pgUniqueViolation {
			return ErrDuplicate
		}
		return err
	})
}

// DeleteDraft: xoá ảnh của hồ sơ đang soạn. ErrNotEditable / ErrNotFound.
func (r *photoRepository) DeleteDraft(ctx context.Context, id, houseID int64) (*model.Photo, error) {
	var out *model.Photo
	err := inTx(ctx, r.db.Begin, func(tx pgx.Tx) error {
		if err := lockDraft(ctx, tx, houseID); err != nil {
			return err
		}
		p, err := scanPhoto(tx.QueryRow(ctx, `DELETE FROM photos WHERE id = $1 AND house_id = $2 RETURNING `+photoCols, id, houseID))
		out = p
		return err
	})
	return out, err
}

// InsertVideoDraft: thêm video vào hồ sơ đang soạn, tối đa max video. ErrNotEditable / ErrFull / ErrDuplicate.
func (r *photoRepository) InsertVideoDraft(ctx context.Context, v *model.Video, max int) error {
	return inTx(ctx, r.db.Begin, func(tx pgx.Tx) error {
		if err := lockDraft(ctx, tx, v.HouseID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM videos WHERE house_id = $1`, v.HouseID).Scan(&n); err != nil {
			return err
		}
		if n >= max {
			return ErrFull
		}
		err := tx.QueryRow(ctx, `INSERT INTO videos (house_id, sha256, mime, size) VALUES ($1, $2, $3, $4)
			RETURNING id, created_at`, v.HouseID, v.SHA256, v.Mime, v.Size).Scan(&v.ID, &v.CreatedAt)
		if pgErrCode(err) == pgUniqueViolation {
			return ErrDuplicate
		}
		return err
	})
}

// DeleteVideoDraft: xoá video của hồ sơ đang soạn. ErrNotEditable / ErrNotFound.
func (r *photoRepository) DeleteVideoDraft(ctx context.Context, id, houseID int64) (*model.Video, error) {
	var out *model.Video
	err := inTx(ctx, r.db.Begin, func(tx pgx.Tx) error {
		if err := lockDraft(ctx, tx, houseID); err != nil {
			return err
		}
		v, err := scanVideo(tx.QueryRow(ctx, `DELETE FROM videos WHERE id = $1 AND house_id = $2 RETURNING `+videoCols, id, houseID))
		out = v
		return err
	})
	return out, err
}

// UpdateDraftFields: CTV sửa thông tin hồ sơ đang soạn. ErrNotEditable / ErrNotFound.
func (r *houseRepository) UpdateDraftFields(ctx context.Context, id int64, f model.HouseFields) error {
	return inTx(ctx, r.db.Begin, func(tx pgx.Tx) error {
		if err := lockDraft(ctx, tx, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE houses SET address = $2, floors = $3, stage = $4, contractor_name = $5, contractor_phone = $6
			WHERE id = $1`, id, f.Address, f.Floors, f.Stage, f.ContractorName, f.ContractorPhone)
		return err
	})
}

// SubmitDraft: gửi duyệt như Submit, nhưng chỉ khi bộ ảnh / video còn đúng sig (bộ đã dùng tính tiền).
// ErrNotEditable (vừa đổi trạng thái) / ErrChanged (ảnh / video vừa thêm hoặc xoá) / ErrNotFound.
func (r *houseRepository) SubmitDraft(ctx context.Context, id int64, amount int64, lat, lng *float64, dup string, queueDrive bool, sig MediaSig) error {
	return inTx(ctx, r.db.Begin, func(tx pgx.Tx) error {
		if err := lockDraft(ctx, tx, id); err != nil {
			return err
		}
		var now MediaSig
		if err := tx.QueryRow(ctx, `SELECT
				(SELECT COUNT(*) FROM photos WHERE house_id = $1), (SELECT COALESCE(SUM(id), 0) FROM photos WHERE house_id = $1),
				(SELECT COUNT(*) FROM videos WHERE house_id = $1), (SELECT COALESCE(SUM(id), 0) FROM videos WHERE house_id = $1)`, id).
			Scan(&now.Photos, &now.PhotoSum, &now.Videos, &now.VideoSum); err != nil {
			return fmt.Errorf("media sig: %w", err)
		}
		if now != sig {
			return ErrChanged
		}
		drive := "drive"
		if queueDrive {
			drive = "'cho'"
		}
		_, err := tx.Exec(ctx, `UPDATE houses SET status = 'cho_duyet', note = '', amount = $2, lat = $3, lng = $4, dup = $5, drive = `+drive+`,
			submitted_at = COALESCE(submitted_at, NOW()) WHERE id = $1`, id, amount, lat, lng, dup)
		return err
	})
}
