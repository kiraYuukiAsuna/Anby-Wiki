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
		p.parser = NewParser(0, nil)
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
	PlanningInput    json.RawMessage
	SourceOnly       bool
	AutoApprovePlan  bool
	Provider         string
	Model            string
	MaxInputTokens   int
	ChunkCharacters  int
	QualityThreshold float64
	ConfirmedPlanID  *uuid.UUID
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
		limit := DefaultURLPolicy().MaxBytes
		meta, err := store.Head(ctx, storageKey)
		if err != nil {
			return nil, ErrFetchFailed
		}
		if meta.Size < 0 || meta.Size > limit {
			return nil, ErrSourceTooLarge
		}
		reader, err := store.Get(ctx, storageKey)
		if err != nil {
			return nil, ErrFetchFailed
		}
		defer reader.Close()
		content := make([]byte, int(meta.Size))
		if _, err := io.ReadFull(reader, content); err != nil {
			return nil, ErrFetchFailed
		}
		var extra [1]byte
		if read, err := io.ReadFull(reader, extra[:]); read != 0 || !errors.Is(err, io.EOF) {
			return nil, ErrFetchFailed
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
		if err := p.scanner.Scan(ctx, bytes.NewReader(acquired.Content)); err != nil {
			return nil, ErrMalware
		}
		return acquired, nil
	})
}

type acquireSource func(context.Context) (*AcquiredSource, error)

func (p *Pipeline) run(ctx context.Context, request PipelineRequest, acquire acquireSource) (pipelineResult *PipelineResult, pipelineErr error) {
	if err := p.validate(request.SourceOnly); err != nil {
		return nil, err
	}
	if acquire == nil || request.JobID == uuid.Nil || request.WikiID == uuid.Nil ||
		request.ActorID == uuid.Nil || strings.TrimSpace(request.RunKey) == "" {
		return nil, ErrInvalidJob
	}
	run, err := p.jobs.BeginRun(ctx, request.JobID, request.RunKey)
	if err != nil {
		return nil, err
	}
	defer func() {
		if pipelineErr == nil {
			return
		}
		code := ErrorImportFailed
		if errors.Is(pipelineErr, context.DeadlineExceeded) || errors.Is(pipelineErr, ai.ErrTimeout) {
			code = ErrorImportTimeout
		}
		failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := p.jobs.Fail(failCtx, request.JobID, run.ID, newFailure("", code, pipelineErr)); err != nil {
			pipelineErr = fmt.Errorf("%w: failed to record import failure", pipelineErr)
		}
	}()
	job, err := p.jobs.DetailJob(ctx, request.JobID)
	if err != nil {
		return nil, err
	}
	if job.WikiID != request.WikiID || job.InitiatedBy != request.ActorID {
		return nil, ErrInvalidJob
	}
	result := &PipelineResult{ProposalIDs: []uuid.UUID{}, Unresolved: []EntityResolution{}}
	var current *StageRun
	var extracted *ExtractResult
	fail := func(stage *StageRun, code string, cause error) (*PipelineResult, error) {
		if stage != nil {
			failure := newFailure(stage.Stage, code, cause)
			if code == ErrorQualityGate {
				failure = extractionFailure(extracted, max(DefaultQualityThreshold, request.QualityThreshold))
			}
			if errors.Is(cause, ErrModelInputTooLarge) {
				if failure.Details == nil {
					failure.Details = &FailureDetails{}
				}
				failure.Details.MaxInputTokens = request.MaxInputTokens
				if failure.Details.MaxInputTokens <= 0 {
					failure.Details.MaxInputTokens = DefaultModelMaxInputTokens
				}
			}
			failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if err := p.jobs.Fail(failCtx, request.JobID, run.ID, failure); err != nil {
				return nil, fmt.Errorf("%w: failed to record import failure: %v", cause, err)
			}
		}
		return nil, cause
	}

	version, sourceLabel, err := p.prepareParsedSource(ctx, request, run.ID, acquire, fail)
	if err != nil {
		return nil, err
	}
	result.SourceVersionID = version.Version.ID
	parseOutput := version.Version.ID.String()
	if request.SourceOnly {
		if err := p.jobs.Succeed(ctx, request.JobID, run.ID, version.Version.ID, nil); err != nil {
			return nil, err
		}
		result.Job, _ = p.jobs.DetailJob(ctx, request.JobID)
		return result, nil
	}

	current, err = p.jobs.StartStage(ctx, run.ID, StageExtract, &parseOutput)
	if err != nil {
		return nil, err
	}
	extracted, err = p.extraction.Extract(ctx, ExtractParams{SourceVersionID: version.Version.ID,
		SourceLabel: sourceLabel, Chunks: version.Chunks, Provider: request.Provider, Model: request.Model,
		MaxInputTokens: request.MaxInputTokens,
		ImportJobID:    &request.JobID, ImportRunID: &run.ID})
	if err != nil {
		return fail(current, extractionErrorCode(err), err)
	}
	threshold := request.QualityThreshold
	if threshold < DefaultQualityThreshold {
		threshold = DefaultQualityThreshold
	}
	planningCandidates, err := trustedPlanningCandidates(extracted.Candidates, threshold)
	if err != nil {
		return fail(current, ErrorQualityGate, ErrQualityGate)
	}
	extracted.Candidates = planningCandidates
	extractOutput := extracted.Extraction.ID.String()
	if err := p.jobs.CompleteStage(ctx, request.JobID, current, &extractOutput); err != nil {
		return nil, err
	}

	current, err = p.jobs.StartStage(ctx, run.ID, StagePlan, &extractOutput)
	if err != nil {
		return nil, err
	}
	planInput := HashBytes([]byte(extractOutput + "\x00" + string(request.PlanningInput)))
	planned, err := p.planner.Plan(ctx, PlanParams{
		SourceVersionID: version.Version.ID, SourceLabel: sourceLabel,
		PreferredTitle: request.Title, Instructions: request.Instructions,
		RouteMode: request.RouteMode, TargetPageID: request.PageID,
		WikiID: request.WikiID, Chunks: version.Chunks, Candidates: extracted.Candidates,
		Provider: request.Provider, Model: request.Model, MaxInputTokens: request.MaxInputTokens,
		QualityThreshold: threshold,
		PlanningInput:    request.PlanningInput, ActorID: request.ActorID,
		InputHash: planInput, ImportJobID: &request.JobID, ImportRunID: &run.ID,
	})
	if err != nil {
		return fail(current, planErrorCode(err), err)
	}
	planOutput := planned.Record.ID.String()
	autoConfirm := request.AutoApprovePlan &&
		planned.Plan.Quality != nil && planned.Plan.Quality.Passed
	if err := p.jobs.CompletePlanStage(
		ctx, request.JobID, current, planned.Record.ID, autoConfirm,
	); err != nil {
		return nil, err
	}
	if planned.Plan.Quality == nil || !planned.Plan.Quality.Passed {
		if err := p.jobs.RequireAction(
			ctx, request.JobID, run.ID, version.Version.ID, ActionQualityGate,
		); err != nil {
			return nil, err
		}
		result.Job, _ = p.jobs.DetailJob(ctx, request.JobID)
		return result, nil
	}
	planConfirmed := request.ConfirmedPlanID != nil &&
		*request.ConfirmedPlanID == planned.Record.ID
	if !request.AutoApprovePlan && !planConfirmed {
		if err := p.jobs.RequireAction(
			ctx, request.JobID, run.ID, version.Version.ID, ActionConfirmPlan,
		); err != nil {
			return nil, err
		}
		result.Job, _ = p.jobs.DetailJob(ctx, request.JobID)
		return result, nil
	}
	selectedCandidates := selectCandidatesForPlan(extracted.Candidates, planned.Plan)

	current, err = p.jobs.StartStage(ctx, run.ID, StageMatch, &planOutput)
	if err != nil {
		return nil, err
	}
	resolutions, err := p.matcher.Match(ctx, request.WikiID, request.PageID, selectedCandidates.Entities)
	if err != nil {
		return fail(current, ErrorEntityMatchFailed, err)
	}
	matchReview := buildMatchReview(selectedCandidates, resolutions)
	if len(matchReview.Candidates) > 0 {
		matchJSON, _ := json.Marshal(matchReview)
		matchOutput := HashBytes(matchJSON)
		if err := p.jobs.CompleteStage(ctx, request.JobID, current, &matchOutput); err != nil {
			return nil, err
		}
		if err := p.jobs.RequireActionWithDetails(
			ctx, request.JobID, run.ID, version.Version.ID, ActionResolveEntities,
			matchReviewErrorJSON(matchReview),
		); err != nil {
			return nil, err
		}
		result.Unresolved = matchReview.Resolutions
		result.Job, _ = p.jobs.DetailJob(ctx, request.JobID)
		return result, nil
	}
	decisions, err := p.classifier.Classify(ctx, selectedCandidates.Claims, resolutions)
	if err != nil {
		return fail(current, ErrorClaimClassifyFailed, err)
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
		AuthorActorID: job.AuthorActorID,
		Plan:          planned.Plan, Resolutions: resolutions, Decisions: decisions})
	if err != nil {
		return fail(current, ErrorProposalComposeFailed, err)
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
				return fail(current, ErrorReviewSubmitFailed, err)
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

func trustedPlanningCandidates(candidates *Candidates, threshold float64) (*Candidates, error) {
	if candidates == nil || candidates.PromptInjectionDetected {
		return nil, ErrQualityGate
	}
	if passesExtractionQualityGate(candidates, threshold) {
		return candidates, nil
	}
	return &Candidates{
		SchemaVersion:   candidates.SchemaVersion,
		SourceVersionID: candidates.SourceVersionID,
		Entities:        []EntityCandidate{},
		Claims:          []ClaimCandidate{},
		QualityScore:    candidates.QualityScore,
	}, nil
}

func buildMatchReview(candidates *Candidates, resolutions []EntityResolution) *MatchReview {
	review := &MatchReview{
		Resolutions: append([]EntityResolution(nil), resolutions...),
		Candidates:  []EntityMatchReview{}, SkippedClaims: []SkippedClaimReview{},
	}
	if candidates == nil {
		return review
	}
	entities := make(map[uuid.UUID]EntityCandidate, len(candidates.Entities))
	for _, candidate := range candidates.Entities {
		entities[candidate.CandidateID] = candidate
	}
	ambiguous := map[uuid.UUID]bool{}
	for _, resolution := range resolutions {
		if resolution.Outcome != EntityAmbiguous {
			continue
		}
		ambiguous[resolution.CandidateID] = true
		candidate := entities[resolution.CandidateID]
		review.Candidates = append(review.Candidates, EntityMatchReview{
			CandidateID: resolution.CandidateID,
			TypeKey:     candidate.TypeKey,
			Label:       candidate.Label,
			Aliases:     append([]string(nil), candidate.Aliases...),
			Outcome:     resolution.Outcome,
			EntityID:    resolution.EntityID,
			Score:       resolution.Score,
			Reason:      resolution.Reason,
			Alternatives: append(
				[]EntityAlternative(nil),
				resolution.Alternatives...,
			),
		})
	}
	if len(ambiguous) == 0 {
		return review
	}
	for _, claim := range candidates.Claims {
		switch {
		case claim.Subject.CandidateID != nil && ambiguous[*claim.Subject.CandidateID]:
			review.SkippedClaims = append(review.SkippedClaims, SkippedClaimReview{
				CandidateID: claim.CandidateID,
				Reason:      "subject entity requires human resolution",
			})
		default:
			valueID, referencesCandidate, valid := claimValueCandidateReference(claim.Value)
			if valid && referencesCandidate && ambiguous[valueID] {
				review.SkippedClaims = append(review.SkippedClaims, SkippedClaimReview{
					CandidateID: claim.CandidateID,
					Reason:      "value entity requires human resolution",
				})
			}
		}
	}
	return review
}

func matchReviewErrorJSON(review *MatchReview) []byte {
	value, _ := json.Marshal(map[string]any{
		"stage":        StageMatch,
		"code":         "entity_resolution_required",
		"match_review": review,
	})
	return value
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
		if err := p.jobs.CompleteParseStage(
			ctx, request.JobID, parseStage, checkpoint.Version.ID, true,
		); err != nil {
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
	if request.SourceOnly {
		switch acquired.MIMEType {
		case "text/html", "text/plain", MIMEMarkdown, "application/json", "text/csv":
		default:
			_, failed := fail(fetchStage, ErrorSourceOnlyFormat, ErrSourceOnlyFormat)
			return nil, "", failed
		}
	}
	inputHash := acquired.ContentHash
	asset, err := p.evidence.StoreAsset(ctx, evidence.StoreAssetParams{WikiID: request.WikiID,
		Name: acquired.Filename, Content: bytes.NewReader(acquired.Content), MimeType: acquired.MIMEType, ActorID: request.ActorID})
	if err != nil {
		_, failed := fail(fetchStage, ErrorAssetStoreFailed, err)
		return nil, "", failed
	}
	sourceID := request.SourceID
	bibliography := inferSourceMetadata(acquired)
	sourceLabel := strings.TrimSpace(request.Title)
	if sourceLabel == "" {
		sourceLabel = strings.TrimSpace(bibliography.Title)
	}
	if sourceLabel == "" {
		sourceLabel = acquired.Filename
	}
	if sourceID == nil {
		params := evidence.CreateSourceParams{
			WikiID:     request.WikiID,
			SourceType: inferredSourceType(acquired), AssetID: &asset.Asset.ID,
			Title: sourceLabel, Author: bibliography.Author,
			Publisher: bibliography.Publisher, PublishedAt: bibliography.PublishedAt,
			Metadata: bibliography.Metadata, ActorID: request.ActorID,
		}
		if acquired.URL != "" {
			params.URL = acquired.URL
		}
		source, err := p.evidence.CreateSource(ctx, params)
		if err != nil {
			_, failed := fail(fetchStage, ErrorSourceCreateFailed, err)
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
	parser := p.parser
	if request.ChunkCharacters > 0 {
		parser = NewParser(request.ChunkCharacters, p.parser.generator)
	}
	parsed, err := parser.Parse(ctx, ParseRequest{
		Filename:    acquired.Filename,
		MIMEType:    acquired.MIMEType,
		Content:     acquired.Content,
		Provider:    request.Provider,
		Model:       request.Model,
		ImportJobID: &request.JobID,
		ImportRunID: &runID,
	})
	if err != nil {
		// Asset and Source were deliberately persisted before parsing so a failed
		// parser never destroys the original evidence.
		_, failed := fail(parseStage, parseErrorCode(err), err)
		return nil, "", failed
	}
	artifactJSON, err := parsed.ArtifactJSON()
	if err != nil {
		_, failed := fail(parseStage, ErrorParseArtifactFailed, err)
		return nil, "", failed
	}
	extractedAsset, err := p.evidence.StoreAsset(
		ctx,
		evidence.StoreAssetParams{
			WikiID:   request.WikiID,
			Name:     acquired.Filename + ".parse.json",
			Content:  bytes.NewReader(artifactJSON),
			MimeType: "application/json",
			ActorID:  request.ActorID,
		},
	)
	if err != nil {
		_, failed := fail(parseStage, ErrorParseArtifactStoreFailed, err)
		return nil, "", failed
	}
	version, err := p.evidence.AddSourceVersion(ctx, evidence.AddSourceVersionParams{
		WikiID: request.WikiID, SourceID: *sourceID,
		VersionHash: acquired.ContentHash,
		RawAssetID:  &asset.Revision.ID, ExtractedAssetID: &extractedAsset.Revision.ID,
		ParseTrack: parsed.Track, ParserName: parsed.ParserName,
		ParserVersion: parsed.ParserVersion, ParserFingerprint: parsed.ParserFingerprint,
		ParseMetadata: parsed.MetadataJSON(),
		FetchedAt:     time.Now().UTC(), Chunks: parsed.Chunks,
	})
	if err != nil {
		_, failed := fail(parseStage, ErrorSourceVersionFailed, err)
		return nil, "", failed
	}
	if err := p.jobs.CompleteParseStage(
		ctx, request.JobID, parseStage, version.Version.ID, false,
	); err != nil {
		return nil, "", err
	}
	return version, sourceLabel, nil
}

func (p *Pipeline) loadParseCheckpoint(ctx context.Context, request PipelineRequest) (*evidence.SourceVersionContent, error) {
	resume, err := p.repository.FindLatestParseCheckpoint(ctx, request.JobID)
	if err != nil || resume == nil {
		return nil, err
	}
	checkpoint, err := p.evidence.LoadSourceVersionContent(
		ctx, request.WikiID, resume.SourceVersionID,
	)
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
		return ErrorUnsafeURL
	case errors.Is(err, ErrSourceTooLarge):
		return ErrorSourceTooLarge
	case errors.Is(err, ErrUnsupportedMIME):
		return ErrorUnsupportedMIME
	case errors.Is(err, ErrMalware):
		return ErrorMalwareDetected
	default:
		return ErrorFetchFailed
	}
}

func parseErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPDFExtractorUnavailable):
		return ErrorPDFExtractorUnavailable
	case errors.Is(err, ErrPDFTextTooLarge):
		return ErrorPDFTextTooLarge
	case errors.Is(err, ErrPDFRasterizerUnavailable):
		return ErrorPDFRasterizerUnavailable
	case errors.Is(err, ErrPDFPageLimitExceeded):
		return ErrorPDFPageLimitExceeded
	case errors.Is(err, ErrAdvancedParserUnavailable):
		return ErrorAdvancedParserUnavailable
	case errors.Is(err, ErrVisionPageAnalysisFailed):
		return ErrorVisionPageAnalysisFailed
	case errors.Is(err, ErrOCRUnavailable):
		return ErrorOCRUnavailable
	case errors.Is(err, ErrOCRImageTooLarge):
		return ErrorOCRImageTooLarge
	case errors.Is(err, ErrOCRTextTooLarge):
		return ErrorOCRTextTooLarge
	case errors.Is(err, ErrOCRNoText):
		return ErrorOCRNoText
	case errors.Is(err, ErrOCRFailed):
		return ErrorOCRFailed
	default:
		return ErrorParseFailed
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
	case "text/plain", MIMEMarkdown:
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
		return ErrorExtractionOutputTruncated
	case errors.Is(err, ai.ErrInvalidOutput):
		return ErrorExtractionInvalidOutput
	case errors.Is(err, ai.ErrProvider):
		return ErrorExtractionProviderFailed
	case errors.Is(err, ai.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return ErrorExtractionTimeout
	case errors.Is(err, ErrEvidenceRequired):
		return ErrorExtractionEvidenceInvalid
	case errors.Is(err, ErrModelInputTooLarge):
		return ErrorExtractionInputTooLarge
	default:
		return ErrorExtractionFailed
	}
}

func (p *Pipeline) validate(sourceOnly bool) error {
	if p.jobs == nil || p.repository == nil || p.evidence == nil || p.parser == nil {
		return fmt.Errorf("%w: pipeline dependencies", ErrInvalidJob)
	}
	if !sourceOnly && (p.extraction == nil || p.planner == nil || p.matcher == nil ||
		p.classifier == nil || p.composer == nil || p.reviews == nil) {
		return fmt.Errorf("%w: pipeline dependencies", ErrInvalidJob)
	}
	return nil
}

func planErrorCode(err error) string {
	var providerErr *ai.ProviderError
	switch {
	case errors.As(err, &providerErr) && providerErr.Code == "output_truncated":
		return ErrorPagePlanOutputTruncated
	case errors.Is(err, ai.ErrInvalidOutput):
		return ErrorPagePlanInvalidOutput
	case errors.Is(err, ai.ErrProvider):
		return ErrorPagePlanProviderFailed
	case errors.Is(err, ai.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return ErrorPagePlanTimeout
	case errors.Is(err, ErrEvidenceRequired):
		return ErrorPagePlanEvidenceInvalid
	case errors.Is(err, ErrModelInputTooLarge):
		return ErrorPagePlanInputTooLarge
	case errors.Is(err, ErrPlanTargetConflict):
		return ErrorPagePlanTargetConflict
	case errors.Is(err, ErrNoPagePlan):
		return ErrorNoPagePlan
	case errors.Is(err, ErrQualityGate):
		return ErrorPagePlanQualityGate
	default:
		return ErrorPagePlanFailed
	}
}

// DetailJob is a compact read helper used by the pipeline result and API.
func (s *Service) DetailJob(ctx context.Context, jobID uuid.UUID) (*Job, error) {
	return s.repo.GetJob(ctx, nil, jobID)
}
