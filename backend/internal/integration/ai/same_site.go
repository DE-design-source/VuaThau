package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// SameSite: AI so ảnh 2 hồ sơ có phải cùng một công trình không.
type SameSite struct {
	Same       bool    `json:"same"`
	Confidence float64 `json:"confidence"` // 0..1
	Reason     string  `json:"reason"`     // 1 câu tiếng Việt cho admin
}

// Comparer: AI so 2 bộ ảnh. claudeReader có; AI tắt thì người gọi tự xử lý (chỉ dựa vào toạ độ + SĐT nhà thầu).
type Comparer interface {
	SameSite(ctx context.Context, a, b [][]byte) (*SameSite, error)
}

var sameSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"same", "confidence", "reason"},
	"properties": map[string]any{
		"same":       map[string]any{"type": "boolean"},
		"confidence": map[string]any{"type": "number"},
		"reason":     map[string]any{"type": "string"},
	},
}

const samePrompt = `Bạn so ảnh nghiệm thu nhà đang xây ở Việt Nam để chống nộp trùng: mỗi công trình chỉ được nhận một hồ sơ.
Nhóm A và nhóm B là ảnh của hai hồ sơ, chụp ở gần nhau (cách vài chục mét) và cách nhau không quá vài tuần, có thể bởi hai người khác nhau.
Trả JSON:
- same: true nếu A và B chụp CÙNG một căn nhà / công trình (dù góc chụp, giờ chụp, tiến độ khác nhau);
  false nếu là hai căn khác nhau (vd hai nhà liền kề, cùng dãy, cùng nhà thầu nhưng khác căn).
- confidence: độ chắc chắn từ 0 đến 1.
- reason: một câu ngắn tiếng Việt nêu điểm giống / khác quyết định (mặt tiền, số tầng, màu tường, bảng hiệu nhà thầu,
  giấy phép xây dựng, nhà bên cạnh, cột điện, biển số nhà...).
Dấu hiệu cùng công trình: cùng bảng hiệu / giấy phép, cùng mặt tiền và nhà hàng xóm, cùng bố cục cột - sàn - ô cửa
(tiến độ B có thể xa hơn A). Không kết luận chỉ vì cùng tên nhà thầu hay cùng con đường.`

func imageBlocks(label string, images [][]byte) []anthropic.BetaContentBlockParamUnion {
	out := []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(label)}
	for _, b := range images {
		out = append(out, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      base64.StdEncoding.EncodeToString(b),
			MediaType: anthropic.BetaBase64ImageSourceMediaTypeImageJPEG,
		}))
	}
	return out
}

func (c *claudeReader) SameSite(ctx context.Context, a, b [][]byte) (*SameSite, error) {
	if len(a) == 0 || len(b) == 0 {
		return nil, errors.New("thiếu ảnh để so")
	}
	content := append(imageBlocks("Nhóm A:", a), imageBlocks("Nhóm B:", b)...)
	content = append(content, anthropic.NewBetaTextBlock("A và B có phải cùng một công trình không?"))
	params := anthropic.BetaMessageNewParams{
		Model:     c.model,
		MaxTokens: 4000,
		System: []anthropic.BetaTextBlockParam{{
			Text:         samePrompt,
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		}},
		// Câu hỏi có/không trên ít ảnh: effort thấp cho nhanh (CTV đang chờ bấm Gửi duyệt)
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortLow,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: sameSchema},
		},
		Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(content...)},
	}
	msg, err := c.client.Beta.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("AI so ảnh: %w", err)
	}
	log.Printf("AI so ảnh %d+%d [%s]: vào %d token, ra %d token", len(a), len(b), msg.Model,
		msg.Usage.InputTokens, msg.Usage.OutputTokens)
	if msg.StopReason != anthropic.BetaStopReasonEndTurn {
		return nil, fmt.Errorf("AI so ảnh dừng: %s", msg.StopReason)
	}
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			var out SameSite
			if err := json.Unmarshal([]byte(t.Text), &out); err != nil {
				return nil, fmt.Errorf("AI so ảnh trả dữ liệu không đọc được: %w", err)
			}
			return &out, nil
		}
	}
	return nil, errors.New("AI so ảnh không trả nội dung")
}
