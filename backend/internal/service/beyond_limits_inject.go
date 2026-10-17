package service

import (
	"bytes"
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
