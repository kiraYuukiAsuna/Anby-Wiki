package aiconfig

import "testing"

func TestEffectiveMaxInputTokensDefaultsLegacyConfiguration(t *testing.T) {
	if got := effectiveMaxInputTokens(0); got != DefaultMaxInputTokens {
		t.Fatalf("effective max input tokens=%d, want %d", got, DefaultMaxInputTokens)
	}
}

func TestValidateValuesChecksMaxInputTokens(t *testing.T) {
	valid := func(tokens int) error {
		return validateValues(
			ProviderDeepSeek, "https://api.deepseek.com", "deepseek-chat",
			ResponseFormatJSONObject, tokens, 180, 2,
		)
	}
	if err := valid(DefaultMaxInputTokens); err != nil {
		t.Fatalf("default max input tokens rejected: %v", err)
	}
	if err := valid(MinMaxInputTokens - 1); err == nil {
		t.Fatal("max input tokens below minimum must be rejected")
	}
	if err := valid(MaxMaxInputTokens + 1); err == nil {
		t.Fatal("max input tokens above maximum must be rejected")
	}
}
