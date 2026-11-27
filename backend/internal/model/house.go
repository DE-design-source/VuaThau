package model

import "time"

// Trạng thái hồ sơ.
const (
	StatusDraft    = "nhap"      // CTV đang soạn
	StatusReturned = "tra_lai"   // admin trả lại, CTV sửa
	StatusPending  = "cho_duyet" // chờ admin duyệt
	StatusRejected = "tu_choi"
	StatusApproved = "da_duyet" // chờ kế toán thanh toán
	StatusPaid     = "da_chi"
)

// StatusOrder: thứ tự hiển thị.
var StatusOrder = []string{StatusDraft, StatusReturned, StatusPending, StatusRejected, StatusApproved, StatusPaid}

// StatusInfo: nhãn nội bộ, nhãn cho CTV, tông màu (draft|warn|wait|bad|ok|done).
type StatusInfo struct {
	Label    string `json:"label"`
	CTVLabel string `json:"ctv_label"`
	Tone     string `json:"tone"`
}

var Statuses = map[string]StatusInfo{
	StatusDraft:    {"Đang soạn", "Chưa gửi", "draft"},
	StatusReturned: {"Trả lại, chờ CTV sửa", "Cần sửa lại", "warn"},
	StatusPending:  {"Chờ duyệt", "Đang chờ duyệt", "wait"},
	StatusRejected: {"Từ chối", "Bị từ chối", "bad"},
	StatusApproved: {"Chờ thanh toán", "Đã duyệt", "ok"},
	StatusPaid:     {"Đã thanh toán", "Đã nhận tiền", "done"},
}

// CTV sửa được hồ sơ ở các trạng thái này.
func Editable(status string) bool { return status == StatusDraft || status == StatusReturned }

// AI đọc ảnh.
const (
	AIReading = "dang_doc"
	AIDone    = "xong"
	AIFailed  = "loi"
)

// Giai đoạn thi công (8 bước, theo thứ tự). Đổi danh sách: sửa cả hướng dẫn AI (integration/ai) và stageOf (inbox_sync).
var Stages = []string{"Chuẩn bị mặt bằng", "Ép cọc, đào móng", "Thi công móng", "Xây phần thô", "Xây tường, đổ mái", "Điện nước âm tường", "Hoàn thiện", "Bàn giao"}

// Field: các ô thông tin công trình CTV phải điền.
type Field struct{ Key, Label string }

var Fields = []Field{
	{"address", "địa chỉ công trình"}, {"contractor_name", "tên nhà thầu"}, {"contractor_phone", "SĐT nhà thầu"},
	{"floors", "số tầng"}, {"stage", "giai đoạn thi công"},
}

// House: 1 hồ sơ (1 căn nhà đang xây). Các trường ctv_* và cover/photos_* được ghép khi đọc.
type House struct {
	ID              int64             `json:"id"`
	CTVID           int64             `json:"ctv_id"`
	CTVName         string            `json:"ctv_name"`
	CTVPhone        string            `json:"ctv_phone,omitempty"`
	CTVBank         string            `json:"ctv_bank,omitempty"`
	Status          string            `json:"status"`
	Address         string            `json:"address"`
	Lat             *float64          `json:"lat"`
	Lng             *float64          `json:"lng"`
	ContractorName  string            `json:"contractor_name"`
	ContractorPhone string            `json:"contractor_phone"`
	Floors          *int              `json:"floors"`
	Stage           string            `json:"stage"`
	Note            string            `json:"note"`
	Dup             string            `json:"dup"`
	Amount          int64             `json:"amount"`
	AIState         string            `json:"ai_state"`
	AIJSON          []byte            `json:"-"`
	AIDiff          map[string]string `json:"ai_diff,omitempty"` // ô đang lưu khác giá trị AI đọc trên ảnh: khoá → giá trị AI
	BatchID         *int64            `json:"batch_id"`
	Drive           string            `json:"-"`
	CreatedAt       time.Time         `json:"created_at"`
	Source          string            `json:"source"` // app | inbox (Messenger)
	DupOf           *int64            `json:"dup_of"` // hồ sơ trùng công trình (đã chụp trước)
	SubmittedAt     *time.Time        `json:"submitted_at"`
	Province        string            `json:"province"` // mã tỉnh mới (34 tỉnh, từ 1/7/2025), vd VN79
	Scope           string            `json:"scope"`    // loại chụp admin chọn: ngoai | trong_ngoai ('' = theo AI / hồ sơ cũ)
	ScopeAI         string            `json:"scope_ai"` // AI gợi ý từ ảnh: ngoai | trong_ngoai ('' = chưa có)
	ProvinceName    string            `json:"province_name"`
	CoverID         *int64            `json:"cover_id"`
	PhotosTotal     int               `json:"photos_total"`
	PhotosValid     int               `json:"photos_valid"`
	VIP             bool              `json:"vip"` // chụp công trình này đầu tiên (không có hồ sơ nào gần đó gửi sớm hơn)
}

// Loại chụp: chỉ bên ngoài công trình / cả bên trong lẫn bên ngoài (tính tiền khác nhau).
const (
	ScopeOut  = "ngoai"
	ScopeBoth = "trong_ngoai"
)

// EffScope: loại chụp đang áp dụng: admin chọn, không thì AI gợi ý.
// Hồ sơ Messenger chưa ai chọn: chỉ chụp ngoài (góp ý 23/11/2026: hồ sơ Messenger không qua AI, trước đây bị tính tiền
// như chụp trong + ngoài). Hồ sơ app cũ chưa có gợi ý (chuỗi rỗng): tính tiền như trước.
func (h *House) EffScope() string {
	if h.Scope != "" {
		return h.Scope
	}
	if h.ScopeAI == "" && h.Source == "inbox" {
		return ScopeOut
	}
	return h.ScopeAI
}

// HouseFields: các ô thông tin công trình (CTV lưu, admin sửa).
type HouseFields struct {
	Address         string `json:"address"`
	Floors          *int   `json:"floors"`
	Stage           string `json:"stage"`
	ContractorName  string `json:"contractor_name"`
	ContractorPhone string `json:"contractor_phone"`
}

// Value lấy giá trị 1 ô theo khoá (để so sánh, kiểm tra thiếu).
func (h *House) Value(key string) string {
	switch key {
	case "address":
		return h.Address
	case "contractor_name":
		return h.ContractorName
	case "contractor_phone":
		return h.ContractorPhone
	case "stage":
		return h.Stage
	case "floors":
		if h.Floors != nil && *h.Floors > 0 {
			return itoa(*h.Floors)
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Photo: 1 ảnh của hồ sơ. Problems được tính khi đọc (luật kiểm ảnh).
type Photo struct {
	ID       int64    `json:"id"`
	HouseID  int64    `json:"house_id"`
	SHA256   string   `json:"-"`
	DHash    string   `json:"-"`
	ExifTime string   `json:"exif_time"`
	AITime   string   `json:"ai_time"`
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
	Address  string   `json:"address"`
	IsSite   *bool    `json:"is_site"`
	DupOf    *int64   `json:"dup_of"`
	Mime     string   `json:"-"`
	Problems []string `json:"problems"`
	// PhoneMask: vùng có SĐT cần che khi sale / CTV / Drive xem (JSON [{x,y,w,h}] tỉ lệ 0..1); nil = chưa quét.
	PhoneMask []byte `json:"-"`
	// TextMask: mọi dòng chữ trên ảnh (chế độ "Ẩn thông tin" trên web); nil = chưa quét (migration 030).
	TextMask []byte `json:"-"`
	// Masked: số SĐT đã che trên ảnh (admin thấy để biết)
	Masked int `json:"masked"`
	// CoverScore: AI chấm 0..10 độ hợp làm ảnh đại diện; 0 = chưa chấm (migration 031)
	CoverScore int `json:"cover_score"`
}

// Log: 1 dòng lịch sử thao tác trên hồ sơ / phiếu chi.
type Log struct {
	ID       int64     `json:"id"`
	HouseID  *int64    `json:"house_id"`
	BatchID  *int64    `json:"batch_id"`
	UserID   *int64    `json:"-"`
	UserName string    `json:"user_name"`
	Action   string    `json:"action"`
	Note     string    `json:"note"`
	At       time.Time `json:"at"`
}

// Step: 1 bước trong tiến trình 4 bước của hồ sơ.
type Step struct {
	Title string `json:"title"`
	Who   string `json:"who"`
	State string `json:"state"` // done | current | todo | bad
	At    string `json:"at"`
	Hint  string `json:"hint"`
}

// Video: 1 video công trình của hồ sơ (chỉ để xem thêm, không qua AI, không tính tiền).
type Video struct {
	ID        int64     `json:"id"`
	HouseID   int64     `json:"house_id"`
	SHA256    string    `json:"-"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Src       string    `json:"src"` // link xem có chữ ký (không cần token: thẻ <video> / trình phát không gửi được header)
}

// MaxVideos: số video tối đa mỗi hồ sơ.
const MaxVideos = 5
