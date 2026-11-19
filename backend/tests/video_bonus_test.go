package tests

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"vuathau/backend/internal/model"
)

// Hồ sơ có video được cộng thưởng (settings video_bonus, mặc định 10.000đ) khi đủ ảnh để được trả tiền.
func TestVideoBonus(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	ctv := e.register(t, "CTV có video")
	admin := e.staff(t, model.RoleAdmin)
	ctx := context.Background()

	submit := func(seed uint64, n int, video bool) int64 {
		t.Helper()
		files := make([][]byte, n)
		for i := range files {
			files[i] = photo(t, seed+uint64(i))
		}
		r := e.upload(t, "/api/v1/houses", ctv.AccessToken, "files", files, map[string]string{"meta": cameraMeta(n, 10.5+float64(seed%100)/100)})
		expectStatus(t, r, http.StatusCreated)
		var up uploadRes
		decodeData(t, r, &up)
		if video {
			r = e.uploadVideos(t, fmt.Sprintf("/api/v1/houses/%d/videos", *up.HouseID), ctv.AccessToken, []string{"quay.mp4"}, [][]byte{fakeVideo(seed, 50_000)})
			expectStatus(t, r, http.StatusOK)
		}
		r = e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/houses/%d", *up.HouseID), ctv.AccessToken, map[string]any{
			"address": "12 Lê Lợi", "floors": 3, "stage": model.Stages[1], "contractor_name": "Xây dựng ABC",
			"contractor_phone": fmt.Sprintf("09012%05d", seed), "submit": true,
		})
		expectStatus(t, r, http.StatusOK)
		return *up.HouseID
	}
	amount := func(id int64) int64 {
		var a int64
		if err := e.db.QueryRow(ctx, `SELECT amount FROM houses WHERE id = $1`, id).Scan(&a); err != nil {
			t.Fatal(err)
		}
		return a
	}

	withVideo := submit(9700, 6, true)
	noVideo := submit(9710, 6, false)
	if a, b := amount(withVideo), amount(noVideo); a != 60000 || b != 50000 {
		t.Fatalf("có video %d (muốn 60.000), không video %d (muốn 50.000)", a, b)
	}

	// trang chi tiết: tiền + tiền theo loại chụp đều gồm thưởng
	var d struct {
		Pay        int64            `json:"pay"`
		PayScope   map[string]int64 `json:"pay_scope"`
		VideoBonus int64            `json:"video_bonus"`
	}
	r := e.do(t, http.MethodGet, fmt.Sprintf("/api/v1/houses/%d", withVideo), admin.AccessToken, nil)
	decodeData(t, r, &d)
	if d.Pay != 60000 || d.PayScope[model.ScopeOut] != 45000 || d.PayScope[model.ScopeBoth] != 60000 || d.VideoBonus != 10000 {
		t.Fatalf("chi tiết: %+v", d)
	}
	// admin chọn chỉ chụp ngoài: 35.000 + 10.000
	expectStatus(t, e.do(t, http.MethodPut, fmt.Sprintf("/api/v1/houses/%d/scope", withVideo), admin.AccessToken,
		map[string]string{"scope": model.ScopeOut}), http.StatusOK)
	if a := amount(withVideo); a != 45000 {
		t.Fatalf("chỉ ngoài có video: %d", a)
	}

	// đổi mức thưởng ở Cấu hình; /config cho app biết mức thưởng
	r = e.do(t, http.MethodGet, "/api/v1/settings", admin.AccessToken, nil)
	var st model.Settings
	decodeData(t, r, &st)
	st.VideoBonus = 20000
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/settings", admin.AccessToken, st), http.StatusOK)
	var cfg struct {
		VideoBonus int64 `json:"video_bonus"`
	}
	decodeData(t, e.do(t, http.MethodGet, "/api/v1/config", "", nil), &cfg)
	if cfg.VideoBonus != 20000 {
		t.Fatalf("config video_bonus: %d", cfg.VideoBonus)
	}
	st.VideoBonus = -1
	expectStatus(t, e.do(t, http.MethodPut, "/api/v1/settings", admin.AccessToken, st), http.StatusBadRequest)
}
