package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ai"
)

func TestFailureProviderDiagnosticsNeverExposeRawErrors(t *testing.T) {
	const secret = "secret-key-and-private-source"
	tests := []struct {
		code       string
		wantCode   string
		wantHTTP   int
		wantRetry  bool
		wantPhrase string
	}{
		{"http_401", "http_401", 401, false, "认证"},
		{"http_429", "http_429", 429, true, "限流"},
		{"http_503", "http_503", 503, true, "不可用"},
		{"authentication_failed", "authentication_failed", 0, false, "认证"},
		{"invalid_request", "invalid_request", 0, false, "参数"},
		{"rate_limited", "rate_limited", 0, true, "限流"},
		{secret, "", 0, true, "供应商"},
		{"http_401 " + secret, "", 0, true, "供应商"},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			cause := fmt.Errorf("page context: %w", &ai.ProviderError{Code: test.code,
				Err: fmt.Errorf("%w: %s", ai.ErrProvider, secret)})
			failure := newFailure(StageExtract, ErrorExtractionProviderFailed, cause)
			if strings.Contains(string(failure.JSON()), secret) {
				t.Fatal("raw provider error or arbitrary code leaked")
			}
			if failure.Details.ProviderCode != test.wantCode || failure.Details.HTTPStatus != test.wantHTTP ||
				failure.Retryable != test.wantRetry || !strings.Contains(failure.Message, test.wantPhrase) {
				t.Fatalf("unexpected public diagnostic: %s", failure.JSON())
			}
		})
	}
	failure := newFailure(StageParse, ErrorParseFailed, errors.New(secret))
	if strings.Contains(string(failure.JSON()), secret) {
		t.Fatal("raw parser error leaked")
	}
}

func TestExtractionFailureExplainsInjectionAndImmutableReuse(t *testing.T) {
	record := &Extraction{ID: uuid.New(), SourceVersionID: uuid.New(), Model: "test-model"}
	candidates := &Candidates{PromptInjectionDetected: true, QualityScore: 0.95,
		Entities: []EntityCandidate{{Label: "private label", Evidence: []CandidateEvidence{{Quotation: "private source"}}}}}
	failure := extractionFailure(&ExtractResult{Extraction: record, Candidates: candidates, Reused: true}, 0.7)
	if failure.Retryable || !strings.Contains(failure.Message, "提示注入") ||
		failure.Details.PromptInjectionDetected == nil || !*failure.Details.PromptInjectionDetected ||
		failure.Details.ExtractionReused == nil || !*failure.Details.ExtractionReused ||
		*failure.Details.QualityScore != 0.95 || *failure.Details.EntityCount != 1 {
		t.Fatalf("unexpected extraction diagnostic: %s", failure.JSON())
	}
	if strings.Contains(string(failure.JSON()), "private") {
		t.Fatal("candidate content leaked")
	}
	var decoded Failure
	if err := json.Unmarshal(failure.JSON(), &decoded); err != nil || decoded.SchemaVersion != 1 {
		t.Fatal("invalid failure envelope")
	}
	// A high quality score never overrides the injection gate.
	if _, err := trustedPlanningCandidates(candidates, 0.7); !errors.Is(err, ErrQualityGate) {
		t.Fatal("injection policy changed")
	}
}

func TestFailureReadEnrichmentPreservesActionPayload(t *testing.T) {
	raw := json.RawMessage(`{"stage":"match","code":"entity_resolution_required","match_review":{"candidates":[]}}`)
	if string(enrichFailure(raw)) != string(raw) {
		t.Fatal("human match review payload changed")
	}
	current := newFailure(StageExtract, ErrorExtractionTimeout, nil).JSON()
	if string(enrichFailure(current)) != string(current) {
		t.Fatal("versioned historical failure changed")
	}
}

func TestParseFailurePreservesSafePageAndProviderCause(t *testing.T) {
	cause := &visionPageError{page: 7, cause: &ai.ProviderError{
		Code: "authentication_failed", Err: errors.New("private supplier response"),
	}}
	failure := newFailure(StageParse, parseErrorCode(cause), cause)
	if !errors.Is(cause, ErrVisionPageAnalysisFailed) || failure.Code != ErrorVisionPageAnalysisFailed ||
		failure.Details.Page != 7 || failure.Details.ProviderCode != "authentication_failed" ||
		failure.Retryable || strings.Contains(string(failure.JSON()), "private") {
		t.Fatalf("page failure lost its safe cause: %s", failure.JSON())
	}
	sourceError := &sourceHTTPError{status: 503}
	failure = newFailure(StageFetch, acquisitionErrorCode(sourceError), sourceError)
	if !errors.Is(sourceError, ErrFetchFailed) || failure.Code != ErrorFetchFailed ||
		failure.Details.HTTPStatus != 503 || !failure.Retryable {
		t.Fatal("source HTTP status was discarded")
	}
}
