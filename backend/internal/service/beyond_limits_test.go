package service

import "testing"

func TestDetectBeyondLimitsPlatform(t *testing.T) {
	cases := map[string]BeyondLimitsPlatform{
		"deepseek-v4-flash":        BeyondLimitsPlatformDeepSeek,
		"deepseek-chat":            BeyondLimitsPlatformDeepSeek,
		"cn:deepseek-v4.1-flash":   BeyondLimitsPlatformDeepSeek,
		"cn:deepseek-v4.1":         BeyondLimitsPlatformDeepSeek,
		"openai:deepseek-v4-flash": BeyondLimitsPlatformDeepSeek,
		"deepseek/deepseek-chat":   BeyondLimitsPlatformDeepSeek,
		"vendor/ds-deepseek-r1":    BeyondLimitsPlatformDeepSeek,
		"gemini-3-flash":           BeyondLimitsPlatformGemini,
		"google/gemini-3-pro":      BeyondLimitsPlatformGemini,
		"cn:gemini-3.8-flash-high": BeyondLimitsPlatformGemini,
		"gpt-5.6":                  BeyondLimitsPlatformOpenAI,
		"claude-sonnet-4-6":        BeyondLimitsPlatformOpenAI,
		"  DEEPSEEK-V4-FLASH  ":    BeyondLimitsPlatformDeepSeek,
		"":                         BeyondLimitsPlatformOpenAI,
	}
	for model, want := range cases {
		if got := DetectBeyondLimitsPlatform(model); got != want {
			t.Fatalf("DetectBeyondLimitsPlatform(%q) = %q, want %q", model, got, want)
		}
	}
}
