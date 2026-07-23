package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestWriteAPI_AuthorizationAndPageProtection(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	svc := page.NewService(page.NewRepository(tdb.Pool), db.NewTxManager(tdb.Pool), id.NewGenerator())
	auth := governance.NewAuthorizationService(tdb.Pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(logger, Deps{Service: "wiki-api", Version: "test"},
		NewWriteAPI(svc, testkit.DefaultWikiID).WithAuthorization(auth), nil, nil, nil, nil)

	editor := tdb.MakeActor(t, "human", "editor")
	headers := map[string]string{"X-Actor-ID": editor.String()}
	status, body := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Role Guard"}, headers)
	if status != http.StatusForbidden {
		t.Fatalf("unassigned create status=%d body=%v", status, body)
	}
	assertErrorShape(t, body, httpx.CodeForbidden)

	assignAPIRole(t, tdb, editor, "editor")
	status, body = doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Role Guard"}, headers)
	if status != http.StatusCreated {
		t.Fatalf("editor create status=%d body=%v", status, body)
	}
	pageID := uuid.MustParse(body["id"].(string))
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO page_protection
		(id,page_id,action_type,required_role_id,created_by)
		SELECT $1,$2,'edit',id,$3 FROM role WHERE role_key='admin'`,
		tdb.NewID(t), pageID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}

	status, body = doJSON(t, router, http.MethodPost, "/api/v1/pages/"+pageID.String()+"/revisions",
		map[string]any{"ast": validASTBody(t, "protected")}, headers)
	if status != http.StatusForbidden {
		t.Fatalf("protected edit status=%d body=%v", status, body)
	}
	assertErrorShape(t, body, httpx.CodeForbidden)

	status, body = doJSON(t, router, http.MethodPost, "/api/v1/pages/"+pageID.String()+"/revisions",
		map[string]any{"ast": validASTBody(t, "system recovery")},
		map[string]string{"X-Actor-ID": testkit.SystemActorID.String()})
	if status != http.StatusCreated {
		t.Fatalf("system recovery status=%d body=%v", status, body)
	}
}

func assignAPIRole(t *testing.T, tdb *testkit.DB, actorID uuid.UUID, roleKey string) {
	t.Helper()
	if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO actor_role (actor_id,role_id,wiki_id)
		SELECT $1,id,$2 FROM role WHERE role_key=$3`, actorID, testkit.DefaultWikiID, roleKey); err != nil {
		t.Fatal(err)
	}
}
