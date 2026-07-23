package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/storage"
)

const DefaultQualityThreshold = 0.70

type Pipeline struct {
	jobs       *Service
	repository *Repository
	evidence   *evidence.Service
	parser     *Parser
	extraction *ExtractionService
	matcher    *EntityMatcher
	classifier *ClaimClassifier
	composer   *ProposalComposer
	reviews    *governance.ReviewService
	fetcher    *Fetcher
	scanner    MalwareScanner
}

type PipelineServices struct {
	Jobs       *Service
	Repository *Repository
	Evidence   *evidence.Service
	Parser     *Parser
	Extraction *ExtractionService
	Matcher    *EntityMatcher
	Classifier *ClaimClassifier
	Composer   *ProposalComposer
	Reviews    *governance.ReviewService
	Fetcher    *Fetcher
	Scanner    MalwareScanner
}

func NewPipeline(services PipelineServices) *Pipeline {
	p := &Pipeline{jobs: services.Jobs, repository: services.Repository, evidence: services.Evidence,
		parser: services.Parser, extraction: services.Extraction, matcher: services.Matcher,
		classifier: services.Classifier, composer: services.Composer, reviews: services.Reviews,
		fetcher: services.Fetcher, scanner: services.Scanner}
	if p.parser == nil {
		p.parser = NewParser(0)
	}
	if p.scanner == nil {
		p.scanner = SignatureScanner{}
	}
	return p
}

type PipelineRequest struct {
	JobID            uuid.UUID
	RunKey           string
	WikiID           uuid.UUID
	ActorID          uuid.UUID
	PageID           *uuid.UUID
	SourceID         *uuid.UUID
	Title            string
	Provider         string
	Model            string
	QualityThreshold float64
}

type UploadRequest struct {
	PipelineRequest
	Filename string
	MIMEType string
	Content  []byte
}

type PipelineResult struct {
	Job             *Job
	SourceVersionID uuid.UUID
	ProposalIDs     []uuid.UUID
	Reused          bool
	Unresolved      []EntityResolution
}

func (p *Pipeline) RunUpload(ctx context.Context, request UploadRequest) (*PipelineResult, error) {
	return p.run(ctx, request.PipelineRequest, func(ctx context.Context) (*AcquiredSource, error) {
		return ValidateUpload(ctx, DefaultURLPolicy(), p.scanner, request.Filename, request.MIMEType, request.Content)
	})
}

// RunStoredUpload acquires a previously validated upload from private object
// storage inside the fetch stage, then repeats all size/MIME/magic/malware/hash
// checks before the model can observe it.
func (p *Pipeline) RunStoredUpload(ctx context.Context, request PipelineRequest, store storage.Store,
	storageKey, filename, mimeType, expectedHash string) (*PipelineResult, error) {
	return p.run(ctx, request, func(ctx context.Context) (*AcquiredSource, error) {
		if store == nil || strings.TrimSpace(storageKey) == "" || strings.TrimSpace(expectedHash) == "" {
			return nil, ErrFetchFailed
		}
		reader, err := store.Get(ctx, storageKey)
		if err != nil {
			return nil, ErrFetchFailed
		}
		defer reader.Close()
		limit := DefaultURLPolicy().MaxBytes
		content, err := io.ReadAll(io.LimitReader(reader, limit+1))
		if err != nil {
			return nil, ErrFetchFailed
		}
		if int64(len(content)) > limit {
			return nil, ErrSourceTooLarge
		}
		acquired, err := ValidateUpload(ctx, DefaultURLPolicy(), p.scanner, filename, mimeType, content)
		if err != nil {
			return nil, err
		}
		if acquired.ContentHash != expectedHash {
			return nil, ErrFetchFailed
		}
		return acquired, nil
	})
}

func (p *Pipeline) RunURL(ctx context.Context, request PipelineRequest, rawURL string) (*PipelineResult, error) {
	if p.fetcher == nil {
		return nil, ErrFetchFailed
	}
	return p.run(ctx, request, func(ctx context.Context) (*AcquiredSource, error) {
		acquired, err := p.fetcher.Fetch(ctx, rawURL)
		if err != nil {
			return nil, err
		}
		if err := p.scanner.Scan(ctx, acquired.Content); err != nil {
			return nil, ErrMalware
		}
		return acquired, nil
	})
}

type acquireSource func(context.Context) (*AcquiredSource, error)

func (p *Pipeline) run(ctx context.Context, request PipelineRequest, acquire acquireSource) (*PipelineResult, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if acquire == nil || request.JobID == uuid.Nil || request.WikiID == uuid.Nil || request.ActorID == uuid.Nil || strings.TrimSpace(request.RunKey) == "" {
		return nil, ErrInvalidJob
	}
	run, err := p.jobs.BeginRun(ctx, request.JobID, request.RunKey)
	if err != nil {
		return nil, err
	}
	result := &PipelineResult{ProposalIDs: []uuid.UUID{}, Unresolved: []EntityResolution{}}
	var current *StageRun
	fail := func(stage *StageRun, code string, cause error) (*PipelineResult, error) {
		if stage != nil {
			_ = p.jobs.Fail(ctx, request.JobID, run.ID, stage, code)
		}
		return nil, cause
	}

	current, err = p.jobs.StartStage(ctx, run.ID, StageFetch, nil)
	if err != nil {
		return nil, err
	}
	acquired, err := acquire(ctx)
	if err != nil {
		return fail(current, acquisitionErrorCode(err), err)
	}
	inputHash := acquired.ContentHash
	asset, err := p.evidence.StoreAsset(ctx, evidence.StoreAssetParams{WikiID: request.WikiID,
		Name: acquired.Filename, Content: bytes.NewReader(acquired.Content), MimeType: acquired.MIMEType, ActorID: request.ActorID})
	if err != nil {
		return fail(current, "asset_store_failed", err)
	}
	sourceID := request.SourceID
	if sourceID == nil {
		title := strings.TrimSpace(request.Title)
		if title == "" {
			title = acquired.Filename
		}
		sourceType := evidence.SourceTypeWebpage
		if acquired.MIMEType == "application/pdf" {
			sourceType = evidence.SourceTypePDF
		}
		params := evidence.CreateSourceParams{SourceType: sourceType, AssetID: &asset.Asset.ID,
			Title: title, ActorID: request.ActorID}
		if acquired.URL != "" {
			params.URL = acquired.URL
		}
		source, err := p.evidence.CreateSource(ctx, params)
		if err != nil {
			return fail(current, "source_create_failed", err)
		}
		sourceID = &source.ID
	}
	fetchOutput := asset.Revision.ContentHash
	if err := p.jobs.CompleteStage(ctx, request.JobID, current, &fetchOutput); err != nil {
		return nil, err
	}

	current, err = p.jobs.StartStage(ctx, run.ID, StageParse, &inputHash)
	if err != nil {
		return nil, err
	}
	chunks, err := p.parser.Parse(acquired.MIMEType, acquired.Content)
	if err != nil {
		// Asset and Source were deliberately persisted before parsing so a failed
		// parser never destroys the original evidence.
		return fail(current, "parse_failed", err)
	}
	version, err := p.evidence.AddSourceVersion(ctx, evidence.AddSourceVersionParams{
		SourceID: *sourceID, VersionHash: acquired.ContentHash, RawAssetID: &asset.Revision.ID,
		FetchedAt: time.Now().UTC(), Chunks: chunks,
	})
	if err != nil {
		return fail(current, "source_version_failed", err)
	}
	result.SourceVersionID = version.Version.ID
	parseOutput := version.Version.ID.String()
	if err := p.jobs.CompleteStage(ctx, request.JobID, current, &parseOutput); err != nil {
		return nil, err
	}

	if prior, err := p.repository.FindSucceededByVersion(ctx, "source_import", version.Version.ID); err == nil {
		for _, stageName := range []string{StageExtract, StageMatch, StageCompose, StageReview} {
			stage, startErr := p.jobs.StartStage(ctx, run.ID, stageName, &parseOutput)
			if startErr != nil {
				return nil, startErr
			}
			if skipErr := p.jobs.SkipStage(ctx, request.JobID, stage, &parseOutput); skipErr != nil {
				return nil, skipErr
			}
		}
		if err := p.jobs.SucceedReused(ctx, request.JobID, run.ID, prior.ProposalID); err != nil {
			return nil, err
		}
		result.Reused = true
		if prior.ProposalID != nil {
			result.ProposalIDs = append(result.ProposalIDs, *prior.ProposalID)
		}
		result.Job, _ = p.jobs.DetailJob(ctx, request.JobID)
		return result, nil
	} else if !errors.Is(err, ErrJobNotFound) {
		return nil, err
	}

	current, err = p.jobs.StartStage(ctx, run.ID, StageExtract, &parseOutput)
	if err != nil {
		return nil, err
	}
	extracted, err := p.extraction.Extract(ctx, ExtractParams{SourceVersionID: version.Version.ID,
		Chunks: version.Chunks, Provider: request.Provider, Model: request.Model,
		ImportJobID: &request.JobID, ImportRunID: &run.ID})
	if err != nil {
		return fail(current, "extraction_failed", err)
	}
	threshold := request.QualityThreshold
	if threshold <= 0 {
		threshold = DefaultQualityThreshold
	}
	if extracted.Candidates.QualityScore < threshold || extracted.Candidates.PromptInjectionDetected {
		return fail(current, "quality_gate", ErrQualityGate)
	}
	extractOutput := extracted.Extraction.ID.String()
	if err := p.jobs.CompleteStage(ctx, request.JobID, current, &extractOutput); err != nil {
		return nil, err
	}

	current, err = p.jobs.StartStage(ctx, run.ID, StageMatch, &extractOutput)
	if err != nil {
		return nil, err
	}
	resolutions, err := p.matcher.Match(ctx, request.WikiID, request.PageID, extracted.Candidates.Entities)
	if err != nil {
		return fail(current, "entity_match_failed", err)
	}
	decisions, err := p.classifier.Classify(ctx, extracted.Candidates.Claims, resolutions)
	if err != nil {
		return fail(current, "claim_classification_failed", err)
	}
	matchJSON, _ := json.Marshal(struct {
		Resolutions []EntityResolution `json:"resolutions"`
		Decisions   []ClaimDecision    `json:"decisions"`
	}{resolutions, decisions})
	matchOutput := HashBytes(matchJSON)
	if err := p.jobs.CompleteStage(ctx, request.JobID, current, &matchOutput); err != nil {
		return nil, err
	}

	current, err = p.jobs.StartStage(ctx, run.ID, StageCompose, &matchOutput)
	if err != nil {
		return nil, err
	}
	composed, err := p.composer.Compose(ctx, ComposeParams{ImportJobID: request.JobID, WikiID: request.WikiID,
		SourceVersionID: version.Version.ID, ActorID: request.ActorID, Candidates: extracted.Candidates,
		Resolutions: resolutions, Decisions: decisions})
	if err != nil {
		return fail(current, "proposal_compose_failed", err)
	}
	result.Unresolved = composed.Unresolved
	for _, proposal := range composed.Proposals {
		result.ProposalIDs = append(result.ProposalIDs, proposal.ID)
	}
	if len(composed.Proposals) == 0 {
		return fail(current, "no_reviewable_proposal", ErrQualityGate)
	}
	composeJSON, _ := json.Marshal(result.ProposalIDs)
	composeOutput := HashBytes(composeJSON)
	if err := p.jobs.CompleteStage(ctx, request.JobID, current, &composeOutput); err != nil {
		return nil, err
	}
	current, err = p.jobs.StartStage(ctx, run.ID, StageReview, &composeOutput)
	if err != nil {
		return nil, err
	}
	for _, proposal := range composed.Proposals {
		if proposal.Status == governance.ProposalDraft {
			if _, err := p.reviews.Submit(ctx, proposal.ID); err != nil {
				return fail(current, "review_submit_failed", err)
			}
		}
	}
	reviewOutput := composeOutput
	if err := p.jobs.CompleteStage(ctx, request.JobID, current, &reviewOutput); err != nil {
		return nil, err
	}
	primaryProposalID := &result.ProposalIDs[0]
	if err := p.jobs.Succeed(ctx, request.JobID, run.ID, version.Version.ID, primaryProposalID); err != nil {
		return nil, err
	}
	result.Job, _ = p.jobs.DetailJob(ctx, request.JobID)
	return result, nil
}

func acquisitionErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrUnsafeURL):
		return "unsafe_url"
	case errors.Is(err, ErrSourceTooLarge):
		return "source_too_large"
	case errors.Is(err, ErrUnsupportedMIME):
		return "unsupported_mime"
	case errors.Is(err, ErrMalware):
		return "malware_detected"
	default:
		return "fetch_failed"
	}
}

func (p *Pipeline) validate() error {
	if p.jobs == nil || p.repository == nil || p.evidence == nil || p.parser == nil || p.extraction == nil ||
		p.matcher == nil || p.classifier == nil || p.composer == nil || p.reviews == nil {
		return fmt.Errorf("%w: pipeline dependencies", ErrInvalidJob)
	}
	return nil
}

// DetailJob is a compact read helper used by the pipeline result and API.
func (s *Service) DetailJob(ctx context.Context, jobID uuid.UUID) (*Job, error) {
	return s.repo.GetJob(ctx, nil, jobID)
}
