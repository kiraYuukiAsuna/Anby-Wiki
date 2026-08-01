package knowledge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
)

// appendKnowledgeAudit writes an immutable audit record inside the same
// transaction as the authoritative Knowledge mutation.
func (s *Service) appendKnowledgeAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	eventType, aggregateType string,
	aggregateID uuid.UUID,
	payloadValue any,
) error {
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		return fmt.Errorf("knowledge: encode %s audit payload: %w", eventType, err)
	}
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	return s.pages.InsertAuditEvent(ctx, tx, &page.AuditEvent{
		ID:            auditID,
		ActorID:       actorID,
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       payload,
	})
}

// appendKnowledgeMutation couples the immutable audit record with a durable
// projection invalidation event. Both either commit with the authoritative
// write or roll back with it.
func (s *Service) appendKnowledgeMutation(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	auditEventType, outboxEventType, aggregateType string,
	aggregateID uuid.UUID,
	payloadValue any,
) error {
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		return fmt.Errorf("knowledge: encode %s event payload: %w", auditEventType, err)
	}
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	outboxID, err := s.ids.New()
	if err != nil {
		return err
	}
	if err := s.pages.InsertAuditEvent(ctx, tx, &page.AuditEvent{
		ID:            auditID,
		ActorID:       actorID,
		EventType:     auditEventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       payload,
	}); err != nil {
		return err
	}
	return s.pages.InsertOutboxEvent(ctx, tx, &page.OutboxEvent{
		ID:            outboxID,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     outboxEventType,
		Payload:       payload,
	})
}
