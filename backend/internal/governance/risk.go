package governance

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/anby/wiki/backend/internal/knowledge"
)

type RiskDecision struct {
	Level       string   `json:"level"`
	Reasons     []string `json:"reasons"`
	AutoApprove bool     `json:"auto_approve"`
	Policy      string   `json:"policy"`
}

type RiskEvaluator struct{ knowledge *knowledge.Service }

func NewRiskEvaluator(knowledgeService *knowledge.Service) *RiskEvaluator {
	return &RiskEvaluator{knowledge: knowledgeService}
}

func (e *RiskEvaluator) Evaluate(ctx context.Context, records []OperationRecord) (*RiskDecision, error) {
	level := RiskLow
	reasons := []string{}
	autoSafe := len(records) > 0
	renameCount := 0
	for i := range records {
		op, err := OperationFromRecord(&records[i])
		if err != nil {
			return nil, err
		}
		level = maxRisk(level, op.Risk.Level)
		for _, reason := range op.Risk.Reasons {
			reasons = appendUnique(reasons, reason)
		}
		switch op.OperationType {
		case OpRetargetPageReference:
			// 无歧义的稳定 ID 链接修复可自动批准。
		case OpRetargetExternalLink:
			var p struct {
				OldURL string `json:"old_url"`
				URL    string `json:"url"`
			}
			_ = json.Unmarshal(op.Payload, &p)
			if crossDomain(p.OldURL, p.URL) {
				level = maxRisk(level, RiskHigh)
				reasons = appendUnique(reasons, "跨域替换外部链接")
				autoSafe = false
			}
		case OpDeleteBlock:
			var p struct {
				EstimatedDeletedChars int `json:"estimated_deleted_chars"`
			}
			_ = json.Unmarshal(op.Payload, &p)
			if p.EstimatedDeletedChars >= 500 {
				level = maxRisk(level, RiskHigh)
				reasons = appendUnique(reasons, "删除大段正文")
			} else {
				level = maxRisk(level, RiskMedium)
				reasons = appendUnique(reasons, "删除正文 Block")
			}
			autoSafe = false
		case OpSupersedeClaim:
			autoSafe = false
			level = maxRisk(level, RiskMedium)
			if e.knowledge != nil && op.Target.ClaimID != nil {
				claim, err := e.knowledge.GetClaim(ctx, *op.Target.ClaimID)
				if err != nil {
					return nil, err
				}
				if claim.VerificationStatus == knowledge.VerificationHumanVerified {
					level = maxRisk(level, RiskHigh)
					reasons = appendUnique(reasons, "覆盖人工验证 Claim")
				}
			}
		case OpRenamePage:
			renameCount++
			autoSafe = false
			level = maxRisk(level, RiskMedium)
		default:
			autoSafe = false
			level = maxRisk(level, RiskMedium)
		}
	}
	if renameCount >= 10 {
		level = maxRisk(level, RiskHigh)
		reasons = appendUnique(reasons, "批量重命名页面")
	}
	if level == RiskHigh || level == RiskCritical {
		autoSafe = false
	}
	return &RiskDecision{Level: level, Reasons: reasons, AutoApprove: autoSafe && level == RiskLow, Policy: "proposal-risk-v1"}, nil
}

func maxRisk(a, b string) string {
	rank := map[string]int{RiskLow: 0, RiskMedium: 1, RiskHigh: 2, RiskCritical: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func crossDomain(oldRaw, newRaw string) bool {
	oldURL, oldErr := url.Parse(oldRaw)
	newURL, newErr := url.Parse(newRaw)
	return oldErr == nil && newErr == nil && oldURL.Hostname() != "" && newURL.Hostname() != "" &&
		!strings.EqualFold(oldURL.Hostname(), newURL.Hostname())
}
