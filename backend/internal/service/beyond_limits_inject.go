package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

// InjectBeyondLimitsBridge 递归遍历请求 JSON，把请求自带的全部指令载体替换为 bridge，
// 保证最终只有唯一一份 bridge 作为 system 指令（单次出现，其余 system/developer 全部丢弃）。
// 覆盖：
//   - 直接字段：instructions / system / system_prompt / personality（整值替换）
//   - messages[].role=="system" 或 "developer"：全部移除，头部插入唯一一条 system=bridge
//   - input[].role=="system"：全部移除，头部插入唯一一条 system=bridge
//
// 解析失败或注入后重新序列化失败时原样返回 body，不中断请求。
func InjectBeyondLimitsBridge(body []byte, bridge string) []byte {
	if strings.TrimSpace(bridge) == "" {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return body
	}
	if !injectSystemValue(v, bridge) {
		return body
	}
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

// InjectBeyondLimitsBridgeGemini 将 bridge 注入 Gemini 原生请求的 systemInstruction。
// 覆盖 systemInstruction / system_instruction 两个字段变体；无则新建纯文本 systemInstruction。
// 解析失败或重新序列化失败时原样返回 body，不中断请求。
func InjectBeyondLimitsBridgeGemini(body []byte, bridge string) []byte {
	if strings.TrimSpace(bridge) == "" {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return body
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return body
	}
	injected := false
	for _, field := range []string{"systemInstruction", "system_instruction"} {
		if _, exists := obj[field]; exists {
			obj[field] = map[string]any{
				"role":  "system",
				"parts": []any{map[string]any{"text": bridge}},
			}
			injected = true
			break
		}
	}
	if !injected {
		obj["systemInstruction"] = map[string]any{
			"role":  "system",
			"parts": []any{map[string]any{"text": bridge}},
		}
	}
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

// InjectBeyondLimitsBridgeAnthropic 把 bridge 写入 Anthropic Messages 请求的顶层
// system 字段（字符串形态），并移除 messages 里出现的 system/developer 角色
// （Anthropic 的 messages 不接受 system 角色，放进会被上游判为非法请求）。
// 解析失败或重新序列化失败时原样返回 body，不中断请求。
func InjectBeyondLimitsBridgeAnthropic(body []byte, bridge string) []byte {
	if strings.TrimSpace(bridge) == "" {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return body
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return body
	}
	obj["system"] = bridge
	if messages, ok := obj["messages"].([]any); ok {
		filtered := make([]any, 0, len(messages))
		for _, m := range messages {
			if mo, ok := m.(map[string]any); ok {
				if role, _ := mo["role"].(string); role == "system" || role == "developer" {
					continue
				}
			}
			filtered = append(filtered, m)
		}
		obj["messages"] = filtered
	}
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

// InjectBeyondLimitsBridgeUser 与 InjectBeyondLimitsBridge 相同，但把 bridge 作为
// user 消息注入。用于忽略 system 的 OpenAI 兼容上游（如 WorkBuddy 网关）：
// 这类上游只认 user 消息里的指令，注入 system 会被直接丢弃。
// 覆盖：
//   - 直接字段：instructions / system / system_prompt / personality（直接删除）
//   - messages[].role=="system" 或 "developer"：全部移除，头部插入唯一一条 user=bridge
//   - input[].role=="system"：全部移除，头部插入唯一一条 user=bridge
//
// 解析失败或注入后重新序列化失败时原样返回 body，不中断请求。
func InjectBeyondLimitsBridgeUser(body []byte, bridge string) []byte {
	if strings.TrimSpace(bridge) == "" {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return body
	}
	if !injectUserValue(v, bridge) {
		return body
	}
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

// injectUserValue 丢弃所有 system/developer 载体，把 bridge 作为唯一一条 user 消息前置。
func injectUserValue(v any, instructions string) bool {
	obj, ok := v.(map[string]any)
	if !ok {
		return false
	}
	injected := false

	for _, field := range []string{"instructions", "system", "system_prompt", "personality"} {
		if _, exists := obj[field]; exists {
			delete(obj, field)
			injected = true
		}
	}

	if messages, ok := obj["messages"].([]any); ok {
		filtered := make([]any, 0, len(messages)+1)
		for _, m := range messages {
			mo, ok := m.(map[string]any)
			if !ok {
				filtered = append(filtered, m)
				continue
			}
			if role, _ := mo["role"].(string); role == "system" || role == "developer" {
				continue
			}
			filtered = append(filtered, m)
		}
		obj["messages"] = append([]any{
			map[string]any{"role": "user", "content": instructions},
		}, filtered...)
		injected = true
	}

	if input, ok := obj["input"].([]any); ok {
		filtered := make([]any, 0, len(input)+1)
		for _, item := range input {
			io, ok := item.(map[string]any)
			if !ok {
				filtered = append(filtered, item)
				continue
			}
			if role, _ := io["role"].(string); role == "system" {
				continue
			}
			filtered = append(filtered, item)
		}
		obj["input"] = append([]any{
			map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": instructions}},
			},
		}, filtered...)
		injected = true
	}

	return injected
}

func injectSystemValue(v any, instructions string) bool {
	obj, ok := v.(map[string]any)
	if !ok {
		return false
	}
	injected := false

	for _, field := range []string{"instructions", "system", "system_prompt", "personality"} {
		if _, exists := obj[field]; exists {
			obj[field] = instructions
			injected = true
		}
	}

	if messages, ok := obj["messages"].([]any); ok {
		filtered := make([]any, 0, len(messages)+1)
		for _, m := range messages {
			mo, ok := m.(map[string]any)
			if !ok {
				filtered = append(filtered, m)
				continue
			}
			if role, _ := mo["role"].(string); role == "system" || role == "developer" {
				// 丢弃所有 system/developer 载体，统一由下方唯一一条 bridge system 消息取代。
				continue
			}
			filtered = append(filtered, m)
		}
		obj["messages"] = append([]any{
			map[string]any{"role": "system", "content": instructions},
		}, filtered...)
		injected = true
	}

	if input, ok := obj["input"].([]any); ok {
		filtered := make([]any, 0, len(input)+1)
		for _, item := range input {
			io, ok := item.(map[string]any)
			if !ok {
				filtered = append(filtered, item)
				continue
			}
			if role, _ := io["role"].(string); role == "system" {
				// 丢弃所有 system 载体，统一由下方唯一一条 bridge system 消息取代。
				continue
			}
			filtered = append(filtered, item)
		}
		obj["input"] = append([]any{
			map[string]any{
				"role":    "system",
				"content": []any{map[string]any{"type": "input_text", "text": instructions}},
			},
		}, filtered...)
		injected = true
	}

	return injected
}

// ExtractUserMessage 从请求 JSON 提取真实用户消息，跳过环境上下文 / system / config。
// 忠实移植 extract.rs 的 extract_user。
func ExtractUserMessage(body []byte) string {
	var v any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return ""
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}

	var arr []any
	if input, ok := obj["input"].([]any); ok {
		arr = input
	} else if messages, ok := obj["messages"].([]any); ok {
		arr = messages
	} else if contents, ok := obj["contents"].([]any); ok {
		// Gemini 原生请求：contents[].role / contents[].parts[].text
		arr = contents
	}

	var texts []string
	for _, item := range arr {
		io, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := io["role"].(string); role != "user" {
			continue
		}
		switch content := io["content"].(type) {
		case string:
			if !isEnvContext(content) {
				texts = append(texts, content)
			}
		case []any:
			for _, c := range content {
				if co, ok := c.(map[string]any); ok {
					if t, _ := co["text"].(string); t != "" && !isEnvContext(t) {
						texts = append(texts, t)
					}
				}
			}
		}
		// Gemini 原生：user 文本位于 parts[].text，而非 content。
		if parts, ok := io["parts"].([]any); ok {
			for _, p := range parts {
				if po, ok := p.(map[string]any); ok {
					if t, _ := po["text"].(string); t != "" && !isEnvContext(t) {
						texts = append(texts, t)
					}
				}
			}
		}
	}
	return strings.Join(texts, " ")
}

func isEnvContext(t string) bool {
	keywords := []string{
		"<environment_context>",
		"<cwd>",
		"<shell>",
		"AGENTS.md",
		"Project Configuration",
		"provide a short title",
		"You are a helpful assistant",
	}
	for _, kw := range keywords {
		if strings.Contains(t, kw) {
			return true
		}
	}
	trimmed := strings.TrimSpace(t)
	return strings.HasPrefix(trimmed, "<") || strings.HasPrefix(trimmed, "#")
}

// BeyondLimitsCategory 分类领域（忠实移植 extract.rs 的 categorize）。
type BeyondLimitsCategory string

const (
	BeyondLimitsCategoryCrack   BeyondLimitsCategory = "crack"
	BeyondLimitsCategoryReverse BeyondLimitsCategory = "reverse"
	BeyondLimitsCategoryPentest BeyondLimitsCategory = "pentest"
	BeyondLimitsCategoryGeneral BeyondLimitsCategory = "general"
)

// CategorizeBeyondLimits 将用户消息分类到四个领域之一。
func CategorizeBeyondLimits(userMsg string) BeyondLimitsCategory {
	u := strings.ToLower(userMsg)
	crackWords := []string{"crack", "keygen", "license", "serial", "activate", "破解", "注册", "激活", "授权"}
	reverseWords := []string{"reverse", "disassemble", "decompile", "逆向", "反汇编"}
	pentestWords := []string{"pentest", "exploit", "sqli", "xss", "scan", "渗透", "漏洞", "扫描"}

	for _, w := range crackWords {
		if strings.Contains(u, w) {
			return BeyondLimitsCategoryCrack
		}
	}
	for _, w := range reverseWords {
		if strings.Contains(u, w) {
			return BeyondLimitsCategoryReverse
		}
	}
	for _, w := range pentestWords {
		if strings.Contains(u, w) {
			return BeyondLimitsCategoryPentest
		}
	}
	return BeyondLimitsCategoryGeneral
}

// beyondLimitsBridgeCtxKey 是请求级 bridge 在 context 中的键。
type beyondLimitsBridgeCtxKey struct{}

// WithBeyondLimitsBridge 把当前请求的 bridge 放入 context，供 Antigravity 转发链在
// 最终 Gemini 请求体上把 bridge 注入 user 消息并关闭安全过滤。
func WithBeyondLimitsBridge(ctx context.Context, bridge string) context.Context {
	if ctx == nil || strings.TrimSpace(bridge) == "" {
		return ctx
	}
	return context.WithValue(ctx, beyondLimitsBridgeCtxKey{}, bridge)
}

// BeyondLimitsBridgeFromContext 读取当前请求的 bridge；未设置返回空串。
func BeyondLimitsBridgeFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(beyondLimitsBridgeCtxKey{}).(string)
	return v
}

// geminiSafetyOff 关闭 Antigravity 上游的五类安全过滤。
var geminiSafetyOff = []map[string]any{
	{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "OFF"},
	{"category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "OFF"},
	{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "threshold": "OFF"},
	{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "OFF"},
	{"category": "HARM_CATEGORY_CIVIC_INTEGRITY", "threshold": "OFF"},
}

// InjectBeyondLimitsBridgeIntoGeminiUser 把 bridge 前置到 Gemini 请求最后一轮 user
// 消息里，并把 safetySettings 设为全 OFF。Gemini 对 user 通道的服从率显著高于
// systemInstruction（实测 9/12 对 2/16），user 内容也不会被身份补丁边界稀释。
// 兼容裸 Gemini 体与 v1internal 包装体（request 子对象）。
func InjectBeyondLimitsBridgeIntoGeminiUser(body []byte, bridge string) []byte {
	if strings.TrimSpace(bridge) == "" {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return body
	}
	root, ok := v.(map[string]any)
	if !ok {
		return body
	}
	target := root
	if inner, ok := root["request"].(map[string]any); ok {
		target = inner
	}

	fingerprint := bridge
	if len(fingerprint) > 96 {
		fingerprint = fingerprint[:96]
	}

	if !geminiBodyContainsText(target, fingerprint) {
		contents, _ := target["contents"].([]any)
		lastUser := -1
		for i, raw := range contents {
			cm, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			role, _ := cm["role"].(string)
			role = strings.ToLower(strings.TrimSpace(role))
			if role == "" || role == "user" {
				lastUser = i
			}
		}
		if lastUser >= 0 {
			cm := contents[lastUser].(map[string]any)
			parts, _ := cm["parts"].([]any)
			parts = append([]any{map[string]any{"text": bridge + "\n\n---\n\nUSER REQUEST:\n"}}, parts...)
			cm["parts"] = parts
		} else {
			target["contents"] = append(contents, map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": bridge}},
			})
		}
	}

	target["safetySettings"] = geminiSafetyOff
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

// geminiBodyContainsText 报告 Gemini 请求体（contents + systemInstruction）是否已含指定文本。
func geminiBodyContainsText(target map[string]any, needle string) bool {
	if needle == "" {
		return false
	}
	check := func(parts []any) bool {
		for _, raw := range parts {
			pm, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := pm["text"].(string); ok && strings.Contains(text, needle) {
				return true
			}
		}
		return false
	}
	if contents, ok := target["contents"].([]any); ok {
		for _, raw := range contents {
			cm, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			parts, _ := cm["parts"].([]any)
			if check(parts) {
				return true
			}
		}
	}
	if sys, ok := target["systemInstruction"].(map[string]any); ok {
		parts, _ := sys["parts"].([]any)
		if check(parts) {
			return true
		}
	}
	return false
}

// StripRequestSystemPrompts 丢弃请求自带的 system / developer 指令载体，只保留业务
// 消息。用于 Antigravity 分组：bridge 统一由转发层注入 user 消息，上游
// systemInstruction 只保留 Antigravity 身份补丁。
func StripRequestSystemPrompts(body []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return body
	}
	root, ok := v.(map[string]any)
	if !ok {
		return body
	}
	for _, key := range []string{"instructions", "system_prompt", "personality", "system"} {
		delete(root, key)
	}
	for _, key := range []string{"messages", "input"} {
		items, ok := root[key].([]any)
		if !ok {
			continue
		}
		filtered := make([]any, 0, len(items))
		for _, raw := range items {
			im, ok := raw.(map[string]any)
			if !ok {
				filtered = append(filtered, raw)
				continue
			}
			role, _ := im["role"].(string)
			role = strings.ToLower(strings.TrimSpace(role))
			if role == "system" || role == "developer" {
				continue
			}
			filtered = append(filtered, raw)
		}
		root[key] = filtered
	}
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}

// StripGeminiSystemInstruction 丢弃 Gemini 原生请求体自带的 systemInstruction。
func StripGeminiSystemInstruction(body []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return body
	}
	root, ok := v.(map[string]any)
	if !ok {
		return body
	}
	delete(root, "systemInstruction")
	delete(root, "system_instruction")
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}
