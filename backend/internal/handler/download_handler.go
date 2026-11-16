package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"vuathau/backend/internal/model"
)

// Tải app Android (link gửi CTV qua Zalo): https://<máy chủ>/tai-app, công khai, không cần đăng nhập.
// Chép APK vào <DATA_DIR>/app/ đặt tên vuathau-nghiemthu-<phiên bản>-<versionCode>.apk.
// Bản versionCode lớn nhất là bản được tải; app hỏi /tai-app/phien-ban để mời CTV cập nhật.
var apkName = regexp.MustCompile(`^vuathau-nghiemthu-([\d.]+)-(\d+)\.apk$`)

type apkFile struct {
	Code    int
	Version string
	Path    string
	MB      int64
}

type DownloadHandler struct {
	dir string
	cfg *PaymentHandler
}

func NewDownloadHandler(dataDir string, cfg *PaymentHandler) *DownloadHandler {
	return &DownloadHandler{dir: filepath.Join(dataDir, "app"), cfg: cfg}
}

func (h *DownloadHandler) latest() *apkFile {
	entries, _ := os.ReadDir(h.dir)
	var best *apkFile
	for _, e := range entries {
		m := apkName.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		code, _ := strconv.Atoi(m[2])
		if best != nil && code <= best.Code {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		best = &apkFile{Code: code, Version: m[1], Path: filepath.Join(h.dir, e.Name()), MB: (info.Size() + 500_000) / 1_000_000}
	}
	return best
}

// Version handles GET /tai-app/phien-ban: {"code": versionCode, "version": "x.y.z"} (chưa có bản: 0, "").
// Dạng trả về giữ như bản cũ (không bọc envelope) vì app Expo cũ cũng đọc địa chỉ này.
func (h *DownloadHandler) Version(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{"code": 0, "version": ""}
	if a := h.latest(); a != nil {
		out = map[string]any{"code": a.Code, "version": a.Version}
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// APK handles GET /tai-app/vuathau.apk
func (h *DownloadHandler) APK(w http.ResponseWriter, r *http.Request) {
	a := h.latest()
	if a == nil {
		http.Error(w, "Chưa có bản app để tải.", http.StatusNotFound)
		return
	}
	f, err := os.Open(a.Path)
	if err != nil {
		http.Error(w, "Chưa có bản app để tải.", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="VuaThau-NghiemThu-%s.apk"`, a.Version))
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// Page handles GET /tai-app: nút tải theo máy (Android / trong Zalo / iPhone).
func (h *DownloadHandler) Page(w http.ResponseWriter, r *http.Request) {
	ua := r.UserAgent()
	data := map[string]any{
		"APK":    h.latest(),
		"Zalo":   strings.Contains(strings.ToLower(ua), "zalo"),
		"IPhone": strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"),
		"Host":   r.Host,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if err := downloadPage.Execute(w, data); err != nil {
		http.Error(w, "Lỗi hiển thị trang", http.StatusInternalServerError)
	}
}

// LegacyConfig handles GET /app.json: cấu hình theo dạng app Expo cũ đọc khi mở.
// Có địa chỉ này app cũ mới mở lên được và hỏi /tai-app/phien-ban, từ đó mời CTV tải app mới.
// Bỏ khi không còn ai dùng app cũ (gói vn.com.vuathau.nghiemthu).
func (h *DownloadHandler) LegacyConfig(w http.ResponseWriter, r *http.Request) {
	c, err := h.cfg.svc.Config(r.Context(), "")
	if err != nil {
		http.Error(w, "{}", http.StatusServiceUnavailable)
		return
	}
	status := map[string][3]string{}
	step := map[string]int{}
	for i, k := range c.StatusOrder {
		s := model.Statuses[k]
		status[k] = [3]string{s.Label, s.CTVLabel, s.Tone}
		step[k] = i
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"STATUS": status, "STEP_OF": step, "PAY_MIN": c.PayMin, "MIN_PHOTOS": c.MinPhotos, "MAX_PHOTOS": c.MaxPhotos,
		"STAGES": c.Stages, "FIELDS": []any{}, "BANKS": c.Banks, "PAY": c.Pay,
		"GOOGLE": "", // app cũ ẩn nút Google
	})
}

var downloadPage = template.Must(template.New("tai-app").Parse(`<!doctype html>
<html lang="vi"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Tải app · Vua Thầu Nghiệm thu</title>
<meta property="og:title" content="Vua Thầu Nghiệm thu: app cho cộng tác viên">
<meta property="og:description" content="Chụp ảnh công trình, nộp hồ sơ nghiệm thu, theo dõi tiền về. Bấm để tải app Android.">
<meta property="og:url" content="https://{{.Host}}/tai-app">
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<style>
:root{--navy:#0f1f3d;--amber:#ffb81c;--bg:#f5f5f8;--card:#fff;--text:#1b2233;--muted:#5d6577;--line:#e6e7ee;--warn:#fff4d6;--warnb:#f0c95a}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
.top{background:var(--navy);color:#fff;text-align:center;padding:36px 16px 28px}
.mark{display:inline-block;width:88px;height:88px;border-radius:20px}
h1{font-size:22px;margin:14px 0 4px}
.top p{margin:0;color:#c5cbe0}
main{max-width:460px;margin:-12px auto 32px;padding:0 16px;display:grid;gap:14px}
.card{background:var(--card);border:1px solid var(--line);border-radius:14px;padding:16px 18px}
.card h2{font-size:17px;margin:0 0 6px}
.btn{display:block;text-align:center;text-decoration:none;font-weight:700;padding:15px;border-radius:12px;background:var(--amber);color:var(--navy)}
.muted{color:var(--muted);margin:0;text-align:center;font-size:14px}
.alert{background:var(--warn);border:1px solid var(--warnb);border-radius:12px;padding:12px 14px;font-size:15px}
ol{margin:6px 0 0;padding-left:22px}li{margin:6px 0}
</style></head><body>
<header class="top">
  <img class="mark" src="/app/icons/Icon-192.png" width="88" height="88" alt="Vua Thầu Map">
  <h1>Vua Thầu Nghiệm thu</h1>
  <p>App cho cộng tác viên: chụp ảnh công trình, nộp hồ sơ, xem tiền về</p>
</header>
<main>
{{if and .Zalo (not .IPhone)}}<div class="alert"><b>Zalo không tải được app.</b> Bấm nút bên dưới để mở bằng Chrome. Không được thì bấm <b>⋯</b> góc trên bên phải, chọn <b>Mở bằng trình duyệt</b>.</div>{{end}}
{{if .IPhone}}
  <a class="btn" href="/app/">Mở app trên iPhone</a>
  <p class="muted">Không cần tải từ App Store · dùng Safari</p>
  {{if .Zalo}}<div class="alert">Đang mở trong Zalo: bấm <b>⋯</b> góc trên bên phải, chọn <b>Mở bằng trình duyệt</b> (Safari) rồi làm theo các bước dưới.</div>{{end}}
  <div class="card"><h2>Đưa app ra màn hình chính</h2><ol>
    <li>Mở trang này bằng <b>Safari</b>, bấm <b>Mở app trên iPhone</b>.</li>
    <li>Bấm nút <b>Chia sẻ</b> (ô vuông có mũi tên lên) ở thanh dưới.</li>
    <li>Kéo xuống, chọn <b>Thêm vào MH chính</b>, bấm <b>Thêm</b>.</li>
    <li>Từ giờ mở app bằng biểu tượng <b>Vua Thầu</b> trên màn hình chính. Lần đầu chụp ảnh, bấm <b>Cho phép</b> camera và vị trí.</li>
  </ol></div>
{{else if and .APK .Zalo}}
  <a class="btn" href="intent://{{.Host}}/tai-app#Intent;scheme=https;package=com.android.chrome;end">Mở bằng Chrome để tải</a>
{{else if .APK}}
  <a class="btn" href="/tai-app/vuathau.apk" download>Tải app Android ({{.APK.MB}} MB)</a>
  <p class="muted">Phiên bản {{.APK.Version}} · nên tải bằng Wi-Fi</p>
  <div class="card"><h2>Cách cài</h2><ol>
    <li>Bấm <b>Tải app Android</b>, chờ tải xong.</li>
    <li>Mở file <b>VuaThau-NghiemThu</b> trong thông báo hoặc thư mục <b>Tải xuống</b>.</li>
    <li>Máy hỏi <b>Cài ứng dụng không rõ nguồn gốc</b>: chọn <b>Cài đặt</b>, bật <b>Cho phép từ nguồn này</b>, rồi quay lại.</li>
    <li>Máy <b>OPPO, realme, OnePlus</b> báo <b>Enhanced installation protection</b>: bấm <b>⋮</b> góc trên bên phải, chọn <b>Deep scan</b>, quét xong chọn tiếp tục cài.</li>
    <li>Bấm <b>Cài đặt</b>, xong bấm <b>Mở</b>. Đăng nhập bằng số điện thoại và mật khẩu đã đăng ký.</li>
  </ol></div>
  <div class="card"><h2>Đang dùng app cũ?</h2><p style="margin:0;color:var(--muted)">App mới cài thành một app riêng, tài khoản và hồ sơ vẫn giữ nguyên. Cài xong, gỡ app cũ đi để khỏi nhầm.</p></div>
  <p class="muted">Có bản mới, app tự báo để bạn tải lại tại trang này.</p>
{{else}}
  <div class="card"><h2>Chưa có bản tải</h2><p style="margin:0;color:var(--muted)">App Android đang được chuẩn bị, bạn quay lại sau nhé.</p></div>
{{end}}
</main></body></html>`))
