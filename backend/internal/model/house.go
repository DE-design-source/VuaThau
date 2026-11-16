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

// Giai đoạn thi công.
var Stages = []string{"Đào móng / làm móng", "Xây phần thô", "Hoàn thiện", "Đã hoàn công"}

// Field: các ô thông tin công trình CTV phải điền.
type Field struct{ Key, Label string }

var Fields = []Field{
	{"address", "địa chỉ công trình"}, {"contractor_name", "tên nhà thầu"}, {"contractor_phone", "SĐT nhà thầu"},
	{"floors", "số tầng"}, {"stage", "giai đoạn thi công"},
}

// House: 1 hồ sơ (1 căn nhà đang xây). Các trường ctv_* và cover/photos_* được ghép khi đọc.
type House struct {
	ID              int64     `json:"id"`
	CTVID           int64     `json:"ctv_id"`
	CTVName         string    `json:"ctv_name"`
	CTVPhone        string    `json:"ctv_phone,omitempty"`
	CTVBank         string    `json:"ctv_bank,omitempty"`
	Status          string    `json:"status"`
	Address         string    `json:"address"`
	Lat             *float64  `json:"lat"`
	Lng             *float64  `json:"lng"`
	ContractorName  string    `json:"contractor_name"`
	ContractorPhone string    `json:"contractor_phone"`
	Floors          *int      `json:"floors"`
	Stage           string    `json:"stage"`
	Note            string    `json:"note"`
	Dup             string    `json:"dup"`
	Amount          int64     `json:"amount"`
	AIState         string    `json:"ai_state"`
	AIJSON          []byte    `json:"-"`
	BatchID         *int64    `json:"batch_id"`
	Drive           string    `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
	CoverID         *int64    `json:"cover_id"`
	PhotosTotal     int       `json:"photos_total"`
	PhotosValid     int       `json:"photos_valid"`
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
