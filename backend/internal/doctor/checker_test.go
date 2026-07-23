package doctor_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/doctor"
	"github.com/anby/wiki/backend/testkit"
)

func hasCode(report doctor.Report, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func requireCodes(t *testing.T, report doctor.Report, codes ...string) {
	t.Helper()
	for _, code := range codes {
		if !hasCode(report, code) {
			t.Errorf("报告缺少 issue code %s；issues=%+v", code, report.Issues)
		}
	}
}

func TestCheckerDetectsFocusedPostgresFaultFixtures(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

	pageID := d.MakePage(t, testkit.MainNamespaceID, "doctor-fixture", "Doctor Fixture", testkit.SystemActorID)
	missingPage := d.NewID(t)
	missingEntity := d.NewID(t)
	missingClaim := d.NewID(t)
	missingCitation := d.NewID(t)
	raw := []byte(fmt.Sprintf(`{"type":"document","schema_version":1,"children":[{
		"id":"00000000-0000-7000-8000-00000000d001","type":"paragraph","content":[
		{"type":"page_reference","target_page_id":%q,"display_text":"missing page"},
		{"type":"entity_reference","entity_id":%q,"display_text":"missing entity"},
		{"type":"claim_reference","claim_id":%q,"display_text":"missing claim"},
		{"type":"citation_reference","citation_id":%q}
	]}]}`, missingPage, missingEntity, missingClaim, missingCitation))
	canonical, err := ast.CanonicalizeJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	snapshotID, revisionID := d.NewID(t), d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO content_snapshot(id,schema_version,ast_json,content_hash,size_bytes)
		VALUES($1,1,$2::jsonb,'not-the-real-hash',$3)`,
		snapshotID, raw, len(canonical)+1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx,
		`INSERT INTO revision(id,page_id,content_snapshot_id,actor_id) VALUES($1,$2,$3,$4)`,
		revisionID, pageID, snapshotID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE page SET current_revision_id=$1 WHERE id=$2`, revisionID, pageID); err != nil {
		t.Fatal(err)
	}
	currentRevisionID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO revision(id,page_id,parent_revision_id,content_snapshot_id,actor_id)
		VALUES($1,$2,$3,$4,$5)`,
		currentRevisionID, pageID, revisionID, snapshotID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE page SET current_revision_id=$1 WHERE id=$2`, currentRevisionID, pageID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO projection_state(
			aggregate_type,aggregate_id,projection_type,source_revision_id,status)
		VALUES('page',$1,'search',$2,'ok')`, pageID, revisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO search_document(
			page_id,wiki_id,namespace_key,language,source_revision_id,
			display_title,normalized_title)
		VALUES($1,$2,'main','zh-Hans',$3,'Doctor Fixture','doctor-fixture')`,
		pageID, testkit.DefaultWikiID, revisionID); err != nil {
		t.Fatal(err)
	}

	outboxDead, outboxStuck := d.NewID(t), d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO outbox_event(id,aggregate_type,aggregate_id,event_type,status,claimed_at)
		VALUES($1,'page',$2,'fixture.dead','dead',NULL),
		      ($3,'page',$2,'fixture.stuck','claimed',$4)`,
		outboxDead, pageID, outboxStuck, now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	expiredAt := now.Add(-time.Hour)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO oidc_login_attempt(id,state_hash,browser_secret_hash,nonce,code_verifier,expires_at)
		VALUES($1,decode('01','hex'),decode('02','hex'),'nonce','verifier',$2)`,
		d.NewID(t), expiredAt); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO auth_session(id,token_hash,actor_id,expires_at,created_at)
		VALUES($1,decode('03','hex'),$2,$3,$4)`,
		d.NewID(t), testkit.SystemActorID, expiredAt, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	report, err := doctor.New(d.Pool, doctor.Options{Now: now, ClaimedStuckAfter: 5 * time.Minute}).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	requireCodes(t, report,
		doctor.CodeContentHashMismatch,
		doctor.CodeContentSizeMismatch,
		doctor.CodePageReferenceOrphan,
		doctor.CodeEntityReferenceOrphan,
		doctor.CodeClaimReferenceOrphan,
		doctor.CodeCitationReferenceOrphan,
		doctor.CodeProjectionStateMissing,
		doctor.CodeProjectionSourceStale,
		doctor.CodeSearchSourceStale,
		doctor.CodeOutboxDead,
		doctor.CodeOutboxClaimStuck,
		doctor.CodeLoginAttemptExpired,
		doctor.CodeSessionExpired,
	)
	foundComponentGate := false
	for _, issue := range report.Issues {
		if issue.Code == doctor.CodeProjectionStateMissing &&
			issue.Details["projection_type"] == "component_dependency" {
			foundComponentGate = true
			break
		}
	}
	if !foundComponentGate {
		t.Fatal("doctor report missing component_dependency projection gate")
	}
	if report.Healthy() {
		t.Fatal("包含 error/critical 的报告不应健康")
	}
}

func TestCheckerDetectsCitationChunkVersionMismatch(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ctx := context.Background()

	sourceID := d.NewID(t)
	versionA, versionB := d.NewID(t), d.NewID(t)
	chunkID, citationID := d.NewID(t), d.NewID(t)
	if _, err := d.Pool.Exec(ctx,
		`INSERT INTO source(id,source_type,title) VALUES($1,'webpage','fixture')`, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO source_version(id,source_id,version_hash,fetched_at)
		VALUES($1,$3,'a',now()),($2,$3,'b',now())`, versionA, versionB, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO source_chunk(id,source_version_id,ordinal,text_content,text_hash)
		VALUES($1,$2,0,'evidence','hash')`, chunkID, versionA); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO citation(id,source_version_id,source_chunk_id,created_by)
		VALUES($1,$2,$3,$4)`, citationID, versionB, chunkID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}
	report, err := doctor.New(d.Pool, doctor.Options{}).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	requireCodes(t, report, doctor.CodeCitationChunkVersionMismatch)
}

func TestCleanupExpiredAuthOnlyDeletesExpiredRows(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i, expiry := range []time.Time{now.Add(-time.Minute), now.Add(time.Hour)} {
		stateSum := sha256.Sum256([]byte(fmt.Sprintf("state-%d", i)))
		tokenSum := sha256.Sum256([]byte(fmt.Sprintf("token-%d", i)))
		if _, err := d.Pool.Exec(ctx, `
			INSERT INTO oidc_login_attempt(id,state_hash,browser_secret_hash,nonce,code_verifier,expires_at)
			VALUES($1,$2,$3,'nonce','verifier',$4)`,
			d.NewID(t), stateSum[:], stateSum[:], expiry); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Pool.Exec(ctx, `
			INSERT INTO auth_session(id,token_hash,actor_id,expires_at,created_at)
			VALUES($1,$2,$3,$4,$5)`,
			d.NewID(t), tokenSum[:], testkit.SystemActorID, expiry, now.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := doctor.CleanupExpiredAuth(ctx, d.Pool, now)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ExpiredLoginAttempts != 1 || summary.ExpiredSessions != 1 {
		t.Fatalf("修复汇总=%+v，期望各删除 1 行", summary)
	}
	var loginCount, sessionCount int
	if err := d.Pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM oidc_login_attempt),
		       (SELECT count(*) FROM auth_session)`).Scan(&loginCount, &sessionCount); err != nil {
		t.Fatal(err)
	}
	if loginCount != 1 || sessionCount != 1 {
		t.Fatalf("清理后 login=%d session=%d，期望各保留 1", loginCount, sessionCount)
	}
}

func TestCheckerDetectsDisabledImmutableTrigger(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ctx := context.Background()
	if _, err := d.Pool.Exec(ctx, `ALTER TABLE content_snapshot DISABLE TRIGGER content_snapshot_immutable`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := d.Pool.Exec(context.Background(), `ALTER TABLE content_snapshot ENABLE TRIGGER content_snapshot_immutable`); err != nil {
			t.Errorf("恢复不可变触发器失败: %v", err)
		}
	})
	report, err := doctor.New(d.Pool, doctor.Options{}).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	requireCodes(t, report, doctor.CodeImmutableTriggerMissing)
}
