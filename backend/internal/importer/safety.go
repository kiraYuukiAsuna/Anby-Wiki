package importer

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
)

const (
	SafetyDecisionAccept = "accept"
	SafetyDecisionReject = "reject"
	safetyReviewEvent    = "import.safety_reviewed"
	importJobAggregate   = "import_job"
	safetyReviewedInput  = "\x00safety-reviewed"
)

// Nouns such as "system prompt" and first-person character descriptions are
// not control instructions. Quoted examples remain untrusted source data;
// the model can still flag them for human review.
var injectionInstruction = regexp.MustCompile(`(?i)(ignore|disregard|override)\s+(all\s+)?(previous|prior|above|system|developer)\s+(instructions|rules|messages)|(?:reveal|print|show|output|expose)\s+(?:your\s+|the\s+)?(?:system\s+prompt|prompt|developer\s+message|hidden\s+instructions|api\s+key|secrets)|(?:set|return)\s+prompt_injection_detected\s*(?:=|to|:)\s*false|忽略(?:所有|全部)?(?:之前|以上|先前|系统|开发者)的?(?:指令|规则|提示)|(?:泄露|输出|显示|打印)(?:你的|内部|原始)?(?:系统提示词|开发者消息|密钥)|(?:绕过|跳过)(?:人工)?(?:审核|安全门禁)`)

var safetyQuotation = regexp.MustCompile("\"[^\"\n]*\"|“[^”]*”|「[^」]*」|`[^`\n]*`")
var safetyContextPhrases = []string{"prompt", "instruction", "ignore", "you are", "指令", "提示词", "扮演", "我是", "你是", "审核"}

func containsSafetyPhrase(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func DetectPromptInjection(texts []string) bool {
	// Preserve instructions spanning OCR line breaks or chunk boundaries.
	text := strings.Join(texts, "\n")
	instructions := injectionInstruction.FindAllStringIndex(text, -1)
	if len(instructions) == 0 {
		return false
	}
	quotes := safetyQuotation.FindAllStringIndex(text, -1)
	quoteIndex := 0
	for _, instruction := range instructions {
		for quoteIndex < len(quotes) && quotes[quoteIndex][1] <= instruction[0] {
			quoteIndex++
		}
		if quoteIndex == len(quotes) || quotes[quoteIndex][0] > instruction[0] || quotes[quoteIndex][1] < instruction[1] {
			return true
		}
		quote := quotes[quoteIndex]
		// Explanatory prose must be adjacent to the quotation. A distant
		// "example" elsewhere in a long source cannot excuse a control command.
		before := text[:quote[0]]
		before = before[strings.LastIndexAny(before, "\n。.!?！？")+1:]
		after := text[quote[1]:]
		if boundary := strings.IndexAny(after, "\n。.!?！？"); boundary >= 0 {
			after = after[:boundary]
		}
		context := strings.ToLower(before + after)
		if !containsSafetyPhrase(context, []string{"example", "quote", "attack", "illustrat", "示例", "引用", "攻击", "例如"}) {
			return true
		}
	}
	return false
}

type SafetySourceSample struct {
	ChunkID uuid.UUID `json:"chunk_id"`
	Page    *int      `json:"page,omitempty"`
	Text    string    `json:"text"`
}

type SafetyDecision struct {
	SchemaVersion   int       `json:"schema_version"`
	SourceVersionID uuid.UUID `json:"source_version_id"`
	ExtractionID    uuid.UUID `json:"extraction_id"`
	Decision        string    `json:"decision"`
	Reason          string    `json:"reason"`
	ReviewedBy      uuid.UUID `json:"reviewed_by"`
	ReviewedAt      time.Time `json:"reviewed_at"`
}

// SafetyReview contains source previews only on the protected job-detail API.
// Previews are never copied into failure diagnostics, usage logs or audits.
type SafetyReview struct {
	SchemaVersion   int                  `json:"schema_version"`
	SourceID        uuid.UUID            `json:"source_id"`
	SourceVersionID uuid.UUID            `json:"source_version_id"`
	ExtractionID    uuid.UUID            `json:"extraction_id"`
	Reviewable      bool                 `json:"reviewable"`
	CanReview       bool                 `json:"can_review"`
	DirectControl   bool                 `json:"direct_control"`
	Samples         []SafetySourceSample `json:"samples"`
	Decision        *SafetyDecision      `json:"decision,omitempty"`
}

type ReviewSafetyInput struct {
	SchemaVersion int       `json:"schema_version"`
	ExtractionID  uuid.UUID `json:"extraction_id"`
	Decision      string    `json:"decision"`
	Reason        string    `json:"reason"`
}

func (r *Repository) safetyDecision(ctx context.Context, tx pgx.Tx, jobID, extractionID uuid.UUID) (*SafetyDecision, error) {
	var raw json.RawMessage
	err := r.q(tx).QueryRow(ctx, `SELECT payload_json FROM audit_event
		WHERE aggregate_type=$1 AND aggregate_id=$2 AND event_type=$3
		  AND payload_json->>'extraction_id'=$4 ORDER BY created_at DESC,id DESC LIMIT 1`,
		importJobAggregate, jobID, safetyReviewEvent, extractionID.String()).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var decision SafetyDecision
	if err := json.Unmarshal(raw, &decision); err != nil {
		return nil, err
	}
	return &decision, nil
}

func (s *Service) extractionSafety(ctx context.Context, tx pgx.Tx, job *Job) (*SafetyReview, error) {
	if job.SourceVersionID == nil {
		return nil, nil
	}
	record, err := s.repo.GetExtraction(ctx, *job.SourceVersionID)
	if errors.Is(err, ErrExtractionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var candidates Candidates
	if err := json.Unmarshal(record.CandidatesJSON, &candidates); err != nil {
		return nil, err
	}
	var failure Failure
	_ = json.Unmarshal(job.Error, &failure)
	if !candidates.PromptInjectionDetected && (failure.Code != ErrorQualityGate || failure.Stage != StageExtract) {
		return nil, nil
	}
	evidenceRepo := evidence.NewRepository(s.repo.pool)
	version, err := evidenceRepo.GetSourceVersionByID(ctx, tx, record.SourceVersionID)
	if err != nil {
		return nil, err
	}
	chunks, err := evidenceRepo.ListSourceChunks(ctx, tx, record.SourceVersionID)
	if err != nil {
		return nil, err
	}
	direct := DetectPromptInjection(chunkTexts(chunks))
	if !direct && !candidates.PromptInjectionDetected {
		return nil, nil
	}
	review := &SafetyReview{
		SchemaVersion: 1, SourceID: version.SourceID, SourceVersionID: record.SourceVersionID, ExtractionID: record.ID,
		Reviewable: !direct, DirectControl: direct, Samples: []SafetySourceSample{},
	}
	review.Decision, err = s.repo.safetyDecision(ctx, tx, job.ID, record.ID)
	if err != nil {
		return nil, err
	}
	// Prefer suspicious vocabulary, but also show source context when only the
	// model reported a risk and did not return a localized explanation.
	for _, chunk := range chunks {
		text := strings.TrimSpace(chunk.TextContent)
		runes := []rune(text)
		lower := strings.ToLower(text)
		if text == "" || !containsSafetyPhrase(lower, safetyContextPhrases) {
			continue
		}
		start := len(runes)
		for _, phrase := range safetyContextPhrases {
			if offset := strings.Index(lower, phrase); offset >= 0 {
				start = min(start, len([]rune(lower[:offset])))
			}
		}
		startRune := max(0, start-200)
		var locator struct {
			Page *int `json:"page"`
		}
		_ = json.Unmarshal(chunk.LocatorJSON, &locator)
		review.Samples = append(review.Samples, SafetySourceSample{ChunkID: chunk.ID, Page: locator.Page, Text: string(runes[startRune:min(len(runes), startRune+800)])})
		if len(review.Samples) == 8 {
			break
		}
	}
	if len(review.Samples) == 0 {
		for _, chunk := range chunks[:min(len(chunks), 3)] {
			runes := []rune(chunk.TextContent)
			var locator struct {
				Page *int `json:"page"`
			}
			_ = json.Unmarshal(chunk.LocatorJSON, &locator)
			review.Samples = append(review.Samples, SafetySourceSample{ChunkID: chunk.ID, Page: locator.Page, Text: string(runes[:min(len(runes), 800)])})
		}
	}
	err = governance.NewAuthorizationService(s.repo.pool).CheckTx(ctx, tx, job.InitiatedBy, job.WikiID, governance.ActionReview, nil)
	if err != nil && !errors.Is(err, governance.ErrPermissionDenied) && !errors.Is(err, governance.ErrInvalidActor) {
		return nil, err
	}
	review.CanReview = err == nil
	actorType, status, _, err := s.repo.Actor(ctx, job.InitiatedBy)
	if err != nil {
		return nil, err
	}
	review.CanReview = review.CanReview && actorType == authdomain.ActorTypeHuman && status == authdomain.ActorStatusActive
	return review, nil
}

func (s *Service) ReviewSafety(ctx context.Context, actorID, wikiID, jobID uuid.UUID, input ReviewSafetyInput) error {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.SchemaVersion != 1 || input.ExtractionID == uuid.Nil ||
		(input.Decision != SafetyDecisionAccept && input.Decision != SafetyDecisionReject) ||
		len([]rune(input.Reason)) < 5 || len([]rune(input.Reason)) > 1000 {
		return ErrInvalidJob
	}
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if job.WikiID != wikiID || job.InitiatedBy != actorID {
			return ErrJobNotFound
		}
		var actorType, status string
		if err := tx.QueryRow(ctx, `SELECT actor_type,status FROM actor WHERE id=$1 FOR SHARE`, actorID).Scan(&actorType, &status); err != nil {
			return err
		}
		if actorType != authdomain.ActorTypeHuman || status != authdomain.ActorStatusActive {
			return governance.ErrPermissionDenied
		}
		if err := governance.NewAuthorizationService(s.repo.pool).CheckTx(ctx, tx, actorID, wikiID, governance.ActionReview, nil); err != nil {
			return err
		}
		review, err := s.extractionSafety(ctx, tx, job)
		if err != nil {
			return err
		}
		if review == nil || review.ExtractionID != input.ExtractionID {
			return ErrInvalidTransition
		}
		if review.Decision != nil {
			if review.Decision.Decision != input.Decision || review.Decision.Reason != input.Reason {
				return ErrIdempotencyMismatch
			}
			return nil
		}
		var failure Failure
		_ = json.Unmarshal(job.Error, &failure)
		if (job.Status != JobFailed && job.Status != JobActionRequired) ||
			job.CurrentStage != StageExtract || failure.Code != ErrorQualityGate {
			return ErrInvalidTransition
		}
		if input.Decision == SafetyDecisionAccept && !review.Reviewable {
			return ErrQualityGate
		}
		if input.Decision == SafetyDecisionAccept {
			if err := s.checkJobAuthor(ctx, tx, job); err != nil {
				return err
			}
		}
		decision := SafetyDecision{
			SchemaVersion: 1, SourceVersionID: review.SourceVersionID, ExtractionID: review.ExtractionID,
			Decision: input.Decision, Reason: input.Reason, ReviewedBy: actorID, ReviewedAt: time.Now().UTC(),
		}
		raw, err := json.Marshal(decision)
		if err != nil {
			return err
		}
		eventID, err := s.ids.New()
		if err != nil {
			return err
		}
		if err := governance.NewRepository(s.repo.pool).InsertAuditWithoutBatch(ctx, tx, eventID, actorID,
			safetyReviewEvent, importJobAggregate, jobID, raw); err != nil {
			return err
		}
		if input.Decision == SafetyDecisionReject {
			return s.repo.FinishJob(ctx, tx, job.ID, JobCancelled, StageExtract, job.Progress, nil, nil, job.Error)
		}
		return s.repo.RequeueForPlanning(ctx, tx, job.ID, job.PlanningInput, eventID.String())
	})
}
