package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"vuathau/backend/internal/integration/inbox"
	"vuathau/backend/internal/model"
)

// Ảnh CTV gửi trên fanpage mà chatbot KHÔNG ghi vào nhật ký công trình (đo 25/11/2026: ~43% tin có ảnh từ 15/11 bị
// sót: AI không chắc, AI lỗi, hội thoại đang chuyển nhân viên, CTV gửi nhiều đợt). Gom các tin có ảnh liền nhau của
// một người (cách nhau không quá looseGap) thành 1 hồ sơ "Chờ phân loại" trong hàng chờ duyệt; admin xem, điền thông
// tin, tách hoặc từ chối. KHÔNG tự gộp vào hồ sơ có sẵn (19/11/2026 từng gộp nhầm ảnh công trình khác).
// Tin đã thuộc 1 nhật ký công trình thì bỏ qua (nhập theo Logs) và ngắt nhóm của người đó.

const (
	looseCursorKey = "inbox_loose_cursor"
	looseGap       = 30 * time.Minute // tin cách nhau quá mức này là đợt (công trình) khác
	looseSettle    = 3 * time.Hour    // chờ bot ghi nhật ký xong rồi mới coi tin là "bị sót"
	looseBatch     = 500
	LooseLabel     = "Chờ phân loại"
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

// syncLoose: kéo tin có ảnh bị sót (từ con trỏ tới cách đây looseSettle), tạo hồ sơ cho các nhóm đã khép (người gửi
// im quá looseGap). Con trỏ dừng trước nhóm chưa khép; tạo lại cùng nhóm không trùng (ext_id "msg:<tin đầu>", ảnh
// trùng file bị bỏ qua).
func (s *houseService) syncLoose(ctx context.Context, res *SyncStatus) error {
	x := s.inbox
	src, ok := x.src.(inbox.PhotoSource)
	if !ok {
		return nil
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
	stopAt := len(msgs) // con trỏ dừng trước tin đầu của nhóm chưa khép
	for _, g := range groupLoose(msgs, looseGap) {
		if g.last.Add(looseGap).After(cutoff) {
			stopAt = min(stopAt, g.first)
			continue
		}
		added, rejected, err := s.createLoose(ctx, g.msgs)
		if err != nil {
			return fmt.Errorf("ảnh Messenger chưa có hồ sơ (tin %s): %w", g.msgs[0].ID, err)
		}
		if added {
			res.Added++
		}
		if rejected {
			res.Rejected++
		}
	}
	if stopAt == 0 {
		return nil
	}
	last := msgs[stopAt-1]
	return x.state.Set(ctx, looseCursorKey, last.SentAt.Format(time.RFC3339Nano)+"|"+last.ID)
}

// createLoose: 1 nhóm tin có ảnh của 1 người -> hồ sơ "Chờ phân loại" (gửi duyệt như hồ sơ Messenger, không địa chỉ).
func (s *houseService) createLoose(ctx context.Context, msgs []inbox.PhotoMessage) (added, rejected bool, err error) {
	first, last := msgs[0], msgs[len(msgs)-1]
	ctv, err := s.inboxCTV(ctx, first.Contact)
	if err != nil {
		return false, false, err
	}
	hid, created, err := s.houses.CreateInbox(ctx, ctv.ID, "msg:"+first.ID, first.SentAt)
	if err != nil || !created {
		return false, false, err // đã tạo ở lượt trước: admin đang / đã xử lý
	}
	files, metas := []Upload{}, []Meta{}
	for _, m := range msgs {
		sent := m.SentAt.In(VN).Format("2006-01-02 15:04") // giờ CTV gửi tin làm giờ chụp, như hồ sơ Messenger
		for _, im := range m.Images {
			if len(files) >= model.MaxPhotos {
				break
			}
			b, err := s.inbox.src.Image(ctx, im.Key)
			if err != nil {
				_ = s.houses.Delete(ctx, hid)
				return false, false, fmt.Errorf("tải ảnh %s: %w", im.Key, err)
			}
			files = append(files, Upload{Name: fmt.Sprintf("Ảnh %d", len(files)+1), Data: b})
			metas = append(metas, Meta{T: sent})
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
	when := first.SentAt.In(VN).Format("15:04")
	if last.SentAt.Sub(first.SentAt) >= time.Minute {
		when += "–" + last.SentAt.In(VN).Format("15:04")
	}
	when += " " + first.SentAt.In(VN).Format("02/01")
	s.note(ctx, &model.User{}, ActInbox, hid, truncate(fmt.Sprintf(
		"Fanpage Messenger, %d ảnh (%d tin, %s): chatbot không ghi hồ sơ công trình, cần phân loại", ok, len(msgs), when), MaxReasonLen))
	rejected, err = s.submitInbox(ctx, hid, nil, nil, LooseLabel+": ảnh Messenger chatbot không ghi hồ sơ, admin điền địa chỉ / nhà thầu")
	if err != nil {
		return false, false, err
	}
	log.Printf("Messenger: hồ sơ #%d chờ phân loại (%d ảnh, %d tin của %q)", hid, ok, len(msgs), first.Contact.Name)
	return true, rejected, nil
}
