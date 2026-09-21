package main

import (
	"testing"

	"github.com/anby/wiki/backend/internal/ai"
)

func TestClassifyAIConfigProviderError(t *testing.T) {
	tests := []struct {
		code        string
		wantReason  string
		wantMessage string
	}{
		{"authentication_failed", "authentication_failed", "模型供应商拒绝了 API Key"},
		{"rate_limited", "rate_limited", "模型供应商正在限流，请稍后重试"},
		{"invalid_request", "invalid_request", "模型地址、模型 ID 或请求格式不被供应商接受"},
		{"provider_unavailable", "provider_unavailable", "模型供应商暂时不可用"},
		{"invalid_structured_output", "structured_output_invalid", "模型未返回兼容的结构化输出"},
		{"unknown", "provider_error", "模型配置测试失败"},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			reason, message := classifyAIConfigProviderError(&ai.ProviderError{Code: test.code})
			if reason != test.wantReason || message != test.wantMessage {
				t.Fatalf("got (%q,%q), want (%q,%q)",
					reason, message, test.wantReason, test.wantMessage)
			}
		})
	}
}
