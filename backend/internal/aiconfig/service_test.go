package aiconfig

import "testing"

func TestDefaultConfigUsesThreeModelAttempts(t *testing.T) {
	if got := defaultConfig().MaxAttempts; got != 3 {
		t.Fatalf("default max attempts=%d, want 3", got)
	}
}

func TestDefaultConfigUsesCurrentContextAndChunkSizes(t *testing.T) {
	config := defaultConfig()
	if config.MaxInputTokens != 128000 {
		t.Fatalf("default max input tokens=%d, want 128000", config.MaxInputTokens)
	}
	if config.ChunkCharacters != 32000 {
		t.Fatalf("default chunk characters=%d, want 32000", config.ChunkCharacters)
	}
}

func TestEffectiveMaxInputTokensDefaultsLegacyConfiguration(t *testing.T) {
	if got := effectiveMaxInputTokens(0); got != DefaultMaxInputTokens {
		t.Fatalf("effective max input tokens=%d, want %d", got, DefaultMaxInputTokens)
	}
}

func TestEffectiveChunkCharactersDefaultsLegacyConfiguration(t *testing.T) {
	if got := effectiveChunkCharacters(0); got != DefaultChunkCharacters {
		t.Fatalf("effective chunk characters=%d, want %d", got, DefaultChunkCharacters)
	}
}

func TestEffectiveMaxAttemptsUpgradesOnlyLegacyConfiguration(t *testing.T) {
	if got := effectiveMaxAttempts(0, 2); got != 3 {
		t.Fatalf("legacy max attempts=%d, want 3", got)
	}
	if got := effectiveMaxAttempts(DefaultMaxInputTokens, 2); got != 2 {
		t.Fatalf("explicit max attempts=%d, want 2", got)
	}
}

func TestValidateValuesChecksMaxInputTokens(t *testing.T) {
	valid := func(tokens int) error {
		return validateValues(
			ProviderDeepSeek, "https://api.deepseek.com", "deepseek-chat",
			ResponseFormatJSONObject, tokens, DefaultChunkCharacters, 180, 2,
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

func TestValidateValuesChecksChunkCharacters(t *testing.T) {
	valid := func(characters int) error {
		return validateValues(
			ProviderDeepSeek, "https://api.deepseek.com", "deepseek-chat",
			ResponseFormatJSONObject, DefaultMaxInputTokens, characters, 180, 2,
		)
	}
	if err := valid(DefaultChunkCharacters); err != nil {
		t.Fatalf("default chunk characters rejected: %v", err)
	}
	if err := valid(MinChunkCharacters - 1); err == nil {
		t.Fatal("chunk characters below minimum must be rejected")
	}
	if err := valid(MaxChunkCharacters + 1); err == nil {
		t.Fatal("chunk characters above maximum must be rejected")
	}
}
