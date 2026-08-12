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

	"github.com/anby/wiki/backend/internal/ai"
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
	planner    *PagePlanner
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
	Planner    *PagePlanner
	Matcher    *EntityMatcher
	Classifier *ClaimClassifier
	Composer   *ProposalComposer
	Reviews    *governance.ReviewService
	Fetcher    *Fetcher
	Scanner    MalwareScanner
}

func NewPipeline(services PipelineServices) *Pipeline {
	p := &Pipeline{jobs: services.Jobs, repository: services.Repository, evidence: services.Evidence,
		parser: services.Parser, extraction: services.Extraction, planner: services.Planner, matcher: services.Matcher,
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
	Instructions     string
	RouteMode        string
	Provider         string
	Model            string
	MaxInputTokens   int
	QualityThreshold float64
	// ExpectedContentHash binds an upload retry to the immutable content that
	// was accepted by the API. URL jobs leave it empty because their config is
	// already immutable within a job.
	ExpectedContentHash string
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
	request.PipelineRequest.ExpectedContentHash = HashBytes(request.Content)
	return p.run(ctx, request.PipelineRequest, func(ctx context.Context) (*AcquiredSource, error) {
		return ValidateUpload(ctx, DefaultURLPolicy(), p.scanner, request.Filename, request.MIMEType, request.Content)
	})
}

// RunStoredUpload acquires a previously validated upload from private object
// storage inside the fetch stage, then repeats all size/MIME/magic/malware/hash
// checks before the model can observe it.
func (p *Pipeline) RunStoredUpload(ctx context.Context, request PipelineRequest, store storage.Store,
	storageKey, filename, mimeType, expectedHash string) (*PipelineResult, error) {
	request.ExpectedContentHash = expectedHash
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

	version, sourceLabel, err := p.prepareParsedSource(ctx, request, run.ID, acquire, fail)
	if err != nil {
		return nil, err
	}
	result.SourceVersionID = version.Version.ID
	parseOutput := version.Version.ID.String()

	current, err = p.jobs.StartStage(ctx, run.ID, StageExtract, &parseOutput)
	if err != nil {
		return nil, err
	}
	extracted, err := p.extraction.Extract(ctx, ExtractParams{SourceVersionID: version.Version.ID,
		SourceLabel: sourceLabel, Chunks: version.Chunks, Provider: request.Provider, Model: request.Model,
		MaxInputTokens: request.MaxInputTokens,
		ImportJobID:    &request.JobID, ImportRunID: &run.ID})
	if err != nil {
		return fail(current, extractionErrorCode(err), err)
	}
	threshold := request.QualityThreshold
	if threshold <= 0 {
		threshold = DefaultQualityThreshold
	}
	if !passesExtractionQualityGate(extracted.Candidates, threshold) {
		return fail(current, "quality_gate", ErrQualityGate)
	}
	extractOutput := extracted.Extraction.ID.String()
	if err := p.jobs.CompleteStage(ctx, request.JobID, current, &extractOutput); err != nil {
		return nil, err
	}

	current, err = p.jobs.StartStage(ctx, run.ID, StagePlan, &extractOutput)
	if err != nil {
		return nil, err
	}
	planInput := HashBytes([]byte(strings.Join([]string{
		extractOutput,
		strings.TrimSpace(request.Title),
		strings.TrimSpace(request.Instructions),
		strings.TrimSpace(request.RouteMode),
		optionalUUIDString(request.PageID),
	}, "\x00")))
	planned, err := p.planner.Plan(ctx, PlanParams{
		SourceVersionID: version.Version.ID, SourceLabel: sourceLabel,
		PreferredTitle: request.Title, Instructions: request.Instructions,
		RouteMode: request.RouteMode, TargetPageID: request.PageID,
		WikiID: request.WikiID, Chunks: version.Chunks, Candidates: extracted.Candidates,
		Provider: request.Provider, Model: request.Model, MaxInputTokens: request.MaxInputTokens,
		InputHash: planInput, ImportJobID: &request.JobID, ImportRunID: &run.ID,
	})
	if err != nil {
		return fail(current, planErrorCode(err), err)
	}
	if planned.Plan.QualityScore < threshold/2 {
		return fail(current, "page_plan_quality_gate", ErrQualityGate)
	}
	planOutput := planned.Record.ID.String()
	if err := p.jobs.CompleteStage(ctx, request.JobID, current, &planOutput); err != nil {
		return nil, err
	}
	selectedCandidates := selectCandidatesForPlan(extracted.Candidates, planned.Plan)

	current, err = p.jobs.StartStage(ctx, run.ID, StageMatch, &planOutput)
	if err != nil {
		return nil, err
	}
	resolutions, err := p.matcher.Match(ctx, request.WikiID, request.PageID, selectedCandidates.Entities)
	if err != nil {
		return fail(current, "entity_match_failed", err)
	}
	decisions, err := p.classifier.Classify(ctx, selectedCandidates.Claims, resolutions)
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
		SourceVersionID: version.Version.ID, ActorID: request.ActorID, Candidates: selectedCandidates,
		Plan: planned.Plan, Resolutions: resolutions, Decisions: decisions})
	if err != nil {
		return fail(current, "proposal_compose_failed", err)
	}
	result.Unresolved = composed.Unresolved
	for _, proposal := range composed.Proposals {
		result.ProposalIDs = append(result.ProposalIDs, proposal.ID)
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
	if len(composed.Proposals) == 0 {
		// Ignore-only plans and fully deduplicated candidates are valid no-op
		// imports. Preserve the reviewed ImportPlan without fabricating an empty
		// Proposal or presenting an evidence-grounded no-op as a failure.
		if err := p.jobs.SkipStage(ctx, request.JobID, current, &composeOutput); err != nil {
			return nil, err
		}
		if err := p.jobs.Succeed(ctx, request.JobID, run.ID, version.Version.ID, nil); err != nil {
			return nil, err
		}
		result.Job, _ = p.jobs.DetailJob(ctx, request.JobID)
		return result, nil
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

func passesExtractionQualityGate(candidates *Candidates, threshold float64) bool {
	if candidates == nil || candidates.PromptInjectionDetected {
		return false
	}
	// Fact extraction is an enrichment layer. A source with no supported
	// Entity/Claim vocabulary may still produce valuable encyclopedia pages in
	// ImportPlan, whose own evidence and quality gate runs next.
	if len(candidates.Entities) == 0 && len(candidates.Claims) == 0 {
		return true
	}
	if candidates.QualityScore >= threshold {
		return true
	}
	// Evidence validation has already removed every unverifiable candidate.
	// A large source can therefore have a lower aggregate score solely because
	// bad siblings were discarded. Permit that salvage path only when the
	// retained set still has at least half the requested aggregate quality and
	// its average candidate confidence independently meets the full threshold.
	count, confidence := 0, 0.0
	for index := range candidates.Entities {
		if len(candidates.Entities[index].Evidence) == 0 {
			return false
		}
		count++
		confidence += candidates.Entities[index].Confidence
	}
	for index := range candidates.Claims {
		if len(candidates.Claims[index].Evidence) == 0 {
			return false
		}
		count++
		confidence += candidates.Claims[index].Confidence
	}
	return count > 0 && candidates.QualityScore >= threshold/2 && confidence/float64(count) >= threshold
}

func (p *Pipeline) prepareParsedSource(
	ctx context.Context,
	request PipelineRequest,
	runID uuid.UUID,
	acquire acquireSource,
	fail func(*StageRun, string, error) (*PipelineResult, error),
) (*evidence.AddSourceVersionResult, string, error) {
	checkpoint, err := p.loadParseCheckpoint(ctx, request)
	if err != nil {
		return nil, "", err
	}
	if checkpoint != nil {
		fetchOutput := checkpoint.Version.VersionHash
		fetchStage, err := p.jobs.StartStage(ctx, runID, StageFetch, nil)
		if err != nil {
			return nil, "", err
		}
		if err := p.jobs.SkipStage(ctx, request.JobID, fetchStage, &fetchOutput); err != nil {
			return nil, "", err
		}
		parseStage, err := p.jobs.StartStage(ctx, runID, StageParse, &fetchOutput)
		if err != nil {
			return nil, "", err
		}
		parseOutput := checkpoint.Version.ID.String()
		if err := p.jobs.SkipStage(ctx, request.JobID, parseStage, &parseOutput); err != nil {
			return nil, "", err
		}
		return &evidence.AddSourceVersionResult{
			Version: checkpoint.Version,
			Chunks:  checkpoint.Chunks,
			Reused:  true,
		}, checkpoint.Source.Title, nil
	}

	fetchStage, err := p.jobs.StartStage(ctx, runID, StageFetch, nil)
	if err != nil {
		return nil, "", err
	}
	acquired, err := acquire(ctx)
	if err != nil {
		_, failed := fail(fetchStage, acquisitionErrorCode(err), err)
		return nil, "", failed
	}
	inputHash := acquired.ContentHash
	asset, err := p.evidence.StoreAsset(ctx, evidence.StoreAssetParams{WikiID: request.WikiID,
		Name: acquired.Filename, Content: bytes.NewReader(acquired.Content), MimeType: acquired.MIMEType, ActorID: request.ActorID})
	if err != nil {
		_, failed := fail(fetchStage, "asset_store_failed", err)
		return nil, "", failed
	}
	sourceID := request.SourceID
	sourceLabel := strings.TrimSpace(request.Title)
	if sourceLabel == "" {
		sourceLabel = acquired.Filename
	}
	if sourceID == nil {
		params := evidence.CreateSourceParams{SourceType: inferredSourceType(acquired), AssetID: &asset.Asset.ID,
			Title: sourceLabel, ActorID: request.ActorID}
		if acquired.URL != "" {
			params.URL = acquired.URL
		}
		source, err := p.evidence.CreateSource(ctx, params)
		if err != nil {
			_, failed := fail(fetchStage, "source_create_failed", err)
			return nil, "", failed
		}
		sourceID = &source.ID
	}
	fetchOutput := asset.Revision.ContentHash
	if err := p.jobs.CompleteStage(ctx, request.JobID, fetchStage, &fetchOutput); err != nil {
		return nil, "", err
	}

	parseStage, err := p.jobs.StartStage(ctx, runID, StageParse, &inputHash)
	if err != nil {
		return nil, "", err
	}
	chunks, err := p.parser.Parse(ctx, acquired.MIMEType, acquired.Content)
	if err != nil {
		// Asset and Source were deliberately persisted before parsing so a failed
		// parser never destroys the original evidence.
		_, failed := fail(parseStage, parseErrorCode(err), err)
		return nil, "", failed
	}
	version, err := p.evidence.AddSourceVersion(ctx, evidence.AddSourceVersionParams{
		SourceID: *sourceID, VersionHash: acquired.ContentHash, RawAssetID: &asset.Revision.ID,
		FetchedAt: time.Now().UTC(), Chunks: chunks,
	})
	if err != nil {
		_, failed := fail(parseStage, "source_version_failed", err)
		return nil, "", failed
	}
	parseOutput := version.Version.ID.String()
	if err := p.jobs.CompleteStage(ctx, request.JobID, parseStage, &parseOutput); err != nil {
		return nil, "", err
	}
	return version, sourceLabel, nil
}

func (p *Pipeline) loadParseCheckpoint(ctx context.Context, request PipelineRequest) (*evidence.SourceVersionContent, error) {
	resume, err := p.repository.FindLatestParseCheckpoint(ctx, request.JobID)
	if err != nil || resume == nil {
		return nil, err
	}
	checkpoint, err := p.evidence.LoadSourceVersionContent(ctx, resume.SourceVersionID)
	if errors.Is(err, evidence.ErrSourceVersionNotFound) || errors.Is(err, evidence.ErrSourceNotFound) {
		// A missing immutable artifact cannot be resumed safely; reacquire it.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if checkpoint.Version.VersionHash != resume.ContentHash {
		return nil, nil
	}
	if expected := strings.TrimSpace(request.ExpectedContentHash); expected != "" && resume.ContentHash != expected {
		return nil, nil
	}
	if request.SourceID != nil && checkpoint.Version.SourceID != *request.SourceID {
		return nil, nil
	}
	return checkpoint, nil
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

func parseErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPDFExtractorUnavailable):
		return "pdf_extractor_unavailable"
	case errors.Is(err, ErrPDFTextTooLarge):
		return "pdf_text_too_large"
	case errors.Is(err, ErrPDFRasterizerUnavailable):
		return "pdf_rasterizer_unavailable"
	case errors.Is(err, ErrPDFPageLimitExceeded):
		return "pdf_ocr_page_limit_exceeded"
	case errors.Is(err, ErrOCRUnavailable):
		return "ocr_unavailable"
	case errors.Is(err, ErrOCRImageTooLarge):
		return "ocr_image_too_large"
	case errors.Is(err, ErrOCRTextTooLarge):
		return "ocr_text_too_large"
	case errors.Is(err, ErrOCRNoText):
		return "ocr_no_text"
	case errors.Is(err, ErrOCRFailed):
		return "ocr_failed"
	default:
		return "parse_failed"
	}
}

func inferredSourceType(source *AcquiredSource) string {
	switch source.MIMEType {
	case "application/pdf":
		return evidence.SourceTypePDF
	case "image/png", "image/jpeg":
		return evidence.SourceTypeImage
	case "application/json":
		if source.URL != "" {
			return evidence.SourceTypeAPI
		}
		return evidence.SourceTypeDatabase
	case "text/csv":
		return evidence.SourceTypeDatabase
	case "text/plain":
		if source.URL == "" {
			return evidence.SourceTypeBook
		}
	}
	return evidence.SourceTypeWebpage
}

func extractionErrorCode(err error) string {
	var providerErr *ai.ProviderError
	switch {
	case errors.As(err, &providerErr) && providerErr.Code == "output_truncated":
		return "extraction_output_truncated"
	case errors.Is(err, ai.ErrInvalidOutput):
		return "extraction_invalid_output"
	case errors.Is(err, ai.ErrProvider):
		return "extraction_provider_failed"
	case errors.Is(err, ai.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "extraction_timeout"
	case errors.Is(err, ErrEvidenceRequired):
		return "extraction_evidence_invalid"
	default:
		return "extraction_failed"
	}
}

func (p *Pipeline) validate() error {
	if p.jobs == nil || p.repository == nil || p.evidence == nil || p.parser == nil || p.extraction == nil || p.planner == nil ||
		p.matcher == nil || p.classifier == nil || p.composer == nil || p.reviews == nil {
		return fmt.Errorf("%w: pipeline dependencies", ErrInvalidJob)
	}
	return nil
}

func optionalUUIDString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func planErrorCode(err error) string {
	var providerErr *ai.ProviderError
	switch {
	case errors.As(err, &providerErr) && providerErr.Code == "output_truncated":
		return "page_plan_output_truncated"
	case errors.Is(err, ai.ErrInvalidOutput):
		return "page_plan_invalid_output"
	case errors.Is(err, ai.ErrProvider):
		return "page_plan_provider_failed"
	case errors.Is(err, ai.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "page_plan_timeout"
	case errors.Is(err, ErrEvidenceRequired):
		return "page_plan_evidence_invalid"
	case errors.Is(err, ErrPlanTargetConflict):
		return "page_plan_target_conflict"
	case errors.Is(err, ErrNoPagePlan):
		return "no_page_plan"
	default:
		return "page_plan_failed"
	}
}

// DetailJob is a compact read helper used by the pipeline result and API.
func (s *Service) DetailJob(ctx context.Context, jobID uuid.UUID) (*Job, error) {
	return s.repo.GetJob(ctx, nil, jobID)
}
