package service

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/imaging"
	"vuathau/backend/internal/pkg/safe"
)

// Che SĐT trên ảnh công trình (bảng hiệu nhà thầu, băng rôn...): sale, CTV, Google Drive thấy ảnh đã che;
// admin, kế toán xem ảnh gốc. Vùng SĐT do OCR (Tesseract) tìm, lưu theo ảnh (photos.phone_mask):
//   - quét nền: mọi ảnh chưa quét (ảnh mới + ảnh cũ), mới nhất trước;
//   - người bị che xem ảnh chưa quét: quét ngay rồi mới trả ảnh (không lộ SĐT lúc chờ quét nền).
// Máy không có Tesseract (máy dev): không che.

// seesOriginal: vai trò xem ảnh gốc (không che SĐT).
func seesOriginal(u *model.User) bool {
	return u.Role == model.RoleAdmin || u.Role == model.RoleKetoan
}

// phoneMask: vùng SĐT của ảnh; chưa quét thì quét và lưu. known: SĐT nhà thầu của hồ sơ.
func (s *houseService) phoneMask(ctx context.Context, p *model.Photo, known string) ([]imaging.RelBox, error) {
	var boxes []imaging.RelBox
	if p.PhoneMask != nil {
		err := json.Unmarshal(p.PhoneMask, &boxes)
		return boxes, err
	}
	if s.ocr == nil || !s.ocr.Enabled() {
		return nil, nil
	}
	return s.scanPhone(ctx, p.ID, p.SHA256, known)
}

func (s *houseService) scanPhone(ctx context.Context, id int64, sha, known string) ([]imaging.RelBox, error) {
	small, err := s.files.ReadPhoto(sha, true)
	if err != nil {
		return nil, err
	}
	var ks []string
	if known != "" {
		ks = []string{known}
	}
	boxes, err := s.ocr.Phones(ctx, small, ks)
	if err != nil {
		return nil, err
	}
	if boxes == nil {
		boxes = []imaging.RelBox{}
	}
	raw, _ := json.Marshal(boxes)
	if err := s.photos.SetPhoneMask(ctx, id, raw); err != nil {
		return nil, err
	}
	s.files.DeleteMasked(sha)
	return boxes, nil
}

// maskedPhoto: ảnh cho người không xem được gốc: có SĐT thì trả ảnh đã che (cỡ thu nhỏ / thẻ), không thì nil.
func (s *houseService) maskedPhoto(ctx context.Context, p *model.Photo, size string) ([]byte, error) {
	known := ""
	if h, err := s.houses.Get(ctx, p.HouseID); err == nil {
		known = h.ContractorPhone
	}
	boxes, err := s.phoneMask(ctx, p, known)
	if err != nil {
		return nil, err
	}
	if len(boxes) == 0 {
		return nil, nil
	}
	return s.files.ReadMasked(p.SHA256, size == "card", boxes)
}

// RunPhoneMaskScan: quét nền các ảnh chưa quét SĐT, mỗi lượt tối đa 50 ảnh, nghỉ giữa các lượt.
// Dừng khi ctx huỷ. Không có Tesseract: không chạy.
func (s *houseService) RunPhoneMaskScan(ctx context.Context) {
	safe.Run("che SĐT trên ảnh", func() { s.runPhoneMaskScan(ctx) })
}

func (s *houseService) runPhoneMaskScan(ctx context.Context) {
	if s.ocr == nil || !s.ocr.Enabled() {
		log.Printf("Che SĐT trên ảnh: tắt (không có Tesseract)")
		return
	}
	log.Printf("Che SĐT trên ảnh: bật (Tesseract)")
	done := 0
	for {
		todo, err := s.photos.NoPhoneMask(ctx, 50)
		if err != nil {
			log.Printf("che SĐT: %v", err)
		}
		for _, t := range todo {
			if ctx.Err() != nil {
				return
			}
			if _, err := s.scanPhone(ctx, t.ID, t.SHA256, t.ContractorPhone); err != nil {
				log.Printf("che SĐT ảnh #%d: %v", t.ID, err)
				_ = s.photos.SetPhoneMask(ctx, t.ID, []byte("[]")) // ảnh hỏng: bỏ qua, không quét lại mãi
				continue
			}
			done++
		}
		if len(todo) > 0 && len(todo) < 50 {
			log.Printf("che SĐT: đã quét %d ảnh", done)
		}
		wait := 3 * time.Second
		if len(todo) == 0 {
			wait = time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
