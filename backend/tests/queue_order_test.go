package tests

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"vuathau/backend/internal/model"
)

// Hàng chờ duyệt sắp theo ngày tạo (ngày CTV gửi, cột "Ngày tạo"), gửi trước duyệt trước; hồ sơ nhập muộn (Messenger) có ngày cũ
// nằm đúng ngày của nó, không nhảy lên đầu. "Hồ sơ kế tiếp" đi đúng thứ tự đó.
func TestQueueOrderByDate(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	admin := e.staff(t, model.RoleAdmin)
	ctv := e.register(t, "CTV hàng chờ")
	a := e.newHouse(t, ctv, 5, 9600, 10.61) // 18/11
	b := e.newHouse(t, ctv, 5, 9610, 10.63) // 19/11
	c := e.newHouse(t, ctv, 5, 9620, 10.65) // nhập muộn nhất (mã lớn nhất) nhưng ngày 17/11
	ctx := context.Background()
	for id, day := range map[int64]string{a: "2026-11-18 10:00", b: "2026-11-19 10:00", c: "2026-11-17 10:00"} {
		if _, err := e.db.Exec(ctx, `UPDATE houses SET created_at = $2::timestamptz WHERE id = $1`, id, day+"+07"); err != nil {
			t.Fatal(err)
		}
	}
	var q struct {
		Rows []struct {
			ID int64 `json:"id"`
		} `json:"rows"`
	}
	r := e.do(t, http.MethodGet, "/api/v1/admin/queue", admin.AccessToken, nil)
	expectStatus(t, r, http.StatusOK)
	decodeData(t, r, &q)
	got := []int64{}
	for _, x := range q.Rows {
		got = append(got, x.ID)
	}
	if fmt.Sprint(got) != fmt.Sprint([]int64{c, a, b}) {
		t.Fatalf("thứ tự hàng chờ %v, muốn %v", got, []int64{c, a, b})
	}
	// duyệt lần lượt: c (17/11) -> a (18/11) -> b (19/11)
	var res struct {
		NextID *int64 `json:"next_id"`
	}
	r = e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/houses/%d/review", c), admin.AccessToken, map[string]string{"action": "duyet"})
	decodeData(t, r, &res)
	if res.NextID == nil || *res.NextID != a {
		t.Fatalf("sau c: %v", res.NextID)
	}
	r = e.do(t, http.MethodPost, fmt.Sprintf("/api/v1/houses/%d/review", a), admin.AccessToken, map[string]string{"action": "duyet"})
	decodeData(t, r, &res)
	if res.NextID == nil || *res.NextID != b {
		t.Fatalf("sau a: %v", res.NextID)
	}
}
