package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/anby/wiki/backend/internal/platform/storage"
)

type SourceImportConfig struct {
	Source struct {
		Kind        string `json:"kind"`
		URL         string `json:"url,omitempty"`
		StorageKey  string `json:"storage_key,omitempty"`
		Filename    string `json:"filename,omitempty"`
		MIMEType    string `json:"mime_type,omitempty"`
		ContentHash string `json:"content_hash,omitempty"`
	} `json:"source"`
	Title            string     `json:"title,omitempty"`
	PageID           *uuid.UUID `json:"page_id,omitempty"`
	SourceID         *uuid.UUID `json:"source_id,omitempty"`
	QualityThreshold float64    `json:"quality_threshold,omitempty"`
}

type RunnerConfig struct {
	WikiID       uuid.UUID
	Provider     string
	Model        string
	PollInterval time.Duration
	JobTimeout   time.Duration
	Logger       *slog.Logger
	UploadStore  storage.Store
}

type Runner struct {
	jobs     *Service
	pipeline *Pipeline
	config   RunnerConfig
}

func NewRunner(jobs *Service, pipeline *Pipeline, config RunnerConfig) *Runner {
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.JobTimeout <= 0 {
		config.JobTimeout = 2 * time.Minute
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	return &Runner{jobs: jobs, pipeline: pipeline, config: config}
}

// Run consumes queued jobs until cancellation. A claimed job gets a bounded,
// cancellation-independent context so graceful shutdown can finish its state
// transition instead of leaving an unrecoverable running row.
func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.config.JobTimeout)
		processed, err := r.ProcessOne(jobCtx)
		cancel()
		if err != nil {
			r.config.Logger.Warn("导入任务处理失败", slog.Any("error", err))
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Runner) ProcessOne(ctx context.Context) (bool, error) {
	ctx, span := otel.Tracer("github.com/anby/wiki/backend/importer").Start(ctx, "import.process")
	defer span.End()
	job, run, err := r.jobs.ClaimNext(ctx)
	if errors.Is(err, ErrNoQueuedJob) {
		span.SetAttributes(attribute.Bool("import.job_claimed", false))
		return false, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "claim_failed")
		return false, err
	}
	span.SetAttributes(attribute.Bool("import.job_claimed", true))
	config, err := decodeSourceImportConfig(job.Config)
	if err != nil {
		span.SetStatus(codes.Error, "invalid_config")
		return true, r.failInvalidConfig(ctx, job, run)
	}
	span.SetAttributes(attribute.String("import.source_kind", config.Source.Kind))
	request := PipelineRequest{
		JobID: job.ID, RunKey: run.IdempotencyKey, WikiID: r.config.WikiID,
		ActorID: job.InitiatedBy, PageID: config.PageID, SourceID: config.SourceID,
		Title: config.Title, Provider: r.config.Provider, Model: r.config.Model,
		QualityThreshold: config.QualityThreshold,
	}
	switch config.Source.Kind {
	case "url":
		_, err = r.pipeline.RunURL(ctx, request, config.Source.URL)
	case "upload":
		_, err = r.pipeline.RunStoredUpload(ctx, request, r.config.UploadStore, config.Source.StorageKey,
			config.Source.Filename, config.Source.MIMEType, config.Source.ContentHash)
	default:
		err = ErrInvalidJob
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "pipeline_failed")
		return true, fmt.Errorf("job %s: %w", job.ID, err)
	}
	return true, nil
}

func decodeSourceImportConfig(raw json.RawMessage) (*SourceImportConfig, error) {
	var config SourceImportConfig
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, ErrInvalidJob
	}
	validSource := config.Source.Kind == "url" && strings.TrimSpace(config.Source.URL) != ""
	if config.Source.Kind == "upload" {
		validSource = strings.TrimSpace(config.Source.StorageKey) != "" && strings.TrimSpace(config.Source.Filename) != "" &&
			strings.TrimSpace(config.Source.MIMEType) != "" && validSHA256(config.Source.ContentHash)
	}
	if !validSource || config.QualityThreshold < 0 || config.QualityThreshold > 1 {
		return nil, ErrInvalidJob
	}
	return &config, nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func (r *Runner) failInvalidConfig(ctx context.Context, job *Job, run *Run) error {
	stage, err := r.jobs.StartStage(ctx, run.ID, StageFetch, nil)
	if err != nil {
		return err
	}
	return r.jobs.Fail(ctx, job.ID, run.ID, stage, "invalid_config")
}
