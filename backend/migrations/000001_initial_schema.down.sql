-- Drop the complete pre-release schema in reverse dependency order.

BEGIN;

DROP TRIGGER bulk_review_item_assignment_immutable ON bulk_review_batch_item;
DROP FUNCTION protect_bulk_review_item_assignment();
DROP TABLE bulk_review_audit_event;
DROP TABLE bulk_review_batch_item;
DROP TABLE bulk_review_batch;

COMMIT;

BEGIN;

DROP TABLE entity_merge_claim_map;
DROP TABLE entity_merge_label_map;
DROP TABLE entity_merge;

COMMIT;

BEGIN;

DROP INDEX IF EXISTS external_resource_next_check_idx;

ALTER TABLE external_resource
    DROP COLUMN IF EXISTS consecutive_failures,
    DROP COLUMN IF EXISTS lease_token,
    DROP COLUMN IF EXISTS next_check_at;

COMMIT;

BEGIN;
DROP TABLE collection_membership;
DROP TABLE collection;
COMMIT;

BEGIN;
DROP TABLE component_dependency;
COMMIT;

BEGIN;
DROP TRIGGER component_version_freeze ON component_version;
DROP FUNCTION enforce_component_version_freeze();
DROP TABLE component_version;
DROP TABLE component;
COMMIT;

BEGIN;
DROP TABLE block_redirect;
DROP TABLE page_anchor_alias;
COMMIT;

BEGIN;
DROP TABLE working_document_snapshot;
DROP TABLE working_document_update;
DROP TABLE working_document;
COMMIT;

BEGIN;
DROP TABLE auth_session;
DROP TABLE oidc_login_attempt;
DROP TABLE external_identity;
COMMIT;

BEGIN;

DROP TABLE IF EXISTS search_document;
DROP FUNCTION IF EXISTS search_document_update_vector();

COMMIT;

BEGIN;
DROP TRIGGER import_extraction_immutable ON import_extraction;
DROP TABLE import_extraction;
DROP TABLE ai_request_usage;
DROP TABLE prompt_template;
DROP TABLE import_stage_run;
DROP TABLE import_run;
DROP INDEX import_job_succeeded_version_key;
ALTER TABLE import_job
    DROP COLUMN updated_at,
    DROP COLUMN progress,
    DROP COLUMN current_stage,
    DROP COLUMN proposal_id,
    DROP COLUMN source_version_id;
COMMIT;

BEGIN;
DROP TABLE page_protection;
DROP TABLE actor_role;
DROP TABLE role;
COMMIT;

BEGIN;

ALTER TABLE audit_event DROP CONSTRAINT audit_event_change_batch_fk;
ALTER TABLE claim DROP CONSTRAINT claim_change_batch_fk;
ALTER TABLE revision DROP CONSTRAINT revision_change_batch_fk;

DROP TRIGGER proposal_operation_after_submit_immutable ON proposal_operation;
DROP FUNCTION protect_submitted_proposal_operation();

DROP TABLE change_batch;
DROP TABLE merge_conflict;
DROP TABLE review_task;
DROP TABLE proposal_operation;
DROP TABLE proposal;
DROP TABLE import_job;

COMMIT;

BEGIN;

DROP TABLE citation_usage;
DROP TABLE claim_usage;
DROP TABLE entity_mention_projection;

COMMIT;

BEGIN;

DROP TABLE external_link_usage;

COMMIT;

BEGIN;

DROP TABLE rendered_page;

COMMIT;

BEGIN;

ALTER TABLE claim_source DROP CONSTRAINT claim_source_citation_fk;

DROP TABLE citation;
DROP TABLE source_chunk;
DROP TABLE source_version;
DROP TABLE source;
ALTER TABLE asset DROP CONSTRAINT asset_current_revision_fk;
DROP TABLE asset_revision;
DROP TABLE asset;
DROP TABLE external_resource;

COMMIT;

BEGIN;

DROP TABLE page_anchor;
DROP TABLE document_outline_projection;
DROP TABLE page_link_projection;

COMMIT;

BEGIN;

DROP TABLE projection_state;

COMMIT;

BEGIN;

DELETE FROM property WHERE id IN (
    '00000000-0000-7000-8000-000000000401',
    '00000000-0000-7000-8000-000000000402',
    '00000000-0000-7000-8000-000000000403',
    '00000000-0000-7000-8000-000000000404',
    '00000000-0000-7000-8000-000000000405',
    '00000000-0000-7000-8000-000000000406',
    '00000000-0000-7000-8000-000000000407',
    '00000000-0000-7000-8000-000000000408'
);

DELETE FROM entity_type WHERE id IN (
    '00000000-0000-7000-8000-000000000301',
    '00000000-0000-7000-8000-000000000302',
    '00000000-0000-7000-8000-000000000303',
    '00000000-0000-7000-8000-000000000304',
    '00000000-0000-7000-8000-000000000305',
    '00000000-0000-7000-8000-000000000306',
    '00000000-0000-7000-8000-000000000307',
    '00000000-0000-7000-8000-000000000308',
    '00000000-0000-7000-8000-000000000309',
    '00000000-0000-7000-8000-000000000310'
);

COMMIT;

BEGIN;

ALTER TABLE page DROP CONSTRAINT IF EXISTS page_primary_entity_fk;

DROP TABLE IF EXISTS page_entity_binding;
DROP TABLE IF EXISTS claim_source;
DROP TABLE IF EXISTS claim;
DROP TABLE IF EXISTS property;
DROP TABLE IF EXISTS entity_alias;
DROP TABLE IF EXISTS entity_label;
DROP TABLE IF EXISTS entity;
DROP TABLE IF EXISTS entity_type;

DROP FUNCTION IF EXISTS reject_knowledge_delete();

COMMIT;

BEGIN;

DELETE FROM actor      WHERE id = '00000000-0000-7000-8000-000000000201';
DELETE FROM namespace  WHERE wiki_id = '00000000-0000-7000-8000-000000000001';
DELETE FROM wiki_site  WHERE id = '00000000-0000-7000-8000-000000000001';

COMMIT;

BEGIN;

DROP TABLE IF EXISTS outbox_event;
DROP TABLE IF EXISTS audit_event;

ALTER TABLE page DROP CONSTRAINT IF EXISTS page_current_revision_fk;

DROP TABLE IF EXISTS revision;
DROP TABLE IF EXISTS content_snapshot;
DROP TABLE IF EXISTS page_redirect;
DROP TABLE IF EXISTS page_alias;
DROP TABLE IF EXISTS page;

DROP TABLE IF EXISTS actor;
DROP TABLE IF EXISTS namespace;
DROP TABLE IF EXISTS wiki_site;

DROP FUNCTION IF EXISTS check_page_current_revision();
DROP FUNCTION IF EXISTS reject_immutable_mutation();

COMMIT;
