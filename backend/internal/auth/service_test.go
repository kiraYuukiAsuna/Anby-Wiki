package auth_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func setup(t *testing.T) (*testkit.DB, *auth.Service) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	return tdb, auth.NewService(tdb.Pool, db.NewTxManager(tdb.Pool), id.NewGenerator(), time.Hour)
}

func TestLoginAttemptIsBrowserBoundAndSingleUse(t *testing.T) {
	_, service := setup(t)
	ctx := context.Background()
	attempt, err := service.BeginLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConsumeLogin(ctx, attempt.State, "wrong-browser"); !errors.Is(err, auth.ErrInvalidLogin) {
		t.Fatalf("wrong browser err=%v", err)
	}
	consumed, err := service.ConsumeLogin(ctx, attempt.State, attempt.BrowserSecret)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.Nonce != attempt.Nonce || consumed.CodeVerifier != attempt.CodeVerifier {
		t.Fatal("consumed login values changed")
	}
	if _, err := service.ConsumeLogin(ctx, attempt.State, attempt.BrowserSecret); !errors.Is(err, auth.ErrInvalidLogin) {
		t.Fatalf("replay err=%v", err)
	}
}

func TestExternalIdentityMapsToStableActorAndSessionRevokes(t *testing.T) {
	tdb, service := setup(t)
	ctx := context.Background()
	identity := auth.Identity{Issuer: "https://id.example.com", Subject: "subject-1", DisplayName: "Alice"}
	first, err := service.EstablishSession(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.EstablishSession(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if first.ActorID != second.ActorID {
		t.Fatalf("same subject mapped to %s and %s", first.ActorID, second.ActorID)
	}
	var storedHash []byte
	if err := tdb.Pool.QueryRow(ctx, `SELECT token_hash FROM auth_session WHERE actor_id=$1 ORDER BY created_at LIMIT 1`,
		first.ActorID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	expectedHash := sha256.Sum256([]byte(first.Token))
	if string(storedHash) != string(expectedHash[:]) || string(storedHash) == first.Token {
		t.Fatal("session must store only the opaque token hash")
	}
	principal, err := service.ResolveSession(ctx, first.Token)
	if err != nil || principal.ActorID != first.ActorID || principal.Method != "session" {
		t.Fatalf("resolve principal=%+v err=%v", principal, err)
	}
	if err := service.RevokeSession(ctx, first.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveSession(ctx, first.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("revoked session err=%v", err)
	}
}

func TestActorAndRoleChangesTakeEffectOnNextCheck(t *testing.T) {
	tdb, service := setup(t)
	ctx := context.Background()
	session, err := service.EstablishSession(ctx, auth.Identity{
		Issuer: "https://id.example.com", Subject: "subject-2", DisplayName: "Bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	authorization := governance.NewAuthorizationService(tdb.Pool)
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO actor_role (actor_id,role_id,wiki_id)
		SELECT $1,id,$2 FROM role WHERE role_key='editor'`,
		session.ActorID, testkit.DefaultWikiID); err != nil {
		t.Fatal(err)
	}
	if err := authorization.Check(ctx, session.ActorID, testkit.DefaultWikiID, governance.ActionEdit, nil); err != nil {
		t.Fatalf("assigned role denied: %v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `DELETE FROM actor_role WHERE actor_id=$1`, session.ActorID); err != nil {
		t.Fatal(err)
	}
	if err := authorization.Check(ctx, session.ActorID, testkit.DefaultWikiID, governance.ActionEdit, nil); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("revoked role should deny immediately, err=%v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE actor SET status='disabled' WHERE id=$1`, session.ActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveSession(ctx, session.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("disabled actor session err=%v", err)
	}
	if _, err := service.EstablishSession(ctx, auth.Identity{
		Issuer: "https://id.example.com", Subject: "subject-2", DisplayName: "Bob",
	}); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("disabled actor must not receive new session, err=%v", err)
	}
}

func TestDevelopmentHeaderMustBeExplicitlyEnabled(t *testing.T) {
	tdb, service := setup(t)
	actorID := tdb.MakeActor(t, "human", "developer")
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set(auth.DevActorHeader, actorID.String())

	disabled := auth.NewAuthenticator(service, "session", false)
	if _, ok, err := disabled.Authenticate(request.Context(), request); err != nil || ok {
		t.Fatalf("disabled header ok=%v err=%v", ok, err)
	}
	enabled := auth.NewAuthenticator(service, "session", true)
	principal, ok, err := enabled.Authenticate(request.Context(), request)
	if err != nil || !ok || principal.ActorID != actorID || principal.Method != "development_header" {
		t.Fatalf("enabled principal=%+v ok=%v err=%v", principal, ok, err)
	}
}
