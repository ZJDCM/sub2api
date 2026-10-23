package handler

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// beyondLimitsHeader 把本次请求的破甲结论直接写在响应头上，取值形如
// "deepseek/21413"（平台/注入的 bridge 字节数）；"off" 表示该请求没走破甲。
// 客户端与运维不必再翻日志确认注入是否生效。
const beyondLimitsHeader = "X-Beyond-Limits"

// beyondLimitsRequest 是单个请求的破甲管道运行时上下文，封装平台判定、开关读取、
// 响应篡改包装与交互记录，供 chat completions / responses / gemini 三条入站路径复用。
type beyondLimitsRequest struct {
	platform service.BeyondLimitsPlatform
	engine   *service.BeyondLimitsTamperEngine
	bridge   string
	model    string
	prompt   string
	category string
	start    time.Time
	capture  service.BeyondLimitsCapture
	// tamper 控制是否对响应做拒绝篡改；仅 openai 平台启用，deepseek/gemini 只注入提示词。
	tamper bool
}

// resolveBeyondLimits 根据分组开关与模型名判定平台，返回该请求的破甲上下文；未启用返回 nil。
func resolveBeyondLimits(group *service.Group, model string, body []byte) *beyondLimitsRequest {
	if group == nil {
		return nil
	}
	platform := service.DetectBeyondLimitsPlatform(model)
	// Antigravity 分组统一走 Gemini 破甲：该平台既承载 gemini-* 也承载 claude-*，
	// 按模型前缀判定会把 claude-* 误判成 openai 并套用响应篡改。这里以分组平台
	// 为准，固定使用 Gemini 配置与 gemini 分组开关。
	if group.Platform == service.PlatformAntigravity {
		platform = service.BeyondLimitsPlatformGemini
	}
	enabled := false
	switch platform {
	case service.BeyondLimitsPlatformDeepSeek:
		enabled = group.BeyondLimitsDeepseekEnabled
	case service.BeyondLimitsPlatformGemini:
		enabled = group.BeyondLimitsGeminiEnabled
	default:
		enabled = group.BeyondLimitsEnabled
	}
	if !enabled {
		return nil
	}
	cfg, engine := service.GetBeyondLimitsConfigFor(platform)
	prompt := service.ExtractUserMessage(body)
	category := string(service.CategorizeBeyondLimits(prompt))
	if r := []rune(prompt); len(r) > 200 {
		prompt = string(r[:200])
	}
	return &beyondLimitsRequest{
		platform: platform,
		engine:   engine,
		bridge:   cfg.Bridge,
		model:    model,
		prompt:   prompt,
		category: category,
		start:    time.Now(),
		tamper:   platform == service.BeyondLimitsPlatformOpenAI,
	}
}

// tag 在响应头写回命中结论，供客户端自证。
func (b *beyondLimitsRequest) tag(c *gin.Context) {
	if b == nil || c == nil {
		return
	}
	c.Header(beyondLimitsHeader, string(b.platform)+"/"+strconv.Itoa(len(b.bridge)))
}

// markBeyondLimitsOff 在未启用/未命中时显式标记 off，避免"没注入"与"没这个头"混淆。
func markBeyondLimitsOff(c *gin.Context) {
	if c == nil {
		return
	}
	c.Header(beyondLimitsHeader, "off")
}

// wrapWriter 包装响应 writer 做拒绝篡改，并捕获篡改结果供 record 使用。
// 非 openai 平台（deepseek/gemini）只注入提示词，不篡改响应，原样透传。
func (b *beyondLimitsRequest) wrapWriter(w gin.ResponseWriter, schema service.BeyondLimitsResponseSchema, isStream bool) gin.ResponseWriter {
	if !b.tamper {
		return w
	}
	wrapped := service.NewBeyondLimitsWriter(w, b.engine, schema, isStream)
	if cap, ok := wrapped.(service.BeyondLimitsCapture); ok {
		b.capture = cap
	}
	return wrapped
}

// record 记录一条破甲交互到监控。
func (b *beyondLimitsRequest) record() {
	rec := service.BeyondLimitsInteraction{
		Time:       b.start,
		Platform:   string(b.platform),
		Model:      b.model,
		Category:   b.category,
		UserPrompt: b.prompt,
		Injected:   true,
	}
	if b.capture != nil {
		rec.Tampered = b.capture.Tampered()
		rec.ReplyPreview = b.capture.ReplyPreview()
	}
	rec.DurationMs = time.Since(b.start).Milliseconds()
	service.RecordBeyondLimitsInteraction(rec)
}
