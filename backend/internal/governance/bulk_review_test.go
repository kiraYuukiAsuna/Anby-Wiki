package governance_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestBulkReview_SamplingPartialRejectAndFixedWaveApply(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	reviewer := tdb.MakeActor(t, "human", "bulk-reviewer")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pages := page.NewService(pageRepo, txm, ids)
	repo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(repo, txm, ids)
	conflicts := governance.NewConflictService(repo, pages, nil, txm, ids)
	apply := governance.NewApplyService(repo, pages, governance.NewPagePatchEngine(), nil, conflicts, txm, ids)
	bulk := governance.NewBulkReviewService(repo, apply, txm, ids)

	proposalIDs := make([]uuid.UUID, 0, 4)
	for index := 0; index < 4; index++ {
		created, err := pages.CreatePage(ctx, page.CreatePageParams{
			WikiID: testkit.DefaultWikiID, NamespaceID: testkit.MainNamespaceID,
			Title: fmt.Sprintf("Bulk Review %d", index), ActorID: reviewer,
		})
		if err != nil {
			t.Fatal(err)
		}
		blockID := fmt.Sprintf("00000000-0000-7000-8000-%012d", 800+index)
		baseDoc := documentWithParagraphs(t, paragraph(t, blockID, "Base"))
		revision, err := pages.Publish(ctx, page.PublishParams{
			PageID: created.ID, ActorID: reviewer, AST: mustJSON(t, baseDoc),
		})
		if err != nil {
			t.Fatal(err)
		}
		hash, _ := governance.BlockHash(baseDoc.Children[0])
		proposal := makeApprovedPageProposal(t, proposals, reviewer, created.ID, revision.ID,
			blockID, hash, fmt.Sprintf("bulk-%d", index), "Applied")
		if _, err := tdb.Pool.Exec(ctx,
			`UPDATE proposal SET status='in_review',risk_level='medium' WHERE id=$1`, proposal.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := tdb.Pool.Exec(ctx,
			`INSERT INTO review_task (id,proposal_id,status) VALUES ($1,$2,'pending')`,
			tdb.NewID(t), proposal.ID); err != nil {
			t.Fatal(err)
		}
		proposalIDs = append(proposalIDs, proposal.ID)
	}

	batch, err := bulk.Create(ctx, governance.CreateBulkReviewParams{
		ProposalIDs: proposalIDs, CreatedBy: reviewer, SamplePercent: 50, WaveSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.SamplingMode != governance.BulkSamplingSampled || len(batch.Items) != 4 {
		t.Fatalf("batch=%+v", batch)
	}
	selected := selectedItems(batch.Items)
	if len(selected) != 2 {
		t.Fatalf("selected=%d want=2", len(selected))
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE bulk_review_batch_item SET wave=wave+1
		WHERE batch_id=$1 AND proposal_id=$2`, batch.ID, batch.Items[0].ProposalID); err == nil {
		t.Fatal("数据库应拒绝修改冻结的 wave")
	}
	if _, err := bulk.Finalize(ctx, batch.ID, reviewer); !errors.Is(err, governance.ErrBulkReviewIncomplete) {
		t.Fatalf("未完成抽样应拒绝定稿: %v", err)
	}
	if _, err := bulk.Decide(ctx, batch.ID, selected[0].ProposalID, reviewer, false, "证据不足"); err != nil {
		t.Fatal(err)
	}
	if _, err := bulk.Decide(ctx, batch.ID, selected[1].ProposalID, reviewer, true, "抽样通过"); err != nil {
		t.Fatal(err)
	}
	batch, err = bulk.Finalize(ctx, batch.ID, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != governance.BulkReviewReady {
		t.Fatalf("status=%s", batch.Status)
	}

	originalWaves := map[uuid.UUID]int{}
	for _, item := range batch.Items {
		originalWaves[item.ProposalID] = item.Wave
	}
	if _, err := bulk.Pause(ctx, batch.ID, reviewer); err != nil {
		t.Fatal(err)
	}
	if _, err := bulk.ApplyNextWave(ctx, batch.ID, reviewer); !errors.Is(err, governance.ErrBulkReviewPaused) {
		t.Fatalf("暂停后 Apply err=%v", err)
	}
	batch, err = bulk.Resume(ctx, batch.ID, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range batch.Items {
		if item.Wave != originalWaves[item.ProposalID] {
			t.Fatalf("wave 漂移 proposal=%s", item.ProposalID)
		}
	}

	for batch.Status != governance.BulkReviewCompleted {
		result, err := bulk.ApplyNextWave(ctx, batch.ID, reviewer)
		if err != nil {
			t.Fatal(err)
		}
		batch, err = bulk.Get(ctx, result.BatchID, reviewer)
		if err != nil {
			t.Fatal(err)
		}
	}
	var changeBatches int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM change_batch
		WHERE proposal_id = ANY($1)`, proposalIDs).Scan(&changeBatches); err != nil {
		t.Fatal(err)
	}
	if changeBatches != 3 {
		t.Fatalf("change_batch=%d want=3（拒绝项不 Apply）", changeBatches)
	}
	events, err := bulk.Audit(ctx, batch.ID, reviewer)
	if err != nil || len(events) < 8 {
		t.Fatalf("audit events=%d err=%v", len(events), err)
	}
}

func TestBulkReview_HighRiskForcesFullReview(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	reviewer := tdb.MakeActor(t, "human", "risk-reviewer")
	repo := governance.NewRepository(tdb.Pool)
	txm := db.NewTxManager(tdb.Pool)
	bulk := governance.NewBulkReviewService(repo, nil, txm, id.NewGenerator())
	proposalIDs := make([]uuid.UUID, 2)
	for i := range proposalIDs {
		proposalIDs[i] = tdb.NewID(t)
		risk := governance.RiskMedium
		if i == 1 {
			risk = governance.RiskHigh
		}
		if _, err := tdb.Pool.Exec(ctx, `INSERT INTO proposal
			(id,target_type,status,risk_level,created_by,idempotency_key)
			VALUES ($1,'page','in_review',$2,$3,$4)`,
			proposalIDs[i], risk, reviewer, fmt.Sprintf("full-%d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err := tdb.Pool.Exec(ctx,
			`INSERT INTO review_task (id,proposal_id,status) VALUES ($1,$2,'pending')`,
			tdb.NewID(t), proposalIDs[i]); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := bulk.Create(ctx, governance.CreateBulkReviewParams{
		ProposalIDs: proposalIDs, CreatedBy: reviewer, SamplePercent: 10, WaveSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.SamplingMode != governance.BulkSamplingFull ||
		batch.ForceFullReason == nil || *batch.ForceFullReason != "high_or_critical_risk" {
		t.Fatalf("batch=%+v", batch)
	}
	for _, item := range batch.Items {
		if !item.SelectedForReview {
			t.Fatalf("高风险批次存在未全量审核项: %+v", item)
		}
	}
}

func TestBulkReview_AuthorizationCoversEveryProposalAndRevocation(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	repo := governance.NewRepository(tdb.Pool)
	auth := governance.NewAuthorizationService(tdb.Pool)
	apply := governance.NewApplyService(repo, nil, nil, nil, nil, txm, ids).WithAuthorization(auth)
	bulk := governance.NewBulkReviewService(repo, apply, txm, ids).WithAuthorization(auth)

	otherWikiID := tdb.NewID(t)
	otherNamespaceID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO wiki_site
		(id,site_key,name,default_language,settings_json) VALUES ($1,'other','Other Wiki','zh-Hans','{}')`,
		otherWikiID); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO namespace
		(id,wiki_id,namespace_key,canonical_name,display_name,is_content)
		VALUES ($1,$2,'main','Main','Main',true)`, otherNamespaceID, otherWikiID); err != nil {
		t.Fatal(err)
	}
	defaultPageID := tdb.MakePage(t, testkit.MainNamespaceID, "bulk-auth-default", "Bulk Auth Default", testkit.SystemActorID)
	otherPageID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO page
		(id,wiki_id,namespace_id,normalized_title,display_title,language,content_model,status,created_by)
		VALUES ($1,$2,$3,'bulk-auth-other','Bulk Auth Other','zh-Hans','block-v1','active',$4)`,
		otherPageID, otherWikiID, otherNamespaceID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}

	proposalIDs := []uuid.UUID{tdb.NewID(t), tdb.NewID(t)}
	pageIDs := []uuid.UUID{defaultPageID, otherPageID}
	for i := range proposalIDs {
		if _, err := tdb.Pool.Exec(ctx, `INSERT INTO proposal
			(id,target_type,target_id,status,risk_level,created_by,idempotency_key)
			VALUES ($1,'page',$2,'in_review','medium',$3,$4)`,
			proposalIDs[i], pageIDs[i], testkit.SystemActorID, fmt.Sprintf("bulk-auth-%d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err := tdb.Pool.Exec(ctx, `INSERT INTO review_task
			(id,proposal_id,status) VALUES ($1,$2,'pending')`, tdb.NewID(t), proposalIDs[i]); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := bulk.Create(ctx, governance.CreateBulkReviewParams{
		ProposalIDs: proposalIDs, CreatedBy: testkit.SystemActorID, SamplePercent: 100, WaveSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	noRole := tdb.MakeActor(t, "human", "bulk-no-role")
	if _, err := bulk.Get(ctx, batch.ID, noRole); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("无角色读取 err=%v", err)
	}
	if _, err := bulk.Audit(ctx, batch.ID, noRole); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("无角色读取审计 err=%v", err)
	}

	reviewer := tdb.MakeActor(t, "human", "bulk-cross-wiki-reviewer")
	assignRoleForWiki(t, tdb, reviewer, testkit.DefaultWikiID, "reviewer")
	if _, err := bulk.Get(ctx, batch.ID, reviewer); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("跨 Wiki 读取 err=%v", err)
	}
	assignRoleForWiki(t, tdb, reviewer, otherWikiID, "reviewer")
	if _, err := bulk.Get(ctx, batch.ID, reviewer); err != nil {
		t.Fatalf("双 Wiki reviewer 读取 err=%v", err)
	}
	if _, err := bulk.Audit(ctx, batch.ID, reviewer); err != nil {
		t.Fatalf("双 Wiki reviewer 读取审计 err=%v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `DELETE FROM actor_role WHERE actor_id=$1 AND wiki_id=$2`,
		reviewer, otherWikiID); err != nil {
		t.Fatal(err)
	}
	if _, err := bulk.Get(ctx, batch.ID, reviewer); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("撤销 reviewer 后读取 err=%v", err)
	}
	if _, err := bulk.Audit(ctx, batch.ID, reviewer); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("撤销 reviewer 后读取审计 err=%v", err)
	}

	if _, err := tdb.Pool.Exec(ctx, `UPDATE bulk_review_batch SET status='ready' WHERE id=$1`, batch.ID); err != nil {
		t.Fatal(err)
	}
	applier := tdb.MakeActor(t, "human", "bulk-cross-wiki-applier")
	assignRoleForWiki(t, tdb, applier, testkit.DefaultWikiID, "applier")
	if _, err := bulk.Pause(ctx, batch.ID, applier); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("跨 Wiki pause err=%v", err)
	}
	assignRoleForWiki(t, tdb, applier, otherWikiID, "applier")
	if _, err := bulk.Pause(ctx, batch.ID, applier); err != nil {
		t.Fatalf("双 Wiki applier pause err=%v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `DELETE FROM actor_role WHERE actor_id=$1 AND wiki_id=$2`,
		applier, otherWikiID); err != nil {
		t.Fatal(err)
	}
	if _, err := bulk.Resume(ctx, batch.ID, applier); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("撤销 applier 后 resume err=%v", err)
	}
	assignRoleForWiki(t, tdb, applier, otherWikiID, "applier")
	if _, err := bulk.Resume(ctx, batch.ID, applier); err != nil {
		t.Fatalf("双 Wiki applier resume err=%v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `DELETE FROM actor_role WHERE actor_id=$1 AND wiki_id=$2`,
		applier, otherWikiID); err != nil {
		t.Fatal(err)
	}
	if _, err := bulk.ApplyNextWave(ctx, batch.ID, applier); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("撤销 applier 后 Apply err=%v", err)
	}
	var status string
	var waveStarted int
	if err := tdb.Pool.QueryRow(ctx, `SELECT status FROM bulk_review_batch WHERE id=$1`, batch.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM bulk_review_audit_event
		WHERE batch_id=$1 AND event_type='bulk_review.wave_started'`, batch.ID).Scan(&waveStarted); err != nil {
		t.Fatal(err)
	}
	if status != governance.BulkReviewReady || waveStarted != 0 {
		t.Fatalf("越权 Apply 产生副作用 status=%s wave_started=%d", status, waveStarted)
	}
}

func assignRoleForWiki(t *testing.T, tdb *testkit.DB, actorID, wikiID uuid.UUID, roleKey string) {
	t.Helper()
	if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO actor_role (actor_id,role_id,wiki_id)
		SELECT $1,id,$2 FROM role WHERE role_key=$3`, actorID, wikiID, roleKey); err != nil {
		t.Fatal(err)
	}
}

func selectedItems(items []governance.BulkReviewItem) []governance.BulkReviewItem {
	var selected []governance.BulkReviewItem
	for _, item := range items {
		if item.SelectedForReview {
			selected = append(selected, item)
		}
	}
	return selected
}
