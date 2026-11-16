package service

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"vuathau/backend/internal/integration/ai"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/pkg/imaging"
	"vuathau/backend/internal/repository"
)

// Chặn chụp trùng công trình: mỗi công trình chỉ nhận 1 hồ sơ trong DupDays ngày (cấu hình, mặc định 30).
// Ứng viên: hồ sơ đã gửi duyệt trong hạn (trừ bị từ chối), cách dưới DupRadiusM mét, hoặc cùng SĐT nhà thầu ở gần.
// Ảnh giống hệt ảnh hồ sơ ứng viên: trùng ngay. Còn lại AI so ảnh 2 hồ sơ; AI tắt / lỗi thì chỉ chặn khi
// vừa trong bán kính vừa cùng SĐT nhà thầu, không đủ chắc thì chỉ ghi cảnh báo cho admin.
const (
	sameMinConfidence = 0.6 // AI chắc từ mức này mới coi là cùng công trình
	sameMaxCompare    = 2   // so tối đa với bấy nhiêu hồ sơ gần nhất (mỗi lượt AI vài giây)
	samePhotos        = 3   // số ảnh mỗi bên gửi AI so
	phoneNearFactor   = 10  // cùng SĐT nhà thầu: xét trong bán kính gấp bấy nhiêu lần
)

const ActAutoDup = "Tự từ chối: trùng công trình"

// SiteHit: kết quả kiểm trùng. Block: trùng chắc chắn (chặn / tự từ chối); không thì Warn là cảnh báo cho admin.
type SiteHit struct {
	Of     repository.SiteCandidate
	Meters float64 // -1: không có toạ độ
	Block  bool
	Reason string
	Warn   string
}

type siteCand struct {
	c     repository.SiteCandidate
	m     float64
	phone bool
}

// useAI: false với hồ sơ lấy từ Messenger / DB (không cho AI so ảnh): chỉ chặn khi ảnh giống hệt, hoặc cùng chỗ
// và cùng SĐT nhà thầu; còn lại ghi cảnh báo cho admin.
func (s *houseService) checkSite(ctx context.Context, hid int64, lat, lng *float64, contractorPhone string, useAI bool) (*SiteHit, error) {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	cands, err := s.houses.SiteCandidates(ctx, hid, s.now().AddDate(0, 0, -st.DupDays))
	if err != nil || len(cands) == 0 {
		return nil, err
	}
	radius := float64(st.DupRadiusM)
	phone := CleanPhone(contractorPhone)
	near := []siteCand{}
	for _, c := range cands {
		m := -1.0
		if lat != nil && lng != nil && c.Lat != nil && c.Lng != nil {
			m = imaging.Meters(*lat, *lng, *c.Lat, *c.Lng)
		}
		samePhone := len(phone) >= 8 && CleanPhone(c.ContractorPhone) == phone
		if (m >= 0 && m <= radius) || (samePhone && (m < 0 || m <= radius*phoneNearFactor)) {
			near = append(near, siteCand{c, m, samePhone})
		}
	}
	if len(near) == 0 {
		return nil, nil
	}
	slices.SortFunc(near, func(a, b siteCand) int {
		return cmp.Compare(sortKey(a.m), sortKey(b.m))
	})

	ps, err := s.photos.ListByHouse(ctx, hid)
	if err != nil {
		return nil, err
	}
	// ảnh giống hệt (dHash) ảnh của hồ sơ ứng viên
	for _, p := range ps {
		if p.DupOf == nil || *p.DupOf == hid {
			continue
		}
		for _, n := range near {
			if n.c.ID == *p.DupOf {
				return s.hit(n, true, "có ảnh giống hệt ảnh của hồ sơ đó", ""), nil
			}
		}
	}

	comparer, _ := s.ai.(ai.Comparer)
	var mine [][]byte
	if useAI && comparer != nil && s.ai.Enabled() {
		mine = s.compareImages(ps)
	}
	warn := ""
	for i, n := range near {
		var res *ai.SameSite
		if len(mine) > 0 && i < sameMaxCompare {
			res = s.sameSite(ctx, comparer, ps, mine, n.c.ID)
		}
		switch {
		case res != nil && res.Same && res.Confidence >= sameMinConfidence:
			return s.hit(n, true, "AI so ảnh: "+res.Reason, ""), nil
		case res != nil:
			if warn == "" {
				warn = fmt.Sprintf("Gần hồ sơ #%d%s, AI so ảnh thấy khác công trình: %s", n.c.ID, dist(n.m), res.Reason)
			}
		case n.phone && n.m >= 0 && n.m <= radius: // không có AI: cùng chỗ + cùng nhà thầu là đủ chắc
			return s.hit(n, true, "cùng chỗ và cùng SĐT nhà thầu", ""), nil
		default:
			if warn == "" {
				warn = fmt.Sprintf("Gần hồ sơ #%d%s, có thể trùng công trình (chưa so ảnh)", n.c.ID, dist(n.m))
			}
		}
	}
	return &SiteHit{Of: near[0].c, Meters: near[0].m, Warn: warn}, nil
}

func sortKey(m float64) float64 {
	if m < 0 {
		return 1e9 // không có toạ độ: xếp sau
	}
	return m
}

func dist(m float64) string {
	if m < 0 {
		return ""
	}
	return fmt.Sprintf(" (cách khoảng %.0fm)", m)
}

func (s *houseService) hit(n siteCand, block bool, why, warn string) *SiteHit {
	reason := fmt.Sprintf("Trùng công trình hồ sơ #%d: CTV %s đã gửi ngày %s%s, %s.", n.c.ID, n.c.CTVName,
		n.c.SubmittedAt.In(VN).Format("02/01/2006"), dist(n.m), strings.TrimSuffix(why, "."))
	return &SiteHit{Of: n.c, Meters: n.m, Block: block, Reason: truncate(reason, MaxReasonLen), Warn: warn}
}

// compareImages: tối đa samePhotos ảnh thu nhỏ của hồ sơ, ưu tiên ảnh công trình.
func (s *houseService) compareImages(ps []model.Photo) [][]byte {
	order := slices.Clone(ps)
	slices.SortStableFunc(order, func(a, b model.Photo) int {
		ra, rb := a.IsSite == nil || *a.IsSite, b.IsSite == nil || *b.IsSite
		switch {
		case ra && !rb:
			return -1
		case !ra && rb:
			return 1
		}
		return 0
	})
	out := [][]byte{}
	for _, p := range order {
		if len(out) == samePhotos {
			break
		}
		if b, err := s.files.ReadPhoto(p.SHA256, true); err == nil {
			out = append(out, b)
		}
	}
	return out
}

// sameSite: AI so ảnh hồ sơ này với hồ sơ other. Lưu kết quả theo cặp ảnh (ai_cache) để không hỏi lại.
func (s *houseService) sameSite(ctx context.Context, cmp ai.Comparer, mine []model.Photo, mineImgs [][]byte, other int64) *ai.SameSite {
	theirs, err := s.photos.ListByHouse(ctx, other)
	if err != nil || len(theirs) == 0 {
		return nil
	}
	keys := []string{}
	for _, p := range mine {
		keys = append(keys, "a"+p.SHA256)
	}
	for _, p := range theirs {
		keys = append(keys, "b"+p.SHA256)
	}
	slices.Sort(keys)
	key := shaOf([]byte("same-site:" + strings.Join(keys, ",")))
	if c, err := s.photos.CacheGet(ctx, []string{key}); err == nil {
		var r ai.SameSite
		if raw, ok := c[key]; ok && json.Unmarshal(raw, &r) == nil {
			return &r
		}
	}
	imgs := s.compareImages(theirs)
	if len(imgs) == 0 {
		return nil
	}
	actx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	r, err := cmp.SameSite(actx, mineImgs, imgs)
	if err != nil {
		log.Printf("AI so ảnh với hồ sơ #%d: %v", other, err)
		return nil
	}
	r.Reason = truncate(r.Reason, 250)
	if raw, err := json.Marshal(r); err == nil {
		_ = s.photos.CachePut(ctx, key, raw)
	}
	return r
}
