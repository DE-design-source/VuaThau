package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"vuathau/backend/internal/integration/ai"
	"vuathau/backend/internal/integration/gdrive"
	"vuathau/backend/internal/integration/geo"
	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/integration/ocr"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/imaging"
	"vuathau/backend/internal/pkg/safe"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/storage"
)

// Upload: 1 file ảnh gửi lên.
type Upload struct {
	Name string
	Data []byte
}

// Meta: thông tin camera trong app gửi kèm mỗi ảnh (ảnh vẽ lại không có EXIF): giờ chụp, GPS, địa chỉ.
type Meta struct {
	T    string   `json:"t"`
	Lat  *float64 `json:"lat"`
	Lng  *float64 `json:"lng"`
	Addr string   `json:"addr"`
	// nhà thầu app quét trên bảng hiệu (CTV bấm Dùng / chụp trúng bảng hiệu); điền khi hồ sơ chưa có
	CtName  string `json:"ct_name"`
	CtPhone string `json:"ct_phone"`
}

func (m Meta) clean() Meta {
	out := Meta{Addr: truncate(m.Addr, MaxAddressLen), CtName: truncate(strings.TrimSpace(m.CtName), 200), CtPhone: validOrEmpty(CleanPhone(m.CtPhone))}
	if minuteRE.MatchString(m.T) {
		out.T = m.T
	}
	if m.Lat != nil && m.Lng != nil && *m.Lat >= -90 && *m.Lat <= 90 && *m.Lng >= -180 && *m.Lng <= 180 && (*m.Lat != 0 || *m.Lng != 0) {
		out.Lat, out.Lng = m.Lat, m.Lng
	}
	return out
}

// ParseMetas: JSON mảng (nhiều ảnh) hoặc object (1 ảnh). Sai dạng: bỏ qua.
func ParseMetas(s string) []Meta {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var list []Meta
	if json.Unmarshal([]byte(s), &list) == nil {
		return list
	}
	var one Meta
	if json.Unmarshal([]byte(s), &one) == nil {
		return []Meta{one}
	}
	return nil
}

// Tab: 1 mục lọc ở trang Hồ sơ của CTV.
type Tab struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

var ctvTabs = []struct {
	key, label string
	statuses   []string
}{
	{"", "Tất cả", model.StatusOrder},
	{"can_lam", "Cần xử lý", []string{model.StatusDraft, model.StatusReturned}},
	{"dang_duyet", "Đang duyệt", []string{model.StatusPending}},
	{"da_duyet", "Đã duyệt", []string{model.StatusApproved}},
	{"da_nhan", "Đã nhận tiền", []string{model.StatusPaid}},
	{"tu_choi", "Từ chối", []string{model.StatusRejected}},
}

type CTVHome struct {
	Rows  []model.House `json:"rows"`
	Tabs  []Tab         `json:"tabs"`
	Total int           `json:"total"`
	Money model.Money   `json:"money"`
	Todo  int           `json:"todo"`
}

type Income struct {
	Money       model.Money        `json:"money"`
	Bank        model.Bank         `json:"bank"`
	UserBank    string             `json:"user_bank"`
	Pays        []model.Payout     `json:"pays"`
	Adjustments []model.Adjustment `json:"adjustments"`
}

type CheckResult struct {
	Problems []string `json:"problems"`
	AI       bool     `json:"ai"`
}

type UploadResult struct {
	Message  string   `json:"message"`
	Messages []string `json:"messages"`
	HouseID  *int64   `json:"house_id"`
	Added    int      `json:"added"`
}

type Issue struct {
	Index    int      `json:"index"`
	Problems []string `json:"problems"`
}

type HouseDetail struct {
	House      *model.House     `json:"house"`
	Photos     []model.Photo    `json:"photos"`
	Videos     []model.Video    `json:"videos"`
	Logs       []model.Log      `json:"logs"`
	Steps      []model.Step     `json:"steps"`
	Checklist  []CheckItem      `json:"checklist"`
	Todo       []string         `json:"todo"`
	BadFields  []string         `json:"bad_fields"`
	Issues     []Issue          `json:"issues"`
	AIFields   []string         `json:"ai_fields"`
	Valid      int              `json:"valid"`
	Pay        int64            `json:"pay"`
	PayScope   map[string]int64 `json:"pay_scope"`   // tiền theo từng loại chụp (nút chọn ở trang duyệt), đã gồm thưởng video
	VideoBonus int64            `json:"video_bonus"` // thưởng hồ sơ có video (Cấu hình)
	Editable   bool             `json:"editable"`
	QueueLeft  int              `json:"queue_left"`
	Journal    []JournalSet     `json:"journal"` // nhật ký ảnh công trình: bộ ảnh đã duyệt cùng công trình, theo giai đoạn
}

type ReviewResult struct {
	Message string `json:"message"`
	NextID  *int64 `json:"next_id"`
	Left    int    `json:"left"`
}

// OverviewQuery: bộ lọc trang Hồ sơ (admin, kế toán).
type OverviewQuery struct {
	Status   string `json:"status"`
	Q        string `json:"q"`
	CTV      int64  `json:"ctv"`
	Province string `json:"province"`
	From     string `json:"from"`
	To       string `json:"to"`
	Page     int    `json:"-"`
}

type Overview struct {
	Rows      []model.House    `json:"rows"`
	Found     int              `json:"found"`
	Page      int              `json:"page"`
	Pages     int              `json:"pages"`
	Counts    map[string]int   `json:"counts"`
	Amounts   map[string]int64 `json:"amounts"`
	Chips     map[string]int   `json:"chips"`
	CTVTotal  int              `json:"ctv_total"`
	ReadyCTVs int              `json:"ready_ctvs"`
	CTVName   string           `json:"ctv_name"`
	Filter    OverviewQuery    `json:"filter"`
}

type HouseService interface {
	CTVHome(ctx context.Context, u *model.User, tab string) (*CTVHome, error)
	Income(ctx context.Context, u *model.User) (*Income, error)
	// Bản đồ trong app CTV: công trình của mình + chấm công trình CTV khác (chỉ toạ độ).
	CTVMap(ctx context.Context, u *model.User) (*CTVMap, error)
	Geocode(ctx context.Context, lat, lng float64) string
	// Tìm địa điểm theo địa chỉ (ô tìm trên bản đồ).
	SearchPlace(ctx context.Context, u *model.User, q string) (*PlaceSearch, error)
	CheckPhoto(ctx context.Context, u *model.User, f Upload, meta string) (*CheckResult, error)
	Create(ctx context.Context, u *model.User, files []Upload, meta string) (*UploadResult, error)
	AddPhotos(ctx context.Context, u *model.User, id int64, files []Upload, meta string) (*UploadResult, error)
	DeletePhoto(ctx context.Context, u *model.User, id, pid int64) (string, error)
	Save(ctx context.Context, u *model.User, id int64, f model.HouseFields, submit bool) (string, bool, error)
	State(ctx context.Context, u *model.User, id int64) (map[string]string, error)
	Detail(ctx context.Context, u *model.User, id int64) (*HouseDetail, error)
	// size: "" ảnh thu nhỏ 1568px, "card" ảnh cỡ thẻ 640px, "original" ảnh gốc
	PhotoFile(ctx context.Context, u *model.User, pid int64, size string) ([]byte, string, error)
	AddVideos(ctx context.Context, u *model.User, id int64, next func() (*VideoPart, error)) (*UploadResult, error)
	DeleteVideo(ctx context.Context, u *model.User, id, vid int64) (string, error)
	// VideoFile: đường dẫn file + kiểu của video theo link có chữ ký.
	VideoFile(ctx context.Context, vid int64, exp, sig string) (string, string, error)

	Queue(ctx context.Context) ([]model.House, error)
	ApproveMany(ctx context.Context, by *model.User, ids []int64) (string, int, error)
	Review(ctx context.Context, by *model.User, id int64, action, reason string) (*ReviewResult, error)
	Undo(ctx context.Context, by *model.User, id int64) (string, error)
	EditInfo(ctx context.Context, by *model.User, id int64, f model.HouseFields) (string, error)
	SetAmount(ctx context.Context, by *model.User, id int64, amount int64, reason string) (string, error)
	SetScope(ctx context.Context, by *model.User, id int64, scope string) (string, error)
	Split(ctx context.Context, by *model.User, id int64, in SplitInput) (*SplitResult, error)
	Overview(ctx context.Context, q OverviewQuery) (*Overview, error)

	// Bản đồ công trình + số công trình mỗi CTV đã chụp.
	Map(ctx context.Context, q OverviewQuery) (*MapData, error)

	// Trang sale (sale_service.go): không CTV, không tiền.
	SaleMap(ctx context.Context, province string, ctv int64) (*SaleMapData, error)
	// Khách hàng (lấy từ pipeline Kinh doanh của Dezon Pro): admin, kế toán xem
	Customers(ctx context.Context, u *model.User) ([]repository.Customer, error)
	SyncCustomers(ctx context.Context) (int, error)
	StartCustomerSync(every time.Duration)
	// Quy tắc duyệt ảnh cho AI (admin viết ở trang Cấu hình)
	AIRules(ctx context.Context) (*AIRules, error)
	SaveAIRules(ctx context.Context, by *model.User, rules string) (string, *AIRules, error)
	SavePhotoOrder(ctx context.Context, by *model.User, order string) (string, error)
	// Bản đồ công khai (chưa đăng nhập): như sale, không tên CTV; ảnh đại diện qua link có chữ ký.
	PublicMap(ctx context.Context, province string) (*SaleMapData, error)
	PublicProject(ctx context.Context, id int64) (*PublicProject, error)
	// ContractorSites: công trình CTV chụp có SĐT nhà thầu trùng phone (app nhà thầu)
	ContractorSites(ctx context.Context, phone string) ([]NTSite, error)
	PublicPhotoURL(pid int64) string
	PublicPhoto(ctx context.Context, pid int64, sig string) ([]byte, error)
	PublicSearchPlace(ctx context.Context, q string) (*PlaceSearch, error)
	SaleProjects(ctx context.Context, f repository.SaleFilter, page int) (*SaleProjectList, error)
	SaleProject(ctx context.Context, id int64) (*SaleProjectDetail, error)
	Contractors(ctx context.Context, q string) ([]repository.Contractor, error)

	// Hồ sơ Messenger (dezon-inbox): đồng bộ ngay, trạng thái, chạy định kỳ.
	SyncInbox(ctx context.Context) (*SyncStatus, error)
	InboxStatus(ctx context.Context) (*SyncStatus, error)
	StartInboxSync(every time.Duration)

	// Gán tỉnh mới (34 tỉnh) cho hồ sơ cũ chưa có, trả số hồ sơ đã gán.
	BackfillProvinces(ctx context.Context) (int, error)

	// RunDriveSync đẩy các hồ sơ chờ lên Google Drive (chạy nền, gọi lúc khởi động và sau mỗi lần gửi duyệt).
	RunDriveSync()
	// RunPhoneMaskScan: quét nền SĐT trên ảnh để che (chạy tới khi ctx huỷ)
	RunPhoneMaskScan(ctx context.Context)
	// Wait chờ các việc chạy nền (AI, Drive) xong: dùng khi tắt máy chủ và trong test.
	Wait()
}

type houseService struct {
	houses    repository.HouseRepository
	photos    repository.PhotoRepository
	logs      repository.LogRepository
	users     repository.UserRepository
	payments  repository.PaymentRepository
	settings  repository.SettingsRepository
	files     *storage.Files
	ai        ai.Reader
	ocr       ocr.Finder
	geo       Geocoder
	drive     *gdrive.Client // nil: tắt
	public    string
	inbox     *inboxSync                // nil: không lấy hồ sơ Messenger
	mediaKey  []byte                    // ký link xem video
	pub       publicMapCache            // bản đồ công khai: nhớ tạm theo tỉnh
	ctvGeo    ctvGeoCache               // toạ độ địa chỉ CTV (lớp Cộng tác viên trên bản đồ)
	sync      repository.SyncRepository // quy tắc duyệt ảnh của admin (khoá ai_rules)
	rules     aiRulesCache
	customers repository.CustomerRepository // khách hàng của CTV kinh doanh (nil: tắt)
	leads     LeadSource                    // nguồn khách hàng (dezon.tech; nil: tắt)

	bg      sync.WaitGroup
	aiSem   chan struct{} // giới hạn số lượt AI chạy cùng lúc
	driveMu sync.Mutex
	now     func() time.Time
}

// Geocoder: địa chỉ của toạ độ.
type Geocoder interface {
	Address(ctx context.Context, lat, lng float64) (string, error)
	Search(ctx context.Context, q string) ([]geo.Place, error)
}

type HouseDeps struct {
	Houses   repository.HouseRepository
	Photos   repository.PhotoRepository
	Logs     repository.LogRepository
	Users    repository.UserRepository
	Payments repository.PaymentRepository
	Settings repository.SettingsRepository
	Files    *storage.Files
	AI       ai.Reader
	Geo      Geocoder
	// OCR: tìm SĐT trên ảnh để che (nil: không che)
	OCR    ocr.Finder
	Drive  *gdrive.Client
	Public string
	// Hồ sơ Messenger: Inbox nil thì tắt; Auth (tạo CTV) và Sync (con trỏ đồng bộ) cần khi bật.
	Inbox inbox.Source
	Auth  repository.AuthRepository
	Sync  repository.SyncRepository
	// Customers + Leads: khách hàng lấy từ Dezon Pro (Leads nil: tắt)
	Customers repository.CustomerRepository
	Leads     LeadSource
	// MediaKey: khoá ký link xem video (trống: khoá ngẫu nhiên, link hết hiệu lực khi khởi động lại).
	MediaKey string
}

func NewHouseService(d HouseDeps) HouseService {
	if d.AI == nil {
		d.AI = ai.New(false, "", "")
	}
	s := &houseService{houses: d.Houses, photos: d.Photos, logs: d.Logs, users: d.Users, payments: d.Payments,
		settings: d.Settings, files: d.Files, ai: d.AI, ocr: d.OCR, geo: d.Geo, drive: d.Drive, public: d.Public,
		aiSem: make(chan struct{}, 4), now: time.Now, mediaKey: newMediaKey(d.MediaKey), sync: d.Sync, customers: d.Customers, leads: d.Leads}
	if d.Inbox != nil {
		s.inbox = &inboxSync{src: d.Inbox, users: d.Users, auth: d.Auth, state: d.Sync}
	}
	return s
}

func (s *houseService) Wait() { s.bg.Wait() }

func ptr[T any](v T) *T { return &v }

// withStats gắn ảnh bìa, số ảnh, số ảnh đạt cho danh sách hồ sơ.
func withStats(ctx context.Context, photos repository.PhotoRepository, rows []model.House) error {
	ids := make([]int64, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	by, err := photos.ListByHouses(ctx, ids)
	if err != nil {
		return err
	}
	now := time.Now()
	for i := range rows {
		ps := by[rows[i].ID]
		rows[i].PhotosTotal = len(ps)
		rows[i].PhotosValid = FillProblems(ps, now)
		if len(ps) > 0 {
			rows[i].CoverID = ptr(ps[0].ID)
		}
	}
	return nil
}

// load: hồ sơ + quyền xem (CTV chỉ xem hồ sơ của mình).
func (s *houseService) load(ctx context.Context, u *model.User, id int64) (*model.House, error) {
	h, err := s.houses.Get(ctx, id)
	// sale không xem hồ sơ đầy đủ (có CTV, tiền): sale dùng /sale/projects/{id}
	if errors.Is(err, repository.ErrNotFound) || (err == nil && u.Role == model.RoleCTV && h.CTVID != u.ID) || u.Role == model.RoleSale {
		return nil, fail(ErrNotFound, "Không tìm thấy hồ sơ.")
	}
	return h, err
}

func (s *houseService) note(ctx context.Context, by *model.User, action string, houseID int64, text string) {
	if err := s.logs.Add(ctx, by.ID, action, &houseID, nil, text); err != nil {
		log.Printf("ghi lịch sử hồ sơ #%d: %v", houseID, err)
	}
}

// ---------- CTV ----------

func (s *houseService) CTVHome(ctx context.Context, u *model.User, tab string) (*CTVHome, error) {
	statuses := model.StatusOrder
	for _, t := range ctvTabs {
		if t.key == tab {
			statuses = t.statuses
		}
	}
	all, err := s.houses.CTVStatusCounts(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	res := &CTVHome{}
	for _, t := range ctvTabs {
		n := 0
		for _, st := range t.statuses {
			n += all[st]
		}
		res.Tabs = append(res.Tabs, Tab{Key: t.key, Label: t.label, Count: n})
	}
	res.Total = res.Tabs[0].Count
	if res.Rows, err = s.houses.List(ctx, repository.HouseFilter{CTVID: u.ID, Statuses: statuses}, 500, 0); err != nil {
		return nil, err
	}
	if err := withStats(ctx, s.photos, res.Rows); err != nil {
		return nil, err
	}
	if err := s.markVIP(ctx, res.Rows); err != nil {
		return nil, err
	}
	if res.Money, err = s.houses.CTVMoney(ctx, u.ID); err != nil {
		return nil, err
	}
	res.Todo = all[model.StatusDraft] + all[model.StatusReturned]
	return res, nil
}

func (s *houseService) Income(ctx context.Context, u *model.User) (*Income, error) {
	res := &Income{Bank: BankParts(u.Bank), UserBank: u.Bank}
	var err error
	if res.Money, err = s.houses.CTVMoney(ctx, u.ID); err != nil {
		return nil, err
	}
	if res.Pays, err = s.payments.Payouts(ctx, u.ID); err != nil {
		return nil, err
	}
	res.Adjustments, err = s.payments.Adjustments(ctx, u.ID, true)
	return res, err
}

// PlaceSearch: kết quả ô tìm trên bản đồ: công trình trong hệ thống (tên / SĐT nhà thầu, địa chỉ) + địa điểm trên bản đồ.
type PlaceSearch struct {
	Sites  []repository.SiteHit `json:"sites"`
	Places []geo.Place          `json:"places"`
}

func (s *houseService) SearchPlace(ctx context.Context, u *model.User, q string) (*PlaceSearch, error) {
	res := &PlaceSearch{Sites: []repository.SiteHit{}, Places: []geo.Place{}}
	if len([]rune(strings.TrimSpace(q))) < 2 {
		return res, nil
	}
	ctv := int64(0)
	if u.Role == model.RoleCTV {
		ctv = u.ID // CTV: chỉ công trình của mình
	}
	sites, err := s.houses.SearchSites(ctx, q, ctv)
	if err != nil {
		return nil, err
	}
	res.Sites = sites
	if s.geo == nil {
		return res, nil
	}
	ps, err := s.geo.Search(ctx, q)
	if err != nil {
		log.Printf("tìm địa điểm %q: %v", q, err)
		if len(res.Sites) > 0 {
			return res, nil // đã có công trình khớp: không báo lỗi
		}
		return nil, fail(ErrUnavailable, "Chưa tìm được địa điểm, thử lại sau ít giây.")
	}
	res.Places = ps
	return res, nil
}

func (s *houseService) Geocode(ctx context.Context, lat, lng float64) string {
	if s.geo == nil {
		return ""
	}
	a, err := s.geo.Address(ctx, lat, lng)
	if err != nil {
		log.Printf("không lấy được địa chỉ %.4f,%.4f: %v", lat, lng, err)
		return ""
	}
	return a
}

func shaOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// dupHouse: hồ sơ có ảnh gần giống (dHash), nil nếu không có.
func (s *houseService) dupHouse(ctx context.Context, dhash string) (*int64, error) {
	// ponytail: quét toàn bộ dHash mỗi lần nộp, ổn tới ~100k ảnh; nhiều hơn thì chia theo vài bit đầu
	all, err := s.photos.AllDHashes(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range all {
		if imaging.Hamming(dhash, d.DHash) <= DupHamming {
			return ptr(d.HouseID), nil
		}
	}
	return nil, nil
}

func (s *houseService) CheckPhoto(ctx context.Context, u *model.User, f Upload, meta string) (*CheckResult, error) {
	if len(f.Data) > MaxFileBytes {
		return &CheckResult{Problems: []string{"Ảnh quá 20MB"}}, nil
	}
	sha := shaOf(f.Data)
	if hid, err := s.photos.HouseOfSHA(ctx, sha); err != nil {
		return nil, err
	} else if hid > 0 {
		return &CheckResult{Problems: []string{fmt.Sprintf("Ảnh này đã nộp ở hồ sơ #%d", hid)}}, nil
	}
	release, err := takeSlot(ctx)
	if err != nil {
		return nil, err
	}
	img, info, _, err := imaging.Open(f.Data)
	if err != nil {
		release()
		return &CheckResult{Problems: []string{err.Error()}}, nil
	}
	dh := imaging.DHash(img)
	release()
	dhashes.put(sha, dh) // lúc nộp dùng lại, khỏi giải nén lần 2
	dup, err := s.dupHouse(ctx, dh)
	if err != nil {
		return nil, err
	}
	read, aiOK := s.checkAI(ctx, sha, img)
	var it ai.PhotoRead
	hasIt := read != nil && len(read.Photos) > 0
	if hasIt {
		it = read.Photos[0]
	}
	m := Meta{}
	if ms := ParseMetas(meta); len(ms) > 0 {
		m = ms[0].clean()
	}
	p := model.Photo{ExifTime: info.ExifTime, DupOf: dup, Address: m.Addr}
	if p.ExifTime == "" {
		p.ExifTime = m.T
	}
	if hasIt {
		p.AITime = it.Time
		if it.Address != "" {
			p.Address = it.Address
		}
		p.IsSite = ptr(it.IsSite)
	}
	// vị trí: EXIF > camera app gửi kèm > AI đọc trên ảnh (Timemark có khi chỉ in toạ độ)
	switch {
	case info.Lat != nil:
		p.Lat = info.Lat
	case m.Lat != nil:
		p.Lat = m.Lat
	case hasIt && it.Lat != 0:
		p.Lat = ptr(it.Lat)
	}
	return &CheckResult{Problems: PhotoProblems(&p, s.now()), AI: aiOK}, nil
}

// priceOf: tiền 1 hồ sơ: theo số ảnh đạt + loại chụp, cộng thưởng nếu có video.
func (s *houseService) priceOf(ctx context.Context, st model.Settings, hid int64, valid int, scope string) (int64, error) {
	vs, err := s.photos.Videos(ctx, hid)
	if err != nil {
		return 0, err
	}
	return st.WithVideo(st.AmountFor(valid, scope), len(vs)), nil
}

// CheckAIWait: lúc CTV vừa chụp, chờ AI đọc ảnh tối đa chừng này rồi trả lời trước ("AI xem lại khi tải lên");
// AI vẫn đọc tiếp ở nền và lưu kết quả theo sha256, lúc nộp hồ sơ dùng lại không đọc lần nữa.
const CheckAIWait = 10 * time.Second

func (s *houseService) checkAI(ctx context.Context, sha string, img image.Image) (*ai.Read, bool) {
	type res struct {
		r  *ai.Read
		ok bool
	}
	ch := make(chan res, 1)
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		safe.Run("AI kiểm ảnh", func() {
			bctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			r, ok := s.cachedOrRead(bctx, sha, img)
			ch <- res{r, ok}
		})
	}()
	t := time.NewTimer(CheckAIWait)
	defer t.Stop()
	select {
	case x := <-ch:
		return x.r, x.ok
	case <-t.C:
		return nil, false
	case <-ctx.Done():
		return nil, false
	}
}

// decodeSlots: số ảnh được giải nén cùng lúc (ảnh điện thoại 12MP tốn ~50MB RAM mỗi ảnh khi mở):
// nhiều CTV tải cùng lúc thì xếp hàng thay vì làm API hết bộ nhớ.
var decodeSlots = make(chan struct{}, max(2, runtime.NumCPU()))

// dhashes: dHash của ảnh vừa kiểm lúc chụp (theo sha256), giữ tối đa dhashMax ảnh gần nhất trong bộ nhớ.
var dhashes = &dhashCache{m: map[string]string{}}

const dhashMax = 20000

type dhashCache struct {
	mu    sync.Mutex
	m     map[string]string
	order []string
}

func (c *dhashCache) get(sha string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[sha]
	return v, ok
}

func (c *dhashCache) put(sha, dh string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[sha]; ok {
		return
	}
	c.m[sha] = dh
	c.order = append(c.order, sha)
	if len(c.order) > dhashMax {
		delete(c.m, c.order[0])
		c.order = c.order[1:]
	}
}

// takeSlot: giữ 1 chỗ xử lý ảnh (trả hàm nhả); người dùng huỷ (ctx) khi đang chờ thì thôi.
func takeSlot(ctx context.Context) (func(), error) {
	select {
	case decodeSlots <- struct{}{}:
		return func() { <-decodeSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// cachedOrRead: kết quả AI đọc 1 ảnh, dùng lại theo sha256 nếu đã đọc. Tắt AI / lỗi: nil, false.
func (s *houseService) cachedOrRead(ctx context.Context, sha string, img image.Image) (*ai.Read, bool) {
	if c, err := s.photos.CacheGet(ctx, []string{sha}); err == nil {
		if raw, ok := c[sha]; ok {
			var r ai.Read
			if json.Unmarshal(raw, &r) == nil {
				return &r, true
			}
		}
	}
	if !s.ai.Enabled() {
		return nil, false
	}
	release, err := takeSlot(ctx)
	if err != nil {
		return nil, false
	}
	small, err := imaging.SmallJPEG(img, SmallSide)
	release()
	if err != nil {
		return nil, false
	}
	actx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	r, err := s.ai.ReadPhotos(ai.WithRules(actx, s.aiRules(actx)), [][]byte{small})
	if err != nil {
		log.Printf("AI kiểm 1 ảnh: %v", err)
		return nil, false
	}
	if raw, err := json.Marshal(r); err == nil {
		_ = s.photos.CachePut(ctx, sha, raw)
	}
	return r, true
}

// savePhoto lưu 1 ảnh vào hồ sơ. Trả câu báo lỗi nếu không nhận.
// draft: CTV gửi từ app (hồ sơ phải còn nháp / bị trả lại, kiểm lúc ghi); false: nhập từ Messenger.
func (s *houseService) savePhoto(ctx context.Context, houseID int64, data []byte, m Meta, draft bool) (string, error) {
	if len(data) > MaxFileBytes {
		return "ảnh quá 20MB", nil
	}
	sha := shaOf(data)
	if hid, err := s.photos.HouseOfSHA(ctx, sha); err != nil {
		return "", err
	} else if hid > 0 {
		return fmt.Sprintf("ảnh này đã nộp ở hồ sơ #%d", hid), nil
	}
	// dHash: ảnh camera app đã kiểm lúc chụp (/photos/check) thì dùng lại, chỉ đọc EXIF (không giải nén cả ảnh);
	// chưa có thì giải nén + băm (nặng CPU / RAM nhất, xếp hàng theo decodeSlots).
	// Ảnh thu nhỏ không tạo lúc nộp: storage tạo khi xem / AI đọc lần đầu.
	var info imaging.Info
	var format string
	dh, hit := dhashes.get(sha)
	if hit {
		var err error
		if info, format, err = imaging.Probe(data); err != nil {
			return err.Error(), nil
		}
	} else {
		release, err := takeSlot(ctx)
		if err != nil {
			return "", err
		}
		img, inf, f, err := imaging.Open(data)
		if err != nil {
			release()
			return err.Error(), nil
		}
		info, format, dh = inf, f, imaging.DHash(img)
		release()
	}
	dup, err := s.dupHouse(ctx, dh)
	if err != nil {
		return "", err
	}
	if err := s.files.SavePhoto(sha, data, nil); err != nil {
		return "", err
	}
	m = m.clean()
	p := &model.Photo{HouseID: houseID, SHA256: sha, DHash: dh, ExifTime: info.ExifTime, Address: m.Addr, DupOf: dup,
		Mime: imaging.MimeOf(format)}
	if p.ExifTime == "" {
		p.ExifTime = m.T
	}
	p.Lat, p.Lng = info.Lat, info.Lng
	if p.Lat == nil {
		p.Lat, p.Lng = m.Lat, m.Lng
	}
	insert := s.photos.Insert
	if draft {
		insert = func(ctx context.Context, p *model.Photo) error { return s.photos.InsertDraft(ctx, p, model.MaxPhotos) }
	}
	if err := insert(ctx, p); err != nil {
		switch {
		case errors.Is(err, repository.ErrDuplicate):
			return "ảnh này vừa được nộp", nil
		case errors.Is(err, repository.ErrFull):
			return fmt.Sprintf("mỗi hồ sơ tối đa %d ảnh", model.MaxPhotos), nil
		case errors.Is(err, repository.ErrNotEditable):
			return "hồ sơ đã gửi duyệt, không thêm ảnh được", nil
		}
		return "", err
	}
	return "", nil
}

// takeFiles lưu các ảnh vào hồ sơ, trả số ảnh nhận và các câu báo.
func (s *houseService) takeFiles(ctx context.Context, houseID int64, files []Upload, metas []Meta, draft bool) (int, []string, error) {
	have, err := s.photos.Count(ctx, houseID)
	if err != nil {
		return 0, nil, err
	}
	room := max(0, model.MaxPhotos-have)
	msgs := []string{}
	ok := 0
	addr, ctName, ctPhone := "", "", ""
	for k, f := range files {
		if k >= room {
			msgs = append(msgs, fmt.Sprintf("Mỗi hồ sơ tối đa %d ảnh, đã bỏ bớt ảnh thừa.", model.MaxPhotos))
			break
		}
		var m Meta
		if k < len(metas) {
			m = metas[k]
		}
		why, err := s.savePhoto(ctx, houseID, f.Data, m, draft)
		if err != nil {
			return ok, msgs, err
		}
		if why != "" {
			name := f.Name
			if name == "" {
				name = fmt.Sprintf("Ảnh %d", k+1)
			}
			msgs = append(msgs, name+": "+why)
			continue
		}
		ok++
		c := m.clean()
		if addr == "" {
			addr = c.Addr
		}
		if ctName == "" && ctPhone == "" && (c.CtName != "" || c.CtPhone != "") {
			ctName, ctPhone = c.CtName, c.CtPhone
		}
	}
	if ok > 0 && addr != "" { // chụp bằng camera app: điền sẵn địa chỉ cho CTV
		_ = s.houses.FillAddressIfEmpty(ctx, houseID, addr)
	}
	if ok > 0 && (ctName != "" || ctPhone != "") { // nhà thầu quét trên bảng hiệu
		_ = s.houses.FillContractorIfEmpty(ctx, houseID, ctName, ctPhone)
	}
	return ok, msgs, nil
}

func (s *houseService) uploadedMessage(ok int) string {
	if ok == 0 {
		return "Không ảnh nào được nhận, xem lý do bên dưới."
	}
	if s.ai.Enabled() {
		return fmt.Sprintf("Đã tải lên %d ảnh. AI đang đọc chữ trên ảnh.", ok)
	}
	return fmt.Sprintf("Đã tải lên %d ảnh. Đã kiểm tra từng ảnh.", ok)
}

func (s *houseService) Create(ctx context.Context, u *model.User, files []Upload, meta string) (*UploadResult, error) {
	if u.Role != model.RoleCTV {
		return nil, ErrForbidden
	}
	if len(files) == 0 {
		return nil, fail(ErrInvalidInput, "Chưa chọn ảnh nào.")
	}
	hid, err := s.houses.Create(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	ok, msgs, err := s.takeFiles(ctx, hid, files, ParseMetas(meta), true)
	if err != nil || ok == 0 {
		_ = s.houses.Delete(ctx, hid)
		if err != nil {
			return nil, err
		}
		return &UploadResult{Message: s.uploadedMessage(0), Messages: msgs}, nil
	}
	s.note(ctx, u, ActCreate, hid, "")
	s.startAI(hid)
	return &UploadResult{Message: s.uploadedMessage(ok), Messages: msgs, HouseID: ptr(hid), Added: ok}, nil
}

func (s *houseService) AddPhotos(ctx context.Context, u *model.User, id int64, files []Upload, meta string) (*UploadResult, error) {
	h, err := s.load(ctx, u, id)
	if err != nil {
		return nil, err
	}
	if u.Role != model.RoleCTV || !model.Editable(h.Status) {
		return nil, fail(ErrForbidden, "Hồ sơ đang được duyệt, không thêm ảnh được.")
	}
	ok, msgs, err := s.takeFiles(ctx, id, files, ParseMetas(meta), true)
	if err != nil {
		return nil, err
	}
	if ok > 0 {
		s.startAI(id)
	}
	return &UploadResult{Message: s.uploadedMessage(ok), Messages: msgs, HouseID: ptr(id), Added: ok}, nil
}

func (s *houseService) DeletePhoto(ctx context.Context, u *model.User, id, pid int64) (string, error) {
	h, err := s.load(ctx, u, id)
	if err != nil {
		return "", err
	}
	if u.Role != model.RoleCTV || !model.Editable(h.Status) {
		return "", fail(ErrForbidden, "Hồ sơ đang được duyệt, không xoá ảnh được.")
	}
	p, err := s.photos.DeleteDraft(ctx, pid, id)
	if errors.Is(err, repository.ErrNotFound) {
		return "", fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	if errors.Is(err, repository.ErrNotEditable) {
		return "", fail(ErrForbidden, "Hồ sơ đang được duyệt, không xoá ảnh được.")
	}
	if err != nil {
		return "", err
	}
	s.files.DeletePhoto(p.SHA256)
	return "Đã xoá ảnh.", nil
}

func (s *houseService) Save(ctx context.Context, u *model.User, id int64, f model.HouseFields, submit bool) (string, bool, error) {
	h, err := s.load(ctx, u, id)
	if err != nil {
		return "", false, err
	}
	if u.Role != model.RoleCTV || !model.Editable(h.Status) {
		return "", false, fail(ErrForbidden, "Hồ sơ đang được duyệt, không sửa được.")
	}
	if err := s.houses.UpdateDraftFields(ctx, id, CleanFields(f)); err != nil {
		if errors.Is(err, repository.ErrNotEditable) {
			return "", false, fail(ErrForbidden, "Hồ sơ đang được duyệt, không sửa được.")
		}
		return "", false, err
	}
	if !submit {
		return "Đã lưu nháp.", false, nil
	}
	if h.AIState == model.AIReading {
		return "AI đang kiểm tra ảnh, đợi vài giây rồi bấm Gửi duyệt.", false, nil
	}
	if h, err = s.houses.Get(ctx, id); err != nil {
		return "", false, err
	}
	ps, err := s.photos.ListByHouse(ctx, id)
	if err != nil {
		return "", false, err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return "", false, err
	}
	valid := FillProblems(ps, s.now())
	miss := []string{}
	for _, it := range Checklist(h, valid, st.MinPhotos) {
		if !it.OK {
			miss = append(miss, it.Message)
		}
	}
	if len(miss) > 0 {
		return "Chưa gửi được. Còn thiếu: " + strings.Join(miss, "; ") + ".", false, nil
	}
	var lat, lng *float64
	for _, p := range ps {
		if len(p.Problems) == 0 && p.Lat != nil && p.Lng != nil {
			lat, lng = p.Lat, p.Lng
			break
		}
	}
	// Chặn chụp trùng: công trình đã có hồ sơ trong hạn thì không cho gửi
	dup := ""
	hit, err := s.checkSite(ctx, id, lat, lng, h.ContractorPhone, true)
	if err != nil {
		return "", false, err
	}
	if hit != nil && hit.Block {
		s.note(ctx, u, "Chặn gửi: trùng công trình", id, hit.Reason)
		return fmt.Sprintf("Chưa gửi được: công trình này đã có người chụp ngày %s (hồ sơ #%d). Mỗi công trình chỉ nhận 1 hồ sơ trong %d ngày.",
			hit.Of.SubmittedAt.In(VN).Format("02/01/2006"), hit.Of.ID, st.DupDays), false, nil
	}
	if hit != nil {
		dup = truncate(hit.Warn, MaxReasonLen)
	}
	vs, err := s.photos.Videos(ctx, id)
	if err != nil {
		return "", false, err
	}
	amount := st.WithVideo(st.AmountFor(valid, h.EffScope()), len(vs))
	// gửi duyệt chỉ khi ảnh / video không đổi từ lúc đọc ở trên (tiền tính theo đúng bộ này)
	if err := s.houses.SubmitDraft(ctx, id, amount, lat, lng, dup, s.drive != nil, repository.SigOf(ps, vs)); err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound), errors.Is(err, repository.ErrNotEditable):
			return "", false, fail(ErrConflict, "Hồ sơ vừa đổi trạng thái, tải lại trang nhé.")
		case errors.Is(err, repository.ErrChanged):
			return "Ảnh hoặc video vừa thay đổi trong lúc gửi, bấm Gửi duyệt lại nhé.", false, nil
		}
		return "", false, err
	}
	s.note(ctx, u, ActSubmit, id, fmt.Sprintf("%d ảnh hợp lệ, %s", valid, Money(amount)))
	s.assignProvince(ctx, id)
	if s.drive != nil {
		go s.RunDriveSync()
	}
	return "Đã gửi hồ sơ cho admin duyệt.", true, nil
}

func (s *houseService) State(ctx context.Context, u *model.User, id int64) (map[string]string, error) {
	h, err := s.load(ctx, u, id)
	if err != nil {
		return nil, err
	}
	return map[string]string{"ai_state": h.AIState, "status": h.Status}, nil
}

func (s *houseService) Detail(ctx context.Context, u *model.User, id int64) (*HouseDetail, error) {
	h, err := s.load(ctx, u, id)
	if err != nil {
		return nil, err
	}
	ps, err := s.photos.ListByHouse(ctx, id)
	if err != nil {
		return nil, err
	}
	logs, err := s.logs.ForHouse(ctx, id, h.BatchID)
	if err != nil {
		return nil, err
	}
	m, err := s.houses.CTVMoney(ctx, h.CTVID)
	if err != nil {
		return nil, err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	_, left, err := s.houses.NextPending(ctx, 0)
	if err != nil {
		return nil, err
	}
	valid := FillProblems(ps, s.now())
	sortPhotos(ps, s.photoOrder(ctx)) // quy tắc xếp ảnh ở trang Cấu hình
	h.PhotosTotal, h.PhotosValid = len(ps), valid
	if idx, _, err := s.sites(ctx); err == nil {
		h.VIP = idx[h.ID].VIP
	}
	if len(ps) > 0 {
		h.CoverID = ptr(ps[0].ID)
	}
	vs, err := s.photos.Videos(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &HouseDetail{House: h, Photos: ps, Logs: logs, Valid: valid, Pay: st.WithVideo(st.AmountFor(valid, h.EffScope()), len(vs)), QueueLeft: left,
		PayScope: map[string]int64{
			model.ScopeOut:  st.WithVideo(st.AmountFor(valid, model.ScopeOut), len(vs)),
			model.ScopeBoth: st.WithVideo(st.AmountFor(valid, model.ScopeBoth), len(vs)),
		},
		VideoBonus: st.VideoBonus,
		Editable:   u.Role == model.RoleCTV && model.Editable(h.Status), Todo: []string{}, BadFields: []string{},
		Issues: []Issue{}, AIFields: []string{}}
	d.Videos = s.withSrc(vs)
	d.Steps = Steps(h, logs, m.Approved, st.PayMin)
	if d.Journal, err = s.journal(ctx, u, h, st.DupRadiusM); err != nil {
		return nil, err
	}
	d.Checklist = Checklist(h, valid, st.MinPhotos)
	for _, it := range d.Checklist {
		if it.OK {
			continue
		}
		d.Todo = append(d.Todo, it.Message)
		if it.Key != "anh" && h.Value(it.Key) != "" { // có giá trị nhưng sai dạng
			d.BadFields = append(d.BadFields, it.Key)
		}
	}
	for i, p := range ps {
		if len(p.Problems) > 0 {
			d.Issues = append(d.Issues, Issue{Index: i + 1, Problems: p.Problems})
		}
	}
	// ô còn giữ nguyên giá trị AI đọc
	if len(h.AIJSON) > 0 {
		var got map[string]any
		if json.Unmarshal(h.AIJSON, &got) == nil {
			for _, f := range model.Fields {
				v := fmt.Sprint(got[f.Key])
				if got[f.Key] != nil && v != "" && v != "0" && v == h.Value(f.Key) {
					d.AIFields = append(d.AIFields, f.Key)
				}
			}
		}
	}
	return d, nil
}

func (s *houseService) PhotoFile(ctx context.Context, u *model.User, pid int64, size string) ([]byte, string, error) {
	p, err := s.photos.Get(ctx, pid)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, "", fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	if err != nil {
		return nil, "", err
	}
	if u.Role == model.RoleSale {
		if ok, err := s.houses.SaleCanSeeHouse(ctx, p.HouseID); err != nil || !ok {
			return nil, "", fail(ErrNotFound, "Không tìm thấy ảnh.")
		}
	} else if _, err := s.load(ctx, u, p.HouseID); err != nil {
		return nil, "", err
	}
	// sale, CTV: ảnh có SĐT thì trả bản đã che; admin xem thử bản che: size "masked"
	if !seesOriginal(u) || size == "masked" {
		if size != "card" {
			size = ""
		}
		mb, err := s.maskedPhoto(ctx, p, size)
		if err != nil {
			log.Printf("che SĐT ảnh #%d: %v", p.ID, err)
			_ = s.photos.SetPhoneMask(ctx, p.ID, []byte("[]")) // ảnh không quét được: không chặn xem mãi
		} else if mb != nil {
			return mb, "image/jpeg", nil
		}
	}
	var b []byte
	switch size {
	case "card":
		b, err = s.files.ReadCard(p.SHA256)
	case "original":
		b, err = s.files.ReadPhoto(p.SHA256, false)
	default:
		b, err = s.files.ReadPhoto(p.SHA256, true)
	}
	if err != nil {
		return nil, "", fail(ErrNotFound, "Không tìm thấy file ảnh.")
	}
	mime := "image/jpeg"
	if size == "original" {
		mime = p.Mime
	}
	return b, mime, nil
}

// ---------- AI đọc ảnh (chạy nền) ----------

func (s *houseService) startAI(hid int64) {
	if !s.ai.Enabled() {
		return
	}
	if err := s.houses.SetAIState(context.Background(), hid, model.AIReading); err != nil {
		log.Printf("AI hồ sơ #%d: %v", hid, err)
		return
	}
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		s.aiSem <- struct{}{}
		defer func() { <-s.aiSem }()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		var err error
		if !safe.Run(fmt.Sprintf("AI hồ sơ #%d", hid), func() { err = s.runAI(ctx, hid) }) {
			err = errors.New("lỗi bất ngờ (panic)")
		}
		if err != nil {
			log.Printf("AI hồ sơ #%d: %v", hid, err)
			// không để hồ sơ kẹt ở "AI đang đọc"
			_ = s.houses.ApplyAI(context.Background(), hid, model.HouseFields{}, false)
		}
	}()
}

// runAI: AI đọc giờ chụp, địa chỉ, bảng nhà thầu; chỉ điền ô còn trống.
// Ảnh đã được AI kiểm lúc vừa chụp/chọn thì dùng lại kết quả, chỉ gửi AI những ảnh chưa đọc.
func (s *houseService) runAI(ctx context.Context, hid int64) error {
	ps, err := s.photos.ListByHouse(ctx, hid)
	if err != nil {
		return err
	}
	shas := make([]string, len(ps))
	for i, p := range ps {
		shas[i] = p.SHA256
	}
	cacheRaw, err := s.photos.CacheGet(ctx, shas)
	if err != nil {
		return err
	}
	cache := map[string]*ai.Read{}
	for sha, raw := range cacheRaw {
		var r ai.Read
		if json.Unmarshal(raw, &r) == nil {
			cache[sha] = &r
		}
	}
	todo := []model.Photo{}
	images := [][]byte{}
	for _, p := range ps {
		if cache[p.SHA256] != nil {
			continue
		}
		b, err := s.files.ReadPhoto(p.SHA256, true)
		if err != nil {
			continue
		}
		todo = append(todo, p)
		images = append(images, b)
	}
	var r *ai.Read
	ok := len(todo) == 0
	if len(images) > 0 {
		if r, err = s.ai.ReadPhotos(ai.WithRules(ctx, s.aiRules(ctx)), images); err != nil {
			log.Printf("AI hồ sơ #%d: %v", hid, err)
		} else {
			ok = true
		}
	}
	items := map[int64]ai.PhotoRead{}
	for _, p := range ps {
		if c := cache[p.SHA256]; c != nil && len(c.Photos) > 0 {
			items[p.ID] = c.Photos[0]
		}
	}
	if r != nil {
		for _, it := range r.Photos {
			if it.Index >= 0 && it.Index < len(todo) {
				items[todo[it.Index].ID] = it
			}
		}
	}
	for pid, it := range items {
		var lat, lng *float64
		if it.Lat != 0 || it.Lng != 0 {
			lat, lng = ptr(it.Lat), ptr(it.Lng)
		}
		if err := s.photos.ApplyAI(ctx, pid, it.Time, truncate(it.Address, MaxAddressLen), it.IsSite, lat, lng); err != nil {
			return err
		}
	}
	addr := ""
	for _, p := range ps {
		if it, has := items[p.ID]; has && it.Address != "" {
			addr = it.Address
			break
		}
	}
	reads := []*ai.Read{}
	for _, p := range ps {
		if c := cache[p.SHA256]; c != nil {
			reads = append(reads, c)
		}
	}
	if r != nil {
		reads = append(reads, r)
	}
	got := MergeReads(reads)
	got.Address = truncate(addr, MaxAddressLen)
	if err := s.houses.ApplyAI(ctx, hid, got, ok); err != nil {
		return err
	}
	// gợi ý loại chụp: thấy ít nhất 1 ảnh công trình chụp bên trong thì "trong + ngoài"
	if len(items) == 0 {
		return nil
	}
	scope := model.ScopeOut
	for _, it := range items {
		if it.IsSite && it.Inside {
			scope = model.ScopeBoth
			break
		}
	}
	return s.houses.SetScopeAI(ctx, hid, scope)
}

var nonWord = regexp.MustCompile(`[^\p{L}\p{N}]`)

// MergeReads gộp nhiều lượt AI đọc. Nhà thầu: tên thấy trên nhiều ảnh nhất, SĐT lấy cùng biển đó
// (không ghép tên công ty này với số công ty khác); không có thì số trên biển không ghi tên.
// Số tầng lớn nhất, giai đoạn xa nhất.
func MergeReads(rs []*ai.Read) model.HouseFields {
	key := func(s string) string { return nonWord.ReplaceAllString(strings.ToLower(s), "") }
	count := map[string]int{}
	for _, r := range rs {
		if r != nil && r.ContractorName != "" {
			count[key(r.ContractorName)]++
		}
	}
	var best *ai.Read
	for _, r := range rs {
		if r == nil || r.ContractorName == "" {
			continue
		}
		if best == nil {
			best = r
			continue
		}
		cb, cr := count[key(best.ContractorName)], count[key(r.ContractorName)]
		if cr > cb || (cr == cb && best.ContractorPhone == "" && r.ContractorPhone != "") {
			best = r
		}
	}
	out := model.HouseFields{}
	if best != nil {
		out.ContractorName = truncate(best.ContractorName, 200)
		for _, r := range rs {
			if r != nil && r.ContractorName != "" && key(r.ContractorName) == key(best.ContractorName) && r.ContractorPhone != "" {
				out.ContractorPhone = r.ContractorPhone
				break
			}
		}
	}
	if out.ContractorPhone == "" {
		for _, r := range rs {
			if r != nil && r.ContractorName == "" && r.ContractorPhone != "" {
				out.ContractorPhone = r.ContractorPhone
				break
			}
		}
	}
	out.ContractorPhone = CleanPhone(out.ContractorPhone)
	floors, stage := 0, -1
	for _, r := range rs {
		if r == nil {
			continue
		}
		floors = max(floors, r.Floors)
		if i := slices.Index(model.Stages, r.Stage); i > stage {
			stage = i
		}
	}
	if floors > 0 {
		out.Floors = ptr(floors)
	}
	if stage >= 0 {
		out.Stage = model.Stages[stage]
	}
	return out
}

// ---------- Admin ----------

func (s *houseService) Queue(ctx context.Context) ([]model.House, error) {
	rows, err := s.houses.List(ctx, repository.HouseFilter{Statuses: []string{model.StatusPending}}, ListLimit, 0)
	if err != nil {
		return nil, err
	}
	slices.Reverse(rows) // hồ sơ gửi trước duyệt trước
	return rows, withStats(ctx, s.photos, rows)
}

func (s *houseService) ApproveMany(ctx context.Context, by *model.User, ids []int64) (string, int, error) {
	if len(ids) == 0 {
		return "Chưa chọn hồ sơ nào đang chờ duyệt.", 0, nil
	}
	ok, err := s.houses.ApproveMany(ctx, ids)
	if err != nil {
		return "", 0, err
	}
	for _, id := range ok {
		s.note(ctx, by, ActApprove, id, "duyệt nhiều hồ sơ một lúc")
	}
	if len(ok) == 0 {
		return "Chưa chọn hồ sơ nào đang chờ duyệt.", 0, nil
	}
	return fmt.Sprintf("Đã duyệt %d hồ sơ.", len(ok)), len(ok), nil
}

func (s *houseService) Review(ctx context.Context, by *model.User, id int64, action, reason string) (*ReviewResult, error) {
	if _, err := s.load(ctx, by, id); err != nil {
		return nil, err
	}
	to, act, done := "", "", ""
	switch action {
	case "duyet":
		to, act, done = model.StatusApproved, ActApprove, "Đã duyệt"
	case "tra_lai":
		to, act, done = model.StatusReturned, ActReturn, "Đã trả lại"
	case "tu_choi":
		to, act, done = model.StatusRejected, ActReject, "Đã từ chối"
	default:
		return nil, fail(ErrInvalidInput, "Thao tác không hợp lệ.")
	}
	reason = truncate(reason, MaxReasonLen)
	if to != model.StatusApproved && reason == "" {
		return nil, fail(ErrInvalidInput, "Trả lại hoặc từ chối cần ghi lý do để CTV biết.")
	}
	if to == model.StatusApproved {
		reason = ""
	}
	changed, err := s.houses.SetStatus(ctx, id, []string{model.StatusPending}, to, reason)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, fail(ErrConflict, "Hồ sơ không ở trạng thái chờ duyệt.")
	}
	s.note(ctx, by, act, id, reason)
	next, left, err := s.houses.NextPending(ctx, id)
	if err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("%s hồ sơ #%d. ", done, id)
	if left > 0 {
		msg += fmt.Sprintf("Còn %d hồ sơ chờ duyệt.", left)
	} else {
		msg += "Đã xử lý hết hồ sơ chờ duyệt."
	}
	return &ReviewResult{Message: msg, NextID: next, Left: left}, nil
}

func (s *houseService) Undo(ctx context.Context, by *model.User, id int64) (string, error) {
	h, err := s.load(ctx, by, id)
	if err != nil {
		return "", err
	}
	changed, err := s.houses.SetStatus(ctx, id, []string{model.StatusApproved, model.StatusReturned, model.StatusRejected},
		model.StatusPending, "")
	if err != nil {
		return "", err
	}
	if !changed {
		return "", fail(ErrConflict, "Chỉ hoàn tác được hồ sơ đã duyệt, trả lại hoặc từ chối mà chưa thanh toán.")
	}
	s.note(ctx, by, ActUndo, id, "trước đó: "+model.Statuses[h.Status].Label)
	return fmt.Sprintf("Hồ sơ #%d đã về lại Chờ duyệt.", id), nil
}

func (s *houseService) EditInfo(ctx context.Context, by *model.User, id int64, f model.HouseFields) (string, error) {
	h, err := s.load(ctx, by, id)
	if err != nil {
		return "", err
	}
	if h.Status == model.StatusPaid {
		return "", fail(ErrForbidden, "Chỉ sửa được thông tin khi hồ sơ chưa thanh toán.")
	}
	nf := CleanFields(f)
	after := *h
	after.Address, after.Floors, after.Stage, after.ContractorName, after.ContractorPhone =
		nf.Address, nf.Floors, nf.Stage, nf.ContractorName, nf.ContractorPhone
	changed := []string{}
	for _, fl := range model.Fields {
		if h.Value(fl.Key) != after.Value(fl.Key) {
			changed = append(changed, fl.Label)
		}
	}
	if len(changed) == 0 {
		return "Không có gì thay đổi.", nil
	}
	if err := s.houses.UpdateFields(ctx, id, nf); err != nil {
		return "", err
	}
	s.note(ctx, by, ActEdit, id, strings.Join(changed, ", "))
	s.assignProvince(ctx, id)
	return "Đã sửa: " + strings.Join(changed, ", ") + ".", nil
}

func (s *houseService) SetAmount(ctx context.Context, by *model.User, id int64, amount int64, reason string) (string, error) {
	h, err := s.load(ctx, by, id)
	if err != nil {
		return "", err
	}
	if h.Status != model.StatusApproved {
		return "", fail(ErrForbidden, "Chỉ sửa được số tiền của hồ sơ đã duyệt, chưa thanh toán.")
	}
	reason = truncate(reason, 300)
	if amount < 0 || amount > 10_000_000 || reason == "" {
		return "", fail(ErrInvalidInput, "Chưa sửa: nhập số tiền (0 đến 10.000.000đ) và lý do.")
	}
	if err := s.houses.SetAmount(ctx, id, amount); err != nil {
		return "", err
	}
	s.note(ctx, by, ActAmount, id, fmt.Sprintf("%s → %s: %s", Money(h.Amount), Money(amount), reason))
	return fmt.Sprintf("Hồ sơ #%d giờ là %s.", id, Money(amount)), nil
}

// SetScope: admin chọn loại chụp (chỉ ngoài / trong + ngoài) khi duyệt; tiền hồ sơ tính lại theo số ảnh đạt.
func (s *houseService) SetScope(ctx context.Context, by *model.User, id int64, scope string) (string, error) {
	h, err := s.load(ctx, by, id)
	if err != nil {
		return "", err
	}
	if scope != model.ScopeOut && scope != model.ScopeBoth {
		return "", fail(ErrInvalidInput, "Loại chụp không hợp lệ.")
	}
	if h.Status != model.StatusPending && h.Status != model.StatusApproved {
		return "", fail(ErrForbidden, "Chỉ đổi được loại chụp của hồ sơ đang chờ duyệt hoặc đã duyệt, chưa thanh toán.")
	}
	ps, err := s.photos.ListByHouse(ctx, id)
	if err != nil {
		return "", err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return "", err
	}
	amount, err := s.priceOf(ctx, st, id, FillProblems(ps, s.now()), scope)
	if err != nil {
		return "", err
	}
	if err := s.houses.SetScope(ctx, id, scope, amount); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", fail(ErrConflict, "Hồ sơ vừa đổi trạng thái, tải lại trang nhé.")
		}
		return "", err
	}
	label := map[string]string{model.ScopeOut: "Chỉ chụp ngoài", model.ScopeBoth: "Chụp trong + ngoài"}[scope]
	s.note(ctx, by, ActScope, id, fmt.Sprintf("%s, tiền %s → %s", label, Money(h.Amount), Money(amount)))
	return fmt.Sprintf("Hồ sơ #%d: %s, %s.", id, label, Money(amount)), nil
}

// SplitInput: ảnh / video của công trình khác bị gộp vào hồ sơ, và địa chỉ công trình đó.
type SplitInput struct {
	Photos  []int64 `json:"photos"`
	Videos  []int64 `json:"videos"`
	Address string  `json:"address"`
}

type SplitResult struct {
	Message string `json:"message"`
	NewID   int64  `json:"new_id"`
}

// Split: admin tách ảnh / video của công trình khác (chatbot Messenger gắn nhầm vào cùng hồ sơ) ra 1 hồ sơ mới
// cùng CTV, chờ duyệt. Tính lại tiền cả 2 hồ sơ, ghi lịch sử cả 2.
func (s *houseService) Split(ctx context.Context, by *model.User, id int64, in SplitInput) (*SplitResult, error) {
	h, err := s.load(ctx, by, id)
	if err != nil {
		return nil, err
	}
	if h.Status != model.StatusPending {
		return nil, fail(ErrForbidden, "Chỉ tách được hồ sơ đang chờ duyệt.")
	}
	addr := strings.TrimSpace(in.Address)
	if addr == "" {
		return nil, fail(ErrInvalidInput, "Nhập địa chỉ công trình tách ra.")
	}
	if len(in.Photos) == 0 {
		return nil, fail(ErrInvalidInput, "Chọn ảnh của công trình cần tách.")
	}
	ps, err := s.photos.ListByHouse(ctx, id)
	if err != nil {
		return nil, err
	}
	pick := map[int64]bool{}
	for _, p := range in.Photos {
		pick[p] = true
	}
	n := 0
	for _, p := range ps {
		if pick[p.ID] {
			n++
		}
	}
	if n == 0 {
		return nil, fail(ErrInvalidInput, "Ảnh đã chọn không thuộc hồ sơ này, tải lại trang nhé.")
	}
	if n == len(ps) {
		return nil, fail(ErrInvalidInput, "Không tách hết ảnh: hồ sơ cũ phải còn ít nhất 1 ảnh (sửa địa chỉ nếu cả hồ sơ là công trình khác).")
	}
	vids := in.Videos
	if vids == nil {
		vids = []int64{}
	}
	nid, np, nv, err := s.houses.Split(ctx, id, in.Photos, vids, truncate(addr, 500))
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fail(ErrConflict, "Hồ sơ vừa đổi trạng thái, tải lại trang nhé.")
	}
	if err != nil {
		return nil, err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	amounts := map[int64]int64{}
	for _, hid := range []int64{id, nid} {
		ps, err := s.photos.ListByHouse(ctx, hid)
		if err != nil {
			return nil, err
		}
		x, err := s.houses.Get(ctx, hid)
		if err != nil {
			return nil, err
		}
		a, err := s.priceOf(ctx, st, hid, FillProblems(ps, s.now()), x.EffScope())
		if err != nil {
			return nil, err
		}
		if err := s.houses.SetAmount(ctx, hid, a); err != nil {
			return nil, err
		}
		amounts[hid] = a
	}
	s.assignProvince(ctx, nid)
	what := fmt.Sprintf("%d ảnh", np)
	if nv > 0 {
		what += fmt.Sprintf(", %d video", nv)
	}
	s.note(ctx, by, ActSplit, id, fmt.Sprintf("Tách %s sang hồ sơ #%d (%s); tiền %s → %s", what, nid, addr, Money(h.Amount), Money(amounts[id])))
	s.note(ctx, by, ActSplit, nid, fmt.Sprintf("Tách từ hồ sơ #%d: %s, %s", id, what, Money(amounts[nid])))
	return &SplitResult{NewID: nid, Message: fmt.Sprintf("Đã tách %s sang hồ sơ mới #%d (%s). #%d còn %s, #%d: %s.",
		what, nid, addr, id, Money(amounts[id]), nid, Money(amounts[nid]))}, nil
}

// dayRange: "YYYY-MM-DD" -> [đầu ngày, đầu ngày hôm sau) theo giờ Việt Nam.
func dayStart(s string) *time.Time {
	if !ValidDay(s) {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, VN)
	if err != nil {
		return nil
	}
	return &t
}

func (s *houseService) Overview(ctx context.Context, q OverviewQuery) (*Overview, error) {
	if _, ok := model.Statuses[q.Status]; !ok {
		q.Status = ""
	}
	q.Q = strings.TrimSpace(q.Q)
	q.Page = max(1, q.Page)
	if province.ByCode(q.Province) == nil {
		q.Province = ""
	}
	f := repository.HouseFilter{Q: q.Q, CTVID: q.CTV, Province: q.Province}
	if t := dayStart(q.From); t != nil {
		f.From = t
	} else {
		q.From = ""
	}
	if t := dayStart(q.To); t != nil {
		f.To = ptr(t.AddDate(0, 0, 1))
	} else {
		q.To = ""
	}
	res := &Overview{Filter: q, Page: q.Page}
	var err error
	if res.Counts, err = s.houses.CountByStatus(ctx, repository.HouseFilter{}); err != nil {
		return nil, err
	}
	if res.Amounts, err = s.houses.AmountByStatus(ctx); err != nil {
		return nil, err
	}
	if res.Chips, err = s.houses.CountByStatus(ctx, f); err != nil {
		return nil, err
	}
	if q.Status != "" {
		f.Statuses = []string{q.Status}
	}
	if res.Found, err = s.houses.Count(ctx, f); err != nil {
		return nil, err
	}
	res.Pages = pages(res.Found)
	if res.Rows, err = s.houses.List(ctx, f, PageSize, (q.Page-1)*PageSize); err != nil {
		return nil, err
	}
	if err := withStats(ctx, s.photos, res.Rows); err != nil {
		return nil, err
	}
	if err := s.markVIP(ctx, res.Rows); err != nil {
		return nil, err
	}
	if res.CTVTotal, err = s.users.CountCTVs(ctx); err != nil {
		return nil, err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if res.ReadyCTVs, err = s.payments.CountReady(ctx, st.PayMin); err != nil {
		return nil, err
	}
	if q.CTV > 0 {
		if u, err := s.users.GetByID(ctx, q.CTV); err == nil {
			res.CTVName = u.Name
		}
	}
	return res, nil
}

// ---------- Google Drive ----------

// RunDriveSync: đẩy hồ sơ chờ lên Google Drive (chạy nền, có bắt panic).
func (s *houseService) RunDriveSync() {
	safe.Run("đồng bộ Google Drive", s.runDriveSync)
}

func (s *houseService) runDriveSync() {
	if s.drive == nil {
		return
	}
	s.bg.Add(1)
	defer s.bg.Done()
	s.driveMu.Lock() // 1 lượt đồng bộ một lúc
	defer s.driveMu.Unlock()
	ctx := context.Background()
	hs, err := s.houses.DriveQueue(ctx)
	if err != nil {
		log.Printf("Google Drive: %v", err)
		return
	}
	for _, h := range hs {
		fid, err := s.drivePut(ctx, &h)
		if err != nil { // mạng, token hết hạn, thiếu quyền thư mục: ghi log, để lần sau
			log.Printf("Chưa lưu được hồ sơ #%d lên Google Drive: %v", h.ID, err)
			continue
		}
		if err := s.houses.SetDrive(ctx, h.ID, fid); err != nil {
			log.Printf("Google Drive hồ sơ #%d: %v", h.ID, err)
		}
	}
}

func (s *houseService) drivePut(ctx context.Context, h *model.House) (string, error) {
	ps, err := s.photos.ListByHouse(ctx, h.ID)
	if err != nil {
		return "", err
	}
	FillProblems(ps, s.now())
	var b strings.Builder
	fmt.Fprintf(&b, "Hồ sơ #%d: %s/houses/%d\n", h.ID, s.public, h.ID)
	fmt.Fprintf(&b, "Gửi duyệt: %s\n", s.now().In(VN).Format("15:04 02/01/2006"))
	// không ghi SĐT (nhà thầu, CTV): thư mục Drive có thể chia sẻ ra ngoài; ảnh trên Drive cũng đã che SĐT
	fmt.Fprintf(&b, "CTV: %s\n", h.CTVName)
	for _, f := range model.Fields {
		if f.Key == "contractor_phone" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", capFirst(f.Label), h.Value(f.Key))
	}
	fmt.Fprintf(&b, "Số tiền dự kiến: %s\n\nẢnh:\n", Money(h.Amount))
	for i, p := range ps {
		why := "hợp lệ"
		if len(p.Problems) > 0 {
			why = strings.Join(p.Problems, "; ")
		}
		fmt.Fprintf(&b, "%02d: %s\n", i+1, why)
	}
	files := []gdrive.File{{Name: "thong-tin.txt", Mime: "text/plain; charset=utf-8", Data: []byte(b.String())}}
	for i, p := range ps {
		// ảnh có SĐT: đẩy bản đã che (thư mục Drive chia sẻ ra ngoài)
		boxes, err := s.phoneMask(ctx, &p, h.ContractorPhone)
		if err != nil {
			return "", fmt.Errorf("che SĐT ảnh #%d: %w", p.ID, err)
		}
		if len(boxes) > 0 {
			data, err := s.files.ReadMasked(p.SHA256, false, boxes)
			if err != nil {
				return "", err
			}
			files = append(files, gdrive.File{Name: fmt.Sprintf("%02d.jpg", i+1), Mime: "image/jpeg", Data: data})
			continue
		}
		data, err := s.files.ReadPhoto(p.SHA256, false)
		if err != nil {
			return "", err
		}
		files = append(files, gdrive.File{Name: fmt.Sprintf("%02d.%s", i+1, imaging.ExtOf(p.Mime)), Mime: p.Mime, Data: data})
	}
	return s.drive.PutHouse(ctx, h.ID, fmt.Sprintf("HS%05d - %s - %s", h.ID, h.CTVName, h.Address), files)
}
