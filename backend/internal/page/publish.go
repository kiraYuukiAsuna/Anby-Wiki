// Revision 发布事务（M1-T05，设计 §12.2/§16、ADR-0003）。
//
// 一次发布在单个事务内完成：
//  1. SELECT page FOR UPDATE（行锁串行化同一页面的并发发布）；
//  2. 断言 expected_revision_id == page.current_revision_id（乐观锁，首发布要求双 nil）；
//  3. INSERT content_snapshot（canonical AST + 服务端计算的 hash/size）；
//  4. INSERT revision（parent = 旧 current）；
//  5. UPDATE page.current_revision_id（触发器校验同 page，INV-01）；
//  6. INSERT audit_event（revision.published）；
//  7. INSERT outbox_event（page.revision_published，pending，驱动投影重建）。
//
// 任一步失败整体回滚：不留孤立 snapshot/revision，current 指针不移动。
package page

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
)

// beforePublishCommit 发布事务提交前的钩子，仅用于测试注入「写库之后、提交之前」
// 的失败以验证回滚原子性（集成测试在包内赋值，用例结束复位）。生产代码恒为 nil。
var beforePublishCommit func(ctx context.Context, tx pgx.Tx) error

// PublishParams 发布 Revision 的入参。
// ExpectedRevisionID 为发布基线：首发布必须为 nil，后续发布必须等于页面当前 Revision。
// AST 为 Typed Block AST 原始 JSON；content hash/size 由服务端计算，客户端提供的值不被信任。
type PublishParams struct {
	PageID             uuid.UUID
	ActorID            uuid.UUID
	ExpectedRevisionID *uuid.UUID
	AST                json.RawMessage
	Summary            string
	IsMinor            bool
	ChangeBatchID      *uuid.UUID
}

// Publish 原子发布一个 Revision：校验 Actor 与 AST 后，在单事务内写入
// ContentSnapshot/Revision/current 指针/AuditEvent/OutboxEvent。
// 基线不一致返回 ErrStaleRevision；页面不存在或已软删除返回 ErrPageNotFound；
// AST 非法返回 ErrInvalidAST。
func (s *Service) Publish(ctx context.Context, params PublishParams) (revision *Revision, resultErr error) {
	started := time.Now()
	defer func() {
		if s.publishObserver != nil {
			s.publishObserver.ObservePublish(time.Since(started), resultErr)
		}
	}()
	if err := s.checkWriteActor(ctx, params.ActorID); err != nil {
		return nil, err
	}
	snap, err := buildSnapshot(params.AST)
	if err != nil {
		return nil, err
	}
	return s.runPublishTx(ctx, publishDraft{
		pageID:         params.PageID,
		actorID:        params.ActorID,
		expected:       params.ExpectedRevisionID,
		snap:           snap,
		summary:        params.Summary,
		isMinor:        params.IsMinor,
		changeBatchID:  params.ChangeBatchID,
		auditEventType: EventTypeRevisionPublished,
		auditPayload:   func(rev *Revision) json.RawMessage { return publishPayload(rev, false) },
		outboxPayload:  func(rev *Revision) json.RawMessage { return publishPayload(rev, true) },
	})
}

// publishDraft 发布事务核心（runPublishTx）的入参，Publish 与 Rollback 共用。
type publishDraft struct {
	pageID  uuid.UUID
	actorID uuid.UUID
	// expected 为发布基线（乐观锁）；useCurrentExpected 时忽略 expected，
	// 以锁内读到的页面 current 为基线——回滚永远追加在最新头上，本身不会过期。
	expected           *uuid.UUID
	useCurrentExpected bool
	snap               *ContentSnapshot
	// dedupSnapshot 先按 (content_hash, schema_version) 查重复用已有快照行
	//（回滚产生的重复内容不重复存储，M1-T07 决策）。
	dedupSnapshot  bool
	summary        string
	isMinor        bool
	changeBatchID  *uuid.UUID
	auditEventType string
	auditPayload   func(rev *Revision) json.RawMessage
	outboxPayload  func(rev *Revision) json.RawMessage
}

// runPublishTx 发布事务核心：行锁串行化 → 乐观锁断言 → 写快照（可去重）→
// 写 Revision → 移动 current 指针 → 写审计与 Outbox 事件，单事务原子提交。
func (s *Service) runPublishTx(ctx context.Context, d publishDraft) (*Revision, error) {
	var rev *Revision
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		rev, err = s.publishWithinTx(ctx, tx, d)
		if err != nil {
			return err
		}
		if beforePublishCommit != nil {
			return beforePublishCommit(ctx, tx)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rev, nil
}

// PublishInTx 供 Governance Apply 在其外层事务中组合发布；仍由 Page Service
// 完成 Actor、AST、乐观锁、Revision/Audit/Outbox 全部领域约束。
func (s *Service) PublishInTx(ctx context.Context, tx pgx.Tx, params PublishParams) (revision *Revision, resultErr error) {
	started := time.Now()
	defer func() {
		if s.publishObserver != nil {
			s.publishObserver.ObservePublish(time.Since(started), resultErr)
		}
	}()
	if err := s.repo.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
		return nil, err
	}
	snap, err := buildSnapshot(params.AST)
	if err != nil {
		return nil, err
	}
	return s.publishWithinTx(ctx, tx, publishDraft{
		pageID: params.PageID, actorID: params.ActorID, expected: params.ExpectedRevisionID,
		snap: snap, summary: params.Summary, isMinor: params.IsMinor,
		changeBatchID: params.ChangeBatchID, auditEventType: EventTypeRevisionPublished,
		auditPayload:  func(rev *Revision) json.RawMessage { return publishPayload(rev, false) },
		outboxPayload: func(rev *Revision) json.RawMessage { return publishPayload(rev, true) },
	})
}

func (s *Service) publishWithinTx(ctx context.Context, tx pgx.Tx, d publishDraft) (*Revision, error) {
	p, err := s.repo.GetPageByIDForUpdate(ctx, tx, d.pageID)
	if err != nil {
		return nil, err
	}
	if p.DeletedAt != nil {
		return nil, fmt.Errorf("%w: id=%s 已删除", ErrPageNotFound, d.pageID)
	}
	expected := d.expected
	if d.useCurrentExpected {
		expected = p.CurrentRevisionID
	}
	if !expectedMatchesCurrent(expected, p.CurrentRevisionID) {
		return nil, fmt.Errorf("%w: page=%s expected=%v current=%v",
			ErrStaleRevision, d.pageID, expected, p.CurrentRevisionID)
	}

	snap := d.snap
	if d.dedupSnapshot {
		existing, err := s.repo.GetSnapshotByHash(ctx, tx, snap.ContentHash, snap.SchemaVersion)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			snap = existing
		} else if err := s.insertNewSnapshot(ctx, tx, snap); err != nil {
			return nil, err
		}
	} else if err := s.insertNewSnapshot(ctx, tx, snap); err != nil {
		return nil, err
	}

	revID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	rev := &Revision{
		ID: revID, PageID: d.pageID, ParentRevisionID: p.CurrentRevisionID,
		ContentSnapshotID: snap.ID, ActorID: d.actorID, ChangeBatchID: d.changeBatchID,
		Summary: d.summary, IsMinor: d.isMinor, Visibility: VisibilityPublic,
		ContentHash: snap.ContentHash, SchemaVersion: snap.SchemaVersion,
	}
	if err := s.repo.InsertRevision(ctx, tx, rev); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateCurrentRevision(ctx, tx, d.pageID, rev.ID); err != nil {
		return nil, err
	}

	auditID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	if err := s.repo.InsertAuditEvent(ctx, tx, &AuditEvent{
		ID: auditID, ActorID: d.actorID, EventType: d.auditEventType,
		AggregateType: AggregateTypePage, AggregateID: d.pageID,
		ChangeBatchID: d.changeBatchID, Payload: d.auditPayload(rev),
	}); err != nil {
		return nil, err
	}

	outboxID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	if err := s.repo.InsertOutboxEvent(ctx, tx, &OutboxEvent{
		ID: outboxID, AggregateType: AggregateTypePage, AggregateID: d.pageID,
		EventType: OutboxEventRevisionPublished, Payload: d.outboxPayload(rev),
	}); err != nil {
		return nil, err
	}
	return rev, nil
}

// insertNewSnapshot 分配 ID 并插入一条新快照（发布事务内部步骤）。
func (s *Service) insertNewSnapshot(ctx context.Context, tx pgx.Tx, snap *ContentSnapshot) error {
	snapID, err := s.ids.New()
	if err != nil {
		return err
	}
	snap.ID = snapID
	return s.repo.InsertContentSnapshot(ctx, tx, snap)
}

// buildSnapshot 校验 AST 并构造快照：ast.ValidateJSON 按 v1 Schema 校验
// （Schema 已约束 type='document' 且 schema_version=1，此处显式断言以给出稳定错误），
// canonical 序列化后计算 SHA-256 content hash 与 size_bytes。
func buildSnapshot(raw json.RawMessage) (*ContentSnapshot, error) {
	var head struct {
		Type          string `json:"type"`
		SchemaVersion int    `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("%w: 非合法 JSON 对象: %v", ErrInvalidAST, err)
	}
	if head.Type != "document" {
		return nil, fmt.Errorf("%w: type=%q, 期望 document", ErrInvalidAST, head.Type)
	}
	if head.SchemaVersion != ast.SchemaVersion {
		return nil, fmt.Errorf("%w: schema_version=%d, 期望 %d", ErrInvalidAST, head.SchemaVersion, ast.SchemaVersion)
	}
	if err := ast.ValidateJSON(raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAST, err)
	}
	canonical, err := ast.CanonicalizeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAST, err)
	}
	sum := sha256.Sum256(canonical)
	return &ContentSnapshot{
		SchemaVersion: ast.SchemaVersion,
		AST:           canonical,
		ContentHash:   hex.EncodeToString(sum[:]),
		SizeBytes:     len(canonical),
	}, nil
}

// expectedMatchesCurrent 乐观锁断言：首发布要求 expected 与 current 同时为 nil，
// 后续发布要求两者相等；其余组合均为过期基线。
func expectedMatchesCurrent(expected, current *uuid.UUID) bool {
	if expected == nil || current == nil {
		return expected == nil && current == nil
	}
	return *expected == *current
}

// publishPayload 构造审计/Outbox 事件载荷；withSchemaVersion 时附加 schema_version（Outbox 用）。
func publishPayload(rev *Revision, withSchemaVersion bool) json.RawMessage {
	var parent any
	if rev.ParentRevisionID != nil {
		parent = rev.ParentRevisionID.String()
	}
	payload := map[string]any{
		"page_id":            rev.PageID.String(),
		"revision_id":        rev.ID.String(),
		"parent_revision_id": parent,
		"content_hash":       rev.ContentHash,
	}
	if withSchemaVersion {
		payload["schema_version"] = rev.SchemaVersion
	}
	// map[string]any 序列化不会失败（键值均为标量/nil）。
	data, _ := json.Marshal(payload)
	return data
}
