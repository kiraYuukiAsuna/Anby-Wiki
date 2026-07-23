package linkhealth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type Report struct {
	Claimed   int
	OK        int
	Redirect  int
	Broken    int
	Blocked   int
	Proposals int
}

type Runner struct {
	pool       *pgxpool.Pool
	resources  *evidence.ExternalResourceService
	governance *governance.Service
	reviews    *governance.ReviewService
	prober     Prober
	logger     *slog.Logger
}

func NewRunner(pool *pgxpool.Pool, prober Prober, logger *slog.Logger) *Runner {
	if prober == nil {
		prober = NewHTTPProber(nil, nil)
	}
	if logger == nil {
		logger = slog.Default()
	}
	ids := id.NewGenerator()
	return &Runner{
		pool:       pool,
		resources:  evidence.NewExternalResourceService(evidence.NewRepository(pool), ids),
		governance: governance.NewService(governance.NewRepository(pool), db.NewTxManager(pool), ids),
		reviews: governance.NewReviewService(
			governance.NewRepository(pool), db.NewTxManager(pool), ids, governance.NewRiskEvaluator(nil),
		),
		prober: prober,
		logger: logger,
	}
}

// RunOnce claims and processes one bounded batch.
func (r *Runner) RunOnce(ctx context.Context, batchSize int) (Report, error) {
	resources, err := r.resources.ClaimDue(ctx, batchSize)
	if err != nil {
		return Report{}, err
	}
	report := Report{Claimed: len(resources)}
	for i := range resources {
		proposals, status, err := r.process(ctx, resources[i])
		if err != nil {
			return report, err
		}
		report.Proposals += proposals
		switch status {
		case evidence.ExternalResourceStatusOK:
			report.OK++
		case evidence.ExternalResourceStatusRedirect:
			report.Redirect++
		case evidence.ExternalResourceStatusBroken:
			report.Broken++
		case evidence.ExternalResourceStatusBlocked:
			report.Blocked++
		}
	}
	return report, nil
}

// Run polls until cancellation. A short idle interval discovers resources
// created after startup; leases and next_check_at enforce the actual cadence.
func (r *Runner) Run(ctx context.Context, batchSize int, idleInterval time.Duration) error {
	if idleInterval <= 0 {
		idleInterval = time.Minute
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			report, err := r.RunOnce(ctx, batchSize)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				r.logger.Warn("外链健康检查批次失败", slog.Any("error", err))
			} else if report.Claimed > 0 {
				r.logger.Info("外链健康检查批次完成",
					slog.Int("claimed", report.Claimed),
					slog.Int("ok", report.OK),
					slog.Int("redirect", report.Redirect),
					slog.Int("broken", report.Broken),
					slog.Int("blocked", report.Blocked),
					slog.Int("proposals", report.Proposals))
			}
			timer.Reset(idleInterval)
		}
	}
}

func (r *Runner) process(ctx context.Context, resource evidence.ExternalResource) (int, string, error) {
	probe, probeErr := r.prober.Probe(ctx, resource.NormalizedURL)
	if probeErr != nil {
		status := evidence.ExternalResourceStatusBroken
		if errors.Is(probeErr, ErrUnsafeURL) {
			status = evidence.ExternalResourceStatusBlocked
		}
		_, err := r.resources.CompleteCheck(ctx, resource, evidence.ExternalResourceCheckResult{
			Status: status, TransientFailure: errors.Is(probeErr, ErrProbe),
		})
		return 0, status, err
	}

	var targetResource *evidence.ExternalResource
	if probe.TargetURL != nil {
		var err error
		targetResource, err = r.resources.Upsert(ctx, *probe.TargetURL)
		if err != nil {
			return 0, "", err
		}
	}
	var redirectTargetID *uuid.UUID
	if probe.Status == evidence.ExternalResourceStatusRedirect && targetResource != nil {
		redirectTargetID = &targetResource.ID
	}
	if _, err := r.resources.CompleteCheck(ctx, resource, evidence.ExternalResourceCheckResult{
		Status: probe.Status, HTTPStatus: probe.HTTPStatus, ContentHash: probe.ContentHash,
		CanonicalURL: probe.CanonicalURL, RedirectTargetID: redirectTargetID,
	}); err != nil {
		return 0, "", err
	}
	if targetResource == nil {
		return 0, probe.Status, nil
	}
	count, err := r.composeRetargetProposals(ctx, resource.ID, *targetResource, *probe.TargetURL)
	if err != nil {
		if retryErr := r.resources.RetryCheck(ctx, resource); retryErr != nil {
			return count, probe.Status, errors.Join(err, retryErr)
		}
	}
	return count, probe.Status, err
}

type linkUsage struct {
	PageID     uuid.UUID
	RevisionID uuid.UUID
	BlockID    uuid.UUID
	NodeID     string
	ASTJSON    []byte
}

func (r *Runner) composeRetargetProposals(ctx context.Context, sourceID uuid.UUID, target evidence.ExternalResource, targetURL string) (int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT usage.page_id, usage.revision_id, usage.block_id, usage.node_id, snapshot.ast_json
		FROM external_link_usage AS usage
		JOIN page ON page.id = usage.page_id
			AND page.current_revision_id = usage.revision_id
			AND page.status = 'active'
		JOIN revision ON revision.id = usage.revision_id
		JOIN content_snapshot AS snapshot ON snapshot.id = revision.content_snapshot_id
		WHERE usage.external_resource_id = $1
		ORDER BY usage.page_id, usage.block_id, usage.node_id`, sourceID)
	if err != nil {
		return 0, fmt.Errorf("linkhealth: 查询外链 usage 失败: %w", err)
	}
	defer rows.Close()
	var usages []linkUsage
	for rows.Next() {
		var usage linkUsage
		if err := rows.Scan(&usage.PageID, &usage.RevisionID, &usage.BlockID, &usage.NodeID, &usage.ASTJSON); err != nil {
			return 0, fmt.Errorf("linkhealth: 扫描外链 usage 失败: %w", err)
		}
		usages = append(usages, usage)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("linkhealth: 遍历外链 usage 失败: %w", err)
	}

	created := 0
	for _, usage := range usages {
		ok, err := r.composeUsageProposal(ctx, sourceID, target, targetURL, usage)
		if err != nil {
			return created, err
		}
		if ok {
			created++
		}
	}
	return created, nil
}

func (r *Runner) composeUsageProposal(ctx context.Context, sourceID uuid.UUID, target evidence.ExternalResource, targetURL string, usage linkUsage) (bool, error) {
	doc, err := ast.Parse(usage.ASTJSON)
	if err != nil {
		return false, fmt.Errorf("linkhealth: 解析 Revision AST 失败: %w", err)
	}
	location, ok := ast.FindBlock(doc, usage.BlockID.String())
	if !ok {
		return false, nil
	}
	index, err := strconv.Atoi(usage.NodeID)
	if err != nil || index < 0 {
		return false, nil
	}
	nodes, err := location.Block.InlineContent()
	if err != nil || index >= len(nodes) || nodes[index] == nil || nodes[index].Type != ast.InlineExternalLink {
		return false, nil
	}
	currentURL, err := evidence.NormalizeURL(nodes[index].URL)
	if err != nil || currentURL == targetURL {
		return false, nil
	}
	hash, err := governance.BlockHash(location.Block)
	if err != nil {
		return false, err
	}
	key := fmt.Sprintf("link-health:%s:%s:%s:%s:%s",
		sourceID, target.ID, usage.RevisionID, usage.BlockID, usage.NodeID)
	proposal, err := r.governance.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetPage, TargetID: &usage.PageID, BaseRevisionID: &usage.RevisionID,
		RiskLevel: governance.RiskMedium, CreatedBy: systemActorID, IdempotencyKey: key,
	})
	if err != nil {
		return false, err
	}
	operations, err := r.governance.ListOperations(ctx, proposal.ID)
	if err != nil {
		return false, err
	}
	if proposal.Status != governance.ProposalDraft {
		return false, nil
	}
	if len(operations) > 1 {
		return false, fmt.Errorf("linkhealth: Proposal %s 包含 %d 个 Operation", proposal.ID, len(operations))
	}
	blockID := usage.BlockID.String()
	payload, _ := json.Marshal(map[string]string{
		"old_url": nodes[index].URL, "url": targetURL, "display_text": nodes[index].DisplayText,
	})
	operation := governance.OperationV1{
		SchemaVersion: governance.OperationVersion,
		OperationType: governance.OpRetargetExternalLink,
		Base:          governance.OperationBase{RevisionID: &usage.RevisionID},
		Target: governance.OperationTarget{
			PageID: &usage.PageID, BlockID: &blockID, NodeID: &usage.NodeID,
			ExternalResourceID: &target.ID,
		},
		ExpectedHash: &hash,
		Evidence: []governance.OperationEvidence{{
			Note: "External resource canonical or redirect target changed.",
		}},
		Risk: governance.OperationRisk{
			Level: governance.RiskMedium, Reasons: []string{"automated external link retarget requires review"},
		},
		Payload: payload,
	}
	raw, err := json.Marshal(operation)
	if err != nil {
		return false, err
	}
	if len(operations) == 0 {
		if _, err := r.governance.AddOperationV1(ctx, proposal.ID, raw); err != nil {
			return false, err
		}
	}
	if _, err := r.reviews.Submit(ctx, proposal.ID); err != nil {
		// A concurrent retry may have committed Submit after this worker read
		// draft. Re-read through the idempotent create path before failing.
		current, currentErr := r.governance.CreateProposal(ctx, governance.CreateProposalParams{
			TargetType: governance.TargetPage, TargetID: &usage.PageID, BaseRevisionID: &usage.RevisionID,
			RiskLevel: governance.RiskMedium, CreatedBy: systemActorID, IdempotencyKey: key,
		})
		if currentErr == nil && current.Status != governance.ProposalDraft {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

var systemActorID = uuid.MustParse("00000000-0000-7000-8000-000000000201")
