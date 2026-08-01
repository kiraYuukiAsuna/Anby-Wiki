package page

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/platform/storage"
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
	repo             *Repository
	txm              *db.TxManager
	ids              *id.Generator
	publishObserver  PublishObserver
	snapshotStore    storage.Store
	snapshotEnv      string
	snapshotMaxBytes int64
	archiveRetention time.Duration
	archiveBatchSize int
}

// PublishObserver receives only duration and success/failure, never page content or IDs.
type PublishObserver interface {
	ObservePublish(duration time.Duration, err error)
}

// NewService 装配 Page 领域服务。
func NewService(repo *Repository, txm *db.TxManager, ids *id.Generator) *Service {
	return &Service{
		repo: repo, txm: txm, ids: ids,
		snapshotMaxBytes: 64 << 20,
		archiveRetention: 180 * 24 * time.Hour,
		archiveBatchSize: 50,
	}
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
	var result *Page
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = s.CreatePageInTx(ctx, tx, params, nil)
		return err
	})
	return result, err
}

// CreatePageInTx 供 Governance Apply 在同一事务中组合 Page 身份写入。
// Page 的标题占用、Actor、审计和 Outbox 约束仍全部由 Page 领域服务维护。
func (s *Service) CreatePageInTx(
	ctx context.Context,
	tx pgx.Tx,
	params CreatePageParams,
	changeBatchID *uuid.UUID,
) (*Page, error) {
	if err := s.repo.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
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

	if err := s.ensureTitleAvailable(ctx, tx, p.WikiID, p.NamespaceID, normalized, uuid.Nil); err != nil {
		return nil, err
	}
	pageID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	p.ID = pageID
	if err := s.repo.InsertPage(ctx, tx, p); err != nil {
		return nil, err
	}
	if err := s.emitPageEvent(
		ctx, tx, params.ActorID, p, EventTypePageCreated, "", "", changeBatchID,
	); err != nil {
		return nil, err
	}
	return s.repo.GetPageByID(ctx, tx, p.ID)
}

// GetPage 按 ID 查询页面（含软删除页，由调用方判断 DeletedAt）。
func (s *Service) GetPage(ctx context.Context, id uuid.UUID) (*Page, error) {
	return s.repo.GetPageByID(ctx, nil, id)
}

// CurrentContent 读取页面当前 Revision 与 ContentSnapshot（M1-T06 阅读路径，只读）。
// 页面已创建但未发布过返回 (nil, nil, nil)；页面不存在返回 ErrPageNotFound。
func (s *Service) CurrentContent(ctx context.Context, pageID uuid.UUID) (*Revision, *ContentSnapshot, error) {
	revision, snapshot, err := s.repo.GetCurrentRevisionWithSnapshot(
		ctx, nil, pageID,
	)
	if err != nil || snapshot == nil {
		return revision, snapshot, err
	}
	if err := s.hydrateSnapshot(ctx, snapshot); err != nil {
		return nil, nil, err
	}
	return revision, snapshot, nil
}

// CurrentRevisionMetadata negotiates lazy delivery without reading ast_json.
func (s *Service) CurrentRevisionMetadata(
	ctx context.Context,
	pageID uuid.UUID,
) (*Revision, *ContentSnapshot, error) {
	return s.repo.GetCurrentRevisionWithSnapshotMetadata(ctx, nil, pageID)
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
	var result *Page
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = s.RenamePageInTx(ctx, tx, pageID, newTitle, actorID, nil)
		return err
	})
	return result, err
}

// RenamePageInTx 供 Governance Apply 原子组合页面改名与 ChangeBatch。
func (s *Service) RenamePageInTx(
	ctx context.Context,
	tx pgx.Tx,
	pageID uuid.UUID,
	newTitle string,
	actorID uuid.UUID,
	changeBatchID *uuid.UUID,
) (*Page, error) {
	if err := s.repo.CheckWriteActor(ctx, tx, actorID); err != nil {
		return nil, err
	}
	normalized, display, err := normalizePair(newTitle)
	if err != nil {
		return nil, err
	}

	p, err := s.repo.GetPageByIDForUpdate(ctx, tx, pageID)
	if err != nil {
		return nil, err
	}
	if p.DeletedAt != nil {
		return nil, fmt.Errorf("%w: id=%s 已删除", ErrPageNotFound, pageID)
	}
	if normalized == p.NormalizedTitle {
		if display == p.DisplayTitle {
			return p, nil
		}
		oldDisplay := p.DisplayTitle
		// 仅显示名变化不产别名，但仍属于必须审计和可回滚的权威变更。
		if err := s.repo.UpdatePageTitle(ctx, tx, pageID, normalized, display); err != nil {
			return nil, err
		}
		p.DisplayTitle = display
		if err := s.emitPageEvent(
			ctx, tx, actorID, p, EventTypePageRenamed,
			p.NormalizedTitle, oldDisplay, changeBatchID,
		); err != nil {
			return nil, err
		}
		return s.repo.GetPageByID(ctx, tx, pageID)
	}
	if err := s.ensureTitleAvailable(ctx, tx, p.WikiID, p.NamespaceID, normalized, pageID); err != nil {
		return nil, err
	}
	oldNormalized := p.NormalizedTitle
	oldDisplay := p.DisplayTitle
	if err := s.repo.UpdatePageTitle(ctx, tx, pageID, normalized, display); err != nil {
		return nil, err
	}
	aliasID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	if err := s.repo.InsertAlias(ctx, tx, &Alias{
		ID:              aliasID,
		WikiID:          p.WikiID,
		NamespaceID:     p.NamespaceID,
		NormalizedTitle: oldNormalized,
		PageID:          pageID,
		AliasType:       AliasTypeRename,
	}); err != nil {
		return nil, err
	}
	p.NormalizedTitle = normalized
	p.DisplayTitle = display
	if err := s.emitPageEvent(
		ctx, tx, actorID, p, EventTypePageRenamed,
		oldNormalized, oldDisplay, changeBatchID,
	); err != nil {
		return nil, err
	}
	return s.repo.GetPageByID(ctx, tx, pageID)
}

// CreateRedirect creates or replaces the complete redirect target.
func (s *Service) CreateRedirect(
	ctx context.Context,
	sourcePageID uuid.UUID,
	target RedirectTarget,
	actorID uuid.UUID,
) (*PageRedirect, error) {
	var result *PageRedirect
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = s.CreateRedirectInTx(
			ctx, tx, sourcePageID, target, actorID, nil,
		)
		return err
	})
	return result, err
}

// CreateRedirectInTx validates the discriminated target and complete local
// chain before writing, so no edge can introduce a redirect cycle.
func (s *Service) CreateRedirectInTx(
	ctx context.Context,
	tx pgx.Tx,
	sourcePageID uuid.UUID,
	target RedirectTarget,
	actorID uuid.UUID,
	changeBatchID *uuid.UUID,
) (*PageRedirect, error) {
	if err := s.repo.CheckWriteActor(ctx, tx, actorID); err != nil {
		return nil, err
	}
	source, err := s.repo.GetPageByIDForUpdate(ctx, tx, sourcePageID)
	if err != nil {
		return nil, err
	}
	if source.DeletedAt != nil {
		return nil, fmt.Errorf("%w: source=%s", ErrRedirectTargetDeleted, sourcePageID)
	}
	normalized, err := s.normalizeRedirectTarget(ctx, tx, source, target)
	if err != nil {
		return nil, err
	}
	if err := s.validateRedirectChain(ctx, tx, source, normalized); err != nil {
		return nil, err
	}
	previous, err := s.repo.GetRedirectOptional(ctx, tx, sourcePageID)
	if err != nil {
		return nil, err
	}
	var previousTarget *RedirectTarget
	if previous != nil {
		copy := canonicalRedirectTarget(previous.Target)
		previousTarget = &copy
	}
	item, err := s.repo.UpsertRedirect(ctx, tx, sourcePageID, normalized, actorID)
	if err != nil {
		return nil, err
	}
	if err := s.emitRedirectEvent(
		ctx, tx, actorID, source, &normalized, previousTarget, changeBatchID,
	); err != nil {
		return nil, err
	}
	return item, nil
}

// RollbackCreatedPageInTx 软删除由目标 ChangeBatch 创建且之后未被其他写入触碰的 Page。
// 已发布 Revision 保持不可变，Page.current_revision_id 仍可指向其历史版本。
func (s *Service) RollbackCreatedPageInTx(
	ctx context.Context,
	tx pgx.Tx,
	pageID, actorID, changeBatchID uuid.UUID,
) error {
	if err := s.repo.CheckWriteActor(ctx, tx, actorID); err != nil {
		return err
	}
	p, err := s.repo.GetPageByIDForUpdate(ctx, tx, pageID)
	if err != nil {
		return err
	}
	if p.DeletedAt != nil {
		return fmt.Errorf("%w: page=%s 已删除", ErrPageLifecycleStale, pageID)
	}
	if err := s.repo.SoftDeleteCreatedPageByBatch(ctx, tx, pageID, changeBatchID); err != nil {
		return err
	}
	return s.emitPageEvent(
		ctx, tx, actorID, p, EventTypePageDeleted, "", "", &changeBatchID,
	)
}

// RollbackRenameInTx 仅当当前标题仍等于该批次写入值时恢复旧显示标题。
// RenamePageInTx 会保留被撤销的新标题为别名并回收原改名产生的旧标题别名。
func (s *Service) RollbackRenameInTx(
	ctx context.Context,
	tx pgx.Tx,
	pageID uuid.UUID,
	expectedNormalized, expectedDisplay, restoreDisplay string,
	actorID, changeBatchID uuid.UUID,
) (*Page, error) {
	current, err := s.repo.GetPageByIDForUpdate(ctx, tx, pageID)
	if err != nil {
		return nil, err
	}
	if current.DeletedAt != nil ||
		current.NormalizedTitle != expectedNormalized ||
		current.DisplayTitle != expectedDisplay {
		return nil, fmt.Errorf(
			"%w: page=%s expected=%q/%q current=%q/%q",
			ErrPageLifecycleStale, pageID, expectedNormalized, expectedDisplay,
			current.NormalizedTitle, current.DisplayTitle,
		)
	}
	return s.RenamePageInTx(
		ctx, tx, pageID, restoreDisplay, actorID, &changeBatchID,
	)
}

// GetRedirect returns the current authoritative redirect row.
func (s *Service) GetRedirect(
	ctx context.Context,
	sourcePageID uuid.UUID,
) (*PageRedirect, error) {
	return s.repo.GetRedirect(ctx, nil, sourcePageID)
}

// DeleteRedirect removes the current edge through the Page domain service.
func (s *Service) DeleteRedirect(
	ctx context.Context,
	sourcePageID, actorID uuid.UUID,
) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		source, err := s.repo.GetPageByIDForUpdate(ctx, tx, sourcePageID)
		if err != nil {
			return err
		}
		current, err := s.repo.GetRedirect(ctx, tx, sourcePageID)
		if err != nil {
			return err
		}
		if err := s.repo.DeleteRedirect(ctx, tx, sourcePageID); err != nil {
			return err
		}
		previous := canonicalRedirectTarget(current.Target)
		return s.emitRedirectEvent(
			ctx, tx, actorID, source, nil, &previous, nil,
		)
	})
}

// RollbackRedirectInTx restores the complete previous target and refuses when
// any field of the current target no longer matches the batch ledger.
func (s *Service) RollbackRedirectInTx(
	ctx context.Context,
	tx pgx.Tx,
	sourcePageID uuid.UUID,
	expectedTarget RedirectTarget,
	previousTarget *RedirectTarget,
	actorID, changeBatchID uuid.UUID,
) error {
	if err := s.repo.CheckWriteActor(ctx, tx, actorID); err != nil {
		return err
	}
	source, err := s.repo.GetPageByIDForUpdate(ctx, tx, sourcePageID)
	if err != nil {
		return err
	}
	current, err := s.repo.GetRedirectOptional(ctx, tx, sourcePageID)
	if err != nil {
		return err
	}
	if current == nil || !redirectTargetsEqual(current.Target, expectedTarget) {
		return fmt.Errorf(
			"%w: page=%s redirect target no longer matches",
			ErrPageLifecycleStale, sourcePageID,
		)
	}
	if previousTarget != nil {
		_, err := s.CreateRedirectInTx(
			ctx, tx, sourcePageID, *previousTarget, actorID, &changeBatchID,
		)
		return err
	}
	if err := s.repo.DeleteRedirect(ctx, tx, sourcePageID); err != nil {
		return err
	}
	currentTarget := canonicalRedirectTarget(current.Target)
	return s.emitRedirectEvent(
		ctx, tx, actorID, source, nil, &currentTarget, &changeBatchID,
	)
}

// ResolveRedirect follows internal Page and newly-resolved title targets.
// Page sections terminate on a local Page with a stable heading ID; unresolved
// and interwiki targets intentionally return Resolved=false and no local
// content landing.
func (s *Service) ResolveRedirect(
	ctx context.Context,
	pageID uuid.UUID,
	maxHops int,
) (*RedirectResolution, error) {
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
		redirect, err := s.repo.GetRedirectOptional(ctx, nil, current)
		if err != nil {
			return nil, err
		}
		if redirect == nil {
			return &RedirectResolution{
				Page: p, Hops: hops, Resolved: true,
			}, nil
		}
		if hops+1 > maxHops {
			return nil, fmt.Errorf("%w: 超过 %d 跳", ErrRedirectTooDeep, maxHops)
		}
		target := redirect.Target
		switch target.Kind {
		case RedirectTargetPage:
			current = *target.PageID
		case RedirectTargetPageSection:
			targetPage, err := s.repo.GetPageByID(ctx, nil, *target.PageID)
			if err != nil {
				return nil, err
			}
			if targetPage.DeletedAt != nil {
				return nil, fmt.Errorf(
					"%w: id=%s", ErrRedirectTargetDeleted, targetPage.ID,
				)
			}
			target.TargetPageTitle = targetPage.DisplayTitle
			return &RedirectResolution{
				Page: targetPage, TerminalTarget: &target,
				Hops: hops + 1, Resolved: true,
			}, nil
		case RedirectTargetUnresolved:
			resolved, err := s.repo.GetLivePageByTitle(
				ctx, nil, p.WikiID, *target.NamespaceID, target.Title,
			)
			if errors.Is(err, ErrPageNotFound) {
				return &RedirectResolution{
					Page: p, TerminalTarget: &target,
					Hops: hops + 1, Resolved: false,
				}, nil
			}
			if err != nil {
				return nil, err
			}
			current = resolved.ID
		case RedirectTargetInterwiki:
			return &RedirectResolution{
				Page: p, TerminalTarget: &target,
				Hops: hops + 1, Resolved: false,
			}, nil
		default:
			return nil, fmt.Errorf(
				"%w: kind=%q", ErrInvalidRedirectTarget, target.Kind,
			)
		}
	}
}

func (s *Service) normalizeRedirectTarget(
	ctx context.Context,
	tx pgx.Tx,
	source *Page,
	input RedirectTarget,
) (RedirectTarget, error) {
	switch input.Kind {
	case RedirectTargetPage, RedirectTargetPageSection:
		if input.PageID == nil || *input.PageID == uuid.Nil {
			return RedirectTarget{}, ErrInvalidRedirectTarget
		}
		if *input.PageID == source.ID {
			return RedirectTarget{}, fmt.Errorf(
				"%w: 自重定向 %s", ErrRedirectLoop, source.ID,
			)
		}
		targetPage, err := s.repo.GetPageByID(ctx, tx, *input.PageID)
		if err != nil {
			return RedirectTarget{}, err
		}
		if targetPage.DeletedAt != nil {
			return RedirectTarget{}, fmt.Errorf(
				"%w: target=%s", ErrRedirectTargetDeleted, targetPage.ID,
			)
		}
		if targetPage.WikiID != source.WikiID {
			return RedirectTarget{}, fmt.Errorf(
				"%w: 跨 Wiki Page 目标", ErrInvalidRedirectTarget,
			)
		}
		result := RedirectTarget{Kind: input.Kind, PageID: input.PageID}
		if input.Kind == RedirectTargetPage {
			if input.AnchorBlockID != nil || input.NamespaceID != nil ||
				input.Title != "" || input.TargetInterwiki != "" {
				return RedirectTarget{}, ErrInvalidRedirectTarget
			}
			return result, nil
		}
		if input.AnchorBlockID == nil || *input.AnchorBlockID == uuid.Nil ||
			input.NamespaceID != nil || input.Title != "" ||
			input.TargetInterwiki != "" {
			return RedirectTarget{}, ErrInvalidRedirectTarget
		}
		_, snap, err := s.repo.GetCurrentRevisionWithSnapshot(ctx, tx, targetPage.ID)
		if err != nil {
			return RedirectTarget{}, err
		}
		if snap == nil {
			return RedirectTarget{}, fmt.Errorf(
				"%w: 章节目标尚未发布", ErrInvalidRedirectTarget,
			)
		}
		doc, err := ast.Parse(snap.AST)
		if err != nil {
			return RedirectTarget{}, fmt.Errorf(
				"%w: 章节目标 AST: %v", ErrInvalidRedirectTarget, err,
			)
		}
		location, ok := ast.FindBlock(doc, input.AnchorBlockID.String())
		if !ok || location.Block.Type != ast.BlockHeading {
			return RedirectTarget{}, fmt.Errorf(
				"%w: heading=%s", ErrInvalidRedirectTarget, input.AnchorBlockID,
			)
		}
		result.AnchorBlockID = input.AnchorBlockID
		return result, nil

	case RedirectTargetUnresolved:
		if input.NamespaceID == nil || *input.NamespaceID == uuid.Nil ||
			input.PageID != nil || input.AnchorBlockID != nil ||
			input.TargetInterwiki != "" {
			return RedirectTarget{}, ErrInvalidRedirectTarget
		}
		ok, err := s.repo.NamespaceBelongsToWiki(
			ctx, tx, *input.NamespaceID, source.WikiID,
		)
		if err != nil {
			return RedirectTarget{}, err
		}
		if !ok {
			return RedirectTarget{}, ErrNamespaceNotFound
		}
		normalized, err := NormalizeTitle(input.Title)
		if err != nil {
			return RedirectTarget{}, fmt.Errorf(
				"%w: %v", ErrInvalidRedirectTarget, err,
			)
		}
		return RedirectTarget{
			Kind:        RedirectTargetUnresolved,
			NamespaceID: input.NamespaceID, Title: normalized,
		}, nil

	case RedirectTargetInterwiki:
		if input.PageID != nil || input.NamespaceID != nil ||
			input.AnchorBlockID != nil {
			return RedirectTarget{}, ErrInvalidRedirectTarget
		}
		title, err := DisplayTitle(input.Title)
		if err != nil {
			return RedirectTarget{}, fmt.Errorf(
				"%w: 外部标题: %v", ErrInvalidRedirectTarget, err,
			)
		}
		rawURL := strings.TrimSpace(input.TargetInterwiki)
		if len(rawURL) > 2048 {
			return RedirectTarget{}, ErrInvalidRedirectTarget
		}
		parsed, err := url.ParseRequestURI(rawURL)
		if err != nil || parsed.Host == "" ||
			(parsed.Scheme != "https" && parsed.Scheme != "http") ||
			parsed.User != nil {
			return RedirectTarget{}, fmt.Errorf(
				"%w: 外部目标必须是无凭据的 HTTP(S) URL",
				ErrInvalidRedirectTarget,
			)
		}
		return RedirectTarget{
			Kind:  RedirectTargetInterwiki,
			Title: title, TargetInterwiki: parsed.String(),
		}, nil
	default:
		return RedirectTarget{}, fmt.Errorf(
			"%w: kind=%q", ErrInvalidRedirectTarget, input.Kind,
		)
	}
}

func (s *Service) validateRedirectChain(
	ctx context.Context,
	tx pgx.Tx,
	source *Page,
	target RedirectTarget,
) error {
	var current *uuid.UUID
	switch target.Kind {
	case RedirectTargetPage, RedirectTargetPageSection:
		current = target.PageID
	case RedirectTargetUnresolved:
		resolved, err := s.repo.GetLivePageByTitle(
			ctx, tx, source.WikiID, *target.NamespaceID, target.Title,
		)
		if errors.Is(err, ErrPageNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		current = &resolved.ID
	case RedirectTargetInterwiki:
		return nil
	}
	seen := map[uuid.UUID]struct{}{source.ID: {}}
	for depth := 0; current != nil && depth < 64; depth++ {
		if _, exists := seen[*current]; exists {
			return fmt.Errorf("%w: 于 %s", ErrRedirectLoop, *current)
		}
		seen[*current] = struct{}{}
		p, err := s.repo.GetPageByID(ctx, tx, *current)
		if err != nil {
			return err
		}
		if p.DeletedAt != nil || p.WikiID != source.WikiID {
			return ErrRedirectTargetDeleted
		}
		if target.Kind == RedirectTargetPageSection && depth == 0 {
			return nil
		}
		next, err := s.repo.GetRedirectOptional(ctx, tx, p.ID)
		if err != nil {
			return err
		}
		if next == nil {
			return nil
		}
		switch next.Target.Kind {
		case RedirectTargetPage:
			current = next.Target.PageID
		case RedirectTargetPageSection, RedirectTargetInterwiki:
			return nil
		case RedirectTargetUnresolved:
			resolved, err := s.repo.GetLivePageByTitle(
				ctx, tx, source.WikiID,
				*next.Target.NamespaceID, next.Target.Title,
			)
			if errors.Is(err, ErrPageNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			current = &resolved.ID
		default:
			return ErrInvalidRedirectTarget
		}
	}
	if current != nil {
		return fmt.Errorf("%w: 重定向链超过 64 跳", ErrRedirectTooDeep)
	}
	return nil
}

func canonicalRedirectTarget(target RedirectTarget) RedirectTarget {
	return RedirectTarget{
		Kind: target.Kind, PageID: target.PageID,
		NamespaceID: target.NamespaceID, Title: target.Title,
		AnchorBlockID:   target.AnchorBlockID,
		TargetInterwiki: target.TargetInterwiki,
	}
}

func redirectTargetsEqual(left, right RedirectTarget) bool {
	left = canonicalRedirectTarget(left)
	right = canonicalRedirectTarget(right)
	if left.Kind != right.Kind || left.Title != right.Title ||
		left.TargetInterwiki != right.TargetInterwiki {
		return false
	}
	return equalUUIDPointer(left.PageID, right.PageID) &&
		equalUUIDPointer(left.NamespaceID, right.NamespaceID) &&
		equalUUIDPointer(left.AnchorBlockID, right.AnchorBlockID)
}

func equalUUIDPointer(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
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
func (s *Service) emitPageEvent(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	p *Page,
	eventType, oldNormalizedTitle, oldDisplayTitle string,
	changeBatchID *uuid.UUID,
) error {
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
	if oldDisplayTitle != "" {
		payload["old_display_title"] = oldDisplayTitle
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
		ChangeBatchID: changeBatchID,
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

func (s *Service) emitRedirectEvent(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	source *Page,
	target, previousTarget *RedirectTarget,
	changeBatchID *uuid.UUID,
) error {
	payload, _ := json.Marshal(map[string]any{
		"source_page_id":  source.ID,
		"target":          target,
		"previous_target": previousTarget,
		"wiki_id":         source.WikiID,
	})
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	if err := s.repo.InsertAuditEvent(ctx, tx, &AuditEvent{
		ID: auditID, ActorID: actorID, EventType: EventTypePageRedirected,
		AggregateType: AggregateTypePage, AggregateID: source.ID,
		ChangeBatchID: changeBatchID, Payload: payload,
	}); err != nil {
		return err
	}
	outboxID, err := s.ids.New()
	if err != nil {
		return err
	}
	return s.repo.InsertOutboxEvent(ctx, tx, &OutboxEvent{
		ID: outboxID, AggregateType: AggregateTypePage, AggregateID: source.ID,
		EventType: OutboxEventPageRedirected, Payload: payload,
	})
}
