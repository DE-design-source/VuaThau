package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vuathau/backend/internal/pkg/response"
)

// InboxNotifyHandler: UniInbox (dezon-inbox) báo bot vừa ghi hồ sơ công trình CTV gửi qua Messenger.
// Lời báo chỉ có id, không có dữ liệu: nhận báo thì đồng bộ ngay (đọc qua API của UniInbox), nên lời báo giả cũng
// không đưa được dữ liệu sai vào. Chữ ký: X-UniInbox-Signature = "sha256=" + hex(HMAC-SHA256(khoá,
// "<X-UniInbox-Timestamp>.<body>")), lệch giờ quá 5 phút thì từ chối.
type InboxNotifyHandler struct {
	secret string
	kick   func()
	now    func() time.Time
}

// NewInboxNotifyHandler: secret = INBOX_NOTIFY_SECRET (trống: tắt, trả 404); kick: đồng bộ ngay (không chờ).
func NewInboxNotifyHandler(secret string, kick func()) *InboxNotifyHandler {
	return &InboxNotifyHandler{secret: secret, kick: kick, now: time.Now}
}

const notifyMaxSkew = 5 * time.Minute

// Notify handles POST /api/v1/integrations/uniinbox/notify
func (h *InboxNotifyHandler) Notify(w http.ResponseWriter, r *http.Request) {
	if h.secret == "" {
		response.Error(w, http.StatusNotFound, "route not found")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "Không đọc được nội dung.")
		return
	}
	ts := r.Header.Get("X-UniInbox-Timestamp")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, "Thiếu hoặc sai thời điểm ký.")
		return
	}
	if d := h.now().Sub(time.Unix(sec, 0)); d > notifyMaxSkew || d < -notifyMaxSkew {
		response.Error(w, http.StatusUnauthorized, "Lời báo quá cũ.")
		return
	}
	m := hmac.New(sha256.New, []byte(h.secret))
	m.Write([]byte(ts + "." + string(body)))
	want := "sha256=" + hex.EncodeToString(m.Sum(nil))
	if !hmac.Equal([]byte(strings.TrimSpace(r.Header.Get("X-UniInbox-Signature"))), []byte(want)) {
		response.Error(w, http.StatusUnauthorized, "Sai chữ ký.")
		return
	}
	if h.kick != nil {
		h.kick()
	}
	response.Success(w, http.StatusAccepted, map[string]bool{"queued": true})
}
