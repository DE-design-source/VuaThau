package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestFillAIDiff(t *testing.T) {
	ai := func(m map[string]any) []byte { b, _ := json.Marshal(m); return b }
	five := 5
	cases := []struct {
		name string
		h    House
		want map[string]string
	}{
		{"chưa có AI", House{ContractorPhone: "0913197706"}, nil},
		{"khác cách viết: không lệch", House{
			ContractorName: "AN PHÁT GIA", ContractorPhone: "0975067595", Floors: &five, Stage: Stages[1],
			AIJSON: ai(map[string]any{"contractor_name": "CÔNG TY TNHH DỊCH VỤ TƯ VẤN XÂY DỰNG AN PHÁT GIA",
				"contractor_phone": "+84975067595", "floors": 5, "stage": Stages[1], "address": "khác hẳn"}),
		}, nil},
		{"bỏ dấu, viết liền", House{ContractorName: "XÂY DỰNG AN NHIÊN (AN NHIÊN GROUP)",
			AIJSON: ai(map[string]any{"contractor_name": "Xây dựng An Nhiên (AN NHIEN GROUP)"})}, nil},
		{"K CONCEPT / Kconcept Co.,Ltd", House{ContractorName: "K CONCEPT",
			AIJSON: ai(map[string]any{"contractor_name": "Kconcept Co.,Ltd"})}, nil},
		{"ô ghi 2 số có chứa số AI đọc", House{ContractorPhone: "0945678300/0939793908",
			AIJSON: ai(map[string]any{"contractor_phone": "0945678300"})}, nil},
		{"AI đọc trống hoặc 0: không lệch", House{ContractorName: "ABC", Floors: &five,
			AIJSON: ai(map[string]any{"contractor_name": "", "floors": 0})}, nil},
		{"lệch thật", House{
			ContractorName: "Công ty TNHH Nhã Trường Thành", ContractorPhone: "0913197706", Floors: &five, Stage: Stages[1],
			AIJSON: ai(map[string]any{"contractor_name": "M+ DESIGN & CONSTRUCTION", "contractor_phone": "0931906384",
				"floors": 3, "stage": Stages[2]}),
		}, map[string]string{"contractor_name": "M+ DESIGN & CONSTRUCTION", "contractor_phone": "0931906384",
			"floors": "3", "stage": Stages[2]}},
	}
	for _, c := range cases {
		c.h.FillAIDiff()
		if !reflect.DeepEqual(c.h.AIDiff, c.want) {
			t.Errorf("%s: AIDiff = %v, muốn %v", c.name, c.h.AIDiff, c.want)
		}
	}
}
