package auth_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

func TestMachineActorCredentialsE2E(t *testing.T) {
	databaseURL := os.Getenv("AUTH_E2E_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set AUTH_E2E_DATABASE_URL for isolated machine Actor lifecycle tests")
	}
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	schemaName := "machine_actor_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	if _, err := connection.Exec(ctx, "CREATE SCHEMA "+schemaSQL); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := connection.Exec(ctx, "DROP SCHEMA "+schemaSQL+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema, err := os.ReadFile("../../migrations/000001_initial_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	txm, ids := db.NewTxManager(pool), id.NewGenerator()
	service := authdomain.NewService(pool, txm, ids, time.Hour)
	register := func(name string) authdomain.Session {
		t.Helper()
		account, err := service.Register(ctx, authdomain.RegisterInput{
			Username: name, Email: name + "@example.invalid", Password: "machine-test-password", DisplayName: name,
		})
		if err != nil {
			t.Fatal(err)
		}
		return account
	}
	owner, other := register("machine_owner"), register("other_owner")
	input := authdomain.CreateMachineActorParams{
		WikiID: owner.WikiID, CreatedBy: owner.ActorID, ActorType: authdomain.ActorTypeAI,
		DisplayName: "Research Agent", IdempotencyKey: "research",
	}
	machine, err := service.CreateMachineActor(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := service.CreateMachineActor(ctx, input)
	if err != nil || retry.ActorID != machine.ActorID {
		t.Fatalf("creation retry lost identity: %v", err)
	}
	input.DisplayName = "Changed"
	if _, err := service.CreateMachineActor(ctx, input); !errors.Is(err, authdomain.ErrMachineActorConflict) {
		t.Fatalf("creation mismatch: %v", err)
	}
	input.CreatedBy = other.ActorID
	if _, err := service.CreateMachineActor(ctx, input); !errors.Is(err, authdomain.ErrMachineActorForbidden) {
		t.Fatalf("cross-Wiki creation: %v", err)
	}
	otherMachines, err := service.ListCLIMachineActors(ctx, other.ActorID)
	if err != nil || len(otherMachines) != 0 {
		t.Fatalf("account leaked machine Actors: %v", err)
	}
	codeFor := func(by, actorID uuid.UUID) authdomain.CLIAuthorizationCode {
		t.Helper()
		code, err := service.CreateCLIAuthorizationCode(ctx, authdomain.CreateCLIAuthCodeParams{
			IssuedBy: by, ActorID: actorID, Name: "Agent", TokenTTLDays: 30,
		})
		if err != nil {
			t.Fatal(err)
		}
		return code
	}
	if _, err := service.CreateCLIAuthorizationCode(ctx, authdomain.CreateCLIAuthCodeParams{
		IssuedBy: other.ActorID, ActorID: machine.ActorID, Name: "Stolen", TokenTTLDays: 30,
	}); !errors.Is(err, authdomain.ErrMachineActorForbidden) {
		t.Fatalf("cross-account binding: %v", err)
	}
	code := codeFor(owner.ActorID, machine.ActorID)
	// Competing exchanges must commit exactly one token.
	var exchanges [2]authdomain.CLIExchangeResult
	var exchangeErrors [2]error
	var wg sync.WaitGroup
	for index := range exchanges {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exchanges[index], exchangeErrors[index] = service.ExchangeCLIAuthorizationCode(ctx, code.Code)
		}()
	}
	wg.Wait()
	var credential authdomain.CLIExchangeResult
	successes := 0
	for index, err := range exchangeErrors {
		if err == nil {
			successes++
			credential = exchanges[index]
		} else if !errors.Is(err, authdomain.ErrInvalidCLIAuthorizationCode) {
			t.Fatal(err)
		}
	}
	if successes != 1 || credential.ActorID != machine.ActorID || credential.ActorType != authdomain.ActorTypeAI {
		t.Fatal("exchange lost machine identity or one-use guarantee")
	}
	principal, err := service.ResolveCLIToken(ctx, credential.Token, owner.WikiID)
	if err != nil || principal.ActorID != machine.ActorID {
		t.Fatalf("machine token authentication: %v", err)
	}
	if _, err := service.ResolveCLIToken(ctx, credential.Token, other.WikiID); !errors.Is(err, authdomain.ErrUnauthenticated) {
		t.Fatalf("cross-Wiki machine authentication: %v", err)
	}
	authorization := governance.NewAuthorizationService(pool)
	if allowed, err := authorization.HasWikiAccess(ctx, machine.ActorID, owner.WikiID); err != nil || !allowed {
		t.Fatalf("machine own-Wiki read: %v", err)
	}
	if err := authorization.CheckReadPage(ctx, machine.ActorID, other.WikiID, uuid.New()); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("machine cross-Wiki read: %v", err)
	}
	for _, action := range []string{governance.ActionCreate, governance.ActionEdit, governance.ActionReview, governance.ActionApply, governance.ActionManage} {
		if err := authorization.Check(ctx, machine.ActorID, owner.WikiID, action, nil); !errors.Is(err, governance.ErrPermissionDenied) {
			t.Fatalf("machine direct action %s: %v", action, err)
		}
	}
	list, err := service.ListCLITokens(ctx, owner.ActorID)
	if err != nil || len(list) != 1 || list[0].ActorID != machine.ActorID {
		t.Fatalf("owner credential management: %v", err)
	}
	if err := service.RevokeCLIToken(ctx, other.ActorID, credential.TokenInfo.ID); !errors.Is(err, authdomain.ErrCLITokenNotFound) {
		t.Fatalf("cross-account revocation: %v", err)
	}
	if err := service.DeleteCLITokenRecord(ctx, owner.ActorID, credential.TokenInfo.ID); !errors.Is(err, authdomain.ErrActiveCLITokenDeletion) {
		t.Fatalf("active credential deletion: %v", err)
	}
	trust := governance.NewAITrustService(governance.NewRepository(pool), authorization, txm, ids)
	profiles, err := trust.List(ctx, other.WikiID, other.ActorID)
	if err != nil || len(profiles) != 0 {
		t.Fatalf("cross-Wiki trust catalog: %v", err)
	}
	if _, err := trust.Update(ctx, governance.UpdateAITrustProfileParams{
		WikiID: other.WikiID, ActorID: machine.ActorID, UpdatedBy: other.ActorID,
		TrustLevel: governance.AITrustTrusted,
	}); err == nil {
		t.Fatal("cross-Wiki trust update succeeded")
	}
	for _, level := range []string{governance.AITrustUntrusted, governance.AITrustAssisted, governance.AITrustTrusted} {
		for _, sample := range []int{0, 100} {
			if _, err := trust.Update(ctx, governance.UpdateAITrustProfileParams{
				WikiID: owner.WikiID, ActorID: machine.ActorID, UpdatedBy: owner.ActorID,
				TrustLevel: level, RequiredSamplePercent: sample,
			}); err != nil {
				t.Fatal(err)
			}
			decision := &governance.RiskDecision{Level: governance.RiskLow, AutoApprove: true}
			if err := txm.InTx(ctx, func(tx pgx.Tx) error {
				return trust.ApplyPolicy(ctx, tx, &governance.Proposal{
					ID: uuid.New(), WikiID: owner.WikiID, CreatedBy: machine.ActorID,
				}, decision)
			}); err != nil {
				t.Fatal(err)
			}
			if decision.AutoApprove || decision.AITrustLevel != level ||
				decision.SampledForReview != (level == governance.AITrustUntrusted || sample == 100) {
				t.Fatalf("trust=%s sample=%d decision=%+v", level, sample, decision)
			}
		}
	}
	pending := codeFor(owner.ActorID, machine.ActorID)
	for _, status := range []string{authdomain.ActorStatusDisabled, authdomain.ActorStatusActive} {
		if _, err := service.SetMachineActorStatus(ctx, authdomain.SetMachineActorStatusParams{
			WikiID: owner.WikiID, ActorID: machine.ActorID, UpdatedBy: owner.ActorID, Status: status,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ResolveCLIToken(ctx, credential.Token, uuid.Nil); !errors.Is(err, authdomain.ErrUnauthenticated) {
			t.Fatalf("old token revived after %s: %v", status, err)
		}
		if _, err := service.ExchangeCLIAuthorizationCode(ctx, pending.Code); !errors.Is(err, authdomain.ErrInvalidCLIAuthorizationCode) {
			t.Fatalf("pending code revived after %s: %v", status, err)
		}
	}
	freshCode := codeFor(owner.ActorID, machine.ActorID)
	fresh, err := service.ExchangeCLIAuthorizationCode(ctx, freshCode.Code)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate already committed owner/account/role changes in this isolated
	// fixture. Runtime authentication must reread them, with no owner fallback.
	for _, mutation := range []struct{ disable, restore string }{
		{`UPDATE local_account SET status='disabled' WHERE actor_id=$1`, `UPDATE local_account SET status='active' WHERE actor_id=$1`},
		{`UPDATE actor SET status='disabled' WHERE id=$1`, `UPDATE actor SET status='active' WHERE id=$1`},
		{`DELETE FROM actor_role WHERE actor_id=$1`, `INSERT INTO actor_role(actor_id,wiki_id,role_id) SELECT $1,$2,id FROM role WHERE role_key='admin'`},
	} {
		if _, err := pool.Exec(ctx, mutation.disable, owner.ActorID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ResolveCLIToken(ctx, fresh.Token, uuid.Nil); !errors.Is(err, authdomain.ErrUnauthenticated) {
			t.Fatalf("machine survived owner revocation: %v", err)
		}
		args := []any{owner.ActorID}
		if strings.Contains(mutation.restore, "$2") {
			args = append(args, owner.WikiID)
		}
		if _, err := pool.Exec(ctx, mutation.restore, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.RevokeCLIToken(ctx, machine.ActorID, fresh.TokenInfo.ID); err != nil {
		t.Fatalf("CLI self revocation: %v", err)
	}
	if _, err := service.DeleteInactiveCLITokenRecords(ctx, owner.ActorID); err != nil {
		t.Fatal(err)
	}
	adminService := governance.NewUserAdminService(governance.NewRepository(pool), authorization, txm, ids)
	if _, err := adminService.AddWikiMember(ctx, owner.WikiID, owner.ActorID, "other_owner", []string{"admin"}); err != nil {
		t.Fatal(err)
	}
	pendingDeletion := codeFor(owner.ActorID, machine.ActorID)
	if _, err := adminService.DeleteUser(ctx, owner.WikiID, other.ActorID, owner.ActorID); err != nil {
		t.Fatalf("account deletion with machine Actors: %v", err)
	}
	retired, err := service.ListMachineActors(ctx, other.ActorID, owner.WikiID)
	if err != nil || len(retired) != 1 || retired[0].OwnerActorID != uuid.Nil || retired[0].Status != authdomain.ActorStatusDisabled {
		t.Fatalf("retired machine identity lost: %v", err)
	}
	if _, err := service.ExchangeCLIAuthorizationCode(ctx, pendingDeletion.Code); !errors.Is(err, authdomain.ErrInvalidCLIAuthorizationCode) {
		t.Fatalf("deleted owner code survived: %v", err)
	}
	if _, err := service.SetMachineActorStatus(ctx, authdomain.SetMachineActorStatusParams{
		WikiID: owner.WikiID, ActorID: machine.ActorID, UpdatedBy: other.ActorID, Status: authdomain.ActorStatusActive,
	}); !errors.Is(err, authdomain.ErrMachineActorForbidden) {
		t.Fatalf("retired machine was re-enabled: %v", err)
	}
}
