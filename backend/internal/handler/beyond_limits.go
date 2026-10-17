package handler

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

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
