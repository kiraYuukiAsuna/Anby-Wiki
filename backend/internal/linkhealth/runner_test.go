package linkhealth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/linkhealth"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

type proberStub struct {
	result linkhealth.ProbeResult
	err    error
}

func (p proberStub) Probe(context.Context, string) (linkhealth.ProbeResult, error) {
	return p.result, p.err
}

func TestRunnerPersistsRedirectAndCreatesIdempotentRetargetProposal(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "link author")
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "link-health", "Link Health", actorID)
	blockID := tdb.NewID(t)
	sourceURL := "https://old.example/article"
	targetURL := "https://new.example/article"
	astJSON := json.RawMessage(fmt.Sprintf(`{
		"type":"document","schema_version":1,"children":[{
			"type":"paragraph","id":%q,
			"content":[{"type":"external_link","url":%q,"display_text":"Article"}]
		}]}`, blockID.String(), sourceURL))
	pageService := page.NewService(page.NewRepository(tdb.Pool), db.NewTxManager(tdb.Pool), id.NewGenerator())
	revision, err := pageService.Publish(ctx, page.PublishParams{
		PageID: pageID, ActorID: actorID, AST: astJSON,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	resources := evidence.NewExternalResourceService(evidence.NewRepository(tdb.Pool), id.NewGenerator())
	source, err := resources.Upsert(ctx, sourceURL)
	if err != nil {
		t.Fatalf("Upsert source: %v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO external_link_usage (
			external_resource_id, page_id, revision_id, block_id, node_id, link_role)
		VALUES ($1,$2,$3,$4,'0','inline')`,
		source.ID, pageID, revision.ID, blockID); err != nil {
		t.Fatalf("insert usage: %v", err)
	}
	status := int32(200)
	hash := "probe-content-hash"
	runner := linkhealth.NewRunner(tdb.Pool, proberStub{result: linkhealth.ProbeResult{
		Status: evidence.ExternalResourceStatusRedirect, HTTPStatus: &status,
		ContentHash: &hash, CanonicalURL: &targetURL, TargetURL: &targetURL,
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	report, err := runner.RunOnce(ctx, 20)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if report.Claimed != 1 || report.Redirect != 1 || report.Proposals != 1 {
		t.Fatalf("report=%+v", report)
	}
	updated, err := evidence.NewRepository(tdb.Pool).GetExternalResourceByID(ctx, nil, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != evidence.ExternalResourceStatusRedirect || updated.HTTPStatus == nil ||
		*updated.HTTPStatus != 200 || updated.RedirectTargetID == nil ||
		updated.CanonicalURL == nil || *updated.CanonicalURL != targetURL ||
		updated.ConsecutiveFailures != 0 || !updated.NextCheckAt.After(time.Now().Add(23*time.Hour)) {
		t.Fatalf("updated=%+v", updated)
	}

	var proposalID, proposalStatus string
	var operationCount, revisionCount int
	if err := tdb.Pool.QueryRow(ctx, `
		SELECT proposal.id::text, proposal.status, count(operation.id)
		FROM proposal
		JOIN proposal_operation AS operation ON operation.proposal_id = proposal.id
		WHERE proposal.target_type = 'page' AND proposal.target_id = $1
		GROUP BY proposal.id, proposal.status`, pageID).Scan(&proposalID, &proposalStatus, &operationCount); err != nil {
		t.Fatalf("read proposal: %v", err)
	}
	if proposalStatus != "in_review" || operationCount != 1 {
		t.Fatalf("proposal=%s status=%s operations=%d", proposalID, proposalStatus, operationCount)
	}
	var reviewTasks int
	if err := tdb.Pool.QueryRow(ctx, `
		SELECT count(*) FROM review_task WHERE proposal_id=$1 AND status='pending'`, proposalID).Scan(&reviewTasks); err != nil {
		t.Fatal(err)
	}
	if reviewTasks != 1 {
		t.Fatalf("pending review tasks=%d", reviewTasks)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM revision WHERE page_id=$1`, pageID).Scan(&revisionCount); err != nil {
		t.Fatal(err)
	}
	if revisionCount != 1 {
		t.Fatalf("link check created revision: %d", revisionCount)
	}

	if _, err := tdb.Pool.Exec(ctx, `UPDATE external_resource SET next_check_at=now()+interval '2 days'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `
		UPDATE external_resource SET next_check_at=now()-interval '1 second' WHERE id=$1`, source.ID); err != nil {
		t.Fatal(err)
	}
	report, err = runner.RunOnce(ctx, 20)
	if err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}
	if report.Proposals != 0 {
		t.Fatalf("second report duplicated proposal: %+v", report)
	}
	var proposals, operations int
	if err := tdb.Pool.QueryRow(ctx, `
		SELECT count(DISTINCT proposal.id), count(operation.id)
		FROM proposal JOIN proposal_operation AS operation ON operation.proposal_id=proposal.id
		WHERE proposal.target_id=$1`, pageID).Scan(&proposals, &operations); err != nil {
		t.Fatal(err)
	}
	if proposals != 1 || operations != 1 {
		t.Fatalf("idempotency failed: proposals=%d operations=%d", proposals, operations)
	}

}

func TestRunnerRecoversProposalOrchestrationFaults(t *testing.T) {
	for _, fault := range []struct {
		name           string
		installSQL     string
		removeSQL      string
		operationsLeft int
	}{
		{
			name: "after_proposal_before_operation",
			installSQL: `
				CREATE FUNCTION linkhealth_fail_operation() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'injected operation failure'; END $$;
				CREATE TRIGGER linkhealth_fail_operation
				BEFORE INSERT ON proposal_operation
				FOR EACH ROW EXECUTE FUNCTION linkhealth_fail_operation()`,
			removeSQL: `
				DROP TRIGGER linkhealth_fail_operation ON proposal_operation;
				DROP FUNCTION linkhealth_fail_operation()`,
		},
		{
			name: "after_operation_before_submit",
			installSQL: `
				CREATE FUNCTION linkhealth_fail_submit() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'injected submit failure'; END $$;
				CREATE TRIGGER linkhealth_fail_submit
				BEFORE UPDATE OF status ON proposal
				FOR EACH ROW WHEN (NEW.status = 'submitted')
				EXECUTE FUNCTION linkhealth_fail_submit()`,
			removeSQL: `
				DROP TRIGGER linkhealth_fail_submit ON proposal;
				DROP FUNCTION linkhealth_fail_submit()`,
			operationsLeft: 1,
		},
	} {
		t.Run(fault.name, func(t *testing.T) {
			tdb := testkit.Open(t)
			tdb.Reset(t)
			ctx := context.Background()
			actorID := tdb.MakeActor(t, "human", "fault link author")
			pageID := tdb.MakePage(t, testkit.MainNamespaceID, "link-health-fault", "Link Health Fault", actorID)
			blockID := tdb.NewID(t)
			sourceURL := "https://fault-old.example/article"
			targetURL := "https://fault-new.example/article"
			astJSON := json.RawMessage(fmt.Sprintf(`{
				"type":"document","schema_version":1,"children":[{
					"type":"paragraph","id":%q,
					"content":[{"type":"external_link","url":%q,"display_text":"Article"}]
				}]}`, blockID.String(), sourceURL))
			pageService := page.NewService(page.NewRepository(tdb.Pool), db.NewTxManager(tdb.Pool), id.NewGenerator())
			revision, err := pageService.Publish(ctx, page.PublishParams{
				PageID: pageID, ActorID: actorID, AST: astJSON,
			})
			if err != nil {
				t.Fatal(err)
			}
			resources := evidence.NewExternalResourceService(evidence.NewRepository(tdb.Pool), id.NewGenerator())
			source, err := resources.Upsert(ctx, sourceURL)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tdb.Pool.Exec(ctx, `
				INSERT INTO external_link_usage (
					external_resource_id, page_id, revision_id, block_id, node_id, link_role)
				VALUES ($1,$2,$3,$4,'0','inline')`,
				source.ID, pageID, revision.ID, blockID); err != nil {
				t.Fatal(err)
			}
			status := int32(200)
			runner := linkhealth.NewRunner(tdb.Pool, proberStub{result: linkhealth.ProbeResult{
				Status: evidence.ExternalResourceStatusRedirect, HTTPStatus: &status, TargetURL: &targetURL,
			}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if _, err := tdb.Pool.Exec(ctx, fault.installSQL); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = tdb.Pool.Exec(context.Background(), fault.removeSQL)
			})
			if _, err := runner.RunOnce(ctx, 20); err == nil {
				t.Fatal("故障注入后 RunOnce 未失败")
			}
			if _, err := tdb.Pool.Exec(ctx, fault.removeSQL); err != nil {
				t.Fatal(err)
			}

			var proposalID, proposalStatus string
			var operationCount int
			if err := tdb.Pool.QueryRow(ctx, `
				SELECT proposal.id::text, proposal.status, count(operation.id)
				FROM proposal
				LEFT JOIN proposal_operation AS operation ON operation.proposal_id=proposal.id
				WHERE proposal.target_id=$1
				GROUP BY proposal.id`, pageID).Scan(&proposalID, &proposalStatus, &operationCount); err != nil {
				t.Fatal(err)
			}
			if proposalStatus != "draft" || operationCount != fault.operationsLeft {
				t.Fatalf("故障中间态 status=%s operations=%d", proposalStatus, operationCount)
			}
			var nextCheckAt time.Time
			if err := tdb.Pool.QueryRow(ctx,
				`SELECT next_check_at FROM external_resource WHERE id=$1`, source.ID).Scan(&nextCheckAt); err != nil {
				t.Fatal(err)
			}
			if nextCheckAt.After(time.Now().Add(5*time.Minute + 10*time.Second)) {
				t.Fatalf("编排失败未安排短恢复: %s", nextCheckAt)
			}
			if _, err := tdb.Pool.Exec(ctx,
				`UPDATE external_resource SET next_check_at=now()-interval '1 second' WHERE id=$1`, source.ID); err != nil {
				t.Fatal(err)
			}
			report, err := runner.RunOnce(ctx, 20)
			if err != nil {
				t.Fatalf("fault recovery RunOnce: %v", err)
			}
			if report.Proposals != 1 {
				t.Fatalf("fault recovery report=%+v", report)
			}
			var recoveredStatus string
			var recoveredOperations, taskCount int
			if err := tdb.Pool.QueryRow(ctx, `
				SELECT proposal.status, count(DISTINCT operation.id), count(DISTINCT task.id)
				FROM proposal
				LEFT JOIN proposal_operation AS operation ON operation.proposal_id=proposal.id
				LEFT JOIN review_task AS task ON task.proposal_id=proposal.id AND task.status='pending'
				WHERE proposal.id=$1
				GROUP BY proposal.id`, proposalID).
				Scan(&recoveredStatus, &recoveredOperations, &taskCount); err != nil {
				t.Fatal(err)
			}
			if recoveredStatus != "in_review" || recoveredOperations != 1 || taskCount != 1 {
				t.Fatalf("恢复后 status=%s operations=%d tasks=%d",
					recoveredStatus, recoveredOperations, taskCount)
			}
		})
	}
}

func TestRunnerMapsUnsafeProbeToBlockedWithBackoff(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	resources := evidence.NewExternalResourceService(evidence.NewRepository(tdb.Pool), id.NewGenerator())
	resource, err := resources.Upsert(ctx, "https://blocked.example/resource")
	if err != nil {
		t.Fatal(err)
	}
	runner := linkhealth.NewRunner(tdb.Pool, proberStub{err: linkhealth.ErrUnsafeURL}, nil)
	report, err := runner.RunOnce(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if report.Blocked != 1 {
		t.Fatalf("report=%+v", report)
	}
	first, err := evidence.NewRepository(tdb.Pool).GetExternalResourceByID(ctx, nil, resource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != evidence.ExternalResourceStatusBlocked || first.ConsecutiveFailures != 1 ||
		first.NextCheckAt.Before(time.Now().Add(59*time.Minute)) {
		t.Fatalf("first=%+v", first)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE external_resource SET next_check_at=now()-interval '1 second' WHERE id=$1`, resource.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.RunOnce(ctx, 1); err != nil {
		t.Fatal(err)
	}
	second, err := evidence.NewRepository(tdb.Pool).GetExternalResourceByID(ctx, nil, resource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.ConsecutiveFailures != 2 || second.NextCheckAt.Before(time.Now().Add(119*time.Minute)) {
		t.Fatalf("second=%+v", second)
	}
}

func TestRunnerSchedulesTransientProbeFailureForShortRetry(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	resources := evidence.NewExternalResourceService(evidence.NewRepository(tdb.Pool), id.NewGenerator())
	resource, err := resources.Upsert(ctx, "https://temporary-failure.example/resource")
	if err != nil {
		t.Fatal(err)
	}
	runner := linkhealth.NewRunner(tdb.Pool, proberStub{err: linkhealth.ErrProbe}, nil)
	started := time.Now()
	report, err := runner.RunOnce(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if report.Broken != 1 {
		t.Fatalf("report=%+v", report)
	}
	updated, err := evidence.NewRepository(tdb.Pool).GetExternalResourceByID(ctx, nil, resource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != evidence.ExternalResourceStatusBroken || updated.ConsecutiveFailures != 1 ||
		updated.NextCheckAt.Before(started.Add(4*time.Minute+50*time.Second)) ||
		updated.NextCheckAt.After(started.Add(5*time.Minute+10*time.Second)) {
		t.Fatalf("瞬时失败未使用短重试: %+v", updated)
	}
	if _, err := tdb.Pool.Exec(ctx,
		`UPDATE external_resource SET next_check_at=now()-interval '1 second' WHERE id=$1`, resource.ID); err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	if _, err := runner.RunOnce(ctx, 1); err != nil {
		t.Fatal(err)
	}
	updated, err = evidence.NewRepository(tdb.Pool).GetExternalResourceByID(ctx, nil, resource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ConsecutiveFailures != 2 ||
		updated.NextCheckAt.Before(started.Add(59*time.Minute+50*time.Second)) ||
		updated.NextCheckAt.After(started.Add(time.Hour+10*time.Second)) {
		t.Fatalf("第二次瞬时失败未回到常规退避: %+v", updated)
	}
}
