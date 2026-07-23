package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

// 搜索条数限制（SearchEntities.Limit 缺省与上限）。
const (
	defaultSearchLimit = 20
	maxSearchLimit     = 100
)

// Service Entity 领域服务：Entity 身份与标签/别名/绑定的唯一权威写入入口。
// 跨表写入（创建 = entity + labels）在 db.TxManager 单事务内完成；
// 标签/别名写操作先在事务内锁定实体行（FOR UPDATE），
// 保证 merged 校验与后续写入原子，并序列化同一实体的并发写。
//
// 同名不自动合并（MT-M4-NO-AUTOMERGE）：创建只按 canonical_key 判重，
// 标签/别名相同的其他实体不影响创建，搜索会返回全部候选。
type Service struct {
	repo      *Repository
	pages     *page.Repository
	txm       *db.TxManager
	ids       *id.Generator
	citations CitationChecker
}

// NewService 装配 Entity 领域服务。
// pages 用于复用 page 模块的 Actor 准入校验与 Page 存在性查询（规则只维护一份）。
func NewService(repo *Repository, pages *page.Repository, txm *db.TxManager, ids *id.Generator) *Service {
	return &Service{repo: repo, pages: pages, txm: txm, ids: ids}
}

// WithCitationChecker 注入 citation 存在性只读检查（由 evidence 模块实现，
// 装配层注入）。返回 s 以便链式装配。未注入时 AddClaimSource 跳过存在性校验
// （DB 外键 claim_source_citation_fk 仍兜底）；注入后服务层先行领域校验
// （INV-07 完整化，M4-T05）。
func (s *Service) WithCitationChecker(c CitationChecker) *Service {
	s.citations = c
	return s
}

// LabelInput 标签入参。Label 落库前做 NFC + trim + 折叠空白（保留大小写）。
type LabelInput struct {
	Language    string
	Label       string
	Description string
	IsPrimary   bool
}

// CreateEntityParams 创建实体的入参。
// CanonicalKey 为空时取首个主标签的规范化键。
type CreateEntityParams struct {
	WikiID       uuid.UUID
	TypeKey      string
	CanonicalKey string
	Labels       []LabelInput
	ActorID      uuid.UUID
}

// CreateEntity 创建实体：校验 Actor → 校验标签 → 规范化 canonical_key →
// 解析 entity_type → 单事务写入 entity + labels。
// canonical_key 冲突返回 ErrDuplicateEntityKey；标签相同但 canonical_key 不同
// 的已有实体不影响创建（MT-M4-NO-AUTOMERGE，候选交由搜索呈现）。
func (s *Service) CreateEntity(ctx context.Context, params CreateEntityParams) (*Entity, error) {
	var entity *Entity
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		entity, err = s.CreateEntityInTx(ctx, tx, params)
		return err
	})
	return entity, err
}

// CreateEntityInTx 供 Governance Apply 组合事务；全部校验与写入仍在本领域服务内。
func (s *Service) CreateEntityInTx(ctx context.Context, tx pgx.Tx, params CreateEntityParams) (*Entity, error) {
	if err := s.pages.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
		return nil, err
	}
	labels, err := normalizeLabels(params.Labels)
	if err != nil {
		return nil, err
	}
	canonicalKey := params.CanonicalKey
	if strings.TrimSpace(canonicalKey) == "" {
		canonicalKey = firstPrimaryLabel(labels)
	}
	key, err := normalizeKey(canonicalKey)
	if err != nil {
		return nil, err
	}
	entityType, err := s.repo.GetEntityTypeByKey(ctx, tx, params.TypeKey)
	if err != nil {
		return nil, err
	}

	e := &Entity{
		WikiID:       params.WikiID,
		EntityTypeID: entityType.ID,
		CanonicalKey: key,
		Status:       StatusActive,
		CreatedBy:    params.ActorID,
	}
	if _, err := s.repo.GetEntityByCanonicalKey(ctx, tx, e.WikiID, key); err == nil {
		return nil, fmt.Errorf("%w: wiki=%s canonical_key=%q", ErrDuplicateEntityKey, e.WikiID, key)
	} else if !errors.Is(err, ErrEntityNotFound) {
		return nil, err
	}
	entityID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	e.ID = entityID
	if err := s.repo.InsertEntity(ctx, tx, e); err != nil {
		return nil, err
	}
	for i := range labels {
		labels[i].EntityID = entityID
		if err := s.repo.InsertLabel(ctx, tx, &labels[i]); err != nil {
			return nil, err
		}
	}
	return s.repo.GetEntityByID(ctx, tx, e.ID)
}

// GetEntity 按 ID 查询实体（含 merged/deleted，由调用方判断 Status）。
func (s *Service) GetEntity(ctx context.Context, id uuid.UUID) (*Entity, error) {
	return s.repo.GetEntityByID(ctx, nil, id)
}

// GetEntityType 按 ID 查询实体类型。
func (s *Service) GetEntityType(ctx context.Context, id uuid.UUID) (*EntityType, error) {
	return s.repo.GetEntityTypeByID(ctx, nil, id)
}

// ListLabels 列出实体全部标签（只读）。
func (s *Service) ListLabels(ctx context.Context, entityID uuid.UUID) ([]EntityLabel, error) {
	return s.repo.ListLabels(ctx, nil, entityID)
}

// ListAliases 列出实体全部别名（只读）。
func (s *Service) ListAliases(ctx context.Context, entityID uuid.UUID) ([]EntityAlias, error) {
	return s.repo.ListAliases(ctx, nil, entityID)
}

// HasPageBinding 查询页面上下文是否已绑定目标实体。
func (s *Service) HasPageBinding(ctx context.Context, pageID, entityID uuid.UUID) (bool, error) {
	return s.repo.HasPageBinding(ctx, nil, pageID, entityID)
}

// AddLabel 追加标签。同 (language, label) 已存在返回 ErrLabelExists；
// IsPrimary 且该语言已有主标签返回 ErrDuplicatePrimaryLabel
// （服务层在实体行锁内前置检查，DB 部分唯一索引兜底并发）。
func (s *Service) AddLabel(ctx context.Context, entityID uuid.UUID, input LabelInput) (*EntityLabel, error) {
	l, err := normalizeLabel(input)
	if err != nil {
		return nil, err
	}
	l.EntityID = entityID

	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.lockActiveEntity(ctx, tx, entityID); err != nil {
			return err
		}
		if _, err := s.repo.GetLabel(ctx, tx, entityID, l.Language, l.Label); err == nil {
			return fmt.Errorf("%w: entity=%s language=%q label=%q", ErrLabelExists, entityID, l.Language, l.Label)
		} else if !errors.Is(err, ErrLabelNotFound) {
			return err
		}
		if l.IsPrimary {
			if _, err := s.repo.GetPrimaryLabel(ctx, tx, entityID, l.Language); err == nil {
				return fmt.Errorf("%w: entity=%s language=%q", ErrDuplicatePrimaryLabel, entityID, l.Language)
			} else if !errors.Is(err, ErrLabelNotFound) {
				return err
			}
		}
		return s.repo.InsertLabel(ctx, tx, l)
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// RemoveLabel 删除标签。删除主标签时若它是实体仅剩的主标签，
// 返回 ErrNoPrimaryLabel（实体必须始终保有至少一个主标签，与 CreateEntity 不变量一致）。
func (s *Service) RemoveLabel(ctx context.Context, entityID uuid.UUID, language, label string) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.lockActiveEntity(ctx, tx, entityID); err != nil {
			return err
		}
		l, err := s.repo.GetLabel(ctx, tx, entityID, language, label)
		if err != nil {
			return err
		}
		if l.IsPrimary {
			n, err := s.repo.CountPrimaryLabels(ctx, tx, entityID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return fmt.Errorf("%w: entity=%s 仅剩的主标签不可删除", ErrNoPrimaryLabel, entityID)
			}
		}
		return s.repo.DeleteLabel(ctx, tx, entityID, language, label)
	})
}

// SetPrimaryLabel 把指定标签置为该语言的主标签（同事务内先取消旧主标签）。
// 标签不存在返回 ErrLabelNotFound；已是主标签时为 no-op。
func (s *Service) SetPrimaryLabel(ctx context.Context, entityID uuid.UUID, language, label string) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.lockActiveEntity(ctx, tx, entityID); err != nil {
			return err
		}
		l, err := s.repo.GetLabel(ctx, tx, entityID, language, label)
		if err != nil {
			return err
		}
		if l.IsPrimary {
			return nil
		}
		if err := s.repo.ClearPrimaryLabel(ctx, tx, entityID, language); err != nil {
			return err
		}
		return s.repo.SetLabelPrimary(ctx, tx, entityID, language, label)
	})
}

// AliasInput 别名入参。Alias 落库前做 NFC + trim + 折叠空白（保留大小写），
// normalized_alias 按 page 标题同规则规范化（NFC + 折叠空白 + 小写）。
// AliasType 为空时用 AliasTypeCommon。
type AliasInput struct {
	Language  string
	Alias     string
	AliasType string
}

// AddAlias 追加别名。同实体内 normalized_alias 重复（含大小写/空白书写差异）
// 返回 ErrDuplicateAlias。
func (s *Service) AddAlias(ctx context.Context, entityID uuid.UUID, input AliasInput) (*EntityAlias, error) {
	display, err := displayLabel(input.Alias)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeKey(input.Alias)
	if err != nil {
		return nil, err
	}
	language, err := normalizeLanguage(input.Language)
	if err != nil {
		return nil, err
	}
	if input.AliasType == "" {
		input.AliasType = AliasTypeCommon
	}

	a := &EntityAlias{
		EntityID:        entityID,
		Language:        language,
		Alias:           display,
		NormalizedAlias: normalized,
		AliasType:       input.AliasType,
	}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.lockActiveEntity(ctx, tx, entityID); err != nil {
			return err
		}
		if _, err := s.repo.GetAliasByNormalized(ctx, tx, entityID, normalized); err == nil {
			return fmt.Errorf("%w: entity=%s alias=%q", ErrDuplicateAlias, entityID, normalized)
		} else if !errors.Is(err, ErrAliasNotFound) {
			return err
		}
		aliasID, err := s.ids.New()
		if err != nil {
			return err
		}
		a.ID = aliasID
		return s.repo.InsertAlias(ctx, tx, a)
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// RemoveAlias 按 ID 删除别名，未命中返回 ErrAliasNotFound。
func (s *Service) RemoveAlias(ctx context.Context, entityID, aliasID uuid.UUID) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.lockActiveEntity(ctx, tx, entityID); err != nil {
			return err
		}
		return s.repo.DeleteAlias(ctx, tx, entityID, aliasID)
	})
}

// SearchParams 规范化搜索入参。TypeKey 为空不过滤类型；
// Limit <= 0 用默认值，超过上限截断；IncludeMerged 控制是否返回已合并实体。
type SearchParams struct {
	WikiID        uuid.UUID
	Query         string
	TypeKey       string
	Limit         int
	IncludeMerged bool
}

// SearchEntities 规范化搜索（设计 §6.2，M7 全文检索落地前的权威数据直连实现）：
// 查询串先按 page 标题同规则规范化，分两阶段匹配——
//  1. exact：canonical_key 相等 / 标签规范化相等 / normalized_alias 相等；
//  2. fuzzy：上述三者的 ILIKE 前缀/包含（不引入 pg_trgm）。
//
// 每个实体只出现一次；结果带 MatchedOn（canonical/label/alias）与 Exact 标记，
// 排序：exact 优先，同阶段内按 canonical → label → alias。
// 默认只返回 active 实体；IncludeMerged 时含 merged（deleted 永远排除）。
func (s *Service) SearchEntities(ctx context.Context, params SearchParams) ([]SearchResult, error) {
	key, err := normalizeKey(params.Query)
	if err != nil {
		return nil, err
	}
	limit := params.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	var typeKey *string
	if params.TypeKey != "" {
		typeKey = &params.TypeKey
	}

	// 阶段顺序即排序优先级：exact 在前，fuzzy 在后；各阶段内 canonical/label/alias。
	type phase struct {
		matchedOn string
		exact     bool
		run       func() ([]Entity, error)
	}
	pattern := likePattern(key)
	phases := []phase{
		{MatchedOnCanonical, true, func() ([]Entity, error) {
			return s.repo.SearchExactCanonical(ctx, nil, params.WikiID, typeKey, params.IncludeMerged, key, limit)
		}},
		{MatchedOnLabel, true, func() ([]Entity, error) {
			return s.repo.SearchExactLabel(ctx, nil, params.WikiID, typeKey, params.IncludeMerged, key, limit)
		}},
		{MatchedOnAlias, true, func() ([]Entity, error) {
			return s.repo.SearchExactAlias(ctx, nil, params.WikiID, typeKey, params.IncludeMerged, key, limit)
		}},
		{MatchedOnCanonical, false, func() ([]Entity, error) {
			return s.repo.SearchFuzzyCanonical(ctx, nil, params.WikiID, typeKey, params.IncludeMerged, pattern, limit)
		}},
		{MatchedOnLabel, false, func() ([]Entity, error) {
			return s.repo.SearchFuzzyLabel(ctx, nil, params.WikiID, typeKey, params.IncludeMerged, pattern, limit)
		}},
		{MatchedOnAlias, false, func() ([]Entity, error) {
			return s.repo.SearchFuzzyAlias(ctx, nil, params.WikiID, typeKey, params.IncludeMerged, pattern, limit)
		}},
	}

	results := []SearchResult{}
	seen := make(map[uuid.UUID]bool)
	for _, p := range phases {
		entities, err := p.run()
		if err != nil {
			return nil, err
		}
		for i := range entities {
			if seen[entities[i].ID] {
				continue
			}
			seen[entities[i].ID] = true
			e := entities[i]
			results = append(results, SearchResult{Entity: &e, MatchedOn: p.matchedOn, Exact: p.exact})
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// BindPageParams 页面-实体绑定入参。Role 限 BindingRolePrimary/BindingRoleMentioned。
type BindPageParams struct {
	PageID   uuid.UUID
	EntityID uuid.UUID
	Role     string
	Language string
}

// BindPage 绑定页面与实体。Page 必须存在且未软删除；Entity 必须存在且为
// active（merged 返回 ErrEntityMerged）；重复绑定返回 ErrBindingExists。
// 只写 page_entity_binding；page.primary_entity_id 指针的同步不在本 Task 范围。
func (s *Service) BindPage(ctx context.Context, params BindPageParams) (*PageEntityBinding, error) {
	if params.Role != BindingRolePrimary && params.Role != BindingRoleMentioned {
		return nil, fmt.Errorf("%w: role=%q", ErrInvalidBindingRole, params.Role)
	}
	language, err := normalizeLanguage(params.Language)
	if err != nil {
		return nil, err
	}
	p, err := s.pages.GetPageByID(ctx, nil, params.PageID)
	if err != nil {
		return nil, err
	}
	if p.DeletedAt != nil {
		return nil, fmt.Errorf("%w: id=%s 已删除", page.ErrPageNotFound, params.PageID)
	}
	e, err := s.repo.GetEntityByID(ctx, nil, params.EntityID)
	if err != nil {
		return nil, err
	}
	if err := ensureEntityActive(e); err != nil {
		return nil, err
	}

	b := &PageEntityBinding{
		PageID:   params.PageID,
		EntityID: params.EntityID,
		Role:     params.Role,
		Language: language,
	}
	if err := s.repo.InsertBinding(ctx, nil, b); err != nil {
		return nil, err
	}
	return b, nil
}

// UnbindPage 解除页面-实体绑定，未命中返回 ErrBindingNotFound。
func (s *Service) UnbindPage(ctx context.Context, pageID, entityID uuid.UUID, role string) error {
	if role != BindingRolePrimary && role != BindingRoleMentioned {
		return fmt.Errorf("%w: role=%q", ErrInvalidBindingRole, role)
	}
	return s.repo.DeleteBinding(ctx, nil, pageID, entityID, role)
}

// lockActiveEntity 在事务内锁定实体行并要求其处于 active 状态。
// merged 返回 ErrEntityMerged；deleted 视同不存在返回 ErrEntityNotFound。
func (s *Service) lockActiveEntity(ctx context.Context, tx pgx.Tx, entityID uuid.UUID) (*Entity, error) {
	e, err := s.repo.GetEntityByIDForUpdate(ctx, tx, entityID)
	if err != nil {
		return nil, err
	}
	if err := ensureEntityActive(e); err != nil {
		return nil, err
	}
	return e, nil
}

// ensureEntityActive 实体写入守卫：merged 拒绝写入，deleted 视同不存在。
func ensureEntityActive(e *Entity) error {
	switch e.Status {
	case StatusActive:
		return nil
	case StatusMerged:
		return fmt.Errorf("%w: id=%s merged_into=%s", ErrEntityMerged, e.ID, e.MergedIntoEntityID)
	default:
		return fmt.Errorf("%w: id=%s 状态 %q", ErrEntityNotFound, e.ID, e.Status)
	}
}

// normalizeLabels 校验并规范化创建入参的标签列表：
// 至少一个标签、至少一个主标签、同语言至多一个主标签、(language, label) 不重复。
func normalizeLabels(inputs []LabelInput) ([]EntityLabel, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("%w: 创建实体必须提供标签", ErrNoPrimaryLabel)
	}
	labels := make([]EntityLabel, 0, len(inputs))
	seen := make(map[string]bool)
	primaryLangs := make(map[string]bool)
	hasPrimary := false
	for _, input := range inputs {
		l, err := normalizeLabel(input)
		if err != nil {
			return nil, err
		}
		dupKey := l.Language + "\x00" + l.Label
		if seen[dupKey] {
			return nil, fmt.Errorf("%w: language=%q label=%q", ErrLabelExists, l.Language, l.Label)
		}
		seen[dupKey] = true
		if l.IsPrimary {
			hasPrimary = true
			if primaryLangs[l.Language] {
				return nil, fmt.Errorf("%w: language=%q", ErrDuplicatePrimaryLabel, l.Language)
			}
			primaryLangs[l.Language] = true
		}
		labels = append(labels, *l)
	}
	if !hasPrimary {
		return nil, fmt.Errorf("%w: 创建实体必须指定主标签", ErrNoPrimaryLabel)
	}
	return labels, nil
}

// normalizeLabel 校验并规范化单个标签入参。
func normalizeLabel(input LabelInput) (*EntityLabel, error) {
	display, err := displayLabel(input.Label)
	if err != nil {
		return nil, err
	}
	language, err := normalizeLanguage(input.Language)
	if err != nil {
		return nil, err
	}
	return &EntityLabel{
		Language:    language,
		Label:       display,
		Description: input.Description,
		IsPrimary:   input.IsPrimary,
	}, nil
}

// normalizeLanguage 语言标记只做 trim + 非空校验（保留 zh-Hans 等 BCP47 书写）。
func normalizeLanguage(language string) (string, error) {
	language = strings.TrimSpace(language)
	if language == "" {
		return "", fmt.Errorf("%w: 语言标记为空", ErrInvalidLabel)
	}
	return language, nil
}

// firstPrimaryLabel 返回首个主标签文本（CreateEntity 缺省 canonical_key 的推导来源）。
func firstPrimaryLabel(labels []EntityLabel) string {
	for _, l := range labels {
		if l.IsPrimary {
			return l.Label
		}
	}
	return ""
}
