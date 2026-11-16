package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

// Đồng bộ hồ sơ CTV gửi qua Messenger (dezon-inbox, chatbot "Cập nhật công trình") vào hệ thống nghiệm thu:
// mỗi project_logs thành 1 hồ sơ nguồn "inbox", ảnh tải từ kho S3 qua cùng luồng kiểm ảnh như app
// (trùng ảnh, AI đọc Timemark), rồi gửi duyệt và kiểm trùng công trình (trùng: tự từ chối).
// Người gửi ghép vào CTV cùng SĐT; chưa có thì tự tạo tài khoản CTV (chưa đăng nhập được tới khi admin đặt mật khẩu).

const (
	ActInbox      = "Nhận hồ sơ từ Messenger"
	syncCursorKey = "inbox_cursor"
	syncStatusKey = "inbox_status"
	syncBatch     = 20
)

// SyncStatus: lần đồng bộ Messenger gần nhất (trang admin).
type SyncStatus struct {
	Enabled  bool      `json:"enabled"`
	Running  bool      `json:"running"`
	At       *time.Time `json:"at"`
	Added    int       `json:"added"`    // hồ sơ mới lần gần nhất
	Rejected int       `json:"rejected"` // trong đó tự từ chối vì trùng công trình
	Error    string    `json:"error"`
	Total    int       `json:"total"` // tổng hồ sơ từ Messenger
}

type inboxSync struct {
	src   inbox.Source
	users repository.UserRepository
	auth  repository.AuthRepository
	state repository.SyncRepository
	mu    sync.Mutex
	run   bool
	runMu sync.Mutex
}

var (
	coordRE = regexp.MustCompile(`(-?\d{1,3}\.\d+)\s*°?\s*([NSns])?[\s,;]+(-?\d{1,3}\.\d+)\s*°?\s*([EWew])?`)
	bankNo  = regexp.MustCompile(`\d[\d .-]{5,22}\d`)
)

// parseCoord: "11.070510°N, 106.682880°E" -> lat, lng.
func parseCoord(s string) (*float64, *float64) {
	m := coordRE.FindStringSubmatch(s)
	if m == nil {
		return nil, nil
	}
	lat, err1 := strconv.ParseFloat(m[1], 64)
	lng, err2 := strconv.ParseFloat(m[3], 64)
	if err1 != nil || err2 != nil {
		return nil, nil
	}
	if strings.EqualFold(m[2], "S") {
		lat = -lat
	}
	if strings.EqualFold(m[4], "W") {
		lng = -lng
	}
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 || (lat == 0 && lng == 0) {
		return nil, nil
	}
	return &lat, &lng
}

// stageOf: tiến độ bot ghi ("móng", "tầng 2", "hoàn thiện cơ bản"...) -> giai đoạn của hệ thống.
func stageOf(s string) string {
	t := strings.ToLower(s)
	switch {
	case t == "":
		return ""
	case strings.Contains(t, "hoàn công") || strings.Contains(t, "đã xong") || strings.Contains(t, "bàn giao"):
		return model.Stages[3]
	case strings.Contains(t, "hoàn thiện") || strings.Contains(t, "sơn") || strings.Contains(t, "trát") || strings.Contains(t, "ốp"):
		return model.Stages[2]
	case strings.Contains(t, "móng") || strings.Contains(t, "cọc") || strings.Contains(t, "đà kiềng"):
		return model.Stages[0]
	case strings.Contains(t, "thô") || strings.Contains(t, "tầng") || strings.Contains(t, "sàn") || strings.Contains(t, "cột") ||
		strings.Contains(t, "mái") || strings.Contains(t, "dầm"):
		return model.Stages[1]
	}
	return ""
}

// bankOf: ô "Số tài khoản" bot ghi dạng tự do -> "Ngân hàng · Số TK · CHỦ TK" nếu tách được, không thì giữ nguyên chữ.
func bankOf(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	name := ""
	for _, b := range Banks {
		if strings.Contains(lower, strings.ToLower(b)) && len(b) > len(name) {
			name = b
		}
	}
	no := CleanPhone(bankNo.FindString(s))
	if name == "" || !bankNoRE.MatchString(no) {
		return truncate(s, 200)
	}
	owner := strings.Replace(s, bankNo.FindString(s), " ", 1)
	owner = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(name)).ReplaceAllString(owner, " ")
	owner = strings.Trim(strings.Join(strings.Fields(nonWordSpace.ReplaceAllString(owner, " ")), " "), " ")
	if len([]rune(owner)) < 4 {
		owner = ""
	}
	return BankFrom(name, no, strings.ToUpper(owner))
}

var nonWordSpace = regexp.MustCompile(`[^\p{L}\s]`)

func normPhone(s string) string {
	p := CleanPhone(strings.TrimSpace(s))
	if strings.HasPrefix(p, "+84") {
		p = "0" + p[3:]
	} else if strings.HasPrefix(p, "84") && len(p) == 11 {
		p = "0" + p[2:]
	}
	return p
}

// Chatbot có khi ghi 2 số vào 1 ô: "0933.547.039 - 0922.121.030", "0945678300 / 0939793908".
// Gạch nối có dấu cách hai bên mới là ngăn cách (gạch trong số như 0933-547-039 thì không).
var phoneSep = regexp.MustCompile(`\s+[-–—]\s+|[/,;|]|\s+(?i:và|hoặc|or)\s+`)

// firstPhone: SĐT hợp lệ đầu tiên trong ô; không tách được số hợp lệ nào thì giữ như normPhone.
func firstPhone(s string) string {
	for _, part := range phoneSep.Split(s, -1) {
		p := normPhone(part)
		if ValidPhone(p) {
			return p
		}
		// 2 số dính liền không dấu ngăn cách: thử cắt sau 10 / 11 chữ số
		for _, cut := range []int{10, 11} {
			if len(p) > cut && ValidPhone(p[:cut]) && ValidPhone(normPhone(p[cut:])) {
				return p[:cut]
			}
		}
	}
	return normPhone(s)
}

func randomHash() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "!inbox:" + hex.EncodeToString(b) // không phải bcrypt: không đăng nhập được tới khi admin đặt mật khẩu
}

// inboxCTV: CTV của người gửi bên Messenger (đã ghép / cùng SĐT / tạo mới).
func (s *houseService) inboxCTV(ctx context.Context, c inbox.Contact) (*model.User, error) {
	x := s.inbox
	if u, err := x.users.ByInboxContact(ctx, c.ID); err == nil {
		if u.Bank == "" { // bot ghi nhận số tài khoản sau lần đầu
			if b := bankOf(c.Profile["so_tai_khoan"]); b != "" {
				_ = x.users.UpdateBank(ctx, u.ID, b)
			}
		}
		return u, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	phone := normPhone(c.Phone)
	if ValidPhone(phone) {
		u, err := x.auth.FindByLogin(ctx, phone)
		switch {
		case err == nil && u.Role == model.RoleCTV:
			if err := x.users.SetInboxContact(ctx, u.ID, c.ID); err != nil && !errors.Is(err, repository.ErrDuplicate) {
				return nil, err
			}
			return u, nil
		case err == nil: // SĐT của nhân viên: không ghép
			phone = ""
		case !errors.Is(err, repository.ErrNotFound):
			return nil, err
		}
	} else {
		phone = ""
	}
	login := phone
	if login == "" {
		login = "mess_" + strings.ReplaceAll(c.ID, "-", "")[:12]
	}
	name := truncate(strings.TrimSpace(c.Name), 100)
	if name == "" {
		name = "CTV Messenger " + login
	}
	u := &model.User{Username: login, Name: name, Role: model.RoleCTV, PasswordHash: randomHash(),
		Bank: bankOf(c.Profile["so_tai_khoan"])}
	if err := x.auth.CreateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("tạo CTV từ Messenger: %w", err)
	}
	if err := x.users.SetInboxContact(ctx, u.ID, c.ID); err != nil {
		return nil, err
	}
	log.Printf("Messenger: tạo CTV #%d %q", u.ID, name)
	return u, nil
}

// syncOne: 1 hồ sơ Messenger. added: tạo hồ sơ mới; rejected: tự từ chối vì trùng công trình.
func (s *houseService) syncOne(ctx context.Context, l inbox.Log) (added, rejected bool, err error) {
	x := s.inbox
	if len(l.Images) == 0 {
		return false, false, nil
	}
	ctv, err := s.inboxCTV(ctx, l.Contact)
	if err != nil {
		return false, false, err
	}
	hid, created, err := s.houses.CreateInbox(ctx, ctv.ID, l.ID, l.CreatedAt)
	if err != nil {
		return false, false, err
	}
	h, err := s.houses.Get(ctx, hid)
	if err != nil {
		return false, false, err
	}
	if !created && h.Status != model.StatusPending && h.Status != model.StatusDraft {
		return false, false, nil // admin đã xử lý: không đụng
	}
	lat, lng := parseCoord(l.Fields["toa_do"])
	addr := truncate(l.Fields["dia_chi"], MaxAddressLen)
	files, metas := []Upload{}, []Meta{}
	for i, im := range l.Images {
		if i >= model.MaxPhotos {
			break
		}
		b, err := x.src.Image(ctx, im.Key)
		if err != nil {
			return false, false, fmt.Errorf("tải ảnh %s: %w", im.Key, err)
		}
		files = append(files, Upload{Name: fmt.Sprintf("Ảnh %d", i+1), Data: b})
		metas = append(metas, Meta{Lat: lat, Lng: lng, Addr: addr})
	}
	ok, _, err := s.takeFiles(ctx, hid, files, metas)
	if err != nil {
		return false, false, err
	}
	sys := &model.User{}
	if created {
		if ok == 0 { // không ảnh nào nhận được (đều đã nộp ở hồ sơ khác)
			_ = s.houses.Delete(ctx, hid)
			return false, false, nil
		}
		f := CleanFields(model.HouseFields{Address: addr, ContractorName: l.Fields["nha_thau"],
			ContractorPhone: firstPhone(l.Fields["sdt_nha_thau"]), Stage: stageOf(l.Fields["tien_do"])})
		if err := s.houses.UpdateFields(ctx, hid, f); err != nil {
			return false, false, err
		}
		note := "Fanpage Messenger, " + strconv.Itoa(ok) + " ảnh"
		if t := l.Fields["loai_cong_trinh"]; t != "" {
			note += ", " + t
		}
		if t := l.Fields["tien_do"]; t != "" {
			note += ", tiến độ: " + t
		}
		s.note(ctx, sys, ActInbox, hid, truncate(note, MaxReasonLen))
	} else if ok == 0 {
		return false, false, nil // không có ảnh mới
	}
	if s.ai.Enabled() {
		s.aiSem <- struct{}{}
		_ = s.houses.SetAIState(ctx, hid, model.AIReading)
		actx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		if err := s.runAI(actx, hid); err != nil {
			log.Printf("AI hồ sơ Messenger #%d: %v", hid, err)
			_ = s.houses.ApplyAI(ctx, hid, model.HouseFields{}, false)
		}
		cancel()
		<-s.aiSem
	}
	if !created { // hồ sơ đang chờ duyệt có thêm ảnh: cập nhật số tiền theo ảnh đạt, giữ trạng thái
		return false, false, nil
	}
	if h, err = s.houses.Get(ctx, hid); err != nil {
		return false, false, err
	}
	ps, err := s.photos.ListByHouse(ctx, hid)
	if err != nil {
		return false, false, err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return false, false, err
	}
	valid := FillProblems(ps, s.now())
	for _, p := range ps {
		if len(p.Problems) == 0 && p.Lat != nil && p.Lng != nil {
			lat, lng = p.Lat, p.Lng
			break
		}
	}
	hit, err := s.checkSite(ctx, hid, lat, lng, h.ContractorPhone)
	if err != nil {
		return false, false, err
	}
	warn := ""
	if hit != nil && !hit.Block {
		warn = truncate(hit.Warn, MaxReasonLen)
	}
	if err := s.houses.Submit(ctx, hid, st.Amount(valid), lat, lng, warn, s.drive != nil); err != nil {
		return false, false, err
	}
	s.assignProvince(ctx, hid)
	if hit != nil && hit.Block {
		if err := s.houses.MarkDuplicate(ctx, hid, hit.Of.ID, hit.Reason); err != nil {
			return false, false, err
		}
		s.note(ctx, sys, ActAutoDup, hid, hit.Reason)
		return true, true, nil
	}
	return true, false, nil
}

// SyncInbox: lấy các hồ sơ Messenger mới (gọi định kỳ và khi admin bấm "Đồng bộ ngay").
func (s *houseService) SyncInbox(ctx context.Context) (*SyncStatus, error) {
	x := s.inbox
	if x == nil {
		return &SyncStatus{}, fail(ErrInvalidInput, "Chưa cấu hình kết nối dezon-inbox (INBOX_DATABASE_URL).")
	}
	if !x.mu.TryLock() {
		return s.InboxStatus(ctx)
	}
	defer x.mu.Unlock()
	x.runMu.Lock()
	x.run = true
	x.runMu.Unlock()
	defer func() { x.runMu.Lock(); x.run = false; x.runMu.Unlock() }()

	res := SyncStatus{Enabled: true, At: ptr(s.now())}
	err := s.syncAll(ctx, &res)
	if err != nil {
		res.Error = truncate(err.Error(), 300)
		log.Printf("Đồng bộ Messenger: %v", err)
	} else if res.Added > 0 {
		log.Printf("Đồng bộ Messenger: %d hồ sơ mới, %d tự từ chối vì trùng", res.Added, res.Rejected)
	}
	if raw, e := json.Marshal(res); e == nil {
		_ = x.state.Set(ctx, syncStatusKey, string(raw))
	}
	if res.Added > 0 && s.drive != nil {
		go s.RunDriveSync()
	}
	out, _ := s.InboxStatus(ctx)
	return out, nil
}

func (s *houseService) syncAll(ctx context.Context, res *SyncStatus) error {
	x := s.inbox
	cur, _, err := x.state.Get(ctx, syncCursorKey)
	if err != nil {
		return err
	}
	at, id := time.Time{}, ""
	if t, rest, ok := strings.Cut(cur, "|"); ok {
		if p, err := time.Parse(time.RFC3339Nano, t); err == nil {
			at, id = p, rest
		}
	}
	for {
		logs, err := x.src.Logs(ctx, at, id, syncBatch)
		if err != nil {
			return fmt.Errorf("đọc dezon-inbox: %w", err)
		}
		for _, l := range logs {
			added, rejected, err := s.syncOne(ctx, l)
			if err != nil {
				return fmt.Errorf("hồ sơ Messenger %s: %w", l.ID, err)
			}
			if added {
				res.Added++
			}
			if rejected {
				res.Rejected++
			}
			at, id = l.UpdatedAt, l.ID
			if err := x.state.Set(ctx, syncCursorKey, at.Format(time.RFC3339Nano)+"|"+id); err != nil {
				return err
			}
		}
		if len(logs) < syncBatch {
			return nil
		}
	}
}

func (s *houseService) InboxStatus(ctx context.Context) (*SyncStatus, error) {
	out := &SyncStatus{}
	if s.inbox == nil {
		return out, nil
	}
	raw, _, err := s.inbox.state.Get(ctx, syncStatusKey)
	if err != nil {
		return nil, err
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), out)
	}
	out.Enabled = true
	s.inbox.runMu.Lock()
	out.Running = s.inbox.run
	s.inbox.runMu.Unlock()
	n, err := s.houses.CountSource(ctx, "inbox")
	out.Total = n
	return out, err
}

// StartInboxSync: đồng bộ ngay lúc khởi động rồi mỗi every.
func (s *houseService) StartInboxSync(every time.Duration) {
	if s.inbox == nil || every <= 0 {
		return
	}
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			_, _ = s.SyncInbox(ctx)
			cancel()
			time.Sleep(every)
		}
	}()
}
