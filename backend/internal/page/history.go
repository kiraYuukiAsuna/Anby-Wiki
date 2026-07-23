// Revision 历史、结构 Diff 与回滚（M1-T07，设计 §3.3/§4.6）。
//
// 回滚不是修改旧 Revision：读取目标旧版快照的 AST，复用发布事务
// （runPublishTx，expected = 锁内当前 current）追加一个新 Revision；
// 旧 Revision 与旧 ContentSnapshot 不动（INV-02）。内容 hash 与目标版本
// 相同的快照按 (content_hash, schema_version) 查重复用，不重复存储（决策见 README）。
package page

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
)

// 历史列表的分页边界（与契约 components/parameters/PageSize 一致）。
const (
	// DefaultHistoryPageSize 未传 page_size 时的默认每页条数。
	DefaultHistoryPageSize = 20
	// MaxHistoryPageSize 每页条数上限。
	MaxHistoryPageSize = 100
)

// RevisionPage 一页 Revision 历史（按 created_at DESC, id DESC）。
// NextCursor 为 nil 表示没有更多。
type RevisionPage struct {
	Items      []Revision
	NextCursor *string
}

// ListRevisions 游标分页列出页面 Revision 元信息（含冗余自快照的 content_hash/schema_version）。
// cursor 传空取首页；limit <= 0 用 DefaultHistoryPageSize，超过 MaxHistoryPageSize 截断。
// 页面不存在返回 ErrPageNotFound；游标无法解析返回 ErrInvalidCursor。
func (s *Service) ListRevisions(ctx context.Context, pageID uuid.UUID, cursor string, limit int) (*RevisionPage, error) {
	if _, err := s.repo.GetPageByID(ctx, nil, pageID); err != nil {
		return nil, err
	}
	var afterCreatedAt *time.Time
	var afterID *uuid.UUID
	if cursor != "" {
		ts, id, err := decodeHistoryCursor(cursor)
		if err != nil {
			return nil, err
		}
		afterCreatedAt, afterID = &ts, &id
	}
	if limit <= 0 {
		limit = DefaultHistoryPageSize
	}
	if limit > MaxHistoryPageSize {
		limit = MaxHistoryPageSize
	}

	// 多取一条判断是否还有下一页。
	revs, err := s.repo.ListRevisions(ctx, nil, pageID, afterCreatedAt, afterID, limit+1)
	if err != nil {
		return nil, err
	}
	result := &RevisionPage{Items: revs}
	if len(revs) > limit {
		result.Items = revs[:limit]
		last := revs[limit-1]
		cursor := encodeHistoryCursor(last.CreatedAt, last.ID)
		result.NextCursor = &cursor
	}
	return result, nil
}

// GetRevision 读取页面指定 Revision 的详情（含快照 AST）。
// 页面不存在返回 ErrPageNotFound；Revision 不存在或不属于该页面返回 ErrRevisionNotFound。
func (s *Service) GetRevision(ctx context.Context, pageID, revisionID uuid.UUID) (*Revision, *ContentSnapshot, error) {
	if _, err := s.repo.GetPageByID(ctx, nil, pageID); err != nil {
		return nil, nil, err
	}
	return s.repo.GetRevisionWithSnapshot(ctx, nil, pageID, revisionID)
}

// DiffRevisions 计算同一页面两个 Revision 的结构 Diff（ast.Diff，from 为 base，to 为 current）。
// from == to 时返回空 Diff；页面/Revision 不存在的错误语义同 GetRevision。
func (s *Service) DiffRevisions(ctx context.Context, pageID, fromID, toID uuid.UUID) (*ast.DocumentDiff, error) {
	if fromID == toID {
		if _, _, err := s.GetRevision(ctx, pageID, fromID); err != nil {
			return nil, err
		}
		return &ast.DocumentDiff{Changes: []ast.BlockChange{}}, nil
	}
	_, fromSnap, err := s.GetRevision(ctx, pageID, fromID)
	if err != nil {
		return nil, err
	}
	_, toSnap, err := s.GetRevision(ctx, pageID, toID)
	if err != nil {
		return nil, err
	}
	fromDoc, err := ast.Parse(fromSnap.AST)
	if err != nil {
		return nil, fmt.Errorf("page: 解析 from 快照 AST 失败: %w", err)
	}
	toDoc, err := ast.Parse(toSnap.AST)
	if err != nil {
		return nil, fmt.Errorf("page: 解析 to 快照 AST 失败: %w", err)
	}
	return ast.Diff(fromDoc, toDoc)
}

// RollbackParams 回滚的入参。Summary 为空时默认记录「回滚到 {target_revision_id}」。
type RollbackParams struct {
	PageID           uuid.UUID
	TargetRevisionID uuid.UUID
	ActorID          uuid.UUID
	Summary          string
	ChangeBatchID    *uuid.UUID
}

// Rollback 回滚页面到目标旧版本内容：以目标 Revision 的快照 AST 复用发布事务
// 追加一个新 Revision（parent = 锁内当前 current），旧 Revision 与旧快照不动（设计 §3.3）。
// 内容与历史版本相同的快照按 content_hash 复用，不重复存储。
// 目标 Revision 不属于该页面返回 ErrRevisionNotFound；页面不存在/已删除返回 ErrPageNotFound；
// 并发语义与 Publish 一致（行锁串行化，回滚本身以锁内 current 为基线不会过期）。
func (s *Service) Rollback(ctx context.Context, params RollbackParams) (*Revision, error) {
	if err := s.checkWriteActor(ctx, params.ActorID); err != nil {
		return nil, err
	}
	_, targetSnap, err := s.GetRevision(ctx, params.PageID, params.TargetRevisionID)
	if err != nil {
		return nil, err
	}
	// 快照入库时已校验并 canonical 化，这里重建快照仅复用其 hash/size。
	snap, err := buildSnapshot(targetSnap.AST)
	if err != nil {
		return nil, err
	}
	summary := params.Summary
	if summary == "" {
		summary = "回滚到 " + params.TargetRevisionID.String()
	}
	targetID := params.TargetRevisionID
	return s.runPublishTx(ctx, publishDraft{
		pageID:             params.PageID,
		actorID:            params.ActorID,
		useCurrentExpected: true,
		snap:               snap,
		dedupSnapshot:      true,
		summary:            summary,
		changeBatchID:      params.ChangeBatchID,
		auditEventType:     EventTypeRevisionRolledBack,
		auditPayload: func(rev *Revision) json.RawMessage {
			return rollbackPayload(rev, targetID)
		},
		outboxPayload: func(rev *Revision) json.RawMessage { return publishPayload(rev, true) },
	})
}

// RollbackInTx 供 Governance Batch Rollback 组合事务；旧 Revision/Snapshot
// 保持不可变，补偿仍创建新 Revision。
func (s *Service) RollbackInTx(ctx context.Context, tx pgx.Tx, params RollbackParams) (*Revision, error) {
	if err := s.repo.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
		return nil, err
	}
	_, targetSnap, err := s.repo.GetRevisionWithSnapshot(ctx, tx, params.PageID, params.TargetRevisionID)
	if err != nil {
		return nil, err
	}
	snap, err := buildSnapshot(targetSnap.AST)
	if err != nil {
		return nil, err
	}
	summary := params.Summary
	if summary == "" {
		summary = "回滚到 " + params.TargetRevisionID.String()
	}
	targetID := params.TargetRevisionID
	return s.publishWithinTx(ctx, tx, publishDraft{
		pageID: params.PageID, actorID: params.ActorID, useCurrentExpected: true,
		snap: snap, dedupSnapshot: true, summary: summary, changeBatchID: params.ChangeBatchID,
		auditEventType: EventTypeRevisionRolledBack,
		auditPayload:   func(rev *Revision) json.RawMessage { return rollbackPayload(rev, targetID) },
		outboxPayload:  func(rev *Revision) json.RawMessage { return publishPayload(rev, true) },
	})
}

// rollbackPayload 构造回滚审计事件载荷：发布字段 + rolled_back_to。
func rollbackPayload(rev *Revision, targetRevisionID uuid.UUID) json.RawMessage {
	var parent any
	if rev.ParentRevisionID != nil {
		parent = rev.ParentRevisionID.String()
	}
	payload := map[string]any{
		"page_id":            rev.PageID.String(),
		"revision_id":        rev.ID.String(),
		"parent_revision_id": parent,
		"content_hash":       rev.ContentHash,
		"rolled_back_to":     targetRevisionID.String(),
	}
	// map[string]any 序列化不会失败（键值均为标量/nil）。
	data, _ := json.Marshal(payload)
	return data
}

// encodeHistoryCursor 把 (created_at, id) 编码为不透明游标（base64url 的 "unixnano:id"）。
func encodeHistoryCursor(createdAt time.Time, id uuid.UUID) string {
	raw := strconv.FormatInt(createdAt.UTC().UnixNano(), 10) + ":" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeHistoryCursor 解析 encodeHistoryCursor 产出的游标；无法解析返回 ErrInvalidCursor。
func decodeHistoryCursor(cursor string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w: 非 base64url", ErrInvalidCursor)
	}
	nanos, idPart, ok := strings.Cut(string(raw), ":")
	if !ok {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w: 缺少分隔符", ErrInvalidCursor)
	}
	n, err := strconv.ParseInt(nanos, 10, 64)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w: 时间戳非法", ErrInvalidCursor)
	}
	id, err := uuid.Parse(idPart)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w: ID 非法", ErrInvalidCursor)
	}
	return time.Unix(0, n).UTC(), id, nil
}
