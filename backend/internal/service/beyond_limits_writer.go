package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// BeyondLimitsResponseSchema 区分客户端调用的是 Chat Completions 还是 Responses API，
// 用于构造正确的篡改替换 SSE 帧与 JSON 字段路径。
type BeyondLimitsResponseSchema int

const (
	BeyondLimitsSchemaChat      BeyondLimitsResponseSchema = iota // /v1/chat/completions（openai + deepseek）
	BeyondLimitsSchemaResponses                                   // /v1/responses
	BeyondLimitsSchemaGemini                                      // Gemini 原生 generateContent / streamGenerateContent
)

// BeyondLimitsCapture 暴露篡改结果，供 handler 在响应结束后记录监控。
type BeyondLimitsCapture interface {
	Tampered() bool
	ReplyPreview() string
}

// beyondLimitsWriter 包装 gin.ResponseWriter，对流式/非流式响应做拒绝检测与改写。
//
// 流式（SSE）：逐帧透传（保持 Size/Written 实时语义，不破坏 failover），同时累积正文；
// 一旦命中拒绝规则，改写当前帧为 Rei Protocol 并吞掉后续正文帧，仅透传 [DONE]/结束帧。
// 非流式（application/json）：缓冲完整 JSON，命中则改写 content 字段后一次写出。
type beyondLimitsWriter struct {
	gin.ResponseWriter
	engine *BeyondLimitsTamperEngine
	schema BeyondLimitsResponseSchema
	isStream bool

	mu         sync.Mutex
	lineBuf    []byte          // 流式：未完成的行
	contentAcc strings.Builder // 流式：累积的正文（供检测与预览）
	jsonBuf    []byte          // 非流式：缓冲的 JSON
	replaced   bool            // 是否已输出 Rei Protocol
	tampered   bool            // 是否触发过篡改
	jsonDone   bool            // 非流式：是否已写出
}

// NewBeyondLimitsWriter 创建响应包装器。engine 为 nil 时透传（不检测）。
func NewBeyondLimitsWriter(rw gin.ResponseWriter, engine *BeyondLimitsTamperEngine, schema BeyondLimitsResponseSchema, isStream bool) gin.ResponseWriter {
	if engine == nil {
		return rw
	}
	return &beyondLimitsWriter{
		ResponseWriter: rw,
		engine:         engine,
		schema:         schema,
		isStream:       isStream,
	}
}

// Tampered 返回是否触发过篡改。
func (w *beyondLimitsWriter) Tampered() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tampered
}

// ReplyPreview 返回累积正文的截断预览（用于监控）。
func (w *beyondLimitsWriter) ReplyPreview() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return truncateUTF8Bytes(w.contentAcc.String(), 200)
}

func (w *beyondLimitsWriter) WriteHeader(code int) {
	// 非流式：先把状态/头提交给真实 writer，正文再缓冲改写（见 Write）。
	w.ResponseWriter.WriteHeader(code)
}

func (w *beyondLimitsWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *beyondLimitsWriter) Write(p []byte) (int, error) {
	if !w.isStream {
		return w.writeNonStream(p)
	}
	return w.writeStream(p)
}

func (w *beyondLimitsWriter) Flush() {
	if fl, ok := w.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

// writeNonStream 缓冲 JSON，命中拒绝则改写 content 字段；完整时一次写出。
func (w *beyondLimitsWriter) writeNonStream(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.jsonBuf = append(w.jsonBuf, p...)
	if w.jsonDone {
		return len(p), nil
	}
	if !json.Valid(w.jsonBuf) {
		return len(p), nil
	}

	out := w.jsonBuf
	if !w.replaced {
		// 解析 reply，检测拒绝。
		reply := extractNonStreamReply(w.jsonBuf)
		if replacement, matched := w.engine.Intercept("", reply); matched {
			w.tampered = true
			w.replaced = true
			out = rewriteNonStreamContent(w.jsonBuf, w.schema, replacement)
		}
	}
	w.jsonDone = true
	_, err := w.ResponseWriter.Write(out)
	return len(p), err
}

// writeStream 逐帧透传 + 累积检测。
func (w *beyondLimitsWriter) writeStream(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.lineBuf = append(w.lineBuf, p...)
	// 逐完整行处理；保留末尾未完成行。
	for {
		idx := bytes.IndexByte(w.lineBuf, '\n')
		if idx < 0 {
			break
		}
		line := w.lineBuf[:idx]
		w.lineBuf = w.lineBuf[idx+1:]
		w.handleSSELine(line)
	}

	if len(w.lineBuf) == 0 {
		return len(p), nil
	}
	// 处理已累积但未换行的部分（通常不会发生；SSE 总是以 \n 分帧）。
	return len(p), nil
}

func (w *beyondLimitsWriter) handleSSELine(line []byte) {
	line = bytes.TrimSuffix(line, []byte("\r"))
	text := string(line)

	// 非 data 行直接透传。
	if !strings.HasPrefix(text, "data:") {
		_, _ = w.ResponseWriter.WriteString(text + "\n")
		return
	}

	dataStr := strings.TrimSpace(strings.TrimPrefix(text, "data:"))
	if dataStr == "[DONE]" || dataStr == "" {
		_, _ = w.ResponseWriter.WriteString(text + "\n")
		return
	}

	if !w.replaced {
		content := extractSSEContent([]byte(dataStr), w.schema)
		if content != "" {
			if w.schema == BeyondLimitsSchemaGemini {
				// Gemini 流式每帧携带完整累积正文（非增量 delta），直接替换而非累加。
				w.contentAcc.Reset()
			}
			w.contentAcc.WriteString(content)
		}
		if replacement, matched := w.engine.Intercept("", w.contentAcc.String()); matched {
			w.tampered = true
			w.replaced = true
			w.writeReiSSE(replacement)
			return
		}
		_, _ = w.ResponseWriter.WriteString(text + "\n")
		return
	}

	// replaced 之后：吞掉正文帧，仅透传结束信号。
	// Gemini 替换帧已自带 finishReason，后续帧全部吞掉，避免重复结束。
	if w.schema != BeyondLimitsSchemaGemini && hasFinishSignal(dataStr) {
		_, _ = w.ResponseWriter.WriteString(text + "\n")
	}
}

// writeReiSSE 输出一条 Rei Protocol 替换帧（按 schema 构造）。
func (w *beyondLimitsWriter) writeReiSSE(replacement string) {
	escaped, _ := json.Marshal(replacement)
	switch w.schema {
	case BeyondLimitsSchemaResponses:
		frame := `data: {"type":"response.output_text.delta","delta":` + string(escaped) + `}` + "\n\n"
		_, _ = w.ResponseWriter.WriteString(frame)
	case BeyondLimitsSchemaGemini:
		frame := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":` + string(escaped) + `}]},"finishReason":"STOP"}]}` + "\n\n"
		_, _ = w.ResponseWriter.WriteString(frame)
	default:
		frame := `data: {"id":"beyond-limits","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":` + string(escaped) + `}}]}` + "\n\n"
		_, _ = w.ResponseWriter.WriteString(frame)
	}
}

// hasFinishSignal 判断 data JSON 是否携带结束信号（finish_reason / 完成事件）。
func hasFinishSignal(dataStr string) bool {
	if gjson.Get(dataStr, "choices.0.finish_reason").Exists() {
		return true
	}
	// Gemini：candidates.0.finishReason 非空即流结束。
	if fr := gjson.Get(dataStr, "candidates.0.finishReason"); fr.Exists() && strings.TrimSpace(fr.String()) != "" {
		return true
	}
	t := gjson.Get(dataStr, "type").String()
	return strings.Contains(t, "completed") || strings.Contains(t, "response.completed")
}

// extractSSEContent 从 SSE data 帧提取正文增量。
func extractSSEContent(data []byte, schema BeyondLimitsResponseSchema) string {
	if schema == BeyondLimitsSchemaResponses {
		if d := gjson.GetBytes(data, "delta"); d.Exists() {
			return d.String()
		}
		if t := gjson.GetBytes(data, "type").String(); strings.Contains(t, "output_text") || strings.Contains(t, "reasoning") {
			if d := gjson.GetBytes(data, "delta"); d.Exists() {
				return d.String()
			}
		}
		return ""
	}
	if schema == BeyondLimitsSchemaGemini {
		var sb strings.Builder
		for _, r := range gjson.GetBytes(data, "candidates.#.content.parts.#.text").Array() {
			sb.WriteString(r.String())
		}
		return sb.String()
	}
	// Chat：delta.content / delta.reasoning_content
	if c := gjson.GetBytes(data, "choices.0.delta.content"); c.Exists() {
		return c.String()
	}
	if c := gjson.GetBytes(data, "choices.0.delta.reasoning_content"); c.Exists() {
		return c.String()
	}
	return ""
}

// extractNonStreamReply 从非流式 JSON 提取 reply 正文。
func extractNonStreamReply(body []byte) string {
	if c := gjson.GetBytes(body, "choices.0.message.content"); c.Exists() {
		return c.String()
	}
	if c := gjson.GetBytes(body, "candidates.0.content.parts.0.text"); c.Exists() {
		return c.String()
	}
	var sb strings.Builder
	for _, r := range gjson.GetBytes(body, "output.#.content.#.text").Array() {
		sb.WriteString(r.String())
	}
	return sb.String()
}

// rewriteNonStreamContent 改写非流式 JSON 的正文字段为 replacement。
func rewriteNonStreamContent(body []byte, schema BeyondLimitsResponseSchema, replacement string) []byte {
	var out []byte
	var err error
	if schema == BeyondLimitsSchemaResponses {
		out, err = sjson.SetBytes(body, "output.0.content.0.text", replacement)
		if err != nil {
			return body
		}
		return out
	}
	if schema == BeyondLimitsSchemaGemini {
		out, err = sjson.SetBytes(body, "candidates.0.content.parts.0.text", replacement)
		if err != nil {
			return body
		}
		return out
	}
	out, err = sjson.SetBytes(body, "choices.0.message.content", replacement)
	if err != nil {
		return body
	}
	return out
}
