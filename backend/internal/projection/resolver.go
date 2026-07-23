// 未解析链接 Resolver（M3-T04，设计 §5.2）。
//
// 设计 §5.2：目标页面不存在时 AST 保存未解析引用（target_namespace +
// normalized_title + resolution_status），后台 Resolver 在新页面创建后
// 尝试自动解析。Resolver 的自动解析只影响投影展示层（page_link_projection），
// 不修改权威 AST——AST 中的引用保持未解析形态，直到人工/Proposal 修改。
//
// 触发点：
//   - page.created / page.renamed 事件（page 领域在创建/改名事务内写 outbox）：
//     按 (target_namespace_id, target_title=normalized_title) 解析候选；
//   - page.revision_published 事件路径的 hook（WrapPublishedHandler）：对事件页
//     current Revision 投影中的全部未解析行再尝试一次解析——无论 page.created
//     与引用所在 Revision 的发布事件到达顺序如何，最终一致（README「Resolver」节
//     记录了该决策）。
//
// 解析语义：
//   - 候选 = 该 namespace 下 normalized_title 命中的活页面 + 命中别名指向的活页面；
//   - 恰好一个候选 → 把匹配的 unresolved 投影行置为 resolved 并写 target_page_id；
//   - 多个候选（同名页面与别名歧义）→ 保持 unresolved，记 info 日志（含候选数），不建表；
//   - 版本防护（设计 §15）：只更新 source_revision_id 仍是 source 页面
//     current_revision_id 的投影行，旧 Revision 的投影不动。
//
// 幂等：候选匹配与 UPDATE 都是现状驱动的（不依赖事件负载之外的状态），
// 同一事件重复处理结果一致。
package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 页面生命周期事件类型（与 backend/internal/page 的 Outbox 事件常量字符串一致；
// projection 不反向依赖 page 领域包，cmd/worker 注册时用 page 包常量）。
const (
	// EventTypePageCreated 页面已创建。
	EventTypePageCreated = "page.created"
	// EventTypePageRenamed 页面已改名（payload 含 old_normalized_title）。
	EventTypePageRenamed = "page.renamed"
)

// LinkResolver 未解析页面引用的自动解析器（设计 §5.2），实现 Handler
// 消费 page.created / page.renamed 事件。
type LinkResolver struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewLinkResolver 装配 LinkResolver；logger 为 nil 时使用 slog.Default()。
func NewLinkResolver(pool *pgxpool.Pool, logger *slog.Logger) *LinkResolver {
	if logger == nil {
		logger = slog.Default()
	}
	return &LinkResolver{pool: pool, logger: logger}
}

// pageLifecyclePayload page.created / page.renamed 事件载荷（page 领域写入）。
type pageLifecyclePayload struct {
	PageID          string `json:"page_id"`
	NamespaceID     string `json:"namespace_id"`
	NormalizedTitle string `json:"normalized_title"`
}

// Handle 实现 Handler：消费 page.created / page.renamed，对新标题触发解析。
//
// 改名场景（设计 §5.1）：旧标题释放、新标题占用。已 resolved 指向该页的投影行
// 无需动作——target_page_id 解析与标题无关，Page ID 稳定；仅对新标题触发解析
// （旧标题成为别名后是否回补指向旧标题的 unresolved 行，属产品决策，当前不做）。
func (r *LinkResolver) Handle(ctx context.Context, event Event) error {
	var p pageLifecyclePayload
	if err := json.Unmarshal(event.Payload, &p); err != nil {
		return fmt.Errorf("projection: 解析 %s 事件载荷失败: %w", event.EventType, err)
	}
	namespaceID, err := uuid.Parse(p.NamespaceID)
	if err != nil {
		return fmt.Errorf("projection: %s 载荷 namespace_id 非法 %q: %w", event.EventType, p.NamespaceID, err)
	}
	if p.NormalizedTitle == "" {
		return fmt.Errorf("projection: %s 载荷缺 normalized_title", event.EventType)
	}
	return r.ResolveTitle(ctx, namespaceID, p.NormalizedTitle)
}

// WrapPublishedHandler 把 page.revision_published 的 Builder 分发 Handler 与
// 「发布后对该页未解析链接尝试解析」组合为一个 Handler：先分发重建投影，
// 成功后对该页执行 ResolvePageLinks。乱序场景（page.created 先于引用所在
// Revision 的发布事件到达，created 处理时无匹配投影行属正常）由本 hook 兜底，
// 保证无论事件顺序如何最终一致。
func (r *LinkResolver) WrapPublishedHandler(inner Handler) Handler {
	return HandlerFunc(func(ctx context.Context, event Event) error {
		if err := inner.Handle(ctx, event); err != nil {
			return err
		}
		return r.ResolvePageLinks(ctx, event.AggregateID)
	})
}

// ResolveTitle 尝试解析 (namespaceID, normalizedTitle) 下的全部未解析投影行：
// 候选恰好一个时置 resolved，多个候选（歧义）保持 unresolved 并记日志。
// 无匹配投影行（引用投影尚未生成）或无候选均属正常，直接返回 nil。
func (r *LinkResolver) ResolveTitle(ctx context.Context, namespaceID uuid.UUID, normalizedTitle string) error {
	candidates, err := r.matchCandidates(ctx, namespaceID, normalizedTitle)
	if err != nil {
		return err
	}
	switch len(candidates) {
	case 0:
		return nil
	case 1:
		return r.markResolved(ctx, namespaceID, normalizedTitle, candidates[0])
	default:
		// 歧义（同名页面与别名同时命中）：保持 unresolved，记日志 + 候选数，不建表。
		r.logger.Info("未解析引用候选歧义，保持 unresolved",
			slog.String("namespace_id", namespaceID.String()),
			slog.String("normalized_title", normalizedTitle),
			slog.Int("candidate_count", len(candidates)),
		)
		return nil
	}
}

// ResolvePageLinks 对指定 source 页面 current Revision 投影中的全部未解析行
// 逐 (namespace, title) 尝试解析（page.revision_published 路径的 hook）。
// 版本防护与 ResolveTitle/markResolved 一致：只处理 source_revision_id 仍是
// 页面 current_revision_id 的行。
func (r *LinkResolver) ResolvePageLinks(ctx context.Context, pageID uuid.UUID) error {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT plp.target_namespace_id, plp.target_title
		FROM page_link_projection plp
		JOIN page sp ON sp.id = plp.source_page_id
		WHERE plp.source_page_id = $1
		  AND plp.resolution_status = 'unresolved'
		  AND plp.source_revision_id = sp.current_revision_id
		  AND plp.target_namespace_id IS NOT NULL`, pageID)
	if err != nil {
		return fmt.Errorf("projection: 查询页面 %s 的未解析链接失败: %w", pageID, err)
	}
	type unresolvedKey struct {
		namespaceID uuid.UUID
		title       string
	}
	var keys []unresolvedKey
	for rows.Next() {
		var k unresolvedKey
		if err := rows.Scan(&k.namespaceID, &k.title); err != nil {
			rows.Close()
			return fmt.Errorf("projection: 扫描未解析链接失败: %w", err)
		}
		keys = append(keys, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("projection: 迭代未解析链接失败: %w", err)
	}
	for _, k := range keys {
		if err := r.ResolveTitle(ctx, k.namespaceID, k.title); err != nil {
			return err
		}
	}
	return nil
}

// matchCandidates 返回 (namespace, normalizedTitle) 的解析候选：该 namespace 下
// 标题命中的活页面 + 命中别名指向的活页面（UNION 去重）。
func (r *LinkResolver) matchCandidates(ctx context.Context, namespaceID uuid.UUID, normalizedTitle string) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id FROM page
		WHERE namespace_id = $1 AND normalized_title = $2 AND deleted_at IS NULL
		UNION
		SELECT pa.page_id FROM page_alias pa
		JOIN page p ON p.id = pa.page_id
		WHERE pa.namespace_id = $1 AND pa.normalized_title = $2 AND p.deleted_at IS NULL`,
		namespaceID, normalizedTitle)
	if err != nil {
		return nil, fmt.Errorf("projection: 查询解析候选失败: %w", err)
	}
	defer rows.Close()
	var candidates []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("projection: 扫描解析候选失败: %w", err)
		}
		candidates = append(candidates, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: 迭代解析候选失败: %w", err)
	}
	return candidates, nil
}

// markResolved 把 (namespace, title) 匹配、source_revision 仍是 source 页面
// current Revision 的 unresolved 投影行置为 resolved 并写 target_page_id
// （版本防护：旧 Revision 的投影行不动）。display_text 保持 normalized_title
// 原文——AST 未变，展示文本与权威内容一致。
func (r *LinkResolver) markResolved(ctx context.Context, namespaceID uuid.UUID, normalizedTitle string, targetPageID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE page_link_projection plp
		SET resolution_status = 'resolved', target_page_id = $1
		FROM page sp
		WHERE plp.source_page_id = sp.id
		  AND plp.source_revision_id = sp.current_revision_id
		  AND plp.resolution_status = 'unresolved'
		  AND plp.target_namespace_id = $2
		  AND plp.target_title = $3`,
		targetPageID, namespaceID, normalizedTitle)
	if err != nil {
		return fmt.Errorf("projection: 标记链接 resolved 失败: %w", err)
	}
	if tag.RowsAffected() > 0 {
		r.logger.Info("未解析引用已自动解析",
			slog.String("namespace_id", namespaceID.String()),
			slog.String("normalized_title", normalizedTitle),
			slog.String("target_page_id", targetPageID.String()),
			slog.Int64("resolved_rows", tag.RowsAffected()),
		)
	}
	return nil
}
