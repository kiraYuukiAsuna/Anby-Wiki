package evidence_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

// TestUpdateExternalResourceStatusDoesNotCreateRevision 验证 INV-12：资源健康状态
// 独立流转，不修改页面 current_revision_id，也不产生新 Revision/Outbox。
func TestUpdateExternalResourceStatusDoesNotCreateRevision(t *testing.T) {
	tdb, svc, _ := setup(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "alice")
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "inv-12", "INV-12", actorID)
	pageSvc := page.NewService(page.NewRepository(tdb.Pool), db.NewTxManager(tdb.Pool), id.NewGenerator())
	revision, err := pageSvc.Publish(ctx, page.PublishParams{
		PageID:  pageID,
		ActorID: actorID,
		AST:     json.RawMessage(`{"type":"document","schema_version":1,"children":[]}`),
	})
	if err != nil {
		t.Fatalf("发布页面失败: %v", err)
	}
	resource, err := svc.UpsertExternalResource(ctx, "https://example.com/original")
	if err != nil {
		t.Fatalf("创建资源失败: %v", err)
	}

	var revisionsBefore, outboxBefore int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM revision WHERE page_id = $1`, pageID).Scan(&revisionsBefore); err != nil {
		t.Fatalf("统计 Revision 失败: %v", err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event`).Scan(&outboxBefore); err != nil {
		t.Fatalf("统计 Outbox 失败: %v", err)
	}
	httpStatus := int32(200)
	contentHash := " content-hash "
	canonicalURL := "HTTPS://Example.COM:443/canonical/?utm_source=check#fragment"
	updated, err := svc.UpdateExternalResourceStatus(ctx, resource.ID, evidence.UpdateExternalResourceStatusParams{
		Status:       evidence.ExternalResourceStatusOK,
		HTTPStatus:   &httpStatus,
		ContentHash:  &contentHash,
		CanonicalURL: &canonicalURL,
	})
	if err != nil {
		t.Fatalf("UpdateExternalResourceStatus 失败: %v", err)
	}
	if updated.Status != evidence.ExternalResourceStatusOK || updated.HTTPStatus == nil || *updated.HTTPStatus != 200 {
		t.Fatalf("状态更新结果异常: %+v", updated)
	}
	if updated.CanonicalURL == nil || *updated.CanonicalURL != "https://example.com/canonical" {
		t.Fatalf("canonical_url = %v，期望规范化结果", updated.CanonicalURL)
	}
	if updated.ContentHash == nil || *updated.ContentHash != "content-hash" || updated.LastCheckedAt == nil || updated.LastSuccessAt == nil {
		t.Fatalf("hash/check 时间字段异常: %+v", updated)
	}

	var currentRevisionID string
	var revisionsAfter, outboxAfter int
	if err := tdb.Pool.QueryRow(ctx, `SELECT current_revision_id::text FROM page WHERE id = $1`, pageID).Scan(&currentRevisionID); err != nil {
		t.Fatalf("读取 current_revision_id 失败: %v", err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM revision WHERE page_id = $1`, pageID).Scan(&revisionsAfter); err != nil {
		t.Fatalf("统计更新后 Revision 失败: %v", err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event`).Scan(&outboxAfter); err != nil {
		t.Fatalf("统计更新后 Outbox 失败: %v", err)
	}
	if currentRevisionID != revision.ID.String() || revisionsAfter != revisionsBefore || outboxAfter != outboxBefore {
		t.Fatalf("INV-12 失败: current=%s revisions=%d→%d outbox=%d→%d",
			currentRevisionID, revisionsBefore, revisionsAfter, outboxBefore, outboxAfter)
	}
}

func TestUpdateExternalResourceStatusValidation(t *testing.T) {
	tdb, svc, _ := setup(t)
	ctx := context.Background()
	resource, err := svc.UpsertExternalResource(ctx, "https://example.com/status")
	if err != nil {
		t.Fatalf("创建资源失败: %v", err)
	}
	if _, err := svc.UpdateExternalResourceStatus(ctx, resource.ID, evidence.UpdateExternalResourceStatusParams{Status: "healthy"}); !errors.Is(err, evidence.ErrInvalidSourceInput) {
		t.Fatalf("非法 status err = %v，期望 ErrInvalidSourceInput", err)
	}
	badHTTP := int32(99)
	if _, err := svc.UpdateExternalResourceStatus(ctx, resource.ID, evidence.UpdateExternalResourceStatusParams{
		Status: evidence.ExternalResourceStatusBroken, HTTPStatus: &badHTTP,
	}); !errors.Is(err, evidence.ErrInvalidSourceInput) {
		t.Fatalf("非法 http_status err = %v，期望 ErrInvalidSourceInput", err)
	}
	if _, err := svc.UpdateExternalResourceStatus(ctx, tdb.NewID(t), evidence.UpdateExternalResourceStatusParams{
		Status: evidence.ExternalResourceStatusBroken,
	}); !errors.Is(err, evidence.ErrExternalResourceNotFound) {
		t.Fatalf("资源不存在 err = %v，期望 ErrExternalResourceNotFound", err)
	}
}

func TestClaimDueExternalResourcesUsesLease(t *testing.T) {
	tdb, svc, _ := setup(t)
	ctx := context.Background()
	resource, err := svc.UpsertExternalResource(ctx, "https://example.com/lease")
	if err != nil {
		t.Fatal(err)
	}
	system := evidence.NewExternalResourceService(evidence.NewRepository(tdb.Pool), id.NewGenerator())
	first, err := system.ClaimDue(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].ID != resource.ID {
		t.Fatalf("first=%+v", first)
	}
	if first[0].LeaseToken == nil {
		t.Fatal("领取结果缺少 lease token")
	}
	second, err := system.ClaimDue(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("租约内重复领取: %+v", second)
	}
}

func TestCompleteExternalResourceCheckRejectsStaleLease(t *testing.T) {
	tdb, svc, _ := setup(t)
	ctx := context.Background()
	resource, err := svc.UpsertExternalResource(ctx, "https://example.com/stale-lease")
	if err != nil {
		t.Fatal(err)
	}
	system := evidence.NewExternalResourceService(evidence.NewRepository(tdb.Pool), id.NewGenerator())
	first, err := system.ClaimDue(ctx, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	if _, err := tdb.Pool.Exec(ctx,
		`UPDATE external_resource SET next_check_at=now()-interval '1 second' WHERE id=$1`, resource.ID); err != nil {
		t.Fatal(err)
	}
	second, err := system.ClaimDue(ctx, 1)
	if err != nil || len(second) != 1 {
		t.Fatalf("second claim=%+v err=%v", second, err)
	}
	if first[0].LeaseToken == nil || second[0].LeaseToken == nil ||
		*first[0].LeaseToken == *second[0].LeaseToken {
		t.Fatalf("租约未轮换: first=%v second=%v", first[0].LeaseToken, second[0].LeaseToken)
	}

	status := int32(410)
	if _, err := system.CompleteCheck(ctx, first[0], evidence.ExternalResourceCheckResult{
		Status: evidence.ExternalResourceStatusBroken, HTTPStatus: &status,
	}); !errors.Is(err, evidence.ErrExternalResourceLeaseLost) {
		t.Fatalf("陈旧租约完成 err=%v，期望 ErrExternalResourceLeaseLost", err)
	}
	current, err := evidence.NewRepository(tdb.Pool).GetExternalResourceByID(ctx, nil, resource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != evidence.ExternalResourceStatusUnknown || current.HTTPStatus != nil ||
		current.LeaseToken == nil || *current.LeaseToken != *second[0].LeaseToken {
		t.Fatalf("陈旧租约覆盖了新租约状态: %+v", current)
	}
}
