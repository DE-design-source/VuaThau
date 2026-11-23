package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"vuathau/backend/internal/integration/geo"
	"vuathau/backend/internal/repository"
)

// Bản đồ công khai (góp ý 20/11/2026): người chưa đăng nhập xem menu Bản đồ và Xem quy hoạch.
// Dữ liệu như bản đồ sale (công trình sale được xem, ảnh đại diện đã che SĐT) nhưng KHÔNG tên CTV, không ô lọc CTV.
// Ảnh đại diện đi qua link có chữ ký (/api/v1/public/photos/{id}?s=): chỉ ảnh đại diện máy chủ đã phát mới mở được,
// không dò được ảnh khác theo số.

const publicMapTTL = 2 * time.Minute // người ngoài xem nhiều: tính lại tối đa 2 phút 1 lần mỗi tỉnh

type publicMapCache struct {
	mu   sync.Mutex
	data map[string]publicMapEntry
}

type publicMapEntry struct {
	at time.Time
	d  *SaleMapData
}

func (s *houseService) publicSig(pid int64) string {
	m := hmac.New(sha256.New, s.mediaKey)
	fmt.Fprintf(m, "public-photo.%d", pid)
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// PublicMap: bản đồ cho người chưa đăng nhập.
func (s *houseService) PublicMap(ctx context.Context, prov string) (*SaleMapData, error) {
	s.pub.mu.Lock()
	if e, ok := s.pub.data[prov]; ok && s.now().Sub(e.at) < publicMapTTL {
		s.pub.mu.Unlock()
		return e.d, nil
	}
	s.pub.mu.Unlock()

	d, err := s.SaleMap(ctx, prov, 0)
	if err != nil {
		return nil, err
	}
	d.CTVs, d.CTV = []repository.SaleCTV{}, 0
	for i := range d.Points {
		p := &d.Points[i]
		p.CTVName = ""
		if p.CoverID != nil {
			p.CoverURL = fmt.Sprintf("/api/v1/public/photos/%d?s=%s", *p.CoverID, s.publicSig(*p.CoverID))
			p.CoverID = nil
		}
	}
	s.pub.mu.Lock()
	if s.pub.data == nil {
		s.pub.data = map[string]publicMapEntry{}
	}
	s.pub.data[prov] = publicMapEntry{at: s.now(), d: d}
	s.pub.mu.Unlock()
	return d, nil
}

// PublicPhoto: ảnh đại diện cỡ thẻ cho bản đồ công khai; ảnh có SĐT trả bản đã che.
func (s *houseService) PublicPhoto(ctx context.Context, pid int64, sig string) ([]byte, error) {
	if !hmac.Equal([]byte(sig), []byte(s.publicSig(pid))) {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	p, err := s.photos.Get(ctx, pid)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	if err != nil {
		return nil, err
	}
	if ok, err := s.houses.SaleCanSeeHouse(ctx, p.HouseID); err != nil || !ok {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	// luôn đi qua che SĐT (chưa quét thì quét ngay); lỗi quét: không trả ảnh gốc
	mb, err := s.maskedPhoto(ctx, p, "card")
	if err != nil {
		return nil, fail(ErrNotFound, "Không tìm thấy ảnh.")
	}
	if mb != nil {
		return mb, nil
	}
	b, err := s.files.ReadCard(p.SHA256)
	if err != nil {
		return nil, fail(ErrNotFound, "Không tìm thấy file ảnh.")
	}
	return b, nil
}

// PublicSearchPlace: tìm địa điểm cho người chưa đăng nhập: chỉ địa điểm, không tìm trong công trình của hệ thống.
func (s *houseService) PublicSearchPlace(ctx context.Context, q string) (*PlaceSearch, error) {
	res := &PlaceSearch{Sites: []repository.SiteHit{}, Places: []geo.Place{}}
	if len([]rune(strings.TrimSpace(q))) < 3 || s.geo == nil {
		return res, nil
	}
	ps, err := s.geo.Search(ctx, q)
	if err != nil {
		return nil, fail(ErrUnavailable, "Chưa tìm được địa điểm, thử lại sau ít giây.")
	}
	res.Places = ps
	return res, nil
}

// PublicPhotoRef: 1 ảnh công trình trên trang công khai (link có chữ ký, ảnh đã che SĐT).
type PublicPhotoRef struct {
	URL  string `json:"url"`
	Time string `json:"time"`
}

// PublicProject: chi tiết 1 công trình cho người chưa đăng nhập (góp ý 23/11/2026: bản free bấm vào xem được
// chi tiết từng công trình). Như dự án của sale nhưng không SĐT nhà thầu, không CTV, không tiền.
type PublicProject struct {
	ID           int64            `json:"id"`
	Address      string           `json:"address"`
	ProvinceName string           `json:"province_name"`
	Stage        string           `json:"stage"`
	Floors       *int             `json:"floors"`
	Contractor   string           `json:"contractor"`
	At           time.Time        `json:"at"`
	Lat          *float64         `json:"lat"`
	Lng          *float64         `json:"lng"`
	Photos       []PublicPhotoRef `json:"photos"`
}

// PublicProject: chỉ công trình hiện trên bản đồ công khai (như sale được xem).
func (s *houseService) PublicProject(ctx context.Context, id int64) (*PublicProject, error) {
	p, err := s.SaleProject(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &PublicProject{ID: p.ID, Address: p.Address, ProvinceName: p.ProvinceName, Stage: p.Stage, Floors: p.Floors,
		Contractor: p.ContractorName, At: p.At, Lat: p.Lat, Lng: p.Lng, Photos: []PublicPhotoRef{}}
	for _, ph := range p.PhotoList {
		out.Photos = append(out.Photos, PublicPhotoRef{URL: fmt.Sprintf("/api/v1/public/photos/%d?s=%s", ph.ID, s.publicSig(ph.ID)), Time: ph.Time})
	}
	return out, nil
}
