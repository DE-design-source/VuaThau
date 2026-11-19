package model

import "time"

const (
	BatchPaid     = "da_chi"
	BatchCanceled = "huy"
)

// Batch: phiếu chi, 1 lần kế toán trả tiền cho 1 CTV.
type Batch struct {
	ID             int64     `json:"id"`
	Status         string    `json:"status"`
	Total          int64     `json:"total"`
	Ref            string    `json:"ref"`
	Receipt        string    `json:"-"`
	HasReceipt     bool      `json:"has_receipt"`
	ReceiptVersion string    `json:"receipt_version"`
	CTVID          *int64    `json:"ctv_id"`
	CTVName        string    `json:"ctv_name"`
	CTVPhone       string    `json:"ctv_phone"`
	CTVBank        string    `json:"ctv_bank"`
	CreatedBy      *int64    `json:"-"`
	ByName         string    `json:"by_name"`
	CreatedAt      time.Time `json:"created_at"`
	HouseCount     int       `json:"house_count"`
}

// Adjustment: thưởng (> 0) hoặc trừ (< 0) tiền, cộng vào lần thanh toán tới.
type Adjustment struct {
	ID        int64     `json:"id"`
	CTVID     int64     `json:"ctv_id"`
	Amount    int64     `json:"amount"`
	Note      string    `json:"note"`
	BatchID   *int64    `json:"batch_id"`
	CreatedBy *int64    `json:"-"`
	ByName    string    `json:"by_name"`
	CreatedAt time.Time `json:"created_at"`
}

// Money: tiền của 1 CTV (đã cộng thưởng/trừ).
type Money struct {
	Paid      int64 `json:"paid"`      // đã nhận
	Owed      int64 `json:"owed"`      // chờ thanh toán
	Approved  int   `json:"approved"`  // số hồ sơ đã duyệt chưa trả
	Reviewing int64 `json:"reviewing"` // tiền hồ sơ đang chờ duyệt
}

// Payout: 1 lần CTV nhận tiền (phiếu chi chưa huỷ).
type Payout struct {
	ID         int64     `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Ref        string    `json:"ref"`
	HouseCount int       `json:"house_count"`
	Total      int64     `json:"total"`
}

// PayRow: 1 CTV trong danh sách cần thanh toán.
type PayRow struct {
	CTVID      int64  `json:"ctv_id"`
	Name       string `json:"name"`
	Username   string `json:"username"`
	Bank       string `json:"bank"`
	HouseCount int    `json:"house_count"`
	Total      int64  `json:"total"`
}

// Settings: cấu hình admin sửa được. Giá mới áp dụng cho hồ sơ gửi duyệt sau khi sửa.
type Settings struct {
	PriceMin  int64 `json:"price_min"`  // tiền 1 hồ sơ đủ MinPhotos ảnh đạt
	PriceMore int64 `json:"price_more"` // tiền 1 hồ sơ từ MinPhotos+1 ảnh đạt
	MinPhotos int   `json:"min_photos"`
	PayMin    int   `json:"pay_min"` // số hồ sơ đã duyệt để được thanh toán
	// Chặn chụp trùng: công trình trong bán kính DupRadiusM đã có hồ sơ trong DupDays ngày (AI so ảnh xác nhận)
	DupDays    int `json:"dup_days"`
	DupRadiusM int `json:"dup_radius_m"`
	// Hồ sơ chỉ chụp bên ngoài công trình: tiền tối đa (chụp cả trong lẫn ngoài thì theo PriceMin / PriceMore)
	PriceOut int64 `json:"price_out"`
	// Hồ sơ có ít nhất 1 video công trình (và đủ ảnh để được trả tiền): cộng thêm
	VideoBonus int64 `json:"video_bonus"`
}

// DefaultSettings: giá trị khi bảng settings trống.
var DefaultSettings = Settings{PriceMin: 35000, PriceMore: 50000, MinPhotos: 5, PayMin: 5, DupDays: 30, DupRadiusM: 50, PriceOut: 35000,
	VideoBonus: 10000}

// WithVideo: tiền hồ sơ cộng thưởng video (chỉ khi hồ sơ được trả tiền và có video).
func (s Settings) WithVideo(amount int64, videos int) int64 {
	if amount > 0 && videos > 0 {
		return amount + s.VideoBonus
	}
	return amount
}

// AmountFor: tiền 1 hồ sơ theo số ảnh hợp lệ và loại chụp (chỉ ngoài: không quá PriceOut).
func (s Settings) AmountFor(valid int, scope string) int64 {
	a := s.Amount(valid)
	if scope == ScopeOut && a > s.PriceOut {
		return s.PriceOut
	}
	return a
}

// MaxPhotos: số ảnh tối đa của 1 hồ sơ.
const MaxPhotos = 20

// Amount: tiền 1 hồ sơ theo số ảnh hợp lệ.
func (s Settings) Amount(valid int) int64 {
	switch {
	case valid < s.MinPhotos:
		return 0
	case valid == s.MinPhotos:
		return s.PriceMin
	default:
		return s.PriceMore
	}
}
