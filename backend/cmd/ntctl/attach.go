package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/imaging"
	"vuathau/backend/internal/repository"
)

// attach: gắn lại ảnh đã có trong DATA_DIR/anh (theo sha256) vào 1 hồ sơ, dùng khi khôi phục ảnh bị gỡ nhầm.
// Ảnh đang thuộc hồ sơ khác: bỏ qua. Tọa độ, địa chỉ, giờ chụp lấy từ EXIF; thiếu thì lấy theo hồ sơ.
// Sau đó chạy reprice để tính lại tiền.
//
//	ntctl attach -house <mã hồ sơ> -note "lý do" <sha256> [<sha256> ...]
func attach(ctx context.Context, pool *pgxpool.Pool, dataDir string, houseID int64, note string, shas []string) {
	houses := repository.NewHouseRepository(pool)
	photos := repository.NewPhotoRepository(pool)
	h, err := houses.Get(ctx, houseID)
	if err != nil {
		fail(fmt.Errorf("hồ sơ #%d: %w", houseID, err))
	}
	n := 0
	for _, sha := range shas {
		sha = strings.ToLower(strings.TrimSpace(sha))
		if len(sha) != 64 {
			fail(fmt.Errorf("sha256 không hợp lệ: %s", sha))
		}
		if other, err := photos.HouseOfSHA(ctx, sha); err == nil && other > 0 {
			fmt.Printf("%s: bỏ qua (đang ở hồ sơ #%d)\n", sha[:12], other)
			continue
		}
		data, err := os.ReadFile(filepath.Join(dataDir, "anh", sha))
		if err != nil {
			fail(err)
		}
		img, info, format, err := imaging.Open(data)
		if err != nil {
			fail(fmt.Errorf("%s: %w", sha[:12], err))
		}
		p := &model.Photo{HouseID: houseID, SHA256: sha, DHash: imaging.DHash(img), ExifTime: info.ExifTime,
			Address: h.Address, Mime: imaging.MimeOf(format), Lat: info.Lat, Lng: info.Lng}
		if p.Lat == nil {
			p.Lat, p.Lng = h.Lat, h.Lng
		}
		if p.ExifTime == "" { // như đồng bộ Messenger: giờ CTV gửi tin
			p.ExifTime = h.CreatedAt.In(time.FixedZone("ICT", 7*3600)).Format("2006-01-02 15:04")
		}
		if err := photos.Insert(ctx, p); err != nil {
			fail(fmt.Errorf("%s: %w", sha[:12], err))
		}
		n++
		fmt.Printf("%s: đã gắn vào #%d\n", sha[:12], houseID)
	}
	if n == 0 {
		return
	}
	msg := fmt.Sprintf("Khôi phục %d ảnh", n)
	if note != "" {
		msg += ". " + note
	}
	if _, err := pool.Exec(ctx, `INSERT INTO house_logs (house_id, action, note) VALUES ($1, $2, $3)`, houseID, "Khôi phục ảnh", msg); err != nil {
		fail(err)
	}
}
