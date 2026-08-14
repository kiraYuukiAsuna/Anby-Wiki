// Command ai-config-import-env performs the one-time transition from the
// legacy Worker provider environment to the encrypted administrator control
// plane. It writes exclusively through aiconfig.Service and is safe to rerun.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/aiconfig"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var systemActorID = uuid.MustParse("00000000-0000-7000-8000-000000000201")

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "ai-config-import-env: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	masterKey := strings.TrimSpace(os.Getenv("AI_CONFIG_MASTER_KEY"))
	provider := strings.TrimSpace(os.Getenv("AI_PROVIDER"))
	baseURL := strings.TrimSpace(os.Getenv("AI_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("AI_API_KEY"))
	model := strings.TrimSpace(os.Getenv("AI_MODEL"))
	if databaseURL == "" || masterKey == "" || provider == "" || baseURL == "" || apiKey == "" || model == "" {
		return fmt.Errorf("缺少 DATABASE_URL、AI_CONFIG_MASTER_KEY 或 legacy AI Provider 配置")
	}
	enabled := true
	if raw := strings.TrimSpace(os.Getenv("AI_IMPORT_ENABLED")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("AI_IMPORT_ENABLED 非法")
		}
		enabled = value
	}
	responseFormat := aiconfig.ResponseFormatJSONSchema
	if provider == aiconfig.ProviderDeepSeek {
		responseFormat = aiconfig.ResponseFormatJSONObject
	}

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	wikiID, err := page.NewRepository(pool).GetWikiIDBySiteKey(ctx, nil, "default")
	if err != nil {
		return err
	}
	service, err := aiconfig.NewService(
		aiconfig.NewRepository(pool), db.NewTxManager(pool), id.NewGenerator(),
		governance.NewAuthorizationService(pool), masterKey,
	)
	if err != nil {
		return err
	}
	value, err := service.Update(ctx, aiconfig.UpdateParams{
		WikiID: wikiID, ActorID: systemActorID, Enabled: enabled,
		Provider: provider, BaseURL: baseURL, Model: model,
		ResponseFormat: responseFormat, MaxInputTokens: aiconfig.DefaultMaxInputTokens,
		ChunkCharacters:       aiconfig.DefaultChunkCharacters,
		RequestTimeoutSeconds: 180, MaxAttempts: 3, APIKey: apiKey,
	})
	if err != nil {
		return err
	}
	fmt.Printf("AI 配置迁移完成: provider=%s model=%s enabled=%t\n", value.Provider, value.Model, value.Enabled)
	return nil
}
