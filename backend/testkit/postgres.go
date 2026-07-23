package testkit

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

// 种子数据固定 ID（与 backend/migrations/000001_initial_schema.up.sql 一致）。
var (
	// DefaultWikiID 默认站点。
	DefaultWikiID = uuid.MustParse("00000000-0000-7000-8000-000000000001")
	// MainNamespaceID main 命名空间（is_content=true）。
	MainNamespaceID = uuid.MustParse("00000000-0000-7000-8000-000000000101")
	// SystemActorID system actor。
	SystemActorID = uuid.MustParse("00000000-0000-7000-8000-000000000201")
)

// DB 集成测试数据库助手：封装连接、Reset 与常用 Factory。
// 通过 Open 获取；未设置 TEST_DATABASE_URL 时 Open 直接 t.Skip。
type DB struct {
	Pool *pgxpool.Pool
	ids  *id.Generator
}

// Open 读取 TEST_DATABASE_URL 建连；未设置时 t.Skip 跳过集成测试。
// 连接池随测试结束自动关闭。
func Open(t *testing.T) *DB {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过 PostgreSQL 集成测试")
	}
	pool, err := db.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("testkit: 连接测试库失败: %v", err)
	}
	t.Cleanup(pool.Close)
	return &DB{Pool: pool, ids: id.NewGenerator()}
}

// Reset 清空业务表并重新插入初始化 Schema 的等价种子（默认站点/命名空间/system actor），
// 使每个集成用例在一致的数据基线上运行。
//
// knowledge 表（M4-T01 起）：entity/claim 受 no-delete 行级触发器保护，
// TRUNCATE 不触发行级触发器，故 TRUNCATE ... CASCADE 可安全清库。
// entity_type/property 是初始化 Schema 的固定 UUID 种子数据，不在清空清单内，
// Reset 后直接可用（testkit.EntityType* / Property* 常量与之对应）。
//
// evidence 表（M4-T04 起）：asset_revision/source_version/source_chunk/citation
// 受不可变行级触发器保护，与 000001 同理 TRUNCATE 不触发，可安全清库。
//
// 关于不可变触发器：000001 在 revision/content_snapshot/audit_event 上挂的是
// BEFORE UPDATE/DELETE 的行级触发器；TRUNCATE 不触发行级触发器
// （拦截 TRUNCATE 需要显式创建 TRUNCATE 触发器，本库未创建），
// 因此 TRUNCATE ... CASCADE 可安全用于测试清库。
func (d *DB) Reset(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := d.Pool.Exec(ctx, `
		TRUNCATE wiki_site, namespace, actor, page, page_alias, page_redirect,
			content_snapshot, revision, audit_event, outbox_event,
			working_document, working_document_update, working_document_snapshot,
			component_dependency, component, component_version,
			collection_membership, collection,
			entity_merge_claim_map, entity_merge_label_map, entity_merge,
			entity, entity_label, entity_alias, page_entity_binding,
			claim, claim_source,
			external_resource, asset, asset_revision,
			source, source_version, source_chunk, citation,
			import_job, proposal, proposal_operation, review_task,
			merge_conflict, change_batch,
			bulk_review_audit_event, bulk_review_batch_item, bulk_review_batch,
			actor_role, page_protection,
			import_run, import_stage_run, prompt_template, ai_request_usage, import_extraction,
			external_identity, oidc_login_attempt, auth_session CASCADE`); err != nil {
		t.Fatalf("testkit: TRUNCATE 失败: %v", err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO wiki_site (id, site_key, name, default_language, settings_json)
		VALUES ($1, 'default', 'Anby Wiki', 'zh-Hans', '{}'::jsonb)`, DefaultWikiID); err != nil {
		t.Fatalf("testkit: 种子 wiki_site 失败: %v", err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO namespace (id, wiki_id, namespace_key, canonical_name, display_name, is_content) VALUES
			('00000000-0000-7000-8000-000000000101', $1, 'main',       'Main',       '条目', true),
			('00000000-0000-7000-8000-000000000102', $1, 'talk',       'Talk',       '讨论', false),
			('00000000-0000-7000-8000-000000000103', $1, 'user',       'User',       '用户', false),
			('00000000-0000-7000-8000-000000000104', $1, 'project',    'Project',    '项目', false),
			('00000000-0000-7000-8000-000000000105', $1, 'component',  'Component',  '组件', true),
			('00000000-0000-7000-8000-000000000106', $1, 'collection', 'Collection', '合集', true),
			('00000000-0000-7000-8000-000000000107', $1, 'file',       'File',       '文件', true)`,
		DefaultWikiID); err != nil {
		t.Fatalf("testkit: 种子 namespace 失败: %v", err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO actor (id, actor_type, display_name, status)
		VALUES ($1, 'system', 'system', 'active')`, SystemActorID); err != nil {
		t.Fatalf("testkit: 种子 actor 失败: %v", err)
	}
}

// NewID 生成一个 UUIDv7（测试数据主键）。
func (d *DB) NewID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := d.ids.New()
	if err != nil {
		t.Fatalf("testkit: 生成 ID 失败: %v", err)
	}
	return id
}

// MakeActor 插入一个 active 状态的 actor 并返回其 ID。
func (d *DB) MakeActor(t *testing.T, actorType, displayName string) uuid.UUID {
	t.Helper()
	id := d.NewID(t)
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO actor (id, actor_type, display_name, status)
		VALUES ($1, $2, $3, 'active')`, id, actorType, displayName); err != nil {
		t.Fatalf("testkit: MakeActor 失败: %v", err)
	}
	return id
}

// MakePage 直接插入一个最小页面行（绕过领域服务，供搭建测试场景用），
// normalizedTitle/displayTitle 由调用方显式给定，返回页面 ID。
func (d *DB) MakePage(t *testing.T, namespaceID uuid.UUID, normalizedTitle, displayTitle string, createdBy uuid.UUID) uuid.UUID {
	t.Helper()
	id := d.NewID(t)
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO page (id, wiki_id, namespace_id, normalized_title, display_title,
			language, content_model, status, created_by)
		VALUES ($1, $2, $3, $4, $5, 'zh-Hans', 'block-v1', 'active', $6)`,
		id, DefaultWikiID, namespaceID, normalizedTitle, displayTitle, createdBy); err != nil {
		t.Fatalf("testkit: MakePage 失败: %v", err)
	}
	return id
}

// SoftDeletePage 软删除页面（置 deleted_at），供重定向/别名边界场景使用。
func (d *DB) SoftDeletePage(t *testing.T, pageID uuid.UUID) {
	t.Helper()
	if _, err := d.Pool.Exec(context.Background(), `
		UPDATE page SET deleted_at = now() WHERE id = $1`, pageID); err != nil {
		t.Fatalf("testkit: SoftDeletePage 失败: %v", err)
	}
}

// 种子 entity_type 固定 ID（与 backend/migrations/000001_initial_schema.up.sql 一致）。
// entity_type/property 是种子数据，Reset 不清空，可直接引用。
var (
	// EntityTypePersonID person 实体类型。
	EntityTypePersonID = uuid.MustParse("00000000-0000-7000-8000-000000000301")
	// EntityTypeOrganizationID organization 实体类型。
	EntityTypeOrganizationID = uuid.MustParse("00000000-0000-7000-8000-000000000302")
	// EntityTypePlaceID place 实体类型。
	EntityTypePlaceID = uuid.MustParse("00000000-0000-7000-8000-000000000303")
	// EntityTypeWorkID work 实体类型。
	EntityTypeWorkID = uuid.MustParse("00000000-0000-7000-8000-000000000304")
	// EntityTypeCharacterID character 实体类型。
	EntityTypeCharacterID = uuid.MustParse("00000000-0000-7000-8000-000000000305")
	// EntityTypeEventID event 实体类型。
	EntityTypeEventID = uuid.MustParse("00000000-0000-7000-8000-000000000306")
	// EntityTypeProductID product 实体类型。
	EntityTypeProductID = uuid.MustParse("00000000-0000-7000-8000-000000000307")
	// EntityTypeConceptID concept 实体类型。
	EntityTypeConceptID = uuid.MustParse("00000000-0000-7000-8000-000000000308")
	// EntityTypeSpeciesID species 实体类型。
	EntityTypeSpeciesID = uuid.MustParse("00000000-0000-7000-8000-000000000309")
	// EntityTypeSoftwareID software 实体类型。
	EntityTypeSoftwareID = uuid.MustParse("00000000-0000-7000-8000-000000000310")
)

// SetEntityMerged 直接把 entity 置为 merged 状态并指向目标实体。
// 合并功能本身属 M9-T06；测试用它搭建 merged 实体的写入拒绝/搜索排除场景。
func (d *DB) SetEntityMerged(t *testing.T, entityID, mergedIntoID uuid.UUID) {
	t.Helper()
	if _, err := d.Pool.Exec(context.Background(), `
		UPDATE entity SET status = 'merged', merged_into_entity_id = $2, updated_at = now()
		WHERE id = $1`, entityID, mergedIntoID); err != nil {
		t.Fatalf("testkit: SetEntityMerged 失败: %v", err)
	}
}

// 种子 property 固定 ID（与 backend/migrations/000001_initial_schema.up.sql 一致）。
// property 是种子数据，Reset 不清空，可直接引用。
var (
	// PropertyInstanceOfID instance_of（entity，单值）。
	PropertyInstanceOfID = uuid.MustParse("00000000-0000-7000-8000-000000000401")
	// PropertyDeveloperID developer（entity，多值）。
	PropertyDeveloperID = uuid.MustParse("00000000-0000-7000-8000-000000000402")
	// PropertyAuthorID author（entity，多值）。
	PropertyAuthorID = uuid.MustParse("00000000-0000-7000-8000-000000000403")
	// PropertyManufacturerID manufacturer（entity，多值）。
	PropertyManufacturerID = uuid.MustParse("00000000-0000-7000-8000-000000000404")
	// PropertyVoiceActorID voice_actor（entity，多值）。
	PropertyVoiceActorID = uuid.MustParse("00000000-0000-7000-8000-000000000405")
	// PropertyReleaseDateID release_date（date，单值）。
	PropertyReleaseDateID = uuid.MustParse("00000000-0000-7000-8000-000000000406")
	// PropertyLocatedInID located_in（entity，多值）。
	PropertyLocatedInID = uuid.MustParse("00000000-0000-7000-8000-000000000407")
	// PropertyPartOfID part_of（entity，多值）。
	PropertyPartOfID = uuid.MustParse("00000000-0000-7000-8000-000000000408")
)

// MakeCitation 直插最小 source → source_version → citation 链并返回 citation ID
// （绕过领域服务：source/citation 服务属 M4-T05；本工厂供 claim_source.citation_id
// 外键测试场景使用）。version_hash 按 citation ID 派生，
// 保证 source_version 的 (source_id, version_hash) 唯一约束在重复调用下不冲突
// （每次调用都是全新 source，天然不冲突）。
func (d *DB) MakeCitation(t *testing.T, createdBy uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	sourceID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO source (id, source_type, title) VALUES ($1, 'webpage', 'test source')`, sourceID); err != nil {
		t.Fatalf("testkit: MakeCitation 插入 source 失败: %v", err)
	}
	versionID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO source_version (id, source_id, version_hash, fetched_at)
		VALUES ($1, $2, $3, now())`, versionID, sourceID, "v-"+versionID.String()); err != nil {
		t.Fatalf("testkit: MakeCitation 插入 source_version 失败: %v", err)
	}
	citationID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO citation (id, source_version_id, created_by)
		VALUES ($1, $2, $3)`, citationID, versionID, createdBy); err != nil {
		t.Fatalf("testkit: MakeCitation 插入 citation 失败: %v", err)
	}
	return citationID
}

// MakeProperty 直接插入一个自定义 property 并返回其 ID（绕过领域服务，
// 供搭建带 subject_type/target_type/schema_json 约束的 Claim 测试场景；
// 种子 property 均无类型约束，约束路径需要用本工厂覆盖）。
// subjectTypeID/targetTypeID 传 nil 表示无列约束；schemaJSON 传 "" 落库 '{}'。
// property 表不被 Reset 清空（种子表），本工厂按 property_key 幂等 upsert，
// 重复运行/跨用例同 key 时覆盖为最新配置并返回既有行 ID。
func (d *DB) MakeProperty(t *testing.T, propertyKey, valueType string, isMultivalued bool, subjectTypeID, targetTypeID *uuid.UUID, schemaJSON string) uuid.UUID {
	t.Helper()
	if schemaJSON == "" {
		schemaJSON = "{}"
	}
	var id uuid.UUID
	if err := d.Pool.QueryRow(context.Background(), `
		INSERT INTO property (id, property_key, name, value_type, subject_type_id, target_type_id, is_multivalued, schema_json)
		VALUES ($1, $2, $2, $3, $4, $5, $6, $7::jsonb)
		ON CONFLICT (property_key) DO UPDATE SET
			value_type = EXCLUDED.value_type,
			subject_type_id = EXCLUDED.subject_type_id,
			target_type_id = EXCLUDED.target_type_id,
			is_multivalued = EXCLUDED.is_multivalued,
			schema_json = EXCLUDED.schema_json
		RETURNING id`,
		d.NewID(t), propertyKey, valueType, subjectTypeID, targetTypeID, isMultivalued, schemaJSON).Scan(&id); err != nil {
		t.Fatalf("testkit: MakeProperty 失败: %v", err)
	}
	return id
}
