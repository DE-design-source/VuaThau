package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/integration/ocr"
	"vuathau/backend/internal/model"
)

// Ảnh CTV gửi trên fanpage mà chatbot KHÔNG ghi vào nhật ký công trình (đo 25/11/2026: ~43% tin có ảnh từ 15/11 bị
// sót: AI không chắc, AI lỗi, hội thoại đang chuyển nhân viên, CTV gửi nhiều đợt). Gom các tin có ảnh liền nhau của
// một người (cách nhau không quá looseGap) thành 1 đợt; trong đợt, tách công trình theo toạ độ in trên ảnh (dải chữ
// Timemark... góc dưới: CTV hay gửi liền nhiều công trình, vd "4 bộ"); mỗi công trình 1 hồ sơ "Chờ phân loại" trong
// hàng chờ duyệt, địa chỉ tra ngược từ toạ độ; admin xem, điền nhà thầu, tách hoặc từ chối. KHÔNG tự gộp vào hồ sơ
// có sẵn (19/11/2026 từng gộp nhầm ảnh công trình khác). Tin đã thuộc 1 nhật ký công trình thì bỏ qua (nhập theo
// Logs) và ngắt đợt của người đó.

const (
	looseCursorKey = "inbox_loose_cursor"
	looseGap       = 30 * time.Minute // tin cách nhau quá mức này là đợt khác
	looseSettle    = 3 * time.Hour    // chờ bot ghi nhật ký xong rồi mới coi tin là "bị sót"
	looseBatch     = 500
	looseSplitM    = 150.0 // ảnh cách điểm đầu của công trình đang gom quá chừng này (mét): công trình khác
	looseMaxImages = 120   // tối đa ảnh đọc trong 1 đợt
	LooseLabel     = "Chờ phân loại"
	// looseResplitKey: 1 lần, xoá các hồ sơ "Chờ phân loại" chưa ai xử lý (tạo trước khi tách theo toạ độ, có hồ sơ
	// gộp 4 công trình) rồi đọc lại từ đầu.
	looseResplitKey = "inbox_loose_v2"
)

// looseSince: ngày chatbot bắt đầu ghi nhật ký công trình; ảnh trước đó không kéo về.
var looseSince = time.Date(2026, 11, 15, 0, 0, 0, 0, VN)

type looseGroup struct {
	first int // vị trí tin đầu trong danh sách đọc được
	msgs  []inbox.PhotoMessage
	last  time.Time
}

// groupLoose: gom tin không thuộc nhật ký theo người gửi, tin sau cách tin trước không quá gap. Tin thuộc nhật ký
// ngắt nhóm đang mở của người đó.
func groupLoose(msgs []inbox.PhotoMessage, gap time.Duration) []*looseGroup {
	open := map[string]*looseGroup{}
	groups := []*looseGroup{}
	for i, m := range msgs {
		g := open[m.Contact.ID]
		if m.InLog {
			delete(open, m.Contact.ID)
			continue
		}
		if g == nil || m.SentAt.Sub(g.last) > gap {
			g = &looseGroup{first: i}
			groups = append(groups, g)
			open[m.Contact.ID] = g
		}
		g.msgs = append(g.msgs, m)
		g.last = m.SentAt
	}
	return groups
}

// resplitLoose: 1 lần (looseResplitKey): xoá hồ sơ chờ phân loại chưa ai xử lý, đọc lại tin có ảnh từ đầu.
func (s *houseService) resplitLoose(ctx context.Context) error {
	x := s.inbox
	if done, _, err := x.state.Get(ctx, looseResplitKey); err != nil || done != "" {
		return err
	}
	n, err := s.houses.DeleteUntouchedLoose(ctx)
	if err != nil {
		return err
	}
	if err := x.state.Set(ctx, looseCursorKey, ""); err != nil {
		return err
	}
	log.Printf("Messenger: xoá %d hồ sơ chờ phân loại chưa xử lý, tách lại theo toạ độ trên ảnh", n)
	return x.state.Set(ctx, looseResplitKey, s.now().Format(time.RFC3339))
}

// syncLoose: kéo tin có ảnh bị sót (từ con trỏ tới cách đây looseSettle), tạo hồ sơ cho các đợt đã khép (người gửi
// im quá looseGap). Con trỏ dừng trước đợt chưa khép; chạy lại cùng đợt không tạo trùng (ext_id "msg:<tin đầu>"
// [#n], ảnh trùng file bị bỏ qua).
func (s *houseService) syncLoose(ctx context.Context, res *SyncStatus) error {
	x := s.inbox
	src, ok := x.src.(inbox.PhotoSource)
	if !ok {
		return nil
	}
	if err := s.resplitLoose(ctx); err != nil {
		return err
	}
	cur, _, err := x.state.Get(ctx, looseCursorKey)
	if err != nil {
		return err
	}
	at, id := time.Time{}, ""
	if t, rest, ok := strings.Cut(cur, "|"); ok {
		if p, err := time.Parse(time.RFC3339Nano, t); err == nil {
			at, id = p, rest
		}
	}
	cutoff := s.now().Add(-looseSettle)
	msgs := []inbox.PhotoMessage{}
	for pa, pid := at, id; ; {
		batch, err := src.PhotoMessages(ctx, looseSince, pa, pid, looseBatch)
		if err != nil {
			return fmt.Errorf("đọc tin có ảnh UniInbox: %w", err)
		}
		stop := false
		for _, m := range batch {
			if !m.SentAt.Before(cutoff) {
				stop = true
				break
			}
			msgs = append(msgs, m)
		}
		if stop || len(batch) < looseBatch {
			break
		}
		pa, pid = batch[len(batch)-1].SentAt, batch[len(batch)-1].ID
	}
	if len(msgs) == 0 {
		return nil
	}
	stopAt := len(msgs) // con trỏ dừng trước tin đầu của đợt chưa khép
	for _, g := range groupLoose(msgs, looseGap) {
		if g.last.Add(looseGap).After(cutoff) {
			stopAt = min(stopAt, g.first)
			continue
		}
		added, rejected, err := s.createLoose(ctx, g.msgs)
		if err != nil {
			return fmt.Errorf("ảnh Messenger chưa có hồ sơ (tin %s): %w", g.msgs[0].ID, err)
		}
		res.Added += added
		res.Rejected += rejected
	}
	if stopAt == 0 {
		return nil
	}
	last := msgs[stopAt-1]
	return x.state.Set(ctx, looseCursorKey, last.SentAt.Format(time.RFC3339Nano)+"|"+last.ID)
}

// looseShot: 1 ảnh của đợt, toạ độ đọc từ dải chữ in trên ảnh (nil: không đọc được).
type looseShot struct {
	sent     time.Time
	data     []byte
	lat, lng *float64
}

// stampCoord: toạ độ trong chữ đọc từ dải chữ ảnh, chỉ nhận điểm trong vùng Việt Nam (kể cả Hoàng Sa, Trường Sa).
func stampCoord(text string) (*float64, *float64) {
	lat, lng := parseCoord(text)
	if lat == nil || *lat < 8 || *lat > 24 || *lng < 102 || *lng > 118 {
		return nil, nil
	}
	return lat, lng
}

// readShots: tải ảnh của đợt, đọc toạ độ in trên ảnh (2 ảnh cùng lúc: Tesseract tốn RAM).
func (s *houseService) readShots(ctx context.Context, msgs []inbox.PhotoMessage) ([]looseShot, error) {
	type job struct {
		i    int
		key  string
		sent time.Time
	}
	jobs := []job{}
	for _, m := range msgs {
		for _, im := range m.Images {
			if len(jobs) < looseMaxImages {
				jobs = append(jobs, job{len(jobs), im.Key, m.SentAt})
			}
		}
	}
	out := make([]looseShot, len(jobs))
	reader, _ := s.ocr.(ocr.StampReader)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	ch := make(chan job)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				b, err := s.inbox.src.Image(ctx, j.key)
				if err != nil {
					mu.Lock()
					if first == nil {
						first = fmt.Errorf("tải ảnh %s: %w", j.key, err)
					}
					mu.Unlock()
					continue
				}
				sh := looseShot{sent: j.sent, data: b}
				if reader != nil {
					if text, err := reader.Stamp(ctx, b); err == nil {
						sh.lat, sh.lng = stampCoord(text)
					} else if ctx.Err() == nil {
						log.Printf("đọc dải chữ ảnh %s: %v", j.key, err)
					}
				}
				out[j.i] = sh
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	return out, first
}

// splitSites: tách ảnh theo công trình (giữ thứ tự gửi): ảnh có toạ độ cách điểm đầu của công trình đang gom quá
// looseSplitM thì mở công trình mới; ảnh không có toạ độ theo công trình đang gom.
func splitSites(shots []looseShot) [][]looseShot {
	out := [][]looseShot{}
	var cur []looseShot
	var aLat, aLng *float64
	for _, sh := range shots {
		if sh.lat != nil && aLat != nil && distM(*aLat, *aLng, *sh.lat, *sh.lng) > looseSplitM {
			out = append(out, cur)
			cur, aLat, aLng = nil, nil, nil
		}
		if sh.lat != nil && aLat == nil {
			aLat, aLng = sh.lat, sh.lng
		}
		cur = append(cur, sh)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// createLoose: 1 đợt tin có ảnh của 1 người -> mỗi công trình 1 hồ sơ "Chờ phân loại". Hồ sơ đầu (ext_id
// "msg:<tin đầu>") tạo sau cùng: đã có nghĩa là đợt đã xử lý xong.
func (s *houseService) createLoose(ctx context.Context, msgs []inbox.PhotoMessage) (added, rejected int, err error) {
	first := msgs[0]
	primary := "msg:" + first.ID
	if done, err := s.houses.HasExt(ctx, primary); err != nil || done {
		return 0, 0, err // đã tạo ở lượt trước: admin đang / đã xử lý
	}
	ctv, err := s.inboxCTV(ctx, first.Contact)
	if err != nil {
		return 0, 0, err
	}
	shots, err := s.readShots(ctx, msgs)
	if err != nil {
		return 0, 0, err
	}
	sites := splitSites(shots)
	for i := len(sites) - 1; i >= 0; i-- {
		ext := primary
		if i > 0 {
			ext = fmt.Sprintf("%s#%d", primary, i+1)
		}
		a, r, err := s.looseHouse(ctx, ctv, ext, sites[i], len(sites))
		if err != nil {
			return added, rejected, err
		}
		if a {
			added++
		}
		if r {
			rejected++
		}
	}
	return added, rejected, nil
}

// looseHouse: ảnh 1 công trình -> hồ sơ chờ duyệt. Ảnh có toạ độ in trên ảnh lấy toạ độ đó làm vị trí; địa chỉ hồ sơ
// tra ngược từ toạ độ đầu tiên (admin kiểm lại).
func (s *houseService) looseHouse(ctx context.Context, ctv *model.User, ext string, shots []looseShot, sites int) (added, rejected bool, err error) {
	hid, created, err := s.houses.CreateInbox(ctx, ctv.ID, ext, shots[0].sent)
	if err != nil || !created {
		return false, false, err
	}
	files, metas := []Upload{}, []Meta{}
	var lat, lng *float64
	located := 0
	for _, sh := range shots {
		if len(files) >= model.MaxPhotos {
			break
		}
		// giờ CTV gửi tin làm giờ chụp, như hồ sơ Messenger (không lấy giờ in trên ảnh)
		files = append(files, Upload{Name: fmt.Sprintf("Ảnh %d", len(files)+1), Data: sh.data})
		metas = append(metas, Meta{T: sh.sent.In(VN).Format("2006-01-02 15:04"), Lat: sh.lat, Lng: sh.lng})
		if sh.lat != nil {
			located++
			if lat == nil {
				lat, lng = sh.lat, sh.lng
			}
		}
	}
	ok, _, err := s.takeFiles(ctx, hid, files, metas, false)
	if err != nil {
		_ = s.houses.Delete(ctx, hid)
		return false, false, err
	}
	if ok == 0 { // ảnh đều đã có ở hồ sơ khác
		return false, false, s.houses.Delete(ctx, hid)
	}
	addr := ""
	if lat != nil {
		addr = truncate(s.Geocode(ctx, *lat, *lng), MaxAddressLen)
		if addr != "" {
			if err := s.houses.SetAddress(ctx, hid, addr); err != nil {
				return false, false, err
			}
		}
	}
	first, last := shots[0].sent.In(VN), shots[len(shots)-1].sent.In(VN)
	when := first.Format("15:04")
	if last.Sub(first) >= time.Minute {
		when += "–" + last.Format("15:04")
	}
	when += " " + first.Format("02/01")
	note := fmt.Sprintf("Fanpage Messenger, %d ảnh (%s): chatbot không ghi hồ sơ công trình, cần phân loại", ok, when)
	if sites > 1 {
		note += fmt.Sprintf(". Đợt gửi có %d công trình (tách theo toạ độ in trên ảnh)", sites)
	}
	if located > 0 {
		note += fmt.Sprintf(". %d/%d ảnh có toạ độ in trên ảnh", located, len(files))
	}
	s.note(ctx, &model.User{}, ActInbox, hid, truncate(note, MaxReasonLen))
	label := LooseLabel + ": ảnh Messenger chatbot không ghi hồ sơ, admin điền địa chỉ / nhà thầu"
	if addr != "" {
		label = LooseLabel + ": ảnh Messenger chatbot không ghi hồ sơ; địa chỉ tra từ toạ độ in trên ảnh, admin kiểm lại, điền nhà thầu"
	}
	rejected, err = s.submitInbox(ctx, hid, lat, lng, label)
	if err != nil {
		return false, false, err
	}
	log.Printf("Messenger: hồ sơ #%d chờ phân loại (%d ảnh, %d có toạ độ, %s)", hid, ok, located, ctv.Name)
	return true, rejected, nil
}
