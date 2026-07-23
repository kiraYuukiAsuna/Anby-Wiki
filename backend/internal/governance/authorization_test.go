package governance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/testkit"
)

func TestAuthorization_RolesProtectionAndAICannotBypass(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	editor := tdb.MakeActor(t, "human", "editor")
	reviewer := tdb.MakeActor(t, "human", "reviewer")
	ai := tdb.MakeActor(t, "ai", "agent")
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "protected", "Protected", editor)
	auth := governance.NewAuthorizationService(tdb.Pool)

	if err := auth.Check(ctx, editor, testkit.DefaultWikiID, governance.ActionEdit, &pageID); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("unassigned editor err=%v", err)
	}
	assignRole(t, tdb, editor, "editor")
	if err := auth.Check(ctx, editor, testkit.DefaultWikiID, governance.ActionEdit, &pageID); err != nil {
		t.Fatal(err)
	}
	createProtectionID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO page_protection
		(id,namespace_id,normalized_title,action_type,required_role_id,created_by)
		SELECT $1,$2,'reserved title','create',id,$3 FROM role WHERE role_key='admin'`,
		createProtectionID, testkit.MainNamespaceID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}
	if err := auth.CheckCreate(ctx, editor, testkit.DefaultWikiID, testkit.MainNamespaceID, "reserved title"); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("create protection err=%v", err)
	}
	if err := auth.CheckCreate(ctx, editor, testkit.DefaultWikiID, testkit.MainNamespaceID, "ordinary title"); err != nil {
		t.Fatalf("unprotected create err=%v", err)
	}
	assignRole(t, tdb, ai, "applier")
	if err := auth.Check(ctx, ai, testkit.DefaultWikiID, governance.ActionApply, &pageID); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("AI role bypass err=%v", err)
	}

	assignRole(t, tdb, reviewer, "reviewer")
	if err := auth.Check(ctx, reviewer, testkit.DefaultWikiID, governance.ActionReview, &pageID); err != nil {
		t.Fatal(err)
	}
	protectionID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO page_protection
		(id,page_id,action_type,required_role_id,created_by)
		SELECT $1,$2,'review',id,$3 FROM role WHERE role_key='admin'`, protectionID, pageID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}
	if err := auth.Check(ctx, reviewer, testkit.DefaultWikiID, governance.ActionReview, &pageID); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("protection err=%v", err)
	}
	assignRole(t, tdb, reviewer, "admin")
	if err := auth.Check(ctx, reviewer, testkit.DefaultWikiID, governance.ActionReview, &pageID); err != nil {
		t.Fatal(err)
	}
	if err := auth.Check(ctx, testkit.SystemActorID, testkit.DefaultWikiID, governance.ActionBatchRollback, &pageID); err != nil {
		t.Fatalf("system recovery path err=%v", err)
	}
}

func assignRole(t *testing.T, tdb *testkit.DB, actorID uuid.UUID, roleKey string) {
	t.Helper()
	if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO actor_role (actor_id,role_id,wiki_id)
		SELECT $1,id,$2 FROM role WHERE role_key=$3`, actorID, testkit.DefaultWikiID, roleKey); err != nil {
		t.Fatal(err)
	}
}
