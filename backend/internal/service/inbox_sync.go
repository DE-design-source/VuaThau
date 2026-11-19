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
	"unicode/utf8"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/province"
	"vuathau/backend/internal/repository"
)

// Đồng bộ hồ sơ CTV gửi qua Messenger (dezon-inbox, chatbot "Cập nhật công trình") vào hệ thống nghiệm thu:
// mỗi project_logs thành 1 hồ sơ nguồn "inbox", ảnh tải từ kho S3 qua luồng kiểm ảnh như app (trùng ảnh, vị trí),
// rồi gửi duyệt và kiểm trùng công trình (trùng: tự từ chối). KHÔNG dùng AI (không đọc chữ trên ảnh, không so ảnh):
// giờ chụp lấy theo giờ CTV gửi tin; admin duyệt, kế toán thanh toán như hồ sơ app. Chỉ ảnh CTV chụp và nộp qua app
// mới qua AI.
// Người gửi ghép vào CTV cùng SĐT; chưa có thì tự tạo tài khoản CTV (chưa đăng nhập được tới khi admin đặt mật khẩu).

const (
	ActInbox      = "Nhận hồ sơ từ Messenger"
	syncCursorKey = "inbox_cursor"
	syncStatusKey = "inbox_status"
	syncBatch     = 20
)

// SyncStatus: lần đồng bộ Messenger gần nhất (trang admin).
type SyncStatus struct {
	Enabled  bool       `json:"enabled"`
	Running  bool       `json:"running"`
	At       *time.Time `json:"at"`
	Added    int        `json:"added"`    // hồ sơ mới lần gần nhất
	Rejected int        `json:"rejected"` // trong đó tự từ chối vì trùng công trình
	Error    string     `json:"error"`
	Total    int        `json:"total"` // tổng hồ sơ từ Messenger
}

type inboxSync struct {
	src   inbox.Source
	users repository.UserRepository
	auth  repository.AuthRepository
	state repository.SyncRepository
	mu    sync.Mutex
	run   bool
	runMu sync.Mutex
	prune bool // lượt đọc lại 1 lần (resyncKey): gỡ ảnh không thuộc bản ghi chatbot khỏi hồ sơ đang chờ duyệt
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
	has := func(ws ...string) bool {
		for _, w := range ws {
			if strings.Contains(t, w) {
				return true
			}
		}
		return false
	}
	switch {
	case t == "":
		return ""
	case has("hoàn công", "đã xong", "bàn giao", "nghiệm thu"):
		return model.Stages[7]
	case has("hoàn thiện", "sơn", "trát", "ốp", "lát"):
		return model.Stages[6]
	case has("điện", "nước", "chống thấm"):
		return model.Stages[5]
	case has("tường", "mái", "cầu thang"):
		return model.Stages[4]
	case has("thô", "tầng", "sàn", "cột", "dầm"):
		return model.Stages[3]
	case has("cọc", "đào"):
		return model.Stages[1]
	case has("móng", "đà kiềng", "hầm phân"):
		return model.Stages[2]
	case has("mặt bằng", "giấy phép", "dọn", "định vị", "cắm mốc"):
		return model.Stages[0]
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
	newVideos := 0
	if len(l.Videos) > 0 { // video kéo về ở mọi trạng thái; hồ sơ đang chờ duyệt có thêm video thì tính lại tiền (thưởng video)
		if newVideos, err = s.inboxVideos(ctx, hid, l.Videos); err != nil {
			return false, false, err
		}
	}
	if !created && h.Status != model.StatusPending && h.Status != model.StatusDraft {
		return false, false, nil // admin đã xử lý: không đụng
	}
	lat, lng := parseCoord(l.Fields["toa_do"])
	addr := truncate(l.Fields["dia_chi"], MaxAddressLen)
	sent := l.CreatedAt.In(VN).Format("2006-01-02 15:04") // giờ CTV gửi tin: dùng làm giờ chụp (không cho AI đọc giờ in trên ảnh)
	files, metas := []Upload{}, []Meta{}
	for i, im := range pickImages(l.Images, model.MaxPhotos) {
		b, err := x.src.Image(ctx, im.Key)
		if err != nil {
			return false, false, fmt.Errorf("tải ảnh %s: %w", im.Key, err)
		}
		files = append(files, Upload{Name: fmt.Sprintf("Ảnh %d", i+1), Data: b})
		metas = append(metas, Meta{T: sent, Lat: lat, Lng: lng, Addr: addr})
	}
	ok, _, err := s.takeFiles(ctx, hid, files, metas)
	if err != nil {
		return false, false, err
	}
	if x.prune && !created && h.Status == model.StatusPending {
		if n, err := s.pruneInbox(ctx, h, files); err != nil || n > 0 {
			return false, false, err
		}
	}
	sys := &model.User{}
	if created {
		if ok == 0 { // không ảnh nào nhận được (đều đã nộp ở hồ sơ khác)
			_ = s.houses.Delete(ctx, hid)
			// chatbot hay ghi 2 hồ sơ cho cùng 1 bộ ảnh; bản sau có khi ghi địa chỉ chi tiết hơn: chép sang hồ sơ đã nhận
			if len(files) > 0 {
				if xid, err := s.photos.HouseOfSHA(ctx, shaOf(files[0].Data)); err == nil && xid > 0 && xid != hid {
					s.betterAddress(ctx, xid, ctv.ID, addr)
				}
			}
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
		if newVideos > 0 && h.Status == model.StatusPending {
			if err := s.repriceInbox(ctx, h, 0); err != nil {
				return false, false, err
			}
		}
		return false, false, nil // không có ảnh mới
	}
	if !created { // hồ sơ đang chờ duyệt có thêm ảnh: cập nhật số tiền theo ảnh đạt, giữ trạng thái
		if h.Status == model.StatusPending {
			if err := s.repriceInbox(ctx, h, ok); err != nil {
				return false, false, err
			}
		}
		return false, false, nil
	}
	// Messenger không qua AI: mặc định "chỉ chụp ngoài", admin đổi khi duyệt
	if err := s.houses.SetScope(ctx, hid, model.ScopeOut, 0); err != nil {
		return false, false, err
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
	hit, err := s.checkSite(ctx, hid, lat, lng, h.ContractorPhone, false)
	if err != nil {
		return false, false, err
	}
	warn := ""
	if hit != nil && !hit.Block {
		warn = truncate(hit.Warn, MaxReasonLen)
	}
	amount, err := s.priceOf(ctx, st, hid, valid, model.ScopeOut)
	if err != nil {
		return false, false, err
	}
	if err := s.houses.Submit(ctx, hid, amount, lat, lng, warn, s.drive != nil); err != nil {
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

// pickImages: tối đa max ảnh đầu (theo thứ tự gửi).
func pickImages(all []inbox.Image, max int) []inbox.Image {
	if len(all) <= max {
		return all
	}
	return all[:max]
}

var (
	hasDigit = regexp.MustCompile(`\d`)
	houseNo  = regexp.MustCompile(`(^|[\s,/])\d+[\p{L}/-]*(\s|,|$)`) // số nhà đứng riêng: "07 Hối Kiếng", "12/3 ...", "N20 ..." không tính
)

// moreDetailed: addr là địa chỉ chi tiết hơn old: có số nhà đứng riêng, old không có số nào, và dài hơn hẳn.
// ("Đà Nẵng333" không phải số nhà; "07 Hối Kiếng 08, P. Ngũ Hành Sơn, Đà Nẵng" hơn "Đà Nẵng")
func moreDetailed(addr, old string) bool {
	return addr != "" && houseNo.MatchString(addr) && !hasDigit.MatchString(old) &&
		utf8.RuneCountInString(addr) > utf8.RuneCountInString(old)+5
}

// betterAddress: hồ sơ xid (của cùng CTV, chưa duyệt) đang ghi địa chỉ chung chung ("Đà Nẵng"), bản ghi trùng của
// chatbot có số nhà / tên đường: đổi sang địa chỉ chi tiết, ghi lịch sử.
func (s *houseService) betterAddress(ctx context.Context, xid, ctvID int64, addr string) {
	x, err := s.houses.Get(ctx, xid)
	if err != nil || x.CTVID != ctvID || (x.Status != model.StatusPending && x.Status != model.StatusDraft) {
		return
	}
	if !moreDetailed(addr, x.Address) {
		return
	}
	if err := s.houses.SetAddress(ctx, xid, addr); err != nil {
		log.Printf("Messenger: đổi địa chỉ hồ sơ #%d: %v", xid, err)
		return
	}
	s.note(ctx, &model.User{}, ActInbox, xid, truncate(fmt.Sprintf("Địa chỉ chi tiết hơn từ Messenger: %q → %q", x.Address, addr), MaxReasonLen))
}

// repriceInbox: hồ sơ Messenger đang chờ duyệt vừa có thêm ảnh: tính lại tiền theo số ảnh đạt, ghi lịch sử.
func (s *houseService) repriceInbox(ctx context.Context, h *model.House, added int) error {
	ps, err := s.photos.ListByHouse(ctx, h.ID)
	if err != nil {
		return err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return err
	}
	amount, err := s.priceOf(ctx, st, h.ID, FillProblems(ps, s.now()), h.EffScope())
	if err != nil {
		return err
	}
	if amount != h.Amount {
		if err := s.houses.SetAmount(ctx, h.ID, amount); err != nil {
			return err
		}
	}
	what := fmt.Sprintf("thêm %d ảnh", added)
	if added == 0 {
		what = "thêm video"
	}
	s.note(ctx, &model.User{}, ActInbox, h.ID, fmt.Sprintf("Messenger: %s, tiền %s → %s", what, Money(h.Amount), Money(amount)))
	return nil
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
	err := s.backfillVideos(ctx)
	if err == nil {
		err = s.resyncOnce(ctx)
	}
	if err == nil {
		err = s.syncAll(ctx, &res)
	}
	if err == nil && x.prune {
		x.prune = false
		err = x.state.Set(ctx, resyncKey, s.now().Format(time.RFC3339))
	}
	if err == nil {
		err = s.refreshContacts(ctx)
	}
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

// refreshContacts: lấy toàn bộ thông tin bên Messenger của các CTV đã ghép (tên, SĐT, email, nhãn, ghi chú, các ô
// chatbot ghi nhận), lưu để hiện ở trang 1 CTV; ô của web còn trống thì điền: khu vực (tỉnh), số tài khoản, ghi chú.
// CTV tạo từ Messenger lúc chưa có SĐT (đăng nhập "mess_..."): bên Messenger có SĐT (chưa ai dùng) thì lấy làm
// tên đăng nhập; không có thì trang CTV báo admin nhập.
func (s *houseService) refreshContacts(ctx context.Context) error {
	x := s.inbox
	list, err := x.users.InboxCTVs(ctx)
	if err != nil || len(list) == 0 {
		return err
	}
	ids := make([]string, len(list))
	for i, u := range list {
		ids[i] = u.Contact
	}
	infos, err := x.src.Contacts(ctx, ids)
	if err != nil {
		return fmt.Errorf("đọc thông tin CTV dezon-inbox: %w", err)
	}
	for _, u := range list {
		ci, ok := infos[u.Contact]
		if !ok {
			continue
		}
		raw, err := json.Marshal(ci)
		if err != nil {
			continue
		}
		if err := x.users.SetInboxInfo(ctx, u.ID, raw); err != nil {
			return err
		}
		region, note := "", ""
		if u.Region == "" {
			if p := province.FromAddress(ci.Fields["khu_vuc"]); p != nil {
				region = p.Code
			}
		}
		if u.Note == "" {
			note = truncate(ci.Notes, 1000)
		}
		if region != "" || note != "" {
			if err := x.users.FillProfileIfEmpty(ctx, u.ID, region, note); err != nil {
				return err
			}
		}
		if p := normPhone(ci.Phone); strings.HasPrefix(u.Login, "mess_") && ValidPhone(p) {
			if _, err := x.auth.FindByLogin(ctx, p); errors.Is(err, repository.ErrNotFound) {
				if err := x.users.SetLogin(ctx, u.ID, p); err != nil && !errors.Is(err, repository.ErrDuplicate) {
					return err
				}
			}
		}
		if u.Bank == "" {
			if b := bankOf(ci.Fields["so_tai_khoan"]); b != "" {
				if err := x.users.UpdateBank(ctx, u.ID, b); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// pruneInbox: gỡ khỏi hồ sơ Messenger đang chờ duyệt các ảnh không có trong bản ghi chatbot (ảnh lẻ từng gom thêm,
// bỏ từ 19/11/2026 vì hay gộp nhầm công trình khác), tính lại tiền, ghi lịch sử. Trả số ảnh đã gỡ.
func (s *houseService) pruneInbox(ctx context.Context, h *model.House, files []Upload) (int, error) {
	keep := map[string]bool{}
	for _, f := range files {
		keep[shaOf(f.Data)] = true
	}
	ps, err := s.photos.ListByHouse(ctx, h.ID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range ps {
		if keep[strings.TrimSpace(p.SHA256)] {
			continue
		}
		if _, err := s.photos.Delete(ctx, p.ID, h.ID); err != nil && !errors.Is(err, repository.ErrNotFound) {
			return n, err
		}
		n++
	}
	if n == 0 {
		return 0, nil
	}
	if ps, err = s.photos.ListByHouse(ctx, h.ID); err != nil {
		return n, err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return n, err
	}
	amount, err := s.priceOf(ctx, st, h.ID, FillProblems(ps, s.now()), h.EffScope())
	if err != nil {
		return n, err
	}
	if err := s.houses.SetAmount(ctx, h.ID, amount); err != nil {
		return n, err
	}
	s.note(ctx, &model.User{}, ActInbox, h.ID, fmt.Sprintf("Messenger: gỡ %d ảnh không thuộc bản ghi chatbot (ảnh lẻ gom nhầm), còn %d ảnh, tiền %s → %s",
		n, len(ps), Money(h.Amount), Money(amount)))
	log.Printf("Messenger: hồ sơ #%d gỡ %d ảnh lẻ", h.ID, n)
	return n, nil
}

// resyncKey: đọc lại toàn bộ hồ sơ Messenger 1 lần sau bản sửa 19/11/2026 (tối): bỏ ảnh lẻ (gỡ ảnh lẻ đã gom ở hồ
// sơ đang chờ duyệt). Hồ sơ đã duyệt / từ chối không bị đụng; ảnh đã có bị bỏ qua (trùng file). Lượt trước: v2.
const resyncKey = "inbox_resync_v3"

func (s *houseService) resyncOnce(ctx context.Context) error {
	x := s.inbox
	if done, _, err := x.state.Get(ctx, resyncKey); err != nil || done != "" {
		return err
	}
	if err := x.state.Set(ctx, syncCursorKey, ""); err != nil {
		return err
	}
	log.Printf("Messenger: đọc lại toàn bộ hồ sơ (gỡ ảnh lẻ gom nhầm)")
	x.prune = true // đánh dấu xong khi đọc hết không lỗi
	return nil
}
