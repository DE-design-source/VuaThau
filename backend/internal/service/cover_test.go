package service

import (
	"slices"
	"testing"

	"vuathau/backend/internal/model"
)

// Ảnh AI chấm đại diện cao nhất lên đầu; chưa chấm (0 hết) thì giữ thứ tự như "sign_first".
func TestSortPhotosCoverFirst(t *testing.T) {
	ps := []model.Photo{
		{ID: 1, ExifTime: "2026-11-20 08:00", CoverScore: 2},
		{ID: 2, ExifTime: "2026-11-20 08:01", Masked: 1, CoverScore: 3},
		{ID: 3, ExifTime: "2026-11-20 08:02", CoverScore: 9},
		{ID: 4, ExifTime: "2026-11-20 08:03", CoverScore: 5},
	}
	sortPhotos(ps, "cover_first")
	got := []int64{ps[0].ID, ps[1].ID, ps[2].ID, ps[3].ID}
	want := []int64{3, 2, 1, 4}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	old := []model.Photo{{ID: 1, ExifTime: "2026-11-20 08:00"}, {ID: 2, ExifTime: "2026-11-20 08:01", Masked: 1}}
	sortPhotos(old, "cover_first")
	if old[0].ID != 2 {
		t.Fatalf("chưa chấm: ảnh bảng hiệu phải lên đầu, got %d", old[0].ID)
	}
}
