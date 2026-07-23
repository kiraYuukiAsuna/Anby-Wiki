package doctor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
)

const (
	CodeCurrentRevisionMissing       = "DOC_CURRENT_REVISION_MISSING"
	CodeCurrentRevisionOwner         = "DOC_CURRENT_REVISION_OWNER_MISMATCH"
	CodeImmutableTriggerMissing      = "DOC_IMMUTABLE_TRIGGER_MISSING"
	CodeContentHashMismatch          = "DOC_CONTENT_HASH_MISMATCH"
	CodeContentSizeMismatch          = "DOC_CONTENT_SIZE_MISMATCH"
	CodeContentInvalid               = "DOC_CONTENT_INVALID"
	CodeSourceChunkHashMismatch      = "EVIDENCE_SOURCE_CHUNK_HASH_MISMATCH"
	CodeCitationQuoteHashMismatch    = "EVIDENCE_CITATION_QUOTATION_HASH_MISMATCH"
	CodePageReferenceOrphan          = "REF_PAGE_ORPHAN"
	CodeEntityReferenceOrphan        = "REF_ENTITY_ORPHAN"
	CodeClaimReferenceOrphan         = "REF_CLAIM_ORPHAN"
	CodeCitationReferenceOrphan      = "REF_CITATION_ORPHAN"
	CodeProjectionStateMissing       = "PROJECTION_STATE_MISSING"
	CodeProjectionStateError         = "PROJECTION_STATE_ERROR"
	CodeProjectionSourceStale        = "PROJECTION_SOURCE_REVISION_STALE"
	CodeProjectionRowSourceStale     = "PROJECTION_ROW_SOURCE_REVISION_STALE"
	CodeSearchSourceStale            = "SEARCH_SOURCE_REVISION_STALE"
	CodeClaimSourceOrphan            = "EVIDENCE_CLAIM_SOURCE_ORPHAN"
	CodeCitationVersionOrphan        = "EVIDENCE_CITATION_VERSION_ORPHAN"
	CodeCitationChunkVersionMismatch = "EVIDENCE_CITATION_CHUNK_VERSION_MISMATCH"
	CodePublishedClaimNoEvidence     = "EVIDENCE_PUBLISHED_CLAIM_WITHOUT_SOURCE"
	CodeOutboxClaimStuck             = "OUTBOX_CLAIM_STUCK"
	CodeOutboxDead                   = "OUTBOX_DEAD"
	CodeLoginAttemptExpired          = "AUTH_LOGIN_ATTEMPT_EXPIRED"
	CodeSessionExpired               = "AUTH_SESSION_EXPIRED"
)

type Options struct {
	Now               time.Time
	ClaimedStuckAfter time.Duration
}

type Checker struct {
	pool *pgxpool.Pool
	opts Options
}

func New(pool *pgxpool.Pool, opts Options) *Checker {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.ClaimedStuckAfter <= 0 {
		opts.ClaimedStuckAfter = 5 * time.Minute
	}
	return &Checker{pool: pool, opts: opts}
}

func (c *Checker) Run(ctx context.Context) (Report, error) {
	var issues []Issue
	checks := []func(context.Context, *[]Issue) error{
		c.checkCurrentRevisions,
		c.checkImmutableTriggers,
		c.checkContent,
		c.checkEvidenceContentHashes,
		c.checkCurrentASTReferences,
		c.checkProjectionStates,
		c.checkProjectionRows,
		c.checkEvidence,
		c.checkOutbox,
		c.checkExpiredAuth,
	}
	for _, check := range checks {
		if err := check(ctx, &issues); err != nil {
			return Report{}, err
		}
	}
	return NewReport(c.opts.Now, true, issues), nil
}

func appendIssue(issues *[]Issue, code string, severity Severity, category, message, resourceType, resourceID, recommendation string, details map[string]string) {
	*issues = append(*issues, Issue{
		Code: code, Severity: severity, Category: category, Message: message,
		ResourceType: resourceType, ResourceID: resourceID, Recommendation: recommendation, Details: details,
	})
}

func (c *Checker) checkCurrentRevisions(ctx context.Context, issues *[]Issue) error {
	rows, err := c.pool.Query(ctx, `
		SELECT p.id, p.current_revision_id, r.id, r.page_id
		FROM page p
		LEFT JOIN revision r ON r.id = p.current_revision_id
		WHERE p.current_revision_id IS NOT NULL
		  AND (r.id IS NULL OR r.page_id IS DISTINCT FROM p.id)
		ORDER BY p.id`)
	if err != nil {
		return fmt.Errorf("doctor: 检查 current_revision 失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pageID, currentID uuid.UUID
		var revisionID, ownerID *uuid.UUID
		if err := rows.Scan(&pageID, &currentID, &revisionID, &ownerID); err != nil {
			return err
		}
		if revisionID == nil {
			appendIssue(issues, CodeCurrentRevisionMissing, SeverityCritical, "document",
				"页面 current_revision_id 指向不存在的 Revision", "page", pageID.String(),
				"通过 Proposal 或 Page 领域服务恢复正确指针；禁止 doctor 直接修改权威数据",
				map[string]string{"current_revision_id": currentID.String()})
		} else {
			appendIssue(issues, CodeCurrentRevisionOwner, SeverityCritical, "document",
				"页面 current_revision_id 指向其他页面的 Revision", "page", pageID.String(),
				"通过 Proposal 或 Page 领域服务恢复正确指针；检查发布事务与触发器",
				map[string]string{"current_revision_id": currentID.String(), "revision_page_id": ownerID.String()})
		}
	}
	return rows.Err()
}

func (c *Checker) checkImmutableTriggers(ctx context.Context, issues *[]Issue) error {
	expected := map[string]string{
		"revision": "revision_immutable", "content_snapshot": "content_snapshot_immutable",
		"audit_event": "audit_event_immutable", "asset_revision": "asset_revision_immutable",
		"source_version": "source_version_immutable", "source_chunk": "source_chunk_immutable",
		"citation": "citation_immutable", "import_extraction": "import_extraction_immutable",
	}
	for table, trigger := range expected {
		var exists bool
		if err := c.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_trigger t
				JOIN pg_class c ON c.oid=t.tgrelid
				JOIN pg_namespace n ON n.oid=c.relnamespace
				WHERE n.nspname=current_schema() AND c.relname=$1
				  AND t.tgname=$2 AND NOT t.tgisinternal AND t.tgenabled <> 'D'
			)`, table, trigger).Scan(&exists); err != nil {
			return fmt.Errorf("doctor: 检查不可变触发器 %s 失败: %w", trigger, err)
		}
		if !exists {
			appendIssue(issues, CodeImmutableTriggerMissing, SeverityCritical, "immutability",
				"不可变表触发器缺失或被禁用", "table", table,
				"停止权威写入并恢复已审核 Schema；doctor 不自动创建触发器",
				map[string]string{"trigger": trigger})
		}
	}
	return nil
}

func (c *Checker) checkContent(ctx context.Context, issues *[]Issue) error {
	rows, err := c.pool.Query(ctx, `SELECT id, ast_json::text, content_hash, size_bytes FROM content_snapshot ORDER BY id`)
	if err != nil {
		return fmt.Errorf("doctor: 读取内容快照失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		var storedHash string
		var storedSize int
		if err := rows.Scan(&id, &raw, &storedHash, &storedSize); err != nil {
			return err
		}
		canonical, err := ast.CanonicalizeJSON(raw)
		if err != nil {
			appendIssue(issues, CodeContentInvalid, SeverityCritical, "immutability",
				"内容快照不是可规范化的 AST JSON", "content_snapshot", id.String(),
				"隔离受影响 Revision，并通过领域服务发布替代 Revision", nil)
			continue
		}
		sum := sha256.Sum256(canonical)
		actualHash := hex.EncodeToString(sum[:])
		if actualHash != storedHash {
			appendIssue(issues, CodeContentHashMismatch, SeverityCritical, "immutability",
				"内容快照哈希与 canonical AST 不一致", "content_snapshot", id.String(),
				"隔离受影响 Revision；审计写入链路并通过 Proposal/领域服务发布替代内容",
				map[string]string{"stored_hash": storedHash, "actual_hash": actualHash})
		}
		if len(canonical) != storedSize {
			appendIssue(issues, CodeContentSizeMismatch, SeverityError, "immutability",
				"内容快照 size_bytes 与 canonical AST 不一致", "content_snapshot", id.String(),
				"审计发布链路并通过领域服务生成替代 Revision",
				map[string]string{"stored_size": strconv.Itoa(storedSize), "actual_size": strconv.Itoa(len(canonical))})
		}
	}
	return rows.Err()
}

func (c *Checker) checkEvidenceContentHashes(ctx context.Context, issues *[]Issue) error {
	rows, err := c.pool.Query(ctx, `SELECT id,text_content,text_hash FROM source_chunk ORDER BY id`)
	if err != nil {
		return fmt.Errorf("doctor: 读取 SourceChunk 哈希失败: %w", err)
	}
	for rows.Next() {
		var id uuid.UUID
		var content, stored string
		if err := rows.Scan(&id, &content, &stored); err != nil {
			rows.Close()
			return err
		}
		sum := sha256.Sum256([]byte(content))
		actual := hex.EncodeToString(sum[:])
		if actual != stored {
			appendIssue(issues, CodeSourceChunkHashMismatch, SeverityCritical, "immutability",
				"SourceChunk 文本哈希与内容不一致", "source_chunk", id.String(),
				"隔离相关 Citation，并通过 Evidence 领域服务创建替代证据链",
				map[string]string{"stored_hash": stored, "actual_hash": actual})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = c.pool.Query(ctx, `
		SELECT id,quotation,quotation_hash FROM citation
		WHERE quotation IS NOT NULL OR quotation_hash IS NOT NULL
		ORDER BY id`)
	if err != nil {
		return fmt.Errorf("doctor: 读取 Citation quotation 哈希失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var quotation, stored *string
		if err := rows.Scan(&id, &quotation, &stored); err != nil {
			return err
		}
		mismatch := quotation == nil || stored == nil
		actual := ""
		if quotation != nil {
			sum := sha256.Sum256([]byte(*quotation))
			actual = hex.EncodeToString(sum[:])
			mismatch = stored == nil || actual != *stored
		}
		if mismatch {
			details := map[string]string{"actual_hash": actual}
			if stored != nil {
				details["stored_hash"] = *stored
			}
			appendIssue(issues, CodeCitationQuoteHashMismatch, SeverityCritical, "immutability",
				"Citation quotation 与 quotation_hash 不一致", "citation", id.String(),
				"通过 Evidence 领域服务创建替代 Citation，并经 Proposal 更新引用", details)
		}
	}
	return rows.Err()
}

type astReference struct {
	code, table, kind, id string
}

func (c *Checker) checkCurrentASTReferences(ctx context.Context, issues *[]Issue) error {
	rows, err := c.pool.Query(ctx, `
		SELECT p.id, cs.ast_json
		FROM page p
		JOIN revision r ON r.id=p.current_revision_id
		JOIN content_snapshot cs ON cs.id=r.content_snapshot_id
		WHERE p.deleted_at IS NULL
		ORDER BY p.id`)
	if err != nil {
		return fmt.Errorf("doctor: 读取 Current AST 失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pageID uuid.UUID
		var raw []byte
		if err := rows.Scan(&pageID, &raw); err != nil {
			return err
		}
		doc, err := ast.Parse(raw)
		if err != nil {
			continue // 内容错误已由 checkContent 报告。
		}
		var refs []astReference
		if err := ast.Walk(doc, func(node ast.WalkNode) bool {
			if node.Inline == nil {
				return true
			}
			switch node.Inline.Type {
			case ast.InlinePageReference:
				if node.Inline.TargetPageID != "" {
					refs = append(refs, astReference{CodePageReferenceOrphan, "page", "Page", node.Inline.TargetPageID})
				}
			case ast.InlineEntityReference:
				refs = append(refs, astReference{CodeEntityReferenceOrphan, "entity", "Entity", node.Inline.EntityID})
			case ast.InlineClaimReference:
				refs = append(refs, astReference{CodeClaimReferenceOrphan, "claim", "Claim", node.Inline.ClaimID})
			case ast.InlineCitationReference:
				refs = append(refs, astReference{CodeCitationReferenceOrphan, "citation", "Citation", node.Inline.CitationID})
			}
			return true
		}); err != nil {
			return err
		}
		for _, ref := range refs {
			id, err := uuid.Parse(ref.id)
			if err != nil {
				continue // Schema 校验负责格式；这里专注存在性。
			}
			var exists bool
			query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE id=$1)`, ref.table)
			if err := c.pool.QueryRow(ctx, query, id).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				appendIssue(issues, ref.code, SeverityError, "reference",
					"Current AST 引用了不存在的 "+ref.kind, "page", pageID.String(),
					"通过 Proposal 移除或替换悬空引用；禁止直接修改快照",
					map[string]string{"target_id": ref.id})
			}
		}
	}
	return rows.Err()
}

var projectionTypes = []string{
	"page_links", "document_outline", "rendered_page", "external_links",
	"entity_mentions", "component_dependency", "claim_usage", "citation_usage", "search",
}

func (c *Checker) checkProjectionStates(ctx context.Context, issues *[]Issue) error {
	rows, err := c.pool.Query(ctx, `
		SELECT p.id, p.current_revision_id, expected.projection_type,
		       ps.status, ps.source_revision_id
		FROM page p
		CROSS JOIN unnest($1::text[]) expected(projection_type)
		LEFT JOIN projection_state ps
		  ON ps.aggregate_type='page' AND ps.aggregate_id=p.id
		 AND ps.projection_type=expected.projection_type
		WHERE p.deleted_at IS NULL AND p.current_revision_id IS NOT NULL
		  AND (ps.aggregate_id IS NULL OR ps.status <> 'ok'
		       OR ps.source_revision_id IS DISTINCT FROM p.current_revision_id)
		ORDER BY p.id, expected.projection_type`, projectionTypes)
	if err != nil {
		return fmt.Errorf("doctor: 检查 projection_state 失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pageID, currentID uuid.UUID
		var projectionType string
		var status *string
		var sourceID *uuid.UUID
		if err := rows.Scan(&pageID, &currentID, &projectionType, &status, &sourceID); err != nil {
			return err
		}
		code, message := CodeProjectionSourceStale, "投影状态来源 Revision 不是页面 Current Revision"
		if status == nil {
			code, message = CodeProjectionStateMissing, "页面缺少投影状态"
		} else if *status != "ok" {
			code, message = CodeProjectionStateError, "投影状态为 error"
		}
		details := map[string]string{"projection_type": projectionType, "current_revision_id": currentID.String()}
		if sourceID != nil {
			details["source_revision_id"] = sourceID.String()
		}
		appendIssue(issues, code, SeverityError, "projection", message, "page", pageID.String(),
			"确认权威数据健康后运行 worker -rebuild-page <page-id>；可重建投影，不修改权威数据", details)
	}
	return rows.Err()
}

func (c *Checker) checkProjectionRows(ctx context.Context, issues *[]Issue) error {
	queries := []struct {
		name, sql, code string
	}{
		{"page_links", `SELECT DISTINCT source_page_id,source_revision_id FROM page_link_projection pl JOIN page p ON p.id=pl.source_page_id WHERE pl.source_revision_id IS DISTINCT FROM p.current_revision_id`, CodeProjectionRowSourceStale},
		{"document_outline", `SELECT DISTINCT op.page_id,op.revision_id FROM document_outline_projection op JOIN page p ON p.id=op.page_id WHERE op.revision_id IS DISTINCT FROM p.current_revision_id`, CodeProjectionRowSourceStale},
		{"rendered_page", `SELECT rp.page_id,rp.revision_id FROM rendered_page rp JOIN page p ON p.id=rp.page_id WHERE rp.revision_id IS DISTINCT FROM p.current_revision_id`, CodeProjectionRowSourceStale},
		{"external_links", `SELECT DISTINCT eu.page_id,eu.revision_id FROM external_link_usage eu JOIN page p ON p.id=eu.page_id WHERE eu.revision_id IS DISTINCT FROM p.current_revision_id`, CodeProjectionRowSourceStale},
		{"entity_mentions", `SELECT DISTINCT ep.page_id,ep.revision_id FROM entity_mention_projection ep JOIN page p ON p.id=ep.page_id WHERE ep.revision_id IS DISTINCT FROM p.current_revision_id`, CodeProjectionRowSourceStale},
		{"component_dependency", `SELECT DISTINCT cd.page_id,cd.revision_id FROM component_dependency cd JOIN page p ON p.id=cd.page_id WHERE cd.revision_id IS DISTINCT FROM p.current_revision_id`, CodeProjectionRowSourceStale},
		{"claim_usage", `SELECT DISTINCT cu.page_id,cu.revision_id FROM claim_usage cu JOIN page p ON p.id=cu.page_id WHERE cu.revision_id IS DISTINCT FROM p.current_revision_id`, CodeProjectionRowSourceStale},
		{"citation_usage", `SELECT DISTINCT cu.page_id,cu.revision_id FROM citation_usage cu JOIN page p ON p.id=cu.page_id WHERE cu.revision_id IS DISTINCT FROM p.current_revision_id`, CodeProjectionRowSourceStale},
		{"search", `SELECT sd.page_id,sd.source_revision_id FROM search_document sd JOIN page p ON p.id=sd.page_id WHERE sd.source_revision_id IS DISTINCT FROM p.current_revision_id`, CodeSearchSourceStale},
	}
	for _, check := range queries {
		rows, err := c.pool.Query(ctx, check.sql)
		if err != nil {
			return fmt.Errorf("doctor: 检查 %s 来源 Revision 失败: %w", check.name, err)
		}
		for rows.Next() {
			var pageID, sourceID uuid.UUID
			if err := rows.Scan(&pageID, &sourceID); err != nil {
				rows.Close()
				return err
			}
			appendIssue(issues, check.code, SeverityError, "projection",
				"投影数据行来源 Revision 已陈旧", "page", pageID.String(),
				"确认权威数据健康后运行 worker -rebuild-page <page-id>",
				map[string]string{"projection_type": check.name, "source_revision_id": sourceID.String()})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	return nil
}

func (c *Checker) checkEvidence(ctx context.Context, issues *[]Issue) error {
	checks := []struct {
		code, message, resourceType, recommendation, sql string
		severity                                         Severity
	}{
		{CodeClaimSourceOrphan, "ClaimSource 的 Claim 或 Citation 不存在", "claim_source",
			"通过 Claim/Citation 领域服务重建证据关联", `
			SELECT cs.claim_id::text || ':' || cs.citation_id::text
			FROM claim_source cs LEFT JOIN claim c ON c.id=cs.claim_id
			LEFT JOIN citation ci ON ci.id=cs.citation_id
			WHERE c.id IS NULL OR ci.id IS NULL`, SeverityCritical},
		{CodeCitationVersionOrphan, "Citation 的 SourceVersion 或 Source 不存在", "citation",
			"通过 Evidence 领域服务创建替代 Citation，并经 Proposal 更新引用", `
			SELECT ci.id::text FROM citation ci
			LEFT JOIN source_version sv ON sv.id=ci.source_version_id
			LEFT JOIN source s ON s.id=sv.source_id
			WHERE sv.id IS NULL OR s.id IS NULL`, SeverityCritical},
		{CodeCitationChunkVersionMismatch, "Citation 的 SourceChunk 不属于同一 SourceVersion", "citation",
			"通过 Evidence 领域服务创建正确 Citation，并经 Proposal 替换旧引用", `
			SELECT ci.id::text FROM citation ci JOIN source_chunk sc ON sc.id=ci.source_chunk_id
			WHERE sc.source_version_id <> ci.source_version_id`, SeverityError},
		{CodePublishedClaimNoEvidence, "已发布 Claim 没有 ClaimSource 证据", "claim",
			"通过 Proposal 和 Claim 领域服务补充 Citation 证据链", `
			SELECT c.id::text FROM claim c LEFT JOIN claim_source cs ON cs.claim_id=c.id
			WHERE c.status='published' GROUP BY c.id HAVING count(cs.citation_id)=0`, SeverityWarning},
	}
	for _, check := range checks {
		rows, err := c.pool.Query(ctx, check.sql)
		if err != nil {
			return fmt.Errorf("doctor: 检查证据链失败: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			appendIssue(issues, check.code, check.severity, "evidence", check.message,
				check.resourceType, id, check.recommendation, nil)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	return nil
}

func (c *Checker) checkOutbox(ctx context.Context, issues *[]Issue) error {
	rows, err := c.pool.Query(ctx, `
		SELECT id,status,claimed_at FROM outbox_event
		WHERE status='dead'
		   OR (status='claimed' AND claimed_at < $1)
		ORDER BY id`, c.opts.Now.Add(-c.opts.ClaimedStuckAfter))
	if err != nil {
		return fmt.Errorf("doctor: 检查 Outbox 失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var status string
		var claimedAt *time.Time
		if err := rows.Scan(&id, &status, &claimedAt); err != nil {
			return err
		}
		if status == "dead" {
			appendIssue(issues, CodeOutboxDead, SeverityError, "outbox", "Outbox 事件处于 dead 状态",
				"outbox_event", id.String(), "排查 last_error 后使用既有 worker 重放流程；不得直接修改权威聚合", nil)
		} else {
			appendIssue(issues, CodeOutboxClaimStuck, SeverityError, "outbox", "Outbox claimed 超过卡死阈值",
				"outbox_event", id.String(), "确认无存活消费者持有任务，再按 Outbox 运维流程恢复领取",
				map[string]string{"claimed_at": claimedAt.UTC().Format(time.RFC3339), "threshold": c.opts.ClaimedStuckAfter.String()})
		}
	}
	return rows.Err()
}

func (c *Checker) checkExpiredAuth(ctx context.Context, issues *[]Issue) error {
	for _, check := range []struct {
		table, code, message string
	}{
		{"oidc_login_attempt", CodeLoginAttemptExpired, "存在过期 OIDC 登录临时态"},
		{"auth_session", CodeSessionExpired, "存在过期认证会话"},
	} {
		var count int64
		query := fmt.Sprintf(`SELECT count(*) FROM %s WHERE expires_at < $1`, check.table)
		if err := c.pool.QueryRow(ctx, query, c.opts.Now).Scan(&count); err != nil {
			return fmt.Errorf("doctor: 检查过期 auth 状态失败: %w", err)
		}
		if count > 0 {
			appendIssue(issues, check.code, SeverityWarning, "auth", check.message,
				"table", check.table, "运行 doctor --repair-expired-auth 显式清理；该操作不修改权威百科数据",
				map[string]string{"count": strconv.FormatInt(count, 10)})
		}
	}
	return nil
}

func CleanupExpiredAuth(ctx context.Context, pool *pgxpool.Pool, now time.Time) (RepairSummary, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RepairSummary{}, err
	}
	defer tx.Rollback(ctx)
	loginTag, err := tx.Exec(ctx, `DELETE FROM oidc_login_attempt WHERE expires_at < $1`, now)
	if err != nil {
		return RepairSummary{}, fmt.Errorf("doctor: 清理过期登录临时态失败: %w", err)
	}
	sessionTag, err := tx.Exec(ctx, `DELETE FROM auth_session WHERE expires_at < $1`, now)
	if err != nil {
		return RepairSummary{}, fmt.Errorf("doctor: 清理过期会话失败: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RepairSummary{}, err
	}
	return RepairSummary{
		ExpiredLoginAttempts: loginTag.RowsAffected(),
		ExpiredSessions:      sessionTag.RowsAffected(),
	}, nil
}
