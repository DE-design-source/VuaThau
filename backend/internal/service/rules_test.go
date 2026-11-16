package service

import (
	"slices"
	"testing"
	"time"

	"vuathau/backend/internal/integration/ai"
	"vuathau/backend/internal/model"
)

func TestPhotoProblems(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, VN)
	f := func(v float64) *float64 { return &v }
	yes, no := true, false
	dup := int64(7)
	cases := []struct {
		name string
		p    model.Photo
		want []string
	}{
		{"hợp lệ", model.Photo{ExifTime: "2026-11-15 09:00", Lat: f(10.8)}, []string{}},
		{"giờ AI thay EXIF", model.Photo{AITime: "2026-11-15 09:00", Address: "Q1"}, []string{}},
		{"không giờ, không vị trí", model.Photo{}, []string{"Không đọc được thời gian chụp", "Không có vị trí"}},
		{"quá 30 ngày", model.Photo{ExifTime: "2026-10-01 09:00", Lat: f(1)}, []string{"Ảnh chụp quá 30 ngày"}},
		{"tương lai", model.Photo{ExifTime: "2026-11-16 12:00", Lat: f(1)}, []string{"Thời gian chụp ở tương lai"}},
		{"lệch giờ", model.Photo{ExifTime: "2026-11-15 09:00", AITime: "2026-11-15 11:00", Lat: f(1)},
			[]string{"Giờ in trên ảnh lệch giờ chụp thật"}},
		{"không phải công trình + trùng", model.Photo{ExifTime: "2026-11-15 09:00", Lat: f(1), IsSite: &no, DupOf: &dup},
			[]string{"Không thấy công trình đang xây", "Trùng ảnh đã nộp (hồ sơ #7)"}},
		{"là công trình", model.Photo{ExifTime: "2026-11-15 09:00", Lat: f(1), IsSite: &yes}, []string{}},
	}
	for _, c := range cases {
		if got := PhotoProblems(&c.p, now); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAmount(t *testing.T) {
	s := model.DefaultSettings
	for n, want := range map[int]int64{0: 0, 4: 0, 5: 35000, 6: 50000, 20: 50000} {
		if got := s.Amount(n); got != want {
			t.Errorf("Amount(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestBank(t *testing.T) {
	if got := BankFrom("Vietcombank", "0123 456 789", "nguyen van a"); got != "Vietcombank · 0123456789 · NGUYEN VAN A" {
		t.Fatalf("BankFrom = %q", got)
	}
	for _, bad := range [][3]string{{"", "0123456789", "A"}, {"VCB", "12", "A"}, {"VCB", "0123456789", ""}, {"VCB", "12ab5678", "A"}} {
		if BankFrom(bad[0], bad[1], bad[2]) != "" {
			t.Errorf("BankFrom(%v) should fail", bad)
		}
	}
	if b := BankParts("Vietcombank · 0123456789 · NGUYEN VAN A"); b.No != "0123456789" || b.Owner != "NGUYEN VAN A" {
		t.Fatalf("BankParts = %+v", b)
	}
	// dữ liệu cũ gõ tự do: đoán số TK
	if b := BankParts("VCB stk 0123 456 789 chu tk A"); b.No != "0123456789" {
		t.Fatalf("BankParts free text = %+v", b)
	}
}

func TestChecklist(t *testing.T) {
	floors := 3
	h := &model.House{Address: "A", ContractorName: "B", ContractorPhone: "123", Floors: &floors, Stage: model.Stages[0]}
	items := Checklist(h, 4, 5)
	bad := []string{}
	for _, it := range items {
		if !it.OK {
			bad = append(bad, it.Key)
		}
	}
	if !slices.Equal(bad, []string{"anh", "contractor_phone"}) {
		t.Fatalf("not ok = %v", bad)
	}
	if items[3].Message != "SĐT nhà thầu không hợp lệ" {
		t.Fatalf("message = %q", items[3].Message)
	}
}

func TestSteps(t *testing.T) {
	h := &model.House{Status: model.StatusRejected}
	st := Steps(h, nil, 0, 5)
	got := []string{}
	for _, s := range st {
		got = append(got, s.State)
	}
	if !slices.Equal(got, []string{"done", "done", "bad", "todo"}) {
		t.Fatalf("states = %v", got)
	}
	h.Status = model.StatusApproved
	st = Steps(h, nil, 5, 5)
	if st[3].State != "current" || st[3].Hint == "" {
		t.Fatalf("payment step = %+v", st[3])
	}
}

func TestMergeReads(t *testing.T) {
	rs := []*ai.Read{
		{ContractorName: "KIM PHƯỚC", ContractorPhone: "", Floors: 2, Stage: model.Stages[0]},
		{ContractorName: "Kim Phước", ContractorPhone: "0901 111 222", Floors: 3, Stage: model.Stages[1]},
		{ContractorName: "Công ty Khác", ContractorPhone: "0909999999"},
		{ContractorPhone: "0988888888"},
	}
	got := MergeReads(rs)
	if got.ContractorName != "Kim Phước" || got.ContractorPhone != "0901111222" || *got.Floors != 3 || got.Stage != model.Stages[1] {
		t.Fatalf("merge = %+v", got)
	}
	none := MergeReads([]*ai.Read{{ContractorPhone: "0988888888"}})
	if none.ContractorName != "" || none.ContractorPhone != "0988888888" || none.Floors != nil {
		t.Fatalf("merge without name = %+v", none)
	}
}

func TestMoneyAndParse(t *testing.T) {
	for n, want := range map[int64]string{0: "0đ", 35000: "35.000đ", 1250000: "1.250.000đ", -20000: "-20.000đ"} {
		if got := Money(n); got != want {
			t.Errorf("Money(%d) = %q", n, got)
		}
	}
	ms := ParseMetas(`[{"t":"2026-11-15 09:00","lat":10.8,"lng":106.7,"addr":"Q1"},{"t":"bad"}]`)
	if len(ms) != 2 || ms[0].clean().T == "" || ms[1].clean().T != "" {
		t.Fatalf("metas = %+v", ms)
	}
	if one := ParseMetas(`{"lat":0,"lng":0}`); len(one) != 1 || one[0].clean().Lat != nil {
		t.Fatalf("0,0 must be dropped: %+v", one)
	}
}
