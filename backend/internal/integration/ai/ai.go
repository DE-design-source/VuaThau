// Package ai: Claude đọc ảnh nghiệm thu (giờ in trên dải Timemark, địa chỉ, toạ độ, có phải công trình,
// nhà thầu trên bảng hiệu, số tầng, giai đoạn). Port từ photo.ai_read của bản Python.
package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"vuathau/backend/internal/model"
)

// PhotoRead: AI đọc 1 ảnh. Index bắt đầu từ 0 theo thứ tự ảnh gửi.
type PhotoRead struct {
	Index   int     `json:"index"`
	Time    string  `json:"time"`
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	IsSite  bool    `json:"is_site"`
	Inside  bool    `json:"inside"` // ảnh chụp bên trong công trình (trong nhà), không phải mặt ngoài
	Cover   int     `json:"cover"`  // 0..10: hợp làm ảnh đại diện công trình (toàn cảnh, rõ, đúng chiều)
}

// Read: kết quả AI đọc 1 bộ ảnh của 1 căn nhà.
type Read struct {
	Photos          []PhotoRead `json:"photos"`
	ContractorName  string      `json:"contractor_name"`
	ContractorPhone string      `json:"contractor_phone"`
	Floors          int         `json:"floors"`
	Stage           string      `json:"stage"`
}

// Reader đọc ảnh. Nil hoặc Enabled() = false: tắt AI, CTV tự điền.
type Reader interface {
	Enabled() bool
	// ReadPhotos: images là JPEG đã thu nhỏ. Lỗi / bị từ chối: trả lỗi, người gọi coi như AI chưa đọc được.
	ReadPhotos(ctx context.Context, images [][]byte) (*Read, error)
}

type claudeReader struct {
	client anthropic.Client
	model  string
}

// New: AI bật khi enabled (có ANTHROPIC_API_KEY / ANTHROPIC_AUTH_TOKEN, SDK tự đọc).
// workspace: chỉ cần với key cá nhân (sk-ant-usr-...).
func New(enabled bool, modelID, workspace string) Reader {
	if !enabled {
		return disabled{}
	}
	opts := []option.RequestOption{option.WithRequestTimeout(5 * time.Minute), option.WithMaxRetries(2)}
	if workspace != "" {
		opts = append(opts, option.WithHeader("anthropic-workspace-id", workspace))
	}
	return &claudeReader{client: anthropic.NewClient(opts...), model: modelID}
}

type disabled struct{}

func (disabled) Enabled() bool { return false }
func (disabled) ReadPhotos(context.Context, [][]byte) (*Read, error) {
	return nil, errors.New("AI chưa bật")
}

func (c *claudeReader) Enabled() bool { return true }

var stagesEnum = append(append([]any{}, toAny(model.Stages)...), "")

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// schema: JSON schema của kết quả (structured outputs), giống SCHEMA của bản Python.
var schema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"photos", "contractor_name", "contractor_phone", "floors", "stage"},
	"properties": map[string]any{
		"photos": map[string]any{"type": "array", "items": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"index", "time", "address", "lat", "lng", "is_site", "inside", "cover"},
			"properties": map[string]any{
				"index": map[string]any{"type": "integer"}, "time": map[string]any{"type": "string"},
				"address": map[string]any{"type": "string"}, "lat": map[string]any{"type": "number"},
				"lng": map[string]any{"type": "number"}, "is_site": map[string]any{"type": "boolean"},
				"inside": map[string]any{"type": "boolean"}, "cover": map[string]any{"type": "integer"},
			},
		}},
		"contractor_name":  map[string]any{"type": "string"},
		"contractor_phone": map[string]any{"type": "string"},
		"floors":           map[string]any{"type": "integer"},
		"stage":            map[string]any{"type": "string", "enum": stagesEnum},
	},
}

// system: lời dặn mặc định + quy tắc admin (nếu có). Đánh dấu lưu đệm ở khối cuối (lưu cả hai).
func system(rules string) []anthropic.BetaTextBlockParam {
	blocks := []anthropic.BetaTextBlockParam{{Text: prompt}}
	if rules != "" {
		blocks = append(blocks, anthropic.BetaTextBlockParam{Text: "Quy tắc thêm của admin Vua Thầu khi đọc và duyệt ảnh. " +
			"Làm theo các quy tắc này; nếu mâu thuẫn với hướng dẫn ở trên thì ưu tiên quy tắc này. Vẫn trả đúng JSON theo schema.\n\n" + rules})
	}
	blocks[len(blocks)-1].CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
	return blocks
}

var prompt = fmt.Sprintf(`Đây là các ảnh nghiệm thu của MỘT căn nhà đang xây ở Việt Nam, chụp bằng app Timemark (có dải chữ in trên ảnh: ngày giờ, địa chỉ, có thể có toạ độ).
Đọc và trả JSON theo schema:
- photos: mỗi ảnh một phần tử, index bắt đầu từ 0 theo thứ tự ảnh gửi.
  time: ngày giờ in trên ảnh, dạng "YYYY-MM-DD HH:MM" (ngày Việt Nam thường viết dd/mm/yyyy). Không có thì "".
  address: địa chỉ in trên ảnh, giữ nguyên tiếng Việt. Không có thì "".
  lat, lng: toạ độ in trên ảnh nếu có, không có thì 0.
  is_site: true nếu ảnh thực sự chụp công trình nhà đang xây/sửa; false nếu là ảnh màn hình, ảnh chụp lại ảnh, hoặc không phải công trình.
  inside: true nếu ảnh chụp BÊN TRONG căn nhà (đứng trong nhà: thấy trần / sàn / tường phía trong, cầu thang, phòng, ống điện nước trong tường); false nếu chụp mặt ngoài, mặt tiền, từ đường hay từ nhà bên cạnh nhìn vào.
  cover: điểm 0..10 ảnh này hợp làm ẢNH ĐẠI DIỆN công trình đến đâu. Cao (8-10): thấy TOÀN CẢNH căn nhà / mặt tiền từ ngoài đường, đủ các tầng, rõ nét, đủ sáng, đúng chiều (không xoay ngang, không lộn), nhìn là biết công trình đang xây.
    Thấp (0-3): ảnh mờ, nhoè, tối, bị xoay; chụp sát vào tường, sàn, trần, cột, góc nhỏ; bị che khuất (xe, cây, người, tay); chụp bên trong; ảnh bảng hiệu / giấy tờ cận cảnh; ảnh màn hình; is_site false thì 0.
- contractor_name, contractor_phone: nhà thầu / đơn vị thi công trên băng rôn, bảng hiệu, bạt che, biển công trình
  (thường ghi "Đơn vị thi công", "Nhà thầu", "Thi công", tên công ty + SĐT/hotline). Ghi đủ tên như in, SĐT in cạnh tên đó.
  Đọc kỹ cả chữ nhỏ trên mọi ảnh. Không lấy chủ đầu tư, đơn vị thiết kế, hay tên/logo trong dải chữ của app chụp ảnh (vd logo công ty người chụp in cạnh giờ Timemark).
Các ô chữ (time, address, lat/lng, tên, SĐT): chỉ ghi chữ đọc rõ trên ảnh, không bịa. Số điện thoại viết liền, không dấu cách. Không đọc được thì để trống.
Hai ô dưới thì KHÔNG có chữ in sẵn: nhìn công trình trên tất cả ảnh rồi tự đánh giá, luôn điền; chỉ để 0 / "" khi không ảnh nào thấy căn nhà.
- floors: số tầng của căn nhà, kể cả tầng trệt và tầng đang đổ/đang xây, không tính tum, mái. Nếu bảng giấy phép xây dựng ghi số tầng thì lấy số đó; không thì đếm số sàn, số dãy cửa sổ hoặc số đợt cột, giàn giáo. Mới làm móng, chưa lên tầng nào thì 1.
- stage: chọn đúng một trong %q, theo phần tiến độ xa nhất thấy được:
  %q: đất trống, dọn dẹp, rào tôn, bảng giấy phép, cắm mốc; chưa đào.
  %q: máy ép cọc, đầu cọc bê tông, hố móng đang đào.
  %q: thép móng, cốp pha móng, đà kiềng, hầm phân; chưa có cột tầng trệt.
  %q: cột, dầm, sàn bê tông, cốp pha, giàn giáo; chưa xây tường bao.
  %q: tường gạch đã xây (chưa trát), cầu thang, đổ mái.
  %q: đục tường đi ống điện nước, chống thấm; tường chưa trát xong.
  %q: tường đã trát hoặc sơn, ốp lát, lắp cửa, mặt tiền đang làm.
  %q: nhà xong hết, đã tháo giàn giáo, sạch sẽ hoặc có người ở.`,
	model.Stages, model.Stages[0], model.Stages[1], model.Stages[2], model.Stages[3], model.Stages[4], model.Stages[5], model.Stages[6], model.Stages[7])

// Quy tắc duyệt ảnh admin tự viết (trang Cấu hình): gửi kèm sau lời dặn mặc định, ưu tiên hơn khi mâu thuẫn.
type rulesKey struct{}

// WithRules: gắn quy tắc admin vào ctx cho lượt đọc ảnh.
func WithRules(ctx context.Context, rules string) context.Context {
	if strings.TrimSpace(rules) == "" {
		return ctx
	}
	return context.WithValue(ctx, rulesKey{}, strings.TrimSpace(rules))
}

func RulesFrom(ctx context.Context) string {
	r, _ := ctx.Value(rulesKey{}).(string)
	return r
}

// DefaultPrompt: lời dặn mặc định (trang Cấu hình hiện để admin tham khảo khi viết quy tắc thêm).
func DefaultPrompt() string { return prompt }

// MaxRules: độ dài tối đa quy tắc admin (ký tự).
const MaxRules = 8000

func (c *claudeReader) ReadPhotos(ctx context.Context, images [][]byte) (*Read, error) {
	if len(images) == 0 {
		return &Read{}, nil
	}
	content := make([]anthropic.BetaContentBlockParamUnion, 0, len(images)+1)
	for _, b := range images {
		content = append(content, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      base64.StdEncoding.EncodeToString(b),
			MediaType: anthropic.BetaBase64ImageSourceMediaTypeImageJPEG,
		}))
	}
	content = append(content, anthropic.NewBetaTextBlock("Đọc các ảnh trên theo hướng dẫn."))

	params := anthropic.BetaMessageNewParams{
		Model:     c.model,
		MaxTokens: 16000,
		// Lời dặn giống hệt mọi lượt (kèm quy tắc admin): đánh dấu lưu đệm để chỉ tính đủ tiền lần đầu
		System:   system(RulesFrom(ctx)),
		Thinking: anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortMedium,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
		},
		Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(content...)},
	}
	// Bị từ chối (refusal): máy chủ tự chuyển sang model dự phòng (Haiku không có dự phòng)
	if !strings.Contains(c.model, "haiku") {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
	}

	msg, err := c.client.Beta.Messages.New(ctx, params)
	if err != nil {
		var apierr *anthropic.Error
		if errors.As(err, &apierr) {
			switch apierr.StatusCode {
			case http.StatusTooManyRequests:
				return nil, fmt.Errorf("AI bị giới hạn tốc độ: %w", err)
			default:
				return nil, fmt.Errorf("AI lỗi %d: %w", apierr.StatusCode, err)
			}
		}
		return nil, fmt.Errorf("AI mất kết nối: %w", err)
	}
	u := msg.Usage
	log.Printf("AI %d ảnh [%s]: vào %d token, ra %d token, đệm đọc %d, đệm ghi %d", len(images), msg.Model,
		u.InputTokens, u.OutputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens)
	if msg.StopReason != anthropic.BetaStopReasonEndTurn {
		return nil, fmt.Errorf("AI dừng: %s", msg.StopReason)
	}
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			var out Read
			if err := json.Unmarshal([]byte(t.Text), &out); err != nil {
				return nil, fmt.Errorf("AI trả dữ liệu không đọc được: %w", err)
			}
			return &out, nil
		}
	}
	return nil, errors.New("AI không trả nội dung")
}
