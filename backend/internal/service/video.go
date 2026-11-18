package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"path"
	"strconv"
	"strings"
	"time"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/storage"
)

// Video công trình: CTV quay / chọn trên app, video Messenger kéo về cùng hồ sơ.
// Chỉ để admin xem thêm: không qua AI, không tính vào ảnh đạt / tiền.

const (
	MaxVideoBytes = 200 << 20
	videoLinkTTL  = 12 * time.Hour
)

// VideoPart: 1 video gửi lên (đọc dần từ multipart, không giữ cả file trong RAM).
type VideoPart struct {
	Name, Mime string
	Body       io.Reader
}

var videoExt = map[string]string{".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
	".3gp": "video/3gpp", ".mkv": "video/x-matroska"}

// videoMime: kiểu video theo header / đuôi file; "" nếu không phải video.
func videoMime(name, mime string) string {
	mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	if strings.HasPrefix(mime, "video/") {
		return mime
	}
	return videoExt[strings.ToLower(path.Ext(name))]
}

func newMediaKey(k string) []byte {
	if k != "" {
		return []byte("video:" + k)
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}

func (s *houseService) videoSig(id, exp int64) string {
	m := hmac.New(sha256.New, s.mediaKey)
	fmt.Fprintf(m, "%d.%d", id, exp)
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// withSrc: gắn link xem có chữ ký (hết hạn sau 12-13 giờ; làm tròn theo giờ để trình duyệt dùng lại bộ nhớ đệm).
func (s *houseService) withSrc(vs []model.Video) []model.Video {
	exp := s.now().Add(videoLinkTTL).Truncate(time.Hour).Add(time.Hour).Unix()
	for i := range vs {
		vs[i].Src = fmt.Sprintf("/api/v1/videos/%d?e=%d&s=%s", vs[i].ID, exp, s.videoSig(vs[i].ID, exp))
	}
	return vs
}

// saveVideo: lưu 1 video vào hồ sơ. Trả câu báo nếu không nhận.
func (s *houseService) saveVideo(ctx context.Context, hid int64, p VideoPart) (string, error) {
	mime := videoMime(p.Name, p.Mime)
	if mime == "" {
		_, _ = io.Copy(io.Discard, p.Body)
		return "không phải file video (MP4, MOV, WEBM)", nil
	}
	sha, size, err := s.files.SaveVideo(p.Body, MaxVideoBytes)
	if errors.Is(err, storage.ErrTooBig) {
		return fmt.Sprintf("video quá %dMB", MaxVideoBytes>>20), nil
	}
	if err != nil {
		return "", err
	}
	if other, err := s.photos.HouseOfVideo(ctx, sha); err != nil {
		return "", err
	} else if other > 0 { // file y hệt đã thuộc hồ sơ khác: giữ file, không thêm
		if other == hid {
			return "video này đã có trong hồ sơ", nil
		}
		return fmt.Sprintf("video này đã nộp ở hồ sơ #%d", other), nil
	}
	v := &model.Video{HouseID: hid, SHA256: sha, Mime: mime, Size: size}
	if err := s.photos.InsertVideo(ctx, v); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return "video này vừa được nộp", nil
		}
		s.files.DeleteVideo(sha)
		return "", err
	}
	return "", nil
}

// AddVideos: CTV thêm video vào hồ sơ đang sửa được. next trả io.EOF khi hết file.
func (s *houseService) AddVideos(ctx context.Context, u *model.User, id int64, next func() (*VideoPart, error)) (*UploadResult, error) {
	h, err := s.load(ctx, u, id)
	if err != nil {
		return nil, err
	}
	if u.Role != model.RoleCTV || !model.Editable(h.Status) {
		return nil, fail(ErrForbidden, "Hồ sơ đang được duyệt, không thêm video được.")
	}
	have, err := s.photos.Videos(ctx, id)
	if err != nil {
		return nil, err
	}
	room := model.MaxVideos - len(have)
	ok, msgs := 0, []string{}
	for k := 0; ; k++ {
		p, err := next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fail(ErrInvalidInput, "Không đọc được video gửi lên, thử lại.")
		}
		name := p.Name
		if name == "" {
			name = fmt.Sprintf("Video %d", k+1)
		}
		if ok >= room {
			_, _ = io.Copy(io.Discard, p.Body)
			msgs = append(msgs, fmt.Sprintf("%s: mỗi hồ sơ tối đa %d video", name, model.MaxVideos))
			continue
		}
		why, err := s.saveVideo(ctx, id, *p)
		if err != nil {
			return nil, err
		}
		if why != "" {
			msgs = append(msgs, name+": "+why)
			continue
		}
		ok++
	}
	m := fmt.Sprintf("Đã tải lên %d video.", ok)
	if ok == 0 {
		if len(msgs) == 0 {
			return nil, fail(ErrInvalidInput, "Chưa chọn video nào.")
		}
		m = "Không video nào được nhận."
	}
	return &UploadResult{Message: m, Messages: msgs, HouseID: ptr(id), Added: ok}, nil
}

func (s *houseService) DeleteVideo(ctx context.Context, u *model.User, id, vid int64) (string, error) {
	h, err := s.load(ctx, u, id)
	if err != nil {
		return "", err
	}
	if u.Role != model.RoleCTV || !model.Editable(h.Status) {
		return "", fail(ErrForbidden, "Hồ sơ đang được duyệt, không xoá video được.")
	}
	v, err := s.photos.DeleteVideo(ctx, vid, id)
	if errors.Is(err, repository.ErrNotFound) {
		return "", fail(ErrNotFound, "Không tìm thấy video.")
	}
	if err != nil {
		return "", err
	}
	s.files.DeleteVideo(v.SHA256)
	return "Đã xoá video.", nil
}

// VideoFile: file video theo link có chữ ký (đúng chữ ký, chưa hết hạn).
func (s *houseService) VideoFile(ctx context.Context, vid int64, exp, sig string) (string, string, error) {
	e, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || e < s.now().Unix() || !hmac.Equal([]byte(sig), []byte(s.videoSig(vid, e))) {
		return "", "", fail(ErrForbidden, "Link video đã hết hạn, tải lại trang.")
	}
	v, err := s.photos.Video(ctx, vid)
	if errors.Is(err, repository.ErrNotFound) {
		return "", "", fail(ErrNotFound, "Không tìm thấy video.")
	}
	if err != nil {
		return "", "", err
	}
	p, err := s.files.VideoPath(v.SHA256)
	return p, v.Mime, err
}

// ---------- video Messenger ----------

// inboxVideos: tải video của 1 hồ sơ Messenger về (bỏ qua video đã có). Trả số video mới.
func (s *houseService) inboxVideos(ctx context.Context, hid int64, keys []string) (int, error) {
	have, err := s.photos.Videos(ctx, hid)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, k := range keys {
		if len(have)+n >= model.MaxVideos {
			break
		}
		rc, mime, err := s.inbox.src.Open(ctx, k)
		if err != nil {
			log.Printf("Messenger: tải video %s: %v", k, err)
			continue // thiếu 1 video không chặn cả hồ sơ
		}
		why, err := s.saveVideo(ctx, hid, VideoPart{Name: path.Base(k), Mime: mime, Body: rc})
		_ = rc.Close()
		if err != nil {
			return n, err
		}
		if why == "" {
			n++
		}
	}
	return n, nil
}

const videoBackfillKey = "inbox_videos_v1"

// backfillVideos: 1 lần, kéo video của các hồ sơ Messenger đã nhập trước khi có phần video.
func (s *houseService) backfillVideos(ctx context.Context) error {
	x := s.inbox
	if done, _, err := x.state.Get(ctx, videoBackfillKey); err != nil || done != "" {
		return err
	}
	houses, err := s.photos.InboxHouses(ctx)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(houses))
	for k := range houses {
		ids = append(ids, k)
	}
	total := 0
	for len(ids) > 0 {
		chunk := ids[:min(200, len(ids))]
		ids = ids[len(chunk):]
		vs, err := x.src.Videos(ctx, chunk)
		if err != nil {
			return fmt.Errorf("đọc video dezon-inbox: %w", err)
		}
		for logID, keys := range vs {
			n, err := s.inboxVideos(ctx, houses[logID], keys)
			if err != nil {
				return err
			}
			total += n
		}
	}
	log.Printf("Messenger: kéo về %d video cho hồ sơ đã nhập", total)
	return x.state.Set(ctx, videoBackfillKey, s.now().Format(time.RFC3339))
}
