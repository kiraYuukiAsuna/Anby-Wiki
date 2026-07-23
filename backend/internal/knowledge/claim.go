package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
)

// Claim 领域服务：Property/Claim/ClaimSource 的唯一权威写入入口（M4-T03）。
// 与 Entity 服务共用 Service 装配（repo/pages/txm/ids），风格约定相同：
// 跨行写入在 db.TxManager 单事务内完成，并发约束依赖行锁 + 服务层前置检查。
//
// 状态分离（设计 §6.5）：业务状态（Status）与验证状态（VerificationStatus）正交，
// 分别由 transitionClaim / UpdateVerificationStatus 两条路径维护。

// CreateClaimParams 创建 Claim 的入参。
// Rank 空时用 RankNormal；Qualifiers 空时落库 '{}'，非空必须是 JSON object；
// ValidFrom/ValidTo 双开区间，两端都有时必须 valid_to > valid_from。
type CreateClaimParams struct {
	SubjectEntityID uuid.UUID
	PropertyKey     string
	Value           Value
	Qualifiers      json.RawMessage
	Rank            string
	ValidFrom       *time.Time
	ValidTo         *time.Time
	OriginType      string
	ActorID         uuid.UUID
	ChangeBatchID   *uuid.UUID
}

// CreateClaim 创建 Claim。校验链：Actor 准入 → subject 存在且 active（行锁）→
// property 存在 → subject_type/target_type 约束（列 + schema_json 子集）→
// 值形态按 value_type 校验 → rank/origin/有效时间 → 单值约束。
//
// 初始业务状态：origin human → published；ai/import → proposed
// （M5 治理落地前的人工预放行，治理后经 Proposal 审核收紧）。
// 初始验证状态恒为 unverified。
//
// Actor 准入复用 page.CheckWriteActor（human/bot/system 可写，ai actor 拒绝）：
// ai/import 来源的 Claim 由 bot/system 管道 Actor 录入，与 page 模块的
// M5 前准入立场一致（INV-05/INV-08 的 AI 直改限制在 M5-T06/T10 落地）。
func (s *Service) CreateClaim(ctx context.Context, params CreateClaimParams) (*Claim, error) {
	var claim *Claim
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		claim, err = s.CreateClaimInTx(ctx, tx, params)
		return err
	})
	if err != nil {
		return nil, err
	}
	return claim, nil
}

// CreateClaimInTx 供 Governance Apply 组合事务；值校验、单值约束与状态初始化
// 均复用本服务实现，不向治理层暴露 Repository。
func (s *Service) CreateClaimInTx(ctx context.Context, tx pgx.Tx, params CreateClaimParams) (*Claim, error) {
	if err := s.pages.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
		return nil, err
	}
	claimID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	claim, err := s.prepareClaim(ctx, tx, params, claimID, false)
	if err != nil {
		return nil, err
	}
	if err := s.repo.InsertClaim(ctx, tx, claim); err != nil {
		return nil, err
	}
	if err := s.emitClaimChanged(ctx, tx, claim.ID, claim.SubjectEntityID, nil); err != nil {
		return nil, err
	}
	return claim, nil
}

func (s *Service) emitClaimChanged(
	ctx context.Context,
	tx pgx.Tx,
	claimID, subjectEntityID uuid.UUID,
	replacementClaimID *uuid.UUID,
) error {
	eventID, err := s.ids.New()
	if err != nil {
		return err
	}
	return s.repo.InsertClaimChangedEvent(
		ctx, tx, eventID, claimID, subjectEntityID, replacementClaimID,
	)
}

// prepareClaim 在事务内完成 subject 行锁与全部创建校验，返回待插入的 Claim。
// skipSingleValueCheck 仅 SupersedeClaim 使用：新 claim 先于旧 claim 置 superseded
// 插入（superseded_by 外键要求被指向的 claim 已存在），此时旧值仍是 published，
// 单值检查必须跳过——Supersede 语义本身就是替换该 published 值。
func (s *Service) prepareClaim(ctx context.Context, tx pgx.Tx, params CreateClaimParams, claimID uuid.UUID, skipSingleValueCheck bool) (*Claim, error) {
	subject, err := s.lockActiveEntity(ctx, tx, params.SubjectEntityID)
	if err != nil {
		return nil, err
	}
	prop, err := s.repo.GetPropertyByKey(ctx, tx, params.PropertyKey)
	if err != nil {
		return nil, err
	}
	schema, err := parsePropertySchema(prop.SchemaJSON)
	if err != nil {
		return nil, err
	}

	// subject 类型约束：列约束与 schema_json.subject_type（type_key）并列校验。
	if prop.SubjectTypeID != nil && subject.EntityTypeID != *prop.SubjectTypeID {
		return nil, fmt.Errorf("%w: property=%q subject=%s 类型 %s，期望 %s",
			ErrSubjectTypeMismatch, prop.PropertyKey, subject.ID, subject.EntityTypeID, *prop.SubjectTypeID)
	}
	if err := s.checkTypeKeyConstraint(ctx, tx, schema.SubjectType, subject.EntityTypeID, prop.PropertyKey, ErrSubjectTypeMismatch); err != nil {
		return nil, err
	}

	valueJSON, targetEntityID, err := normalizeValue(prop, params.Value)
	if err != nil {
		return nil, err
	}

	// entity 值：目标实体必须存在且 active，类型匹配 target_type 约束。
	if targetEntityID != nil {
		target, err := s.repo.GetEntityByID(ctx, tx, *targetEntityID)
		if err != nil {
			return nil, err
		}
		if err := ensureEntityActive(target); err != nil {
			return nil, err
		}
		if prop.TargetTypeID != nil && target.EntityTypeID != *prop.TargetTypeID {
			return nil, fmt.Errorf("%w: property=%q target=%s 类型 %s，期望 %s",
				ErrTargetTypeMismatch, prop.PropertyKey, target.ID, target.EntityTypeID, *prop.TargetTypeID)
		}
		if err := s.checkTypeKeyConstraint(ctx, tx, schema.TargetType, target.EntityTypeID, prop.PropertyKey, ErrTargetTypeMismatch); err != nil {
			return nil, err
		}
	}

	rank := params.Rank
	if rank == "" {
		rank = RankNormal
	}
	switch rank {
	case RankPreferred, RankNormal, RankDeprecated:
	default:
		return nil, fmt.Errorf("%w: rank=%q", ErrInvalidRank, rank)
	}

	var status string
	switch params.OriginType {
	case OriginHuman:
		status = ClaimStatusPublished
	case OriginAI, OriginImport:
		status = ClaimStatusProposed
	default:
		return nil, fmt.Errorf("%w: origin_type=%q", ErrInvalidOriginType, params.OriginType)
	}

	if params.ValidFrom != nil && params.ValidTo != nil && !params.ValidTo.After(*params.ValidFrom) {
		return nil, fmt.Errorf("%w: valid_from=%s valid_to=%s",
			ErrInvalidValidTime, params.ValidFrom, params.ValidTo)
	}

	qualifiers := params.Qualifiers
	if len(qualifiers) == 0 {
		qualifiers = json.RawMessage(`{}`)
	} else {
		var obj map[string]json.RawMessage
		if uerr := json.Unmarshal(qualifiers, &obj); uerr != nil || obj == nil {
			return nil, fmt.Errorf("%w: qualifiers 必须是 JSON object", ErrInvalidClaimValue)
		}
	}

	// 单值约束：同 subject+property 已有 published claim 时拒绝，
	// 提示调用方改用 SupersedeClaim。计数在 subject 行锁内，序列化并发创建。
	if !prop.IsMultivalued && !skipSingleValueCheck {
		n, err := s.repo.CountPublishedClaims(ctx, tx, subject.ID, prop.ID)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, fmt.Errorf("%w: subject=%s property=%q", ErrClaimNotMultivalued, subject.ID, prop.PropertyKey)
		}
	}

	return &Claim{
		ID:                 claimID,
		SubjectEntityID:    subject.ID,
		PropertyID:         prop.ID,
		ValueType:          prop.ValueType,
		ValueJSON:          valueJSON,
		TargetEntityID:     targetEntityID,
		QualifiersJSON:     qualifiers,
		Rank:               rank,
		Status:             status,
		VerificationStatus: VerificationUnverified,
		ValidFrom:          params.ValidFrom,
		ValidTo:            params.ValidTo,
		OriginType:         params.OriginType,
		ChangeBatchID:      params.ChangeBatchID,
		CreatedBy:          params.ActorID,
	}, nil
}

// checkTypeKeyConstraint 按 schema_json 中的 type_key 约束校验实体类型
// （000004 种子注释：类型约束由服务层按 schema_json 校验）。
// typeKey 为空表示无约束；约束引用的 type_key 未种子按不匹配处理（保守拒绝）。
func (s *Service) checkTypeKeyConstraint(ctx context.Context, tx pgx.Tx, typeKey string, actualTypeID uuid.UUID, propertyKey string, sentinel error) error {
	if typeKey == "" {
		return nil
	}
	want, err := s.repo.GetEntityTypeByKey(ctx, tx, typeKey)
	if err != nil {
		return fmt.Errorf("%w: property=%q 约束 type_key=%q 解析失败: %v", sentinel, propertyKey, typeKey, err)
	}
	if actualTypeID != want.ID {
		return fmt.Errorf("%w: property=%q 实体类型 %s，期望 type_key=%q", sentinel, propertyKey, actualTypeID, typeKey)
	}
	return nil
}

// SupersedeClaimParams Supersede 链入参。
// SubjectEntityID/PropertyKey 是对旧 claim 的乐观断言：与旧 claim 实际值
// 不符返回 ErrClaimSubjectMismatch（防止调用方基于过期认知替换值）。
// OriginType 空时继承旧 claim 的 origin_type。
type SupersedeClaimParams struct {
	ClaimID         uuid.UUID
	SubjectEntityID uuid.UUID
	PropertyKey     string
	Value           Value
	Qualifiers      json.RawMessage
	Rank            string
	ValidFrom       *time.Time
	ValidTo         *time.Time
	OriginType      string
	ActorID         uuid.UUID
	ChangeBatchID   *uuid.UUID
}

// SupersedeClaim 以新值替换旧 Claim（单事务）：
// 旧 claim status→superseded 且 superseded_by 指向新 claim，同时按
// CreateClaim 同款校验创建新 claim。旧 claim 必须处于 published
// （状态机只允许 published→superseded），已 superseded/deprecated/rejected
// 的再 supersede 返回 ErrInvalidClaimTransition。旧 claim 保留可审计，
// 任何校验失败整体回滚。
//
// 并发：旧 claim 行锁（FOR UPDATE）序列化同一 claim 的并发 supersede，
// 后到者看到 superseded 状态被拒绝，恰一成功。
func (s *Service) SupersedeClaim(ctx context.Context, params SupersedeClaimParams) (*Claim, error) {
	var newClaim *Claim
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		newClaim, err = s.SupersedeClaimInTx(ctx, tx, params)
		return err
	})
	if err != nil {
		return nil, err
	}
	return newClaim, nil
}

// SupersedeClaimInTx 是 Supersede 的事务内领域入口。
func (s *Service) SupersedeClaimInTx(ctx context.Context, tx pgx.Tx, params SupersedeClaimParams) (*Claim, error) {
	if err := s.pages.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
		return nil, err
	}
	old, err := s.repo.GetClaimByIDForUpdate(ctx, tx, params.ClaimID)
	if err != nil {
		return nil, err
	}
	if old.SubjectEntityID != params.SubjectEntityID {
		return nil, fmt.Errorf("%w: claim=%s 断言 subject=%s，实际 %s",
			ErrClaimSubjectMismatch, old.ID, params.SubjectEntityID, old.SubjectEntityID)
	}
	prop, err := s.repo.GetPropertyByKey(ctx, tx, params.PropertyKey)
	if err != nil {
		return nil, err
	}
	if old.PropertyID != prop.ID {
		return nil, fmt.Errorf("%w: claim=%s 断言 property=%q，实际 property_id=%s",
			ErrClaimSubjectMismatch, old.ID, params.PropertyKey, old.PropertyID)
	}
	if !canTransitionClaim(old.Status, ClaimStatusSuperseded) {
		return nil, fmt.Errorf("%w: claim=%s 状态 %q 不可 supersede", ErrInvalidClaimTransition, old.ID, old.Status)
	}

	origin := params.OriginType
	if origin == "" {
		origin = old.OriginType
	}
	newID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	// 先按 CreateClaim 同款校验创建新 claim，再把旧 claim 置 superseded
	// 指向它（superseded_by 外键要求新 claim 已存在；单事务保证原子，
	// 任何一步失败整体回滚，旧 claim 不变）。
	// 新 claim 插入时旧值仍是 published，故跳过单值检查——Supersede
	// 语义本身就是替换该 published 值。
	c, err := s.prepareClaim(ctx, tx, CreateClaimParams{
		SubjectEntityID: old.SubjectEntityID,
		PropertyKey:     params.PropertyKey,
		Value:           params.Value,
		Qualifiers:      params.Qualifiers,
		Rank:            params.Rank,
		ValidFrom:       params.ValidFrom,
		ValidTo:         params.ValidTo,
		OriginType:      origin,
		ActorID:         params.ActorID,
		ChangeBatchID:   params.ChangeBatchID,
	}, newID, true)
	if err != nil {
		return nil, err
	}
	if err := s.repo.InsertClaim(ctx, tx, c); err != nil {
		return nil, err
	}
	if err := s.repo.SetClaimSuperseded(ctx, tx, old.ID, newID); err != nil {
		return nil, err
	}
	if err := s.emitClaimChanged(ctx, tx, old.ID, old.SubjectEntityID, &newID); err != nil {
		return nil, err
	}
	return c, nil
}

// transitionClaim 业务状态流转的公共实现：行锁内校验状态机后落库。
// 目标为 published 时补单值不变量：单值 property 同 subject 已有其他
// published claim 时拒绝（CreateClaim 的创建侧检查管不到 proposed→published 路径）。
// 审核权限（谁可以 Publish/Reject）属 M5 治理，本 Task 只保证状态机正确。
func (s *Service) transitionClaim(ctx context.Context, claimID uuid.UUID, to string) (*Claim, error) {
	var result *Claim
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = s.transitionClaimInTx(ctx, tx, claimID, to)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) transitionClaimInTx(ctx context.Context, tx pgx.Tx, claimID uuid.UUID, to string) (*Claim, error) {
	c, err := s.repo.GetClaimByIDForUpdate(ctx, tx, claimID)
	if err != nil {
		return nil, err
	}
	if !canTransitionClaim(c.Status, to) {
		return nil, fmt.Errorf("%w: claim=%s %q→%q", ErrInvalidClaimTransition, claimID, c.Status, to)
	}
	if to == ClaimStatusPublished {
		prop, err := s.repo.GetPropertyByID(ctx, tx, c.PropertyID)
		if err != nil {
			return nil, err
		}
		if !prop.IsMultivalued {
			if _, err := s.lockActiveEntity(ctx, tx, c.SubjectEntityID); err != nil {
				return nil, err
			}
			n, err := s.repo.CountPublishedClaims(ctx, tx, c.SubjectEntityID, c.PropertyID)
			if err != nil {
				return nil, err
			}
			if n > 0 {
				return nil, fmt.Errorf("%w: subject=%s property=%q 已有 published claim，应先 Supersede",
					ErrClaimNotMultivalued, c.SubjectEntityID, prop.PropertyKey)
			}
		}
	}
	if err := s.repo.UpdateClaimStatus(ctx, tx, claimID, to); err != nil {
		return nil, err
	}
	c.Status = to
	if err := s.emitClaimChanged(ctx, tx, c.ID, c.SubjectEntityID, nil); err != nil {
		return nil, err
	}
	return c, nil
}

// PublishClaimInTx 供 Governance Apply 把已审核的 ai/import proposed Claim 正式发布。
func (s *Service) PublishClaimInTx(ctx context.Context, tx pgx.Tx, claimID uuid.UUID) (*Claim, error) {
	c, err := s.repo.GetClaimByID(ctx, tx, claimID)
	if err != nil {
		return nil, err
	}
	if c.Status == ClaimStatusPublished {
		return c, nil
	}
	return s.transitionClaimInTx(ctx, tx, claimID, ClaimStatusPublished)
}

// RollbackClaimInTx 以状态流转/新 Supersede 补偿由 ChangeBatch 创建的 Claim，
// 不删除或回写历史 Claim。返回补偿后代表当前语义的 Claim；新建 Claim 的撤销
// 返回已 deprecated/rejected 的原 Claim。
func (s *Service) RollbackClaimInTx(ctx context.Context, tx pgx.Tx, claimID, actorID uuid.UUID, batchID *uuid.UUID) (*Claim, error) {
	current, err := s.repo.GetClaimByIDForUpdate(ctx, tx, claimID)
	if err != nil {
		return nil, err
	}
	predecessor, err := s.repo.GetClaimPredecessor(ctx, tx, claimID)
	if err != nil {
		return nil, err
	}
	if predecessor == nil {
		switch current.Status {
		case ClaimStatusPublished:
			return s.transitionClaimInTx(ctx, tx, claimID, ClaimStatusDeprecated)
		case ClaimStatusProposed:
			return s.transitionClaimInTx(ctx, tx, claimID, ClaimStatusRejected)
		default:
			return nil, fmt.Errorf("%w: rollback claim=%s status=%s", ErrInvalidClaimTransition, claimID, current.Status)
		}
	}
	prop, err := s.repo.GetPropertyByID(ctx, tx, predecessor.PropertyID)
	if err != nil {
		return nil, err
	}
	value, err := storedClaimValue(predecessor)
	if err != nil {
		return nil, err
	}
	restored, err := s.SupersedeClaimInTx(ctx, tx, SupersedeClaimParams{
		ClaimID: current.ID, SubjectEntityID: current.SubjectEntityID,
		PropertyKey: prop.PropertyKey, Value: value, Qualifiers: predecessor.QualifiersJSON,
		Rank: predecessor.Rank, ValidFrom: predecessor.ValidFrom, ValidTo: predecessor.ValidTo,
		OriginType: OriginHuman, ActorID: actorID, ChangeBatchID: batchID,
	})
	if err != nil {
		return nil, err
	}
	return s.PublishClaimInTx(ctx, tx, restored.ID)
}

func storedClaimValue(c *Claim) (Value, error) {
	switch c.ValueType {
	case ValueTypeString:
		var v string
		if err := json.Unmarshal(c.ValueJSON, &v); err != nil {
			return Value{}, err
		}
		return StringValue(v), nil
	case ValueTypeNumber:
		var v float64
		if err := json.Unmarshal(c.ValueJSON, &v); err != nil {
			return Value{}, err
		}
		return NumberValue(v), nil
	case ValueTypeDate:
		var v string
		if err := json.Unmarshal(c.ValueJSON, &v); err != nil {
			return Value{}, err
		}
		return DateValue(v), nil
	case ValueTypeEntity:
		if c.TargetEntityID == nil {
			return Value{}, ErrInvalidClaimValue
		}
		return EntityValue(*c.TargetEntityID), nil
	case ValueTypeCoordinate:
		var v Coordinate
		if err := json.Unmarshal(c.ValueJSON, &v); err != nil {
			return Value{}, err
		}
		return CoordinateValue(v.Lat, v.Lon), nil
	case ValueTypeComposite:
		return CompositeValue(c.ValueJSON), nil
	default:
		return Value{}, ErrInvalidClaimValue
	}
}

// PublishClaim proposed→published。
func (s *Service) PublishClaim(ctx context.Context, claimID uuid.UUID) (*Claim, error) {
	return s.transitionClaim(ctx, claimID, ClaimStatusPublished)
}

// RejectClaim proposed→rejected（终态）。
func (s *Service) RejectClaim(ctx context.Context, claimID uuid.UUID) (*Claim, error) {
	return s.transitionClaim(ctx, claimID, ClaimStatusRejected)
}

// DeprecateClaim published→deprecated（终态）。
func (s *Service) DeprecateClaim(ctx context.Context, claimID uuid.UUID) (*Claim, error) {
	return s.transitionClaim(ctx, claimID, ClaimStatusDeprecated)
}

// UpdateVerificationStatusParams 验证状态更新入参。
type UpdateVerificationStatusParams struct {
	ClaimID uuid.UUID
	Status  string
	ActorID uuid.UUID
}

// UpdateVerificationStatus 更新验证状态（与业务状态正交，可在任意业务状态下更新）。
// 权限矩阵（checkVerificationPermission，防御性校验）：human 可置全部四种状态；
// ai 只能置 ai_checked；bot/system 无权修改验证状态。
func (s *Service) UpdateVerificationStatus(ctx context.Context, params UpdateVerificationStatusParams) error {
	actor, err := s.pages.GetActorByID(ctx, nil, params.ActorID)
	if err != nil {
		// 保守失败：查不到（含底层错误）一律按无效 Actor 拒绝写入。
		return fmt.Errorf("%w: id=%s: %v", page.ErrInvalidActor, params.ActorID, err)
	}
	if actor.Status != page.StatusActive {
		return fmt.Errorf("%w: id=%s 状态 %q", page.ErrInvalidActor, params.ActorID, actor.Status)
	}
	if err := checkVerificationPermission(actor.ActorType, params.Status); err != nil {
		return err
	}
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		claim, err := s.repo.GetClaimByIDForUpdate(ctx, tx, params.ClaimID)
		if err != nil {
			return err
		}
		if err := s.repo.UpdateClaimVerificationStatus(ctx, tx, params.ClaimID, params.Status); err != nil {
			return err
		}
		return s.emitClaimChanged(ctx, tx, claim.ID, claim.SubjectEntityID, nil)
	})
}

// CitationChecker citation 存在性只读接口（knowledge 侧定义，evidence 模块实现，
// 装配层经 WithCitationChecker 注入）。定义在消费侧使 knowledge 不 import
// evidence，保持 evidence ← knowledge 零依赖（evidence 也不得 import knowledge）。
type CitationChecker interface {
	CitationExists(ctx context.Context, citationID uuid.UUID) (bool, error)
}

// AddClaimSourceParams 新增 Claim 来源入参。
type AddClaimSourceParams struct {
	ClaimID     uuid.UUID
	CitationID  uuid.UUID
	SupportType string
}

// AddClaimSource 绑定 Claim 与 Citation（INV-07：claim 绑定 citation 位）。
// 校验链：citation_id 非 Nil → support_type 枚举 → claim 存在 → claim 非终态 →
// citation 存在（M4-T05 落地，经注入的 CitationChecker；未注入时跳过，
// DB 外键 claim_source_citation_fk 兜底）。
// 重复添加同 (claim, citation) 返回 ErrClaimSourceExists（幂等拒绝）；
// claim 已 superseded/deprecated 返回 ErrClaimTerminal。
func (s *Service) AddClaimSource(ctx context.Context, params AddClaimSourceParams) (*ClaimSource, error) {
	var source *ClaimSource
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		source, err = s.AddClaimSourceInTx(ctx, tx, params)
		return err
	})
	return source, err
}

// AddClaimSourceInTx 供 Governance Apply 组合事务；CitationChecker 仍执行只读
// 存在性校验，最终外键在同一事务提交时兜底。
func (s *Service) AddClaimSourceInTx(ctx context.Context, tx pgx.Tx, params AddClaimSourceParams) (*ClaimSource, error) {
	if params.CitationID == uuid.Nil {
		return nil, fmt.Errorf("%w: citation_id 为 Nil", ErrInvalidClaimValue)
	}
	if !isValidSupportType(params.SupportType) {
		return nil, fmt.Errorf("%w: support_type=%q", ErrInvalidSupportType, params.SupportType)
	}
	c, err := s.repo.GetClaimByID(ctx, tx, params.ClaimID)
	if err != nil {
		return nil, err
	}
	if c.Status == ClaimStatusSuperseded || c.Status == ClaimStatusDeprecated {
		return nil, fmt.Errorf("%w: claim=%s 状态 %q", ErrClaimTerminal, c.ID, c.Status)
	}
	if s.citations != nil {
		exists, err := s.citations.CitationExists(ctx, params.CitationID)
		if err != nil {
			return nil, fmt.Errorf("knowledge: 校验 citation 存在性失败: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("%w: id=%s", ErrCitationNotFound, params.CitationID)
		}
	}
	src := &ClaimSource{
		ClaimID:     params.ClaimID,
		CitationID:  params.CitationID,
		SupportType: params.SupportType,
	}
	if err := s.repo.InsertClaimSource(ctx, tx, src); err != nil {
		return nil, err
	}
	if err := s.emitClaimChanged(ctx, tx, c.ID, c.SubjectEntityID, nil); err != nil {
		return nil, err
	}
	return src, nil
}

// ListClaimsParams Claim 过滤查询入参。PropertyKey/Status/VerificationStatus
// 为空不过滤；Status 非空必须是合法业务状态，VerificationStatus 同理。
type ListClaimsParams struct {
	SubjectEntityID    uuid.UUID
	PropertyKey        string
	Status             string
	VerificationStatus string
}

// ListClaims 按 subject 过滤查询 Claim（含全部业务状态，可审计）。
func (s *Service) ListClaims(ctx context.Context, params ListClaimsParams) ([]Claim, error) {
	var propertyID *uuid.UUID
	if params.PropertyKey != "" {
		prop, err := s.repo.GetPropertyByKey(ctx, nil, params.PropertyKey)
		if err != nil {
			return nil, err
		}
		propertyID = &prop.ID
	}
	var status *string
	if params.Status != "" {
		switch params.Status {
		case ClaimStatusProposed, ClaimStatusPublished, ClaimStatusRejected,
			ClaimStatusSuperseded, ClaimStatusDeprecated:
		default:
			return nil, fmt.Errorf("%w: status=%q", ErrInvalidClaimStatus, params.Status)
		}
		status = &params.Status
	}
	var verificationStatus *string
	if params.VerificationStatus != "" {
		switch params.VerificationStatus {
		case VerificationUnverified, VerificationAIChecked,
			VerificationHumanVerified, VerificationDisputed:
		default:
			return nil, fmt.Errorf("%w: verification_status=%q", ErrInvalidVerificationStatus, params.VerificationStatus)
		}
		verificationStatus = &params.VerificationStatus
	}
	return s.repo.ListClaims(ctx, nil, params.SubjectEntityID, propertyID, status, verificationStatus)
}

// GetClaim 按 ID 查询 Claim（含全部状态，由调用方判断 Status）。
func (s *Service) GetClaim(ctx context.Context, claimID uuid.UUID) (*Claim, error) {
	return s.repo.GetClaimByID(ctx, nil, claimID)
}

// GetProperty 按稳定 ID 读取 Claim 的谓词定义（详情页只读路径）。
func (s *Service) GetProperty(ctx context.Context, propertyID uuid.UUID) (*Property, error) {
	return s.repo.GetPropertyByID(ctx, nil, propertyID)
}

// GetPropertyByKey 按稳定 property_key 读取谓词定义。
func (s *Service) GetPropertyByKey(ctx context.Context, propertyKey string) (*Property, error) {
	return s.repo.GetPropertyByKey(ctx, nil, propertyKey)
}

// ListClaimSources 列出 Claim 的全部来源（只读）。
func (s *Service) ListClaimSources(ctx context.Context, claimID uuid.UUID) ([]ClaimSource, error) {
	return s.repo.ListClaimSources(ctx, nil, claimID)
}
