package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

type stubEntityMergeRepairService struct {
	calls    int
	mergeID  uuid.UUID
	actorID  uuid.UUID
	err      error
	failOnce bool
}

func (s *stubEntityMergeRepairService) CreateEntityMergeRepairProposals(
	_ context.Context,
	mergeID, actorID uuid.UUID,
) (*governance.EntityMergeRepairResult, error) {
	s.calls++
	s.mergeID = mergeID
	s.actorID = actorID
	if s.err != nil && (!s.failOnce || s.calls == 1) {
		return nil, s.err
	}
	return &governance.EntityMergeRepairResult{MergeID: mergeID}, nil
}

func TestEntityMergeRepairHandler_ConsumerRetriesFailure(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	mergeID, actorID, eventID := uuid.New(), uuid.New(), uuid.New()
	payload, err := json.Marshal(entityMergedPayload{MergeID: mergeID, ActorID: actorID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO outbox_event
		(id,aggregate_type,aggregate_id,event_type,payload_json)
		VALUES ($1,'entity_merge',$2,$3,$4)`,
		eventID, mergeID, knowledge.OutboxEventEntityMerged, payload); err != nil {
		t.Fatal(err)
	}
	service := &stubEntityMergeRepairService{
		err: errors.New("temporary database failure"), failOnce: true,
	}
	consumer := projection.New(tdb.Pool, projection.Config{
		PollInterval: 5 * time.Millisecond, BackoffBase: time.Millisecond,
		LeaseDuration: time.Second, MaxAttempts: 3, ShutdownTimeout: time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	consumer.Register(knowledge.OutboxEventEntityMerged, newEntityMergeRepairHandler(service))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = consumer.Run(ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	var status string
	var attempts int
	for time.Now().Before(deadline) {
		if err := tdb.Pool.QueryRow(context.Background(),
			`SELECT status,attempt_count FROM outbox_event WHERE id=$1`, eventID).
			Scan(&status, &attempts); err != nil {
			cancel()
			<-done
			t.Fatal(err)
		}
		if status == "done" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if status != "done" || attempts != 2 || service.calls != 2 {
		t.Fatalf("status=%s attempts=%d service calls=%d", status, attempts, service.calls)
	}
}

func TestEntityMergeRepairHandler_DuplicateDeliveryCallsIdempotentService(t *testing.T) {
	mergeID, actorID := uuid.New(), uuid.New()
	payload, err := json.Marshal(entityMergedPayload{MergeID: mergeID, ActorID: actorID})
	if err != nil {
		t.Fatal(err)
	}
	event := projection.Event{
		AggregateType: "entity_merge", AggregateID: mergeID,
		EventType: knowledge.OutboxEventEntityMerged, Payload: payload,
	}
	service := &stubEntityMergeRepairService{}
	handler := newEntityMergeRepairHandler(service)
	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if service.calls != 2 || service.mergeID != mergeID || service.actorID != actorID {
		t.Fatalf("service calls=%d merge=%s actor=%s", service.calls, service.mergeID, service.actorID)
	}
}

func TestEntityMergeRepairHandler_ReturnsFailureForOutboxRetry(t *testing.T) {
	retryable := errors.New("temporary database failure")
	mergeID, actorID := uuid.New(), uuid.New()
	payload, err := json.Marshal(entityMergedPayload{MergeID: mergeID, ActorID: actorID})
	if err != nil {
		t.Fatal(err)
	}
	service := &stubEntityMergeRepairService{err: retryable}
	handler := newEntityMergeRepairHandler(service)
	err = handler.Handle(context.Background(), projection.Event{
		AggregateType: "entity_merge", AggregateID: mergeID,
		EventType: knowledge.OutboxEventEntityMerged, Payload: payload,
	})
	if !errors.Is(err, retryable) || service.calls != 1 {
		t.Fatalf("Handle err=%v calls=%d", err, service.calls)
	}
}

func TestEntityMergeRepairHandler_RejectsMismatchedPayload(t *testing.T) {
	payload, err := json.Marshal(entityMergedPayload{MergeID: uuid.New(), ActorID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	service := &stubEntityMergeRepairService{}
	handler := newEntityMergeRepairHandler(service)
	err = handler.Handle(context.Background(), projection.Event{
		AggregateType: "entity_merge", AggregateID: uuid.New(),
		EventType: knowledge.OutboxEventEntityMerged, Payload: payload,
	})
	if err == nil || service.calls != 0 {
		t.Fatalf("Handle err=%v calls=%d", err, service.calls)
	}
}

func TestEntityMergeRepairHandler_RolledBackBeforeConsumptionSucceeds(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "worker-merge-repair-rollback")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	knowledgeService := knowledge.NewService(
		knowledge.NewRepository(tdb.Pool), pageRepo, txm, ids,
	)
	source, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software",
		CanonicalKey: "worker-repair-rollback-source", ActorID: actorID,
		Labels: []knowledge.LabelInput{{
			Language: "en", Label: "Rollback Source", IsPrimary: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software",
		CanonicalKey: "worker-repair-rollback-target", ActorID: actorID,
		Labels: []knowledge.LabelInput{{
			Language: "en", Label: "Rollback Target", IsPrimary: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	merge, err := knowledgeService.MergeEntity(ctx, knowledge.MergeEntityParams{
		SourceEntityID: source.ID, TargetEntityID: target.ID, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := knowledgeService.RollbackEntityMerge(ctx, merge.Merge.ID, actorID); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(entityMergedPayload{MergeID: merge.Merge.ID, ActorID: actorID})
	if err != nil {
		t.Fatal(err)
	}
	handler := newEntityMergeRepairHandler(governance.NewService(
		governance.NewRepository(tdb.Pool), txm, ids,
	))
	if err := handler.Handle(ctx, projection.Event{
		AggregateType: "entity_merge", AggregateID: merge.Merge.ID,
		EventType: knowledge.OutboxEventEntityMerged, Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}
}
