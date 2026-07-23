package page

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/render"
)

// DefaultMaxRedirectHops ResolveRedirect 的默认最大跳数。
const DefaultMaxRedirectHops = 5

// 新建页面的默认值（设计 §18.1 种子站点默认语言 zh-Hans，AST schema v1）。
const (
	defaultLanguage     = "zh-Hans"
	defaultContentModel = "block-v1"
)

// Service Page 领域服务：Page 身份的唯一权威写入入口。
// 事务边界由 db.TxManager 提供，跨表写入（改名 = 更新 page + 写别名）在单事务内完成。
type Service struct {
	repo            *Repository
	txm             *db.TxManager
	ids             *id.Generator
	publishObserver PublishObserver
}

// PublishObserver receives only duration and success/failure, never page content or IDs.
type PublishObserver interface {
	ObservePublish(duration time.Duration, err error)
}

// NewService 装配 Page 领域服务。
func NewService(repo *Repository, txm *db.TxManager, ids *id.Generator) *Service {
	return &Service{repo: repo, txm: txm, ids: ids}
}

// WithPublishObserver configures optional publish metrics without changing domain writes.
func (s *Service) WithPublishObserver(observer PublishObserver) *Service {
	s.publishObserver = observer
	return s
}

// CreatePageParams 创建页面的入参。Language/ContentModel 为空时用站点默认值。
type CreatePageParams struct {
	WikiID       uuid.UUID
	NamespaceID  uuid.UUID
	Title        string
	Language     string
	ContentModel string
	ActorID      uuid.UUID
}

// CreatePage 创建页面：校验 Actor → 规范化标题 → 冲突检查 → 插入。
// 标题被同 wiki+namespace 的活页面或别名占用时返回 ErrTitleConflict。
// 同事务写入 audit_event / outbox_event（page.created，驱动未解析链接 Resolver，M3-T04）。
func (s *Service) CreatePage(ctx context.Context, params CreatePageParams) (*Page, error) {
	if err := s.checkWriteActor(ctx, params.ActorID); err != nil {
		return nil, err
	}
	normalized, display, err := normalizePair(params.Title)
	if err != nil {
		return nil, err
	}
	if params.Language == "" {
		params.Language = defaultLanguage
	}
	if params.ContentModel == "" {
		params.ContentModel = defaultContentModel
	}

	p := &Page{
		WikiID:          params.WikiID,
		NamespaceID:     params.NamespaceID,
		NormalizedTitle: normalized,
		DisplayTitle:    display,
		Language:        params.Language,
		ContentModel:    params.ContentModel,
		Status:          StatusActive,
		CreatedBy:       params.ActorID,
	}

	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.ensureTitleAvailable(ctx, tx, p.WikiID, p.NamespaceID, normalized, uuid.Nil); err != nil {
			return err
		}
		pageID, err := s.ids.New()
		if err != nil {
			return err
		}
		p.ID = pageID
		if err := s.repo.InsertPage(ctx, tx, p); err != nil {
			return err
		}
		return s.emitPageEvent(ctx, tx, params.ActorID, p, EventTypePageCreated, "")
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetPageByID(ctx, nil, p.ID)
}

// GetPage 按 ID 查询页面（含软删除页，由调用方判断 DeletedAt）。
func (s *Service) GetPage(ctx context.Context, id uuid.UUID) (*Page, error) {
	return s.repo.GetPageByID(ctx, nil, id)
}

// CurrentContent 读取页面当前 Revision 与 ContentSnapshot（M1-T06 阅读路径，只读）。
// 页面已创建但未发布过返回 (nil, nil, nil)；页面不存在返回 ErrPageNotFound。
func (s *Service) CurrentContent(ctx context.Context, pageID uuid.UUID) (*Revision, *ContentSnapshot, error) {
	return s.repo.GetCurrentRevisionWithSnapshot(ctx, nil, pageID)
}

// RenderedHTML 查询 RenderedPage 投影的 HTML（M3-T05 阅读路径，只读）。
// 仅当投影行的 revision_id 是给定 Revision 且 renderer_version 匹配当前
// render.RendererVersion 时命中（ok=true）；投影缺失、Revision 已落后、
// 渲染器升版后旧行均返回 ok=false，由调用方实时渲染兜底。
func (s *Service) RenderedHTML(ctx context.Context, pageID, revisionID uuid.UUID) (html string, ok bool, err error) {
	return s.repo.GetRenderedHTML(ctx, nil, pageID, revisionID, render.RendererVersion)
}

// NamespaceID 按命名空间 key 解析 ID（API 层创建页面时由 key 到 ID 的转换），
// 未命中返回 ErrNamespaceNotFound。
func (s *Service) NamespaceID(ctx context.Context, wikiID uuid.UUID, namespaceKey string) (uuid.UUID, error) {
	return s.repo.GetNamespaceIDByKey(ctx, nil, wikiID, namespaceKey)
}

// 页面搜索的 limit 边界（M2-T04，契约 /pages/search）。
const (
	// SearchPagesDefaultLimit 未传 limit 或 limit<=0 时的默认返回条数。
	SearchPagesDefaultLimit = 10
	// SearchPagesMaxLimit 单次返回条数上限（超出截断到该值）。
	SearchPagesMaxLimit = 50
)

// SearchPages 编辑器引用选择器的页面搜索（M2-T04，只读）。
// 查询词经 NormalizeTitle 规范化后与 normalized_title 做 ILIKE 匹配；
// 空查询词（trim 后为空）返回空列表而非全量；limit<=0 取默认值，超限截断。
// 排序（Repository 内完成）：规范化标题精确命中 > 前缀 > 包含，同档按标题字典序。
func (s *Service) SearchPages(ctx context.Context, wikiID uuid.UUID, namespaceKey, query string, limit int) ([]PageSearchHit, error) {
	if limit <= 0 {
		limit = SearchPagesDefaultLimit
	}
	if limit > SearchPagesMaxLimit {
		limit = SearchPagesMaxLimit
	}
	namespaceID, err := s.repo.GetNamespaceIDByKey(ctx, nil, wikiID, namespaceKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(query) == "" {
		return []PageSearchHit{}, nil
	}
	normalized, err := NormalizeTitle(query)
	if err != nil {
		return nil, err
	}
	escaped := escapeLikePattern(normalized)
	return s.repo.SearchPages(ctx, nil, wikiID, namespaceID, normalized, "%"+escaped+"%", escaped+"%", limit)
}

// escapeLikePattern 转义 ILIKE 特殊字符（\ % _），与 SQL 中的 ESCAPE '\' 配套。
func escapeLikePattern(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// ResolveTitle 按 (wikiID, namespaceKey, title) 解析页面：
// 规范化后先查活页面，再查 page_alias；命中别名时 ViaAlias=true；
// 都不命中（或别名指向已软删除页面）返回 ErrPageNotFound。
func (s *Service) ResolveTitle(ctx context.Context, wikiID uuid.UUID, namespaceKey, title string) (*ResolvedPage, error) {
	normalized, err := NormalizeTitle(title)
	if err != nil {
		return nil, err
	}
	namespaceID, err := s.repo.GetNamespaceIDByKey(ctx, nil, wikiID, namespaceKey)
	if err != nil {
		return nil, err
	}
	if p, err := s.repo.GetLivePageByTitle(ctx, nil, wikiID, namespaceID, normalized); err == nil {
		return &ResolvedPage{Page: p}, nil
	} else if !errors.Is(err, ErrPageNotFound) {
		return nil, err
	}
	alias, err := s.repo.GetAliasByTitle(ctx, nil, wikiID, namespaceID, normalized)
	if errors.Is(err, errAliasNotFound) {
		return nil, fmt.Errorf("%w: %s:%q", ErrPageNotFound, namespaceKey, normalized)
	}
	if err != nil {
		return nil, err
	}
	p, err := s.repo.GetPageByID(ctx, nil, alias.PageID)
	if err != nil {
		return nil, err
	}
	if p.DeletedAt != nil {
		return nil, fmt.Errorf("%w: %s:%q 指向已删除页面", ErrPageNotFound, namespaceKey, normalized)
	}
	return &ResolvedPage{Page: p, ViaAlias: true}, nil
}

// RenamePage 页面改名：更新 normalized_title/display_title，旧 normalized_title
// 写入 page_alias（alias_type='rename'），Page ID 不变，updated_at 刷新。
// 新标题被同 wiki+namespace 的活页面或其他页面的别名占用时返回 ErrTitleConflict；
// 新标题是本页面自己的旧别名（改回曾用名）时回收该别名。
// 规范化标题实际变化时同事务写入 audit_event / outbox_event（page.renamed，
// 驱动未解析链接 Resolver，M3-T04）；仅显示名变化不发事件（标题占用无变化）。
func (s *Service) RenamePage(ctx context.Context, pageID uuid.UUID, newTitle string, actorID uuid.UUID) (*Page, error) {
	if err := s.checkWriteActor(ctx, actorID); err != nil {
		return nil, err
	}
	normalized, display, err := normalizePair(newTitle)
	if err != nil {
		return nil, err
	}

	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		p, err := s.repo.GetPageByIDForUpdate(ctx, tx, pageID)
		if err != nil {
			return err
		}
		if p.DeletedAt != nil {
			return fmt.Errorf("%w: id=%s 已删除", ErrPageNotFound, pageID)
		}
		if normalized == p.NormalizedTitle {
			// 仅显示名变化（大小写/空白书写差异）：不产别名、不发事件。
			return s.repo.UpdatePageTitle(ctx, tx, pageID, normalized, display)
		}
		if err := s.ensureTitleAvailable(ctx, tx, p.WikiID, p.NamespaceID, normalized, pageID); err != nil {
			return err
		}
		oldNormalized := p.NormalizedTitle
		if err := s.repo.UpdatePageTitle(ctx, tx, pageID, normalized, display); err != nil {
			return err
		}
		aliasID, err := s.ids.New()
		if err != nil {
			return err
		}
		if err := s.repo.InsertAlias(ctx, tx, &Alias{
			ID:              aliasID,
			WikiID:          p.WikiID,
			NamespaceID:     p.NamespaceID,
			NormalizedTitle: oldNormalized,
			PageID:          pageID,
			AliasType:       AliasTypeRename,
		}); err != nil {
			return err
		}
		p.NormalizedTitle = normalized
		p.DisplayTitle = display
		return s.emitPageEvent(ctx, tx, actorID, p, EventTypePageRenamed, oldNormalized)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetPageByID(ctx, nil, pageID)
}

// CreateRedirect 创建/覆盖站内重定向 source → target。
// 两侧页面都必须存在且未软删除；source == target 视为环，返回 ErrRedirectLoop。
func (s *Service) CreateRedirect(ctx context.Context, sourcePageID, targetPageID uuid.UUID) error {
	if sourcePageID == targetPageID {
		return fmt.Errorf("%w: 自重定向 %s", ErrRedirectLoop, sourcePageID)
	}
	for _, id := range []uuid.UUID{sourcePageID, targetPageID} {
		p, err := s.repo.GetPageByID(ctx, nil, id)
		if err != nil {
			return err
		}
		if p.DeletedAt != nil {
			return fmt.Errorf("%w: id=%s", ErrRedirectTargetDeleted, id)
		}
	}
	return s.repo.UpsertRedirect(ctx, nil, sourcePageID, targetPageID)
}

// ResolveRedirect 从 pageID 跟随站内重定向链，返回最终落地页面。
// 无重定向时返回页面自身。maxHops <= 0 时使用 DefaultMaxRedirectHops；
// 链上重访页面返回 ErrRedirectLoop，超过 maxHops 返回 ErrRedirectTooDeep，
// 链上任一目标已软删除返回 ErrRedirectTargetDeleted。
func (s *Service) ResolveRedirect(ctx context.Context, pageID uuid.UUID, maxHops int) (*Page, error) {
	if maxHops <= 0 {
		maxHops = DefaultMaxRedirectHops
	}
	visited := make(map[uuid.UUID]bool)
	current := pageID
	for hops := 0; ; hops++ {
		if visited[current] {
			return nil, fmt.Errorf("%w: 于 %s", ErrRedirectLoop, current)
		}
		visited[current] = true

		p, err := s.repo.GetPageByID(ctx, nil, current)
		if err != nil {
			return nil, err
		}
		if p.DeletedAt != nil {
			return nil, fmt.Errorf("%w: id=%s", ErrRedirectTargetDeleted, current)
		}
		target, err := s.repo.GetRedirectTarget(ctx, nil, current)
		if err != nil {
			return nil, err
		}
		if target == nil {
			return p, nil
		}
		if hops+1 > maxHops {
			return nil, fmt.Errorf("%w: 超过 %d 跳", ErrRedirectTooDeep, maxHops)
		}
		current = *target
	}
}

// normalizePair 计算 (normalized_title, display_title)。
func normalizePair(raw string) (normalized, display string, err error) {
	display, err = DisplayTitle(raw)
	if err != nil {
		return "", "", err
	}
	normalized, err = NormalizeTitle(raw)
	if err != nil {
		return "", "", err
	}
	return normalized, display, nil
}

// checkWriteActor 校验写操作 Actor，委托 Repository.CheckWriteActor
// （导出方法，knowledge 等领域模块复用同一实现）。
func (s *Service) checkWriteActor(ctx context.Context, actorID uuid.UUID) error {
	return s.repo.CheckWriteActor(ctx, nil, actorID)
}

// ensureTitleAvailable 确认 normalized 标题在 wiki+namespace 内可用：
// 未被其他活页面占用，也未被其他页面的别名占用。
// excludePageID 为改名场景下的页面自身：命中自己的旧别名时回收（删除）而非判冲突。
func (s *Service) ensureTitleAvailable(ctx context.Context, tx pgx.Tx, wikiID, namespaceID uuid.UUID, normalized string, excludePageID uuid.UUID) error {
	if p, err := s.repo.GetLivePageByTitle(ctx, tx, wikiID, namespaceID, normalized); err == nil {
		if p.ID != excludePageID {
			return fmt.Errorf("%w: 活页面 %s 已占用 %q", ErrTitleConflict, p.ID, normalized)
		}
	} else if !errors.Is(err, ErrPageNotFound) {
		return err
	}
	alias, err := s.repo.GetAliasByTitle(ctx, tx, wikiID, namespaceID, normalized)
	if errors.Is(err, errAliasNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if alias.PageID == excludePageID {
		// 改回自己的曾用名：回收旧别名，标题重新成为活标题。
		return s.repo.DeleteAlias(ctx, tx, alias.ID)
	}
	owner, err := s.repo.GetPageByID(ctx, tx, alias.PageID)
	if err != nil {
		return err
	}
	if owner.DeletedAt == nil {
		return fmt.Errorf("%w: 页面 %s 的别名已占用 %q", ErrTitleConflict, alias.PageID, normalized)
	}
	// 别名指向已软删除页面：不阻塞，但清除该失效别名避免错误解析。
	return s.repo.DeleteAlias(ctx, tx, alias.ID)
}

// emitPageEvent 在页面生命周期事务内同步写入审计与 Outbox 事件（M3-T04，
// 写法与发布事务 runPublishTx 一致）：page.created / page.renamed 的
// audit_event 与 outbox_event 共用同一 eventType 与同一份 payload
// （page_id/wiki_id/namespace_id/normalized_title/display_title；
// 改名附加 old_normalized_title），驱动 projection 的未解析链接 Resolver。
func (s *Service) emitPageEvent(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, p *Page, eventType, oldNormalizedTitle string) error {
	payload := map[string]any{
		"page_id":          p.ID.String(),
		"wiki_id":          p.WikiID.String(),
		"namespace_id":     p.NamespaceID.String(),
		"normalized_title": p.NormalizedTitle,
		"display_title":    p.DisplayTitle,
	}
	if oldNormalizedTitle != "" {
		payload["old_normalized_title"] = oldNormalizedTitle
	}
	// map[string]any 序列化不会失败（键值均为标量）。
	data, _ := json.Marshal(payload)

	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	if err := s.repo.InsertAuditEvent(ctx, tx, &AuditEvent{
		ID:            auditID,
		ActorID:       actorID,
		EventType:     eventType,
		AggregateType: AggregateTypePage,
		AggregateID:   p.ID,
		Payload:       data,
	}); err != nil {
		return err
	}

	outboxID, err := s.ids.New()
	if err != nil {
		return err
	}
	return s.repo.InsertOutboxEvent(ctx, tx, &OutboxEvent{
		ID:            outboxID,
		AggregateType: AggregateTypePage,
		AggregateID:   p.ID,
		EventType:     eventType,
		Payload:       data,
	})
}
