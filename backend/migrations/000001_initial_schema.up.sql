-- Anby Wiki initial schema. This repository is pre-release; migration history is intentionally squashed.

BEGIN;

CREATE TABLE wiki_site (
    id               uuid        PRIMARY KEY,
    site_key         text        NOT NULL,
    name             text        NOT NULL,
    default_language text        NOT NULL,
    settings_json    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX wiki_site_site_key_key ON wiki_site (site_key);

CREATE TABLE namespace (
    id             uuid        PRIMARY KEY,
    wiki_id        uuid        NOT NULL REFERENCES wiki_site (id),
    namespace_key  text        NOT NULL,
    canonical_name text        NOT NULL,
    display_name   text        NOT NULL,
    is_content     boolean     NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX namespace_wiki_key_key ON namespace (wiki_id, namespace_key);

CREATE TABLE actor (
    id           uuid        PRIMARY KEY,
    actor_type   text        NOT NULL CHECK (actor_type IN ('human', 'anonymous', 'bot', 'ai', 'import', 'system')),
    user_id      uuid,
    display_name text        NOT NULL,
    external_key text,
    status       text        NOT NULL DEFAULT 'active',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE page (
    id                  uuid        PRIMARY KEY,
    wiki_id             uuid        NOT NULL REFERENCES wiki_site (id),
    namespace_id        uuid        NOT NULL REFERENCES namespace (id),
    normalized_title    text        NOT NULL,
    display_title       text        NOT NULL,
    language            text        NOT NULL,
    content_model       text        NOT NULL,
    status              text        NOT NULL DEFAULT 'active',
    current_revision_id uuid,
    primary_entity_id   uuid,
    created_by          uuid        NOT NULL REFERENCES actor (id),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz
);

CREATE UNIQUE INDEX page_live_title_key
    ON page (wiki_id, namespace_id, normalized_title)
    WHERE deleted_at IS NULL;

CREATE TABLE page_alias (
    id               uuid        PRIMARY KEY,
    wiki_id          uuid        NOT NULL REFERENCES wiki_site (id),
    namespace_id     uuid        NOT NULL REFERENCES namespace (id),
    normalized_title text        NOT NULL,
    page_id          uuid        NOT NULL REFERENCES page (id),
    alias_type       text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX page_alias_title_idx ON page_alias (wiki_id, namespace_id, normalized_title);
CREATE INDEX page_alias_page_idx ON page_alias (page_id);

CREATE TABLE page_redirect (
    source_page_id         uuid        PRIMARY KEY REFERENCES page (id),
    target_page_id         uuid        REFERENCES page (id),
    target_namespace_id    uuid        REFERENCES namespace (id),
    target_title           text,
    target_anchor_block_id uuid,
    target_interwiki       text,
    created_at             timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE content_snapshot (
    id             uuid        PRIMARY KEY,
    schema_version integer     NOT NULL,
    ast_json       jsonb       NOT NULL,
    content_hash   text        NOT NULL,
    size_bytes     integer     NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX content_snapshot_hash_idx ON content_snapshot (content_hash, schema_version);

CREATE TABLE revision (
    id                  uuid        PRIMARY KEY,
    page_id             uuid        NOT NULL REFERENCES page (id),
    parent_revision_id  uuid        REFERENCES revision (id),
    content_snapshot_id uuid        NOT NULL REFERENCES content_snapshot (id),
    actor_id            uuid        NOT NULL REFERENCES actor (id),
    change_batch_id     uuid,
    summary             text        NOT NULL DEFAULT '',
    is_minor            boolean     NOT NULL DEFAULT false,
    visibility          text        NOT NULL DEFAULT 'public',
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX revision_page_history_idx ON revision (page_id, created_at DESC, id DESC);
CREATE INDEX revision_actor_idx ON revision (actor_id, created_at DESC);
CREATE INDEX revision_change_batch_idx ON revision (change_batch_id);

ALTER TABLE page
    ADD CONSTRAINT page_current_revision_fk
    FOREIGN KEY (current_revision_id) REFERENCES revision (id);

CREATE OR REPLACE FUNCTION check_page_current_revision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.current_revision_id IS NOT NULL
       AND NOT EXISTS (
           SELECT 1 FROM revision
           WHERE id = NEW.current_revision_id
             AND page_id = NEW.id
       ) THEN
        RAISE EXCEPTION 'page % 的 current_revision_id % 不属于该页面', NEW.id, NEW.current_revision_id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER page_current_revision_check
    BEFORE INSERT OR UPDATE OF current_revision_id ON page
    FOR EACH ROW
    EXECUTE FUNCTION check_page_current_revision();

CREATE TABLE audit_event (
    id              uuid        PRIMARY KEY,
    actor_id        uuid        NOT NULL REFERENCES actor (id),
    event_type      text        NOT NULL,
    aggregate_type  text        NOT NULL,
    aggregate_id    uuid        NOT NULL,
    change_batch_id uuid,
    payload_json    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE outbox_event (
    id              uuid        PRIMARY KEY,
    aggregate_type  text        NOT NULL,
    aggregate_id    uuid        NOT NULL,
    event_type      text        NOT NULL,
    payload_json    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    status          text        NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'claimed', 'done', 'dead')),
    attempt_count   integer     NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    claimed_at      timestamptz,
    processed_at    timestamptz,
    last_error      text
);

CREATE INDEX outbox_event_claim_idx
    ON outbox_event (status, next_attempt_at)
    WHERE status IN ('pending', 'claimed');

CREATE OR REPLACE FUNCTION reject_immutable_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% 表不允许 %（发布后不可变 / 只增不改）', TG_TABLE_NAME, TG_OP;
END;
$$;

CREATE TRIGGER revision_immutable
    BEFORE UPDATE OR DELETE ON revision
    FOR EACH ROW
    EXECUTE FUNCTION reject_immutable_mutation();

CREATE TRIGGER content_snapshot_immutable
    BEFORE UPDATE OR DELETE ON content_snapshot
    FOR EACH ROW
    EXECUTE FUNCTION reject_immutable_mutation();

CREATE TRIGGER audit_event_immutable
    BEFORE UPDATE OR DELETE ON audit_event
    FOR EACH ROW
    EXECUTE FUNCTION reject_immutable_mutation();

COMMIT;

BEGIN;

INSERT INTO wiki_site (id, site_key, name, default_language, settings_json)
VALUES ('00000000-0000-7000-8000-000000000001', 'default', 'Anby Wiki', 'zh-Hans', '{}'::jsonb);

INSERT INTO namespace (id, wiki_id, namespace_key, canonical_name, display_name, is_content) VALUES
    ('00000000-0000-7000-8000-000000000101', '00000000-0000-7000-8000-000000000001', 'main',       'Main',       '条目',   true),
    ('00000000-0000-7000-8000-000000000102', '00000000-0000-7000-8000-000000000001', 'talk',       'Talk',       '讨论',   false),
    ('00000000-0000-7000-8000-000000000103', '00000000-0000-7000-8000-000000000001', 'user',       'User',       '用户',   false),
    ('00000000-0000-7000-8000-000000000104', '00000000-0000-7000-8000-000000000001', 'project',    'Project',    '项目',   false),
    ('00000000-0000-7000-8000-000000000105', '00000000-0000-7000-8000-000000000001', 'component',  'Component',  '组件',   true),
    ('00000000-0000-7000-8000-000000000106', '00000000-0000-7000-8000-000000000001', 'collection', 'Collection', '合集',   true),
    ('00000000-0000-7000-8000-000000000107', '00000000-0000-7000-8000-000000000001', 'file',       'File',       '文件',   true);

INSERT INTO actor (id, actor_type, display_name, status)
VALUES ('00000000-0000-7000-8000-000000000201', 'system', 'system', 'active');

COMMIT;

BEGIN;

CREATE TABLE entity_type (
    id          uuid        PRIMARY KEY,
    type_key    text        NOT NULL,
    name        text        NOT NULL,
    schema_json jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX entity_type_type_key_key ON entity_type (type_key);

CREATE TABLE entity (
    id                    uuid        PRIMARY KEY,
    wiki_id               uuid        NOT NULL REFERENCES wiki_site (id),
    entity_type_id        uuid        NOT NULL REFERENCES entity_type (id),
    canonical_key         text        NOT NULL,
    status                text        NOT NULL DEFAULT 'active'
                                      CHECK (status IN ('active', 'merged', 'deleted')),
    merged_into_entity_id uuid        REFERENCES entity (id),
    created_by            uuid        NOT NULL REFERENCES actor (id),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX entity_wiki_canonical_key_key ON entity (wiki_id, canonical_key);
CREATE INDEX entity_entity_type_idx ON entity (entity_type_id);
CREATE INDEX entity_merged_into_idx ON entity (merged_into_entity_id);

CREATE TABLE entity_label (
    entity_id   uuid    NOT NULL REFERENCES entity (id),
    language    text    NOT NULL,
    label       text    NOT NULL,
    description text    NOT NULL DEFAULT '',
    is_primary  boolean NOT NULL DEFAULT false,
    PRIMARY KEY (entity_id, language, label)
);

CREATE INDEX entity_label_entity_idx ON entity_label (entity_id);

CREATE UNIQUE INDEX entity_label_primary_key
    ON entity_label (entity_id, language)
    WHERE is_primary;

CREATE TABLE entity_alias (
    id               uuid        PRIMARY KEY,
    entity_id        uuid        NOT NULL REFERENCES entity (id),
    language         text        NOT NULL,
    alias            text        NOT NULL,
    normalized_alias text        NOT NULL,
    alias_type       text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX entity_alias_lookup_idx ON entity_alias (language, normalized_alias);
CREATE INDEX entity_alias_entity_idx ON entity_alias (entity_id);

CREATE TABLE property (
    id              uuid        PRIMARY KEY,
    property_key    text        NOT NULL,
    name            text        NOT NULL,
    value_type      text        NOT NULL
                                CHECK (value_type IN ('string', 'number', 'date', 'entity', 'coordinate', 'composite')),
    subject_type_id uuid        REFERENCES entity_type (id),
    target_type_id  uuid        REFERENCES entity_type (id),
    is_multivalued  boolean     NOT NULL DEFAULT false,
    schema_json     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX property_property_key_key ON property (property_key);

CREATE TABLE claim (
    id                 uuid        PRIMARY KEY,
    subject_entity_id  uuid        NOT NULL REFERENCES entity (id),
    property_id        uuid        NOT NULL REFERENCES property (id),
    value_type         text        NOT NULL,
    value_json         jsonb       NOT NULL DEFAULT '{}'::jsonb,
    target_entity_id   uuid        REFERENCES entity (id),
    qualifiers_json    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    rank               text        NOT NULL DEFAULT 'normal'
                                   CHECK (rank IN ('preferred', 'normal', 'deprecated')),
    status             text        NOT NULL DEFAULT 'proposed'
                                   CHECK (status IN ('proposed', 'published', 'rejected', 'superseded', 'deprecated')),
    verification_status text       NOT NULL DEFAULT 'unverified'
                                   CHECK (verification_status IN ('unverified', 'ai_checked', 'human_verified', 'disputed')),
    valid_from         timestamptz,
    valid_to           timestamptz,
    origin_type        text        NOT NULL
                                   CHECK (origin_type IN ('human', 'ai', 'import')),
    change_batch_id    uuid,
    created_by         uuid        NOT NULL REFERENCES actor (id),
    created_at         timestamptz NOT NULL DEFAULT now(),
    superseded_by      uuid        REFERENCES claim (id),
    CONSTRAINT claim_valid_time_check
        CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to > valid_from)
);

CREATE INDEX claim_subject_property_status_idx ON claim (subject_entity_id, property_id, status);
CREATE INDEX claim_target_property_idx ON claim (target_entity_id, property_id);
CREATE INDEX claim_change_batch_idx ON claim (change_batch_id);
CREATE INDEX claim_verification_status_idx ON claim (verification_status, status);

CREATE TABLE claim_source (
    claim_id     uuid        NOT NULL REFERENCES claim (id),
    citation_id  uuid        NOT NULL,
    support_type text        NOT NULL
                             CHECK (support_type IN ('supports', 'contradicts', 'context')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (claim_id, citation_id)
);

CREATE TABLE page_entity_binding (
    page_id      uuid        NOT NULL REFERENCES page (id),
    entity_id    uuid        NOT NULL REFERENCES entity (id),
    binding_role text        NOT NULL
                             CHECK (binding_role IN ('primary', 'mentioned')),
    language     text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, entity_id, binding_role)
);

ALTER TABLE page
    ADD CONSTRAINT page_primary_entity_fk
    FOREIGN KEY (primary_entity_id) REFERENCES entity (id);

CREATE OR REPLACE FUNCTION reject_knowledge_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% 表不允许 DELETE（软删除经 status 字段流转）', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER entity_no_delete
    BEFORE DELETE ON entity
    FOR EACH ROW
    EXECUTE FUNCTION reject_knowledge_delete();

CREATE TRIGGER claim_no_delete
    BEFORE DELETE ON claim
    FOR EACH ROW
    EXECUTE FUNCTION reject_knowledge_delete();

COMMIT;

BEGIN;

INSERT INTO entity_type (id, type_key, name) VALUES
    ('00000000-0000-7000-8000-000000000301', 'person',       '人物'),
    ('00000000-0000-7000-8000-000000000302', 'organization', '组织'),
    ('00000000-0000-7000-8000-000000000303', 'place',        '地点'),
    ('00000000-0000-7000-8000-000000000304', 'work',         '作品'),
    ('00000000-0000-7000-8000-000000000305', 'character',    '角色'),
    ('00000000-0000-7000-8000-000000000306', 'event',        '事件'),
    ('00000000-0000-7000-8000-000000000307', 'product',      '产品'),
    ('00000000-0000-7000-8000-000000000308', 'concept',      '概念'),
    ('00000000-0000-7000-8000-000000000309', 'species',      '物种'),
    ('00000000-0000-7000-8000-000000000310', 'software',     '软件');

INSERT INTO property (id, property_key, name, value_type, is_multivalued) VALUES
    ('00000000-0000-7000-8000-000000000401', 'instance_of',  '实例属于', 'entity', false),
    ('00000000-0000-7000-8000-000000000402', 'developer',    '开发者',   'entity', true),
    ('00000000-0000-7000-8000-000000000403', 'author',       '作者',     'entity', true),
    ('00000000-0000-7000-8000-000000000404', 'manufacturer', '制造商',   'entity', true),
    ('00000000-0000-7000-8000-000000000405', 'voice_actor',  '配音演员', 'entity', true),
    ('00000000-0000-7000-8000-000000000406', 'release_date', '发布日期', 'date',   false),
    ('00000000-0000-7000-8000-000000000407', 'located_in',   '位于',     'entity', true),
    ('00000000-0000-7000-8000-000000000408', 'part_of',      '属于',     'entity', true);

COMMIT;

BEGIN;

CREATE TABLE projection_state (
    aggregate_type     text        NOT NULL,
    aggregate_id       uuid        NOT NULL,
    projection_type    text        NOT NULL,
    source_revision_id uuid        NOT NULL REFERENCES revision (id),
    status             text        NOT NULL CHECK (status IN ('ok', 'error')),
    projected_at       timestamptz NOT NULL DEFAULT now(),
    last_error         text,
    PRIMARY KEY (aggregate_type, aggregate_id, projection_type)
);

COMMIT;

BEGIN;

CREATE TABLE page_link_projection (
    source_page_id        uuid NOT NULL REFERENCES page (id),
    source_revision_id    uuid NOT NULL REFERENCES revision (id),
    source_block_id       uuid NOT NULL,
    source_node_id        text NOT NULL,
    target_page_id        uuid REFERENCES page (id),
    target_namespace_id   uuid REFERENCES namespace (id),
    target_title          text,
    target_anchor_block_id uuid,
    resolution_status     text NOT NULL CHECK (resolution_status IN ('resolved', 'unresolved')),
    display_text          text NOT NULL DEFAULT '',
    PRIMARY KEY (source_page_id, source_block_id, source_node_id)
);

CREATE INDEX page_link_projection_target_page_idx
    ON page_link_projection (target_page_id, source_page_id);
CREATE INDEX page_link_projection_unresolved_idx
    ON page_link_projection (target_namespace_id, target_title);
CREATE INDEX page_link_projection_source_revision_idx
    ON page_link_projection (source_page_id, source_revision_id);

CREATE TABLE document_outline_projection (
    page_id                 uuid NOT NULL REFERENCES page (id),
    revision_id             uuid NOT NULL REFERENCES revision (id),
    heading_block_id        uuid NOT NULL,
    parent_heading_block_id uuid,
    level                   int  NOT NULL CHECK (level BETWEEN 1 AND 6),
    title                   text NOT NULL,
    position_key            text NOT NULL,
    PRIMARY KEY (page_id, heading_block_id)
);

CREATE INDEX document_outline_projection_revision_idx
    ON document_outline_projection (page_id, revision_id);

CREATE TABLE page_anchor (
    page_id                 uuid NOT NULL REFERENCES page (id),
    revision_id             uuid NOT NULL REFERENCES revision (id),
    heading_block_id        uuid NOT NULL,
    parent_heading_block_id uuid,
    level                   int  NOT NULL CHECK (level BETWEEN 1 AND 6),
    title                   text NOT NULL,
    current_slug            text NOT NULL,
    position_key            text NOT NULL,
    PRIMARY KEY (page_id, heading_block_id)
);

CREATE INDEX page_anchor_slug_idx
    ON page_anchor (page_id, current_slug);

COMMIT;

BEGIN;

CREATE TABLE external_resource (
    id                 uuid        PRIMARY KEY,
    original_url       text        NOT NULL,
    normalized_url     text        NOT NULL,
    canonical_url      text,
    domain             text        NOT NULL,
    path               text        NOT NULL DEFAULT '',
    http_status        integer,
    content_hash       text,
    status             text        NOT NULL DEFAULT 'unknown'
                                   CHECK (status IN ('unknown', 'ok', 'redirect', 'broken', 'blocked')),
    redirect_target_id uuid        REFERENCES external_resource (id),
    last_checked_at    timestamptz,
    last_success_at    timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX external_resource_normalized_url_key ON external_resource (normalized_url);
CREATE INDEX external_resource_domain_idx ON external_resource (domain);
CREATE INDEX external_resource_status_checked_idx ON external_resource (status, last_checked_at);

CREATE TABLE asset (
    id                  uuid        PRIMARY KEY,
    wiki_id             uuid        NOT NULL REFERENCES wiki_site (id),
    name                text        NOT NULL,
    current_revision_id uuid,
    status              text        NOT NULL DEFAULT 'active'
                                    CHECK (status IN ('active', 'deleted')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX asset_wiki_name_key ON asset (wiki_id, name) WHERE status = 'active';

CREATE TABLE asset_revision (
    id            uuid        PRIMARY KEY,
    asset_id      uuid        NOT NULL REFERENCES asset (id),
    storage_key   text        NOT NULL,
    content_hash  text        NOT NULL,
    mime_type     text        NOT NULL,
    size_bytes    bigint      NOT NULL CHECK (size_bytes >= 0),
    width         integer,
    height        integer,
    metadata_json jsonb       NOT NULL DEFAULT '{}'::jsonb,
    actor_id      uuid        NOT NULL REFERENCES actor (id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX asset_revision_asset_idx ON asset_revision (asset_id);
CREATE INDEX asset_revision_content_hash_idx ON asset_revision (content_hash);

ALTER TABLE asset
    ADD CONSTRAINT asset_current_revision_fk
    FOREIGN KEY (current_revision_id) REFERENCES asset_revision (id);

CREATE TABLE source (
    id                   uuid        PRIMARY KEY,
    source_type          text        NOT NULL
                                     CHECK (source_type IN ('webpage', 'pdf', 'book', 'image', 'video', 'api', 'database')),
    external_resource_id uuid        REFERENCES external_resource (id),
    asset_id             uuid        REFERENCES asset (id),
    title                text        NOT NULL,
    author               text,
    publisher            text,
    published_at         timestamptz,
    metadata_json        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX source_external_resource_idx ON source (external_resource_id);
CREATE INDEX source_asset_idx ON source (asset_id);

CREATE TABLE source_version (
    id                 uuid        PRIMARY KEY,
    source_id          uuid        NOT NULL REFERENCES source (id),
    version_hash       text        NOT NULL,
    raw_asset_id       uuid        REFERENCES asset_revision (id),
    extracted_asset_id uuid        REFERENCES asset_revision (id),
    fetched_at         timestamptz NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX source_version_source_hash_key ON source_version (source_id, version_hash);

CREATE TABLE source_chunk (
    id                uuid        PRIMARY KEY,
    source_version_id uuid        NOT NULL REFERENCES source_version (id),
    ordinal           integer     NOT NULL,
    locator_json      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    text_content      text        NOT NULL,
    text_hash         text        NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX source_chunk_version_ordinal_key ON source_chunk (source_version_id, ordinal);
CREATE INDEX source_chunk_text_hash_idx ON source_chunk (text_hash);

CREATE TABLE citation (
    id                uuid        PRIMARY KEY,
    source_version_id uuid        NOT NULL REFERENCES source_version (id),
    source_chunk_id   uuid        REFERENCES source_chunk (id),
    locator_json      jsonb,
    quotation         text,
    quotation_hash    text,
    created_by        uuid        NOT NULL REFERENCES actor (id),
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX citation_source_version_idx ON citation (source_version_id);

ALTER TABLE claim_source
    ADD CONSTRAINT claim_source_citation_fk
    FOREIGN KEY (citation_id) REFERENCES citation (id);

CREATE TRIGGER asset_revision_immutable
    BEFORE UPDATE OR DELETE ON asset_revision
    FOR EACH ROW
    EXECUTE FUNCTION reject_immutable_mutation();

CREATE TRIGGER source_version_immutable
    BEFORE UPDATE OR DELETE ON source_version
    FOR EACH ROW
    EXECUTE FUNCTION reject_immutable_mutation();

CREATE TRIGGER source_chunk_immutable
    BEFORE UPDATE OR DELETE ON source_chunk
    FOR EACH ROW
    EXECUTE FUNCTION reject_immutable_mutation();

CREATE TRIGGER citation_immutable
    BEFORE UPDATE OR DELETE ON citation
    FOR EACH ROW
    EXECUTE FUNCTION reject_immutable_mutation();

COMMIT;

BEGIN;

CREATE TABLE rendered_page (
    page_id          uuid PRIMARY KEY REFERENCES page (id),
    revision_id      uuid NOT NULL REFERENCES revision (id),
    renderer_version text NOT NULL,
    html_content     text NOT NULL,
    content_hash     text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX rendered_page_revision_idx
    ON rendered_page (revision_id);

COMMIT;

BEGIN;

CREATE TABLE external_link_usage (
    external_resource_id uuid NOT NULL REFERENCES external_resource (id),
    page_id              uuid NOT NULL REFERENCES page (id),
    revision_id          uuid NOT NULL REFERENCES revision (id),
    block_id             uuid NOT NULL,
    node_id              text NOT NULL,
    link_role            text NOT NULL DEFAULT 'inline'
                                 CHECK (link_role IN ('inline', 'reference')),
    PRIMARY KEY (external_resource_id, page_id, block_id, node_id)
);

CREATE INDEX external_link_usage_page_revision_idx ON external_link_usage (page_id, revision_id);
CREATE INDEX external_link_usage_resource_page_idx ON external_link_usage (external_resource_id, page_id);

COMMIT;

BEGIN;

CREATE TABLE entity_mention_projection (
    page_id      uuid NOT NULL REFERENCES page (id),
    revision_id  uuid NOT NULL REFERENCES revision (id),
    block_id     uuid NOT NULL,
    node_id      text NOT NULL,
    entity_id    uuid NOT NULL REFERENCES entity (id),
    mention_text text NOT NULL,
    PRIMARY KEY (page_id, block_id, node_id)
);

CREATE INDEX entity_mention_projection_entity_page_idx
    ON entity_mention_projection (entity_id, page_id);
CREATE INDEX entity_mention_projection_page_revision_idx
    ON entity_mention_projection (page_id, revision_id);

CREATE TABLE claim_usage (
    claim_id    uuid NOT NULL REFERENCES claim (id),
    page_id     uuid NOT NULL REFERENCES page (id),
    revision_id uuid NOT NULL REFERENCES revision (id),
    block_id    uuid NOT NULL,
    node_id     text NOT NULL,
    PRIMARY KEY (page_id, block_id, node_id)
);

CREATE INDEX claim_usage_claim_page_idx ON claim_usage (claim_id, page_id);
CREATE INDEX claim_usage_page_revision_idx ON claim_usage (page_id, revision_id);

CREATE TABLE citation_usage (
    citation_id uuid NOT NULL REFERENCES citation (id),
    page_id     uuid NOT NULL REFERENCES page (id),
    revision_id uuid NOT NULL REFERENCES revision (id),
    block_id    uuid NOT NULL,
    node_id     text NOT NULL,
    claim_id    uuid REFERENCES claim (id),
    PRIMARY KEY (page_id, block_id, node_id)
);

CREATE INDEX citation_usage_citation_page_idx ON citation_usage (citation_id, page_id);
CREATE INDEX citation_usage_page_revision_idx ON citation_usage (page_id, revision_id);
CREATE INDEX citation_usage_claim_idx ON citation_usage (claim_id) WHERE claim_id IS NOT NULL;

COMMIT;

BEGIN;

CREATE TABLE import_job (
    id              uuid        PRIMARY KEY,
    job_type        text        NOT NULL,
    status          text        NOT NULL DEFAULT 'queued'
                                CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    initiated_by    uuid        NOT NULL REFERENCES actor (id),
    idempotency_key text        NOT NULL,
    config_json     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    started_at      timestamptz,
    finished_at     timestamptz,
    error_json      jsonb
);

CREATE UNIQUE INDEX import_job_idempotency_key
    ON import_job (initiated_by, idempotency_key);
CREATE INDEX import_job_status_created_idx
    ON import_job (status, created_at);

CREATE TABLE proposal (
    id                 uuid        PRIMARY KEY,
    import_job_id      uuid        REFERENCES import_job (id),
    target_type        text        NOT NULL
                                   CHECK (target_type IN ('page', 'entity', 'claim', 'collection', 'external_resource')),
    target_id          uuid,
    base_revision_id   uuid        REFERENCES revision (id),
    base_state_version integer,
    status             text        NOT NULL DEFAULT 'draft'
                                   CHECK (status IN (
                                       'draft', 'submitted', 'in_review', 'approved', 'rejected',
                                       'conflicted', 'applying', 'applied', 'failed', 'rolled_back'
                                   )),
    risk_level         text        NOT NULL DEFAULT 'low'
                                   CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    risk_reasons_json  jsonb       NOT NULL DEFAULT '[]'::jsonb,
    policy_decision_json jsonb     NOT NULL DEFAULT '{}'::jsonb,
    created_by         uuid        NOT NULL REFERENCES actor (id),
    idempotency_key    text        NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT proposal_base_nonnegative
        CHECK (base_state_version IS NULL OR base_state_version >= 0),
    CONSTRAINT proposal_risk_reasons_array
        CHECK (jsonb_typeof(risk_reasons_json) = 'array'),
    CONSTRAINT proposal_policy_decision_object
        CHECK (jsonb_typeof(policy_decision_json) = 'object')
);

CREATE UNIQUE INDEX proposal_creator_idempotency_key
    ON proposal (created_by, idempotency_key);
CREATE INDEX proposal_status_risk_created_idx
    ON proposal (status, risk_level, created_at);
CREATE INDEX proposal_target_idx
    ON proposal (target_type, target_id);
CREATE INDEX proposal_import_job_idx
    ON proposal (import_job_id);

CREATE TABLE proposal_operation (
    id               uuid        PRIMARY KEY,
    proposal_id      uuid        NOT NULL REFERENCES proposal (id),
    sequence         integer     NOT NULL CHECK (sequence > 0),
    schema_version   integer     NOT NULL DEFAULT 1 CHECK (schema_version = 1),
    operation_type   text        NOT NULL,
    target_page_id   uuid        REFERENCES page (id),
    target_block_id  text,
    target_node_id   text,
    target_entity_id uuid        REFERENCES entity (id),
    target_claim_id  uuid        REFERENCES claim (id),
    target_json      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    expected_hash    text,
    base_json        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    evidence_json    jsonb       NOT NULL DEFAULT '[]'::jsonb,
    risk_json        jsonb       NOT NULL DEFAULT '{"level":"low","reasons":[]}'::jsonb,
    payload_json     jsonb       NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT proposal_operation_sequence_key UNIQUE (proposal_id, sequence),
    CONSTRAINT proposal_operation_evidence_array
        CHECK (jsonb_typeof(evidence_json) = 'array'),
    CONSTRAINT proposal_operation_base_object
        CHECK (jsonb_typeof(base_json) = 'object'),
    CONSTRAINT proposal_operation_target_object
        CHECK (jsonb_typeof(target_json) = 'object'),
    CONSTRAINT proposal_operation_risk_object
        CHECK (jsonb_typeof(risk_json) = 'object'),
    CONSTRAINT proposal_operation_payload_object
        CHECK (jsonb_typeof(payload_json) = 'object')
);

CREATE INDEX proposal_operation_page_idx
    ON proposal_operation (target_page_id);
CREATE INDEX proposal_operation_entity_idx
    ON proposal_operation (target_entity_id);
CREATE INDEX proposal_operation_claim_idx
    ON proposal_operation (target_claim_id);

CREATE TABLE review_task (
    id              uuid        PRIMARY KEY,
    proposal_id     uuid        NOT NULL REFERENCES proposal (id),
    status          text        NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    reviewer_id     uuid        REFERENCES actor (id),
    decision_reason text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    reviewed_at     timestamptz,
    CONSTRAINT review_task_decision_fields CHECK (
        (status = 'pending' AND reviewer_id IS NULL AND reviewed_at IS NULL)
        OR
        (status <> 'pending' AND reviewer_id IS NOT NULL AND reviewed_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX review_task_one_pending_per_proposal
    ON review_task (proposal_id) WHERE status = 'pending';
CREATE INDEX review_task_queue_idx
    ON review_task (status, created_at);

CREATE TABLE merge_conflict (
    id                  uuid        PRIMARY KEY,
    proposal_id         uuid        NOT NULL REFERENCES proposal (id),
    page_id             uuid        REFERENCES page (id),
    conflict_type       text        NOT NULL
                                    CHECK (conflict_type IN ('revision', 'block_hash', 'claim_state', 'semantic')),
    target_block_id     text,
    target_claim_id     uuid        REFERENCES claim (id),
    base_revision_id    uuid        REFERENCES revision (id),
    current_revision_id uuid        REFERENCES revision (id),
    base_value_json     jsonb,
    current_value_json  jsonb,
    proposed_value_json jsonb,
    status              text        NOT NULL DEFAULT 'open'
                                    CHECK (status IN ('open', 'resolved', 'dismissed')),
    resolved_by         uuid        REFERENCES actor (id),
    resolution_json     jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    resolved_at         timestamptz,
    CONSTRAINT merge_conflict_resolution_fields CHECK (
        (status = 'open' AND resolved_by IS NULL AND resolved_at IS NULL)
        OR
        (status <> 'open' AND resolved_by IS NOT NULL AND resolved_at IS NOT NULL)
    )
);

CREATE INDEX merge_conflict_open_idx
    ON merge_conflict (proposal_id, status);

CREATE TABLE change_batch (
    id             uuid        PRIMARY KEY,
    import_job_id  uuid        REFERENCES import_job (id),
    proposal_id    uuid        NOT NULL UNIQUE REFERENCES proposal (id),
    actor_id       uuid        NOT NULL REFERENCES actor (id),
    status         text        NOT NULL DEFAULT 'applying'
                               CHECK (status IN ('applying', 'applied', 'failed', 'rollback_pending', 'rolled_back')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    rolled_back_at timestamptz
);

CREATE INDEX change_batch_import_job_idx ON change_batch (import_job_id);
CREATE INDEX change_batch_actor_created_idx ON change_batch (actor_id, created_at);

ALTER TABLE revision
    ADD CONSTRAINT revision_change_batch_fk
    FOREIGN KEY (change_batch_id) REFERENCES change_batch (id);

ALTER TABLE claim
    ADD CONSTRAINT claim_change_batch_fk
    FOREIGN KEY (change_batch_id) REFERENCES change_batch (id);

ALTER TABLE audit_event
    ADD CONSTRAINT audit_event_change_batch_fk
    FOREIGN KEY (change_batch_id) REFERENCES change_batch (id);

CREATE OR REPLACE FUNCTION protect_submitted_proposal_operation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    owning_proposal_id uuid;
    owning_status text;
BEGIN
    owning_proposal_id := COALESCE(OLD.proposal_id, NEW.proposal_id);
    SELECT status INTO owning_status FROM proposal WHERE id = owning_proposal_id;
    IF owning_status IS DISTINCT FROM 'draft' THEN
        RAISE EXCEPTION 'proposal % 已提交，operation 不可修改', owning_proposal_id;
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$;

CREATE TRIGGER proposal_operation_after_submit_immutable
    BEFORE UPDATE OR DELETE ON proposal_operation
    FOR EACH ROW
    EXECUTE FUNCTION protect_submitted_proposal_operation();

COMMIT;

BEGIN;

CREATE TABLE role (
    id          uuid PRIMARY KEY,
    role_key    text NOT NULL UNIQUE,
    name        text NOT NULL,
    description text NOT NULL DEFAULT ''
);

CREATE TABLE actor_role (
    actor_id  uuid        NOT NULL REFERENCES actor (id),
    role_id   uuid        NOT NULL REFERENCES role (id),
    wiki_id   uuid        NOT NULL REFERENCES wiki_site (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, role_id, wiki_id)
);

CREATE INDEX actor_role_wiki_actor_idx ON actor_role (wiki_id, actor_id);

CREATE TABLE page_protection (
    id               uuid        PRIMARY KEY,
    page_id          uuid        REFERENCES page (id),
    namespace_id     uuid        REFERENCES namespace (id),
    normalized_title text,
    action_type      text        NOT NULL
                                  CHECK (action_type IN ('create', 'edit', 'rename', 'review', 'apply', 'batch_rollback')),
    required_role_id uuid        NOT NULL REFERENCES role (id),
    expires_at       timestamptz,
    created_by       uuid        NOT NULL REFERENCES actor (id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT page_protection_scope CHECK (
        page_id IS NOT NULL OR (namespace_id IS NOT NULL AND normalized_title IS NOT NULL)
    )
);

CREATE INDEX page_protection_page_action_idx
    ON page_protection (page_id, action_type, expires_at);
CREATE INDEX page_protection_title_action_idx
    ON page_protection (namespace_id, normalized_title, action_type, expires_at);

INSERT INTO role (id, role_key, name, description) VALUES
    ('00000000-0000-7000-8000-000000000801', 'editor',   '编辑者', '创建、编辑与改名页面'),
    ('00000000-0000-7000-8000-000000000802', 'reviewer', '审核者', '审核 Proposal'),
    ('00000000-0000-7000-8000-000000000803', 'applier',  '应用者', '应用与回滚 ChangeBatch'),
    ('00000000-0000-7000-8000-000000000804', 'admin',    '管理员', '站点内全部治理权限');

COMMIT;

BEGIN;

ALTER TABLE import_job
    ADD COLUMN source_version_id uuid REFERENCES source_version (id),
    ADD COLUMN proposal_id uuid REFERENCES proposal (id),
    ADD COLUMN current_stage text NOT NULL DEFAULT 'queued'
        CHECK (current_stage IN ('queued','fetch','parse','extract','match','compose','review','complete')),
    ADD COLUMN progress integer NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

CREATE UNIQUE INDEX import_job_succeeded_version_key
    ON import_job (job_type, source_version_id)
    WHERE status = 'succeeded' AND source_version_id IS NOT NULL;

CREATE TABLE import_run (
    id              uuid        PRIMARY KEY,
    import_job_id   uuid        NOT NULL REFERENCES import_job (id),
    attempt         integer     NOT NULL CHECK (attempt > 0),
    idempotency_key text        NOT NULL,
    status          text        NOT NULL DEFAULT 'running'
                                CHECK (status IN ('running','succeeded','failed','cancelled')),
    error_json      jsonb,
    started_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz,
    UNIQUE (import_job_id, attempt),
    UNIQUE (import_job_id, idempotency_key),
    CONSTRAINT import_run_finish_check CHECK (
        (status = 'running' AND finished_at IS NULL)
        OR (status <> 'running' AND finished_at IS NOT NULL)
    )
);

CREATE INDEX import_run_job_started_idx ON import_run (import_job_id, started_at DESC);

CREATE TABLE import_stage_run (
    id          uuid        PRIMARY KEY,
    import_run_id uuid      NOT NULL REFERENCES import_run (id),
    stage       text        NOT NULL
                            CHECK (stage IN ('fetch','parse','extract','match','compose','review')),
    status      text        NOT NULL DEFAULT 'running'
                            CHECK (status IN ('running','succeeded','failed','skipped','cancelled')),
    input_hash  text,
    output_hash text,
    error_json  jsonb,
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    UNIQUE (import_run_id, stage),
    CONSTRAINT import_stage_finish_check CHECK (
        (status = 'running' AND finished_at IS NULL)
        OR (status <> 'running' AND finished_at IS NOT NULL)
    )
);

CREATE TABLE prompt_template (
    id                   uuid        PRIMARY KEY,
    prompt_key           text        NOT NULL,
    version              integer     NOT NULL CHECK (version > 0),
    system_template      text        NOT NULL,
    user_template        text        NOT NULL,
    output_schema_json   jsonb       NOT NULL,
    content_hash         text        NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    active               boolean     NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (prompt_key, version),
    CONSTRAINT prompt_schema_object CHECK (jsonb_typeof(output_schema_json) = 'object')
);

CREATE UNIQUE INDEX prompt_template_one_active_key
    ON prompt_template (prompt_key) WHERE active;

CREATE TABLE ai_request_usage (
    id              uuid        PRIMARY KEY,
    import_job_id   uuid        REFERENCES import_job (id),
    import_run_id   uuid        REFERENCES import_run (id),
    provider        text        NOT NULL,
    model           text        NOT NULL,
    prompt_key      text        NOT NULL,
    prompt_version  integer     NOT NULL,
    attempt_count   integer     NOT NULL CHECK (attempt_count > 0),
    input_tokens    integer     NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens   integer     NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    latency_ms      bigint      NOT NULL CHECK (latency_ms >= 0),
    status          text        NOT NULL CHECK (status IN ('succeeded','failed','timeout','invalid_output')),
    error_code      text,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_request_usage_job_created_idx ON ai_request_usage (import_job_id, created_at);
CREATE INDEX ai_request_usage_provider_model_idx ON ai_request_usage (provider, model, created_at);

CREATE TABLE import_extraction (
    id                  uuid        PRIMARY KEY,
    source_version_id   uuid        NOT NULL REFERENCES source_version (id),
    schema_version      integer     NOT NULL DEFAULT 1 CHECK (schema_version = 1),
    prompt_key          text        NOT NULL,
    prompt_version      integer     NOT NULL,
    model               text        NOT NULL,
    candidates_json     jsonb       NOT NULL,
    quality_score       double precision NOT NULL CHECK (quality_score BETWEEN 0 AND 1),
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_version_id, schema_version),
    CONSTRAINT import_extraction_candidates_object CHECK (jsonb_typeof(candidates_json) = 'object')
);

CREATE TRIGGER import_extraction_immutable
    BEFORE UPDATE OR DELETE ON import_extraction
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_mutation();

COMMIT;

BEGIN;

CREATE TABLE search_document (
    page_id            uuid        PRIMARY KEY REFERENCES page (id) ON DELETE CASCADE,
    wiki_id            uuid        NOT NULL REFERENCES wiki_site (id),
    namespace_key      text        NOT NULL,
    language           text        NOT NULL,
    source_revision_id uuid        NOT NULL REFERENCES revision (id),
    display_title      text        NOT NULL,
    normalized_title   text        NOT NULL,
    aliases            text[]      NOT NULL DEFAULT '{}'::text[],
    body_text          text        NOT NULL DEFAULT '',
    entity_id          uuid        REFERENCES entity (id),
    entity_type        text,
    entity_terms       text[]      NOT NULL DEFAULT '{}'::text[],
    search_vector      tsvector    NOT NULL DEFAULT ''::tsvector,
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION search_document_update_vector()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('simple', NEW.display_title || ' ' || NEW.normalized_title), 'A') ||
        setweight(to_tsvector('simple', array_to_string(NEW.aliases, ' ')), 'B') ||
        setweight(to_tsvector('simple', array_to_string(NEW.entity_terms, ' ')), 'B') ||
        setweight(to_tsvector('simple', NEW.body_text), 'C');
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER search_document_vector_trigger
    BEFORE INSERT OR UPDATE OF display_title, normalized_title, aliases, entity_terms, body_text
    ON search_document
    FOR EACH ROW
    EXECUTE FUNCTION search_document_update_vector();

CREATE INDEX search_document_vector_idx ON search_document USING gin (search_vector);
CREATE INDEX search_document_filter_idx
    ON search_document (wiki_id, namespace_key, language, entity_type);
CREATE INDEX search_document_entity_idx ON search_document (entity_id) WHERE entity_id IS NOT NULL;

COMMIT;

BEGIN;

CREATE TABLE external_identity (
    issuer       text        NOT NULL,
    subject      text        NOT NULL,
    actor_id     uuid        NOT NULL REFERENCES actor (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (issuer, subject),
    UNIQUE (actor_id, issuer)
);

CREATE INDEX external_identity_actor_idx ON external_identity (actor_id);

CREATE TABLE oidc_login_attempt (
    id                  uuid        PRIMARY KEY,
    state_hash          bytea       NOT NULL UNIQUE,
    browser_secret_hash bytea       NOT NULL,
    nonce               text        NOT NULL,
    code_verifier       text        NOT NULL,
    expires_at          timestamptz NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX oidc_login_attempt_expires_idx ON oidc_login_attempt (expires_at);

CREATE TABLE auth_session (
    id         uuid        PRIMARY KEY,
    token_hash bytea       NOT NULL UNIQUE,
    actor_id   uuid        NOT NULL REFERENCES actor (id),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT auth_session_expiry_check CHECK (expires_at > created_at)
);

CREATE INDEX auth_session_actor_active_idx
    ON auth_session (actor_id, expires_at)
    WHERE revoked_at IS NULL;
CREATE INDEX auth_session_expires_idx ON auth_session (expires_at);

COMMIT;

BEGIN;

CREATE TABLE working_document (
    id                uuid        PRIMARY KEY,
    page_id           uuid        NOT NULL REFERENCES page (id),
    base_revision_id  uuid        REFERENCES revision (id),
    schema_version    integer     NOT NULL DEFAULT 1 CHECK (schema_version = 1),
    crdt_codec        text        NOT NULL DEFAULT 'yjs-v1' CHECK (crdt_codec = 'yjs-v1'),
    latest_sequence   bigint      NOT NULL DEFAULT 0 CHECK (latest_sequence >= 0),
    status            text        NOT NULL DEFAULT 'active'
                                CHECK (status IN ('active', 'closed', 'discarded')),
    created_by        uuid        NOT NULL REFERENCES actor (id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX working_document_active_page_idx
    ON working_document (page_id) WHERE status = 'active';
CREATE INDEX working_document_page_created_idx
    ON working_document (page_id, created_at DESC);

CREATE TABLE working_document_update (
    document_id      uuid        NOT NULL REFERENCES working_document (id) ON DELETE CASCADE,
    sequence         bigint      NOT NULL CHECK (sequence > 0),
    actor_id         uuid        NOT NULL REFERENCES actor (id),
    client_id        uuid        NOT NULL,
    client_update_id uuid        NOT NULL,
    update_bytes     bytea       NOT NULL
                                CHECK (octet_length(update_bytes) BETWEEN 1 AND 1048576),
    update_hash      bytea       NOT NULL CHECK (octet_length(update_hash) = 32),
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, sequence),
    UNIQUE (document_id, client_id, client_update_id)
);

CREATE INDEX working_document_update_actor_idx
    ON working_document_update (actor_id, created_at DESC);

CREATE TABLE working_document_snapshot (
    id             uuid        PRIMARY KEY,
    document_id    uuid        NOT NULL REFERENCES working_document (id) ON DELETE CASCADE,
    up_to_sequence bigint      NOT NULL CHECK (up_to_sequence >= 0),
    state_bytes    bytea       NOT NULL
                               CHECK (octet_length(state_bytes) BETWEEN 1 AND 16777216),
    state_hash     bytea       NOT NULL CHECK (octet_length(state_hash) = 32),
    created_by     uuid        NOT NULL REFERENCES actor (id),
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (document_id, up_to_sequence)
);

CREATE INDEX working_document_snapshot_latest_idx
    ON working_document_snapshot (document_id, up_to_sequence DESC);

COMMIT;

BEGIN;

CREATE TABLE page_anchor_alias (
    page_id          uuid        NOT NULL REFERENCES page (id),
    alias_slug       text        NOT NULL,
    heading_block_id uuid        NOT NULL,
    source_revision_id uuid      NOT NULL REFERENCES revision (id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, alias_slug)
);

CREATE INDEX page_anchor_alias_block_idx
    ON page_anchor_alias (page_id, heading_block_id);

CREATE TABLE block_redirect (
    source_page_id  uuid        NOT NULL REFERENCES page (id),
    source_block_id uuid        NOT NULL,
    target_page_id  uuid        NOT NULL REFERENCES page (id),
    target_block_id uuid        NOT NULL,
    created_by      uuid        NOT NULL REFERENCES actor (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source_page_id, source_block_id),
    CHECK (source_page_id <> target_page_id OR source_block_id <> target_block_id)
);

CREATE INDEX block_redirect_target_idx
    ON block_redirect (target_page_id, target_block_id);

COMMIT;

BEGIN;

CREATE TABLE component (
    id            uuid        PRIMARY KEY,
    component_key text        NOT NULL UNIQUE,
    name          text        NOT NULL,
    created_by    uuid        NOT NULL REFERENCES actor (id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (component_key ~ '^[a-z][a-z0-9._-]{0,63}$'),
    CHECK (btrim(name) <> '')
);

CREATE TABLE component_version (
    component_id     uuid        NOT NULL REFERENCES component (id),
    version          integer     NOT NULL CHECK (version > 0),
    props_schema_json jsonb      NOT NULL,
    renderer_ref     text        NOT NULL,
    status           text        NOT NULL DEFAULT 'draft'
                                 CHECK (status IN ('draft', 'published', 'deprecated')),
    created_by       uuid        NOT NULL REFERENCES actor (id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    published_at     timestamptz,
    PRIMARY KEY (component_id, version),
    CHECK (btrim(renderer_ref) <> ''),
    CHECK (
        (status = 'draft' AND published_at IS NULL)
        OR (status IN ('published', 'deprecated') AND published_at IS NOT NULL)
    )
);

CREATE FUNCTION enforce_component_version_freeze() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'draft' THEN
            RAISE EXCEPTION 'published component version is immutable';
        END IF;
        RETURN OLD;
    END IF;

    IF NEW.component_id <> OLD.component_id
       OR NEW.version <> OLD.version
       OR NEW.created_by <> OLD.created_by
       OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'component version identity is immutable';
    END IF;

    IF OLD.status <> 'draft'
       AND (NEW.props_schema_json <> OLD.props_schema_json
            OR NEW.renderer_ref <> OLD.renderer_ref) THEN
        RAISE EXCEPTION 'published component version is immutable';
    END IF;

    IF OLD.status = 'draft' AND NEW.status NOT IN ('draft', 'published') THEN
        RAISE EXCEPTION 'invalid component version transition';
    ELSIF OLD.status = 'published' AND NEW.status NOT IN ('published', 'deprecated') THEN
        RAISE EXCEPTION 'invalid component version transition';
    ELSIF OLD.status = 'deprecated' AND NEW.status <> 'deprecated' THEN
        RAISE EXCEPTION 'invalid component version transition';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER component_version_freeze
BEFORE UPDATE OR DELETE ON component_version
FOR EACH ROW EXECUTE FUNCTION enforce_component_version_freeze();

COMMIT;

BEGIN;

CREATE TABLE component_dependency (
    page_id          uuid    NOT NULL REFERENCES page (id),
    revision_id      uuid    NOT NULL REFERENCES revision (id),
    block_id         uuid    NOT NULL,
    component_id     uuid    NOT NULL,
    component_version integer NOT NULL CHECK (component_version > 0),
    entity_id        uuid    NOT NULL REFERENCES entity (id),
    PRIMARY KEY (page_id, block_id),
    FOREIGN KEY (component_id, component_version)
        REFERENCES component_version (component_id, version)
);

CREATE INDEX component_dependency_component_page_idx
    ON component_dependency (component_id, component_version, page_id);
CREATE INDEX component_dependency_entity_page_idx
    ON component_dependency (entity_id, page_id);
CREATE INDEX component_dependency_page_revision_idx
    ON component_dependency (page_id, revision_id);

COMMIT;

BEGIN;

CREATE TABLE collection (
    id                  uuid        PRIMARY KEY,
    wiki_id             uuid        NOT NULL REFERENCES wiki_site (id),
    collection_type     text        NOT NULL CHECK (collection_type IN ('manual', 'rule')),
    title               text        NOT NULL CHECK (btrim(title) <> ''),
    description_page_id uuid        REFERENCES page (id),
    query_json          jsonb,
    created_by          uuid        NOT NULL REFERENCES actor (id),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT collection_query_shape CHECK (
        (collection_type = 'manual' AND query_json IS NULL) OR
        (collection_type = 'rule' AND jsonb_typeof(query_json) = 'object')
    )
);

CREATE INDEX collection_wiki_title_idx ON collection (wiki_id, title, id);

CREATE TABLE collection_membership (
    collection_id     uuid        NOT NULL REFERENCES collection (id) ON DELETE CASCADE,
    page_id           uuid        REFERENCES page (id),
    entity_id         uuid        REFERENCES entity (id),
    member_type       text        NOT NULL CHECK (member_type IN ('page', 'entity')),
    source_type       text        NOT NULL CHECK (source_type IN ('manual', 'rule')),
    sort_key          text        NOT NULL CHECK (btrim(sort_key) <> ''),
    source_revision_id uuid       NOT NULL REFERENCES revision (id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT collection_membership_target CHECK (
        (member_type = 'page' AND page_id IS NOT NULL AND entity_id IS NULL) OR
        (member_type = 'entity' AND entity_id IS NOT NULL AND page_id IS NULL)
    ),
    CONSTRAINT collection_membership_unique_target UNIQUE NULLS NOT DISTINCT
        (collection_id, page_id, entity_id)
);

CREATE INDEX collection_membership_collection_sort_idx
    ON collection_membership (collection_id, sort_key, member_type, page_id, entity_id);
CREATE INDEX collection_membership_page_idx
    ON collection_membership (page_id) WHERE page_id IS NOT NULL;
CREATE INDEX collection_membership_entity_idx
    ON collection_membership (entity_id) WHERE entity_id IS NOT NULL;
CREATE INDEX collection_membership_source_revision_idx
    ON collection_membership (source_revision_id);

COMMIT;

BEGIN;

ALTER TABLE external_resource
    ADD COLUMN next_check_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN lease_token uuid,
    ADD COLUMN consecutive_failures integer NOT NULL DEFAULT 0
        CHECK (consecutive_failures >= 0);

CREATE INDEX external_resource_next_check_idx
    ON external_resource (next_check_at, id);

COMMIT;

BEGIN;

CREATE TABLE entity_merge (
    id                uuid        PRIMARY KEY,
    source_entity_id  uuid        NOT NULL REFERENCES entity (id),
    target_entity_id  uuid        NOT NULL REFERENCES entity (id),
    actor_id          uuid        NOT NULL REFERENCES actor (id),
    status            text        NOT NULL DEFAULT 'applied'
                                  CHECK (status IN ('applied', 'rolled_back')),
    reason            text        NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    rolled_back_at    timestamptz,
    rolled_back_by    uuid        REFERENCES actor (id),
    CHECK (source_entity_id <> target_entity_id),
    CHECK ((status = 'applied' AND rolled_back_at IS NULL AND rolled_back_by IS NULL)
        OR (status = 'rolled_back' AND rolled_back_at IS NOT NULL AND rolled_back_by IS NOT NULL))
);

CREATE UNIQUE INDEX entity_merge_active_source_key
    ON entity_merge (source_entity_id) WHERE status = 'applied';
CREATE INDEX entity_merge_target_idx ON entity_merge (target_entity_id, created_at);

CREATE TABLE entity_merge_label_map (
    merge_id          uuid    NOT NULL REFERENCES entity_merge (id),
    language          text    NOT NULL,
    source_label      text    NOT NULL,
    target_label      text    NOT NULL,
    action            text    NOT NULL
                              CHECK (action IN ('copied', 'demoted_primary', 'existing')),
    target_is_primary boolean NOT NULL,
    PRIMARY KEY (merge_id, language, source_label)
);

CREATE TABLE entity_merge_claim_map (
    merge_id     uuid NOT NULL REFERENCES entity_merge (id),
    old_claim_id uuid NOT NULL REFERENCES claim (id),
    new_claim_id uuid NOT NULL REFERENCES claim (id),
    old_status   text NOT NULL,
    new_status   text NOT NULL,
    PRIMARY KEY (merge_id, old_claim_id),
    UNIQUE (merge_id, new_claim_id),
    CHECK (old_claim_id <> new_claim_id)
);

CREATE INDEX entity_merge_claim_map_old_idx ON entity_merge_claim_map (old_claim_id);
CREATE INDEX entity_merge_claim_map_new_idx ON entity_merge_claim_map (new_claim_id);

COMMIT;

BEGIN;

CREATE TABLE bulk_review_batch (
    id                 uuid        PRIMARY KEY,
    created_by         uuid        NOT NULL REFERENCES actor (id),
    status             text        NOT NULL DEFAULT 'reviewing'
                                   CHECK (status IN ('reviewing', 'ready', 'applying', 'paused', 'completed')),
    sampling_mode      text        NOT NULL
                                   CHECK (sampling_mode IN ('sampled', 'full')),
    sample_percent     integer     NOT NULL CHECK (sample_percent BETWEEN 1 AND 100),
    force_full_reason  text,
    wave_size          integer     NOT NULL CHECK (wave_size BETWEEN 1 AND 1000),
    current_wave       integer     NOT NULL DEFAULT 0 CHECK (current_wave >= 0),
    created_at         timestamptz NOT NULL DEFAULT now(),
    finalized_at       timestamptz,
    paused_at          timestamptz,
    completed_at       timestamptz,
    CONSTRAINT bulk_review_batch_full_reason CHECK (
        sampling_mode <> 'full' OR force_full_reason IS NOT NULL
    )
);

CREATE INDEX bulk_review_batch_status_created_idx
    ON bulk_review_batch (status, created_at);

CREATE TABLE bulk_review_batch_item (
    batch_id          uuid        NOT NULL REFERENCES bulk_review_batch (id),
    proposal_id       uuid        NOT NULL UNIQUE REFERENCES proposal (id),
    position          integer     NOT NULL CHECK (position > 0),
    wave              integer     NOT NULL CHECK (wave > 0),
    selected_for_review boolean   NOT NULL,
    decision          text        NOT NULL DEFAULT 'pending'
                                 CHECK (decision IN ('pending', 'approved', 'rejected')),
    decision_reason   text,
    reviewed_by       uuid        REFERENCES actor (id),
    reviewed_at       timestamptz,
    apply_status      text        NOT NULL DEFAULT 'pending'
                                 CHECK (apply_status IN ('pending', 'applied', 'failed', 'skipped')),
    change_batch_id   uuid        UNIQUE REFERENCES change_batch (id),
    apply_error_code  text,
    applied_at        timestamptz,
    PRIMARY KEY (batch_id, proposal_id),
    CONSTRAINT bulk_review_batch_item_position_key UNIQUE (batch_id, position),
    CONSTRAINT bulk_review_batch_item_review_fields CHECK (
        (decision = 'pending' AND reviewed_by IS NULL AND reviewed_at IS NULL)
        OR
        (decision <> 'pending' AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
    ),
    CONSTRAINT bulk_review_batch_item_reject_reason CHECK (
        decision <> 'rejected' OR length(btrim(decision_reason)) > 0
    ),
    CONSTRAINT bulk_review_batch_item_apply_fields CHECK (
        (apply_status = 'applied' AND change_batch_id IS NOT NULL AND applied_at IS NOT NULL)
        OR
        (apply_status = 'skipped' AND decision = 'rejected')
        OR
        apply_status IN ('pending', 'failed')
    )
);

CREATE INDEX bulk_review_batch_item_wave_idx
    ON bulk_review_batch_item (batch_id, wave, apply_status, position);

CREATE TABLE bulk_review_audit_event (
    id             uuid        PRIMARY KEY,
    batch_id       uuid        NOT NULL REFERENCES bulk_review_batch (id),
    actor_id       uuid        NOT NULL REFERENCES actor (id),
    event_type     text        NOT NULL,
    proposal_id    uuid        REFERENCES proposal (id),
    wave           integer,
    payload_json   jsonb       NOT NULL DEFAULT '{}'::jsonb
                               CHECK (jsonb_typeof(payload_json) = 'object'),
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX bulk_review_audit_event_batch_created_idx
    ON bulk_review_audit_event (batch_id, created_at, id);

CREATE OR REPLACE FUNCTION protect_bulk_review_item_assignment()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.batch_id <> OLD.batch_id
       OR NEW.proposal_id <> OLD.proposal_id
       OR NEW.position <> OLD.position
       OR NEW.wave <> OLD.wave
       OR NEW.selected_for_review <> OLD.selected_for_review THEN
        RAISE EXCEPTION 'bulk review item assignment is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER bulk_review_item_assignment_immutable
    BEFORE UPDATE ON bulk_review_batch_item
    FOR EACH ROW
    EXECUTE FUNCTION protect_bulk_review_item_assignment();

COMMIT;
