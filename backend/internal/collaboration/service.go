// Package collaboration owns WorkingDocument persistence and recovery.
package collaboration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

const (
	MaxUpdateBytes   = 1 << 20
	MaxSnapshotBytes = 16 << 20
)

var (
	ErrDocumentNotFound     = errors.New("collaboration: working document not found")
	ErrDocumentInactive     = errors.New("collaboration: working document is not active")
	ErrInvalidActor         = errors.New("collaboration: actor cannot edit working documents")
	ErrInvalidUpdate        = errors.New("collaboration: invalid update")
	ErrIdempotencyConflict  = errors.New("collaboration: idempotency key reused with different update")
	ErrInvalidSnapshot      = errors.New("collaboration: invalid snapshot")
	ErrDocumentPageMismatch = errors.New("collaboration: working document does not belong to page")
	ErrSequenceMismatch     = errors.New("collaboration: working document sequence mismatch")
)

type Document struct {
	ID             uuid.UUID
	PageID         uuid.UUID
	BaseRevisionID *uuid.UUID
	SchemaVersion  int
	CRDTCodec      string
	LatestSequence int64
	Status         string
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Update struct {
	DocumentID     uuid.UUID
	Sequence       int64
	ActorID        uuid.UUID
	ClientID       uuid.UUID
	ClientUpdateID uuid.UUID
	Bytes          []byte
	Hash           [sha256.Size]byte
	CreatedAt      time.Time
}

type Snapshot struct {
	ID           uuid.UUID
	DocumentID   uuid.UUID
	UpToSequence int64
	State        []byte
	Hash         [sha256.Size]byte
	CreatedBy    uuid.UUID
	CreatedAt    time.Time
}

type Recovery struct {
	Document Document
	Snapshot *Snapshot
	Updates  []Update
}

type Service struct {
	pool db.Querier
	txm  *db.TxManager
	ids  *id.Generator
}

func NewService(pool db.Querier, txm *db.TxManager, ids *id.Generator) *Service {
	return &Service{pool: pool, txm: txm, ids: ids}
}

// Open returns the active document for a page or creates one based on the
// page's current Revision. The page lock serializes concurrent first opens.
func (s *Service) Open(ctx context.Context, pageID, actorID uuid.UUID) (Document, error) {
	var result Document
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := requireEditor(ctx, tx, actorID); err != nil {
			return err
		}
		var baseRevisionID *uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT current_revision_id FROM page
			WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, pageID).Scan(&baseRevisionID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrDocumentNotFound
			}
			return err
		}
		doc, err := getActiveByPage(ctx, tx, pageID)
		if err == nil {
			result = doc
			return nil
		}
		if !errors.Is(err, ErrDocumentNotFound) {
			return err
		}
		documentID, err := s.ids.New()
		if err != nil {
			return err
		}
		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO working_document
			(id,page_id,base_revision_id,created_by) VALUES ($1,$2,$3,$4)`,
			documentID, pageID, baseRevisionID, actorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_event
			(id,actor_id,event_type,aggregate_type,aggregate_id,payload_json)
			VALUES ($1,$2,'working_document.created','working_document',$3,
				jsonb_build_object('page_id',$4::uuid,'base_revision_id',$5::uuid))`,
			auditID, actorID, documentID, pageID, baseRevisionID); err != nil {
			return err
		}
		result, err = getByID(ctx, tx, documentID)
		return err
	})
	if err != nil {
		return Document{}, fmt.Errorf("collaboration: open working document: %w", err)
	}
	return result, nil
}

// Append stores one opaque Yjs update and allocates the next server sequence.
func (s *Service) Append(
	ctx context.Context,
	documentID, actorID, clientID, clientUpdateID uuid.UUID,
	value []byte,
) (Update, error) {
	if len(value) == 0 || len(value) > MaxUpdateBytes ||
		clientID == uuid.Nil || clientUpdateID == uuid.Nil {
		return Update{}, ErrInvalidUpdate
	}
	hash := sha256.Sum256(value)
	var result Update
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := requireEditor(ctx, tx, actorID); err != nil {
			return err
		}
		var status string
		var latest int64
		if err := tx.QueryRow(ctx, `SELECT wd.status,wd.latest_sequence
			FROM working_document wd JOIN page p ON p.id=wd.page_id
			WHERE wd.id=$1 AND p.deleted_at IS NULL FOR UPDATE OF wd`,
			documentID).Scan(&status, &latest); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrDocumentNotFound
			}
			return err
		}
		if status != "active" {
			return ErrDocumentInactive
		}
		existing, err := getUpdateByClientKey(ctx, tx, documentID, clientID, clientUpdateID)
		if err == nil {
			if existing.Hash != hash {
				return ErrIdempotencyConflict
			}
			result = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		result = Update{
			DocumentID: documentID, Sequence: latest + 1, ActorID: actorID,
			ClientID: clientID, ClientUpdateID: clientUpdateID, Bytes: bytes.Clone(value), Hash: hash,
		}
		if err := tx.QueryRow(ctx, `INSERT INTO working_document_update
			(document_id,sequence,actor_id,client_id,client_update_id,update_bytes,update_hash)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`,
			documentID, result.Sequence, actorID, clientID, clientUpdateID, value, hash[:],
		).Scan(&result.CreatedAt); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE working_document
			SET latest_sequence=$2,updated_at=now() WHERE id=$1`, documentID, result.Sequence)
		return err
	})
	if err != nil {
		return Update{}, fmt.Errorf("collaboration: append update: %w", err)
	}
	return result, nil
}

// AppendCASInTx appends an opaque Yjs delta only when the durable cursor still
// matches expectedSequence. Governance uses this after validating the
// corresponding Current/Merged AST pair in the same transaction.
func (s *Service) AppendCASInTx(
	ctx context.Context,
	tx pgx.Tx,
	documentID, pageID, actorID, clientID, clientUpdateID uuid.UUID,
	expectedSequence int64,
	value []byte,
) (Update, error) {
	if expectedSequence < 0 || len(value) == 0 || len(value) > MaxUpdateBytes ||
		clientID == uuid.Nil || clientUpdateID == uuid.Nil {
		return Update{}, ErrInvalidUpdate
	}
	if err := requireEditor(ctx, tx, actorID); err != nil {
		return Update{}, err
	}
	hash := sha256.Sum256(value)
	var documentPageID uuid.UUID
	var status string
	var latest int64
	if err := tx.QueryRow(ctx, `SELECT wd.page_id,wd.status,wd.latest_sequence
		FROM working_document wd JOIN page p ON p.id=wd.page_id
		WHERE wd.id=$1 AND p.deleted_at IS NULL FOR UPDATE OF wd`,
		documentID).Scan(&documentPageID, &status, &latest); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Update{}, ErrDocumentNotFound
		}
		return Update{}, err
	}
	if documentPageID != pageID {
		return Update{}, ErrDocumentPageMismatch
	}
	if status != "active" {
		return Update{}, ErrDocumentInactive
	}
	existing, err := getUpdateByClientKey(ctx, tx, documentID, clientID, clientUpdateID)
	if err == nil {
		if existing.Hash != hash {
			return Update{}, ErrIdempotencyConflict
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Update{}, err
	}
	if latest != expectedSequence {
		return Update{}, fmt.Errorf("%w: expected=%d actual=%d",
			ErrSequenceMismatch, expectedSequence, latest)
	}
	result := Update{
		DocumentID: documentID, Sequence: latest + 1, ActorID: actorID,
		ClientID: clientID, ClientUpdateID: clientUpdateID,
		Bytes: bytes.Clone(value), Hash: hash,
	}
	if err := tx.QueryRow(ctx, `INSERT INTO working_document_update
		(document_id,sequence,actor_id,client_id,client_update_id,update_bytes,update_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`,
		result.DocumentID, result.Sequence, result.ActorID, result.ClientID,
		result.ClientUpdateID, result.Bytes, result.Hash[:]).Scan(&result.CreatedAt); err != nil {
		return Update{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE working_document
		SET latest_sequence=$2,updated_at=now() WHERE id=$1`, documentID, result.Sequence); err != nil {
		return Update{}, err
	}
	return result, nil
}

// SaveSnapshot persists a complete Yjs state. When compact is true, updates
// covered by the snapshot are removed in the same transaction.
func (s *Service) SaveSnapshot(
	ctx context.Context,
	documentID, actorID uuid.UUID,
	upToSequence int64,
	state []byte,
	compact bool,
) (Snapshot, error) {
	if len(state) == 0 || len(state) > MaxSnapshotBytes || upToSequence < 0 {
		return Snapshot{}, ErrInvalidSnapshot
	}
	hash := sha256.Sum256(state)
	var result Snapshot
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		shouldAudit := false
		if err := requireEditor(ctx, tx, actorID); err != nil {
			return err
		}
		var status string
		var latest int64
		if err := tx.QueryRow(ctx, `SELECT status,latest_sequence FROM working_document
			WHERE id=$1 FOR UPDATE`, documentID).Scan(&status, &latest); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrDocumentNotFound
			}
			return err
		}
		if status != "active" {
			return ErrDocumentInactive
		}
		if upToSequence > latest {
			return ErrInvalidSnapshot
		}
		existing, err := getSnapshotAt(ctx, tx, documentID, upToSequence)
		switch {
		case err == nil:
			if existing.Hash != hash {
				return ErrIdempotencyConflict
			}
			result = existing
		case errors.Is(err, pgx.ErrNoRows):
			snapshotID, err := s.ids.New()
			if err != nil {
				return err
			}
			result = Snapshot{
				ID: snapshotID, DocumentID: documentID, UpToSequence: upToSequence,
				State: bytes.Clone(state), Hash: hash, CreatedBy: actorID,
			}
			if err := tx.QueryRow(ctx, `INSERT INTO working_document_snapshot
				(id,document_id,up_to_sequence,state_bytes,state_hash,created_by)
				VALUES ($1,$2,$3,$4,$5,$6) RETURNING created_at`,
				snapshotID, documentID, upToSequence, state, hash[:], actorID,
			).Scan(&result.CreatedAt); err != nil {
				return err
			}
			shouldAudit = true
		default:
			return err
		}
		if compact {
			tag, err := tx.Exec(ctx, `DELETE FROM working_document_update
				WHERE document_id=$1 AND sequence<=$2`, documentID, upToSequence)
			if err != nil {
				return err
			}
			shouldAudit = shouldAudit || tag.RowsAffected() > 0
		}
		if !shouldAudit {
			return nil
		}
		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO audit_event
			(id,actor_id,event_type,aggregate_type,aggregate_id,payload_json)
			VALUES ($1,$2,'working_document.snapshotted','working_document',$3,
				jsonb_build_object('up_to_sequence',$4::bigint,'compacted',$5::boolean))`,
			auditID, actorID, documentID, upToSequence, compact)
		return err
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("collaboration: save snapshot: %w", err)
	}
	return result, nil
}

// LoadRecovery returns the latest snapshot plus all subsequent updates.
func (s *Service) LoadRecovery(ctx context.Context, documentID uuid.UUID) (Recovery, error) {
	return s.LoadSince(ctx, documentID, 0)
}

// LoadSince returns updates after a reconnect cursor. If the cursor predates
// compacted updates, the latest snapshot is returned before its subsequent
// updates.
func (s *Service) LoadSince(
	ctx context.Context,
	documentID uuid.UUID,
	lastSequence int64,
) (Recovery, error) {
	if lastSequence < 0 {
		return Recovery{}, ErrInvalidUpdate
	}
	document, err := getByID(ctx, s.pool, documentID)
	if err != nil {
		return Recovery{}, fmt.Errorf("collaboration: load recovery: %w", err)
	}
	recovery := Recovery{Document: document}
	after := int64(0)
	snapshot, err := getLatestSnapshot(ctx, s.pool, documentID)
	if err == nil {
		if lastSequence < snapshot.UpToSequence {
			recovery.Snapshot = &snapshot
			after = snapshot.UpToSequence
		} else {
			after = lastSequence
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Recovery{}, fmt.Errorf("collaboration: load recovery snapshot: %w", err)
	} else {
		after = lastSequence
	}
	rows, err := s.pool.Query(ctx, `SELECT document_id,sequence,actor_id,client_id,
		client_update_id,update_bytes,update_hash,created_at
		FROM working_document_update WHERE document_id=$1 AND sequence>$2 ORDER BY sequence`,
		documentID, after)
	if err != nil {
		return Recovery{}, fmt.Errorf("collaboration: load recovery updates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		update, err := scanUpdate(rows)
		if err != nil {
			return Recovery{}, err
		}
		recovery.Updates = append(recovery.Updates, update)
	}
	if err := rows.Err(); err != nil {
		return Recovery{}, err
	}
	return recovery, nil
}

func requireEditor(ctx context.Context, q db.Querier, actorID uuid.UUID) error {
	var actorType, status string
	if err := q.QueryRow(ctx, `SELECT actor_type,status FROM actor WHERE id=$1`, actorID).
		Scan(&actorType, &status); err != nil {
		return ErrInvalidActor
	}
	if status != "active" || (actorType != "human" && actorType != "system") {
		return ErrInvalidActor
	}
	return nil
}

func getActiveByPage(ctx context.Context, q db.Querier, pageID uuid.UUID) (Document, error) {
	document, err := scanDocument(q.QueryRow(
		ctx, documentSelect+` WHERE page_id=$1 AND status='active'`, pageID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrDocumentNotFound
	}
	return document, err
}

func getByID(ctx context.Context, q db.Querier, documentID uuid.UUID) (Document, error) {
	document, err := scanDocument(q.QueryRow(ctx, documentSelect+` WHERE id=$1`, documentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrDocumentNotFound
	}
	return document, err
}

const documentSelect = `SELECT id,page_id,base_revision_id,schema_version,crdt_codec,
	latest_sequence,status,created_by,created_at,updated_at FROM working_document`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDocument(row rowScanner) (Document, error) {
	var document Document
	err := row.Scan(
		&document.ID, &document.PageID, &document.BaseRevisionID, &document.SchemaVersion,
		&document.CRDTCodec, &document.LatestSequence, &document.Status,
		&document.CreatedBy, &document.CreatedAt, &document.UpdatedAt,
	)
	return document, err
}

func getUpdateByClientKey(
	ctx context.Context,
	q db.Querier,
	documentID, clientID, clientUpdateID uuid.UUID,
) (Update, error) {
	return scanUpdate(q.QueryRow(ctx, `SELECT document_id,sequence,actor_id,client_id,
		client_update_id,update_bytes,update_hash,created_at
		FROM working_document_update
		WHERE document_id=$1 AND client_id=$2 AND client_update_id=$3`,
		documentID, clientID, clientUpdateID))
}

func scanUpdate(row rowScanner) (Update, error) {
	var update Update
	var hash []byte
	err := row.Scan(
		&update.DocumentID, &update.Sequence, &update.ActorID, &update.ClientID,
		&update.ClientUpdateID, &update.Bytes, &hash, &update.CreatedAt,
	)
	if err == nil {
		copy(update.Hash[:], hash)
	}
	return update, err
}

func getSnapshotAt(
	ctx context.Context,
	q db.Querier,
	documentID uuid.UUID,
	upToSequence int64,
) (Snapshot, error) {
	return scanSnapshot(q.QueryRow(ctx, snapshotSelect+
		` WHERE document_id=$1 AND up_to_sequence=$2`, documentID, upToSequence))
}

func getLatestSnapshot(ctx context.Context, q db.Querier, documentID uuid.UUID) (Snapshot, error) {
	return scanSnapshot(q.QueryRow(ctx, snapshotSelect+
		` WHERE document_id=$1 ORDER BY up_to_sequence DESC LIMIT 1`, documentID))
}

const snapshotSelect = `SELECT id,document_id,up_to_sequence,state_bytes,state_hash,
	created_by,created_at FROM working_document_snapshot`

func scanSnapshot(row rowScanner) (Snapshot, error) {
	var snapshot Snapshot
	var hash []byte
	err := row.Scan(
		&snapshot.ID, &snapshot.DocumentID, &snapshot.UpToSequence, &snapshot.State,
		&hash, &snapshot.CreatedBy, &snapshot.CreatedAt,
	)
	if err == nil {
		copy(snapshot.Hash[:], hash)
	}
	return snapshot, err
}
