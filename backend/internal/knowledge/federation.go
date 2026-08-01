package knowledge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrFederatedWikiNotFound  = errors.New("knowledge: federated wiki 不存在")
	ErrFederatedWikiExists    = errors.New("knowledge: federated wiki key 已存在")
	ErrFederationLinkNotFound = errors.New("knowledge: entity federation link 不存在")
	ErrFederationLinkExists   = errors.New("knowledge: entity federation link 已存在")
	ErrInvalidFederation      = errors.New("knowledge: federation 参数非法")
	ErrFederatedWikiDisabled  = errors.New("knowledge: federated wiki 已停用")
)

const (
	FederationTrustUntrusted = "untrusted"
	FederationTrustReference = "reference"
	FederationTrustTrusted   = "trusted"

	FederatedWikiActive   = "active"
	FederatedWikiDisabled = "disabled"

	FederationSameAs   = "same_as"
	FederationBroader  = "broader"
	FederationNarrower = "narrower"
	FederationRelated  = "related"

	FederationUnverified    = "unverified"
	FederationHumanVerified = "human_verified"
	FederationDisputed      = "disputed"

	FederationLinkActive     = "active"
	FederationLinkDeprecated = "deprecated"

	DefaultFederationPageSize = 30
	MaxFederationPageSize     = 100
)

var federationKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,99}$`)

type FederatedWiki struct {
	ID                uuid.UUID
	WikiID            uuid.UUID
	WikiKey           string
	DisplayName       string
	BaseURL           string
	EntityURLTemplate string
	TrustLevel        string
	Status            string
	Metadata          json.RawMessage
	CreatedBy         uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type RegisterFederatedWikiParams struct {
	WikiID            uuid.UUID
	WikiKey           string
	DisplayName       string
	BaseURL           string
	EntityURLTemplate string
	TrustLevel        string
	Status            string
	Metadata          json.RawMessage
	ActorID           uuid.UUID
}

type UpdateFederatedWikiParams struct {
	DisplayName       string
	BaseURL           string
	EntityURLTemplate string
	TrustLevel        string
	Status            string
	Metadata          json.RawMessage
	ActorID           uuid.UUID
}

type EntityFederationLink struct {
	ID                 uuid.UUID
	WikiID             uuid.UUID
	LocalEntityID      uuid.UUID
	RemoteWikiID       uuid.UUID
	RemoteEntityID     string
	RemoteCanonicalKey string
	RemoteLabel        string
	RelationType       string
	VerificationStatus string
	Status             string
	Metadata           json.RawMessage
	CreatedBy          uuid.UUID
	UpdatedBy          uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CreateFederationLinkParams struct {
	WikiID             uuid.UUID
	LocalEntityID      uuid.UUID
	RemoteWikiID       uuid.UUID
	RemoteEntityID     string
	RemoteCanonicalKey string
	RemoteLabel        string
	RelationType       string
	VerificationStatus string
	Metadata           json.RawMessage
	ActorID            uuid.UUID
}

type UpdateFederationLinkParams struct {
	RemoteCanonicalKey string
	RemoteLabel        string
	RelationType       string
	VerificationStatus string
	Status             string
	Metadata           json.RawMessage
	ActorID            uuid.UUID
}

type FederationLinkView struct {
	EntityFederationLink
	LocalCanonicalKey string
	LocalLabel        string
	LocalStatus       string
	RemoteWikiKey     string
	RemoteWikiName    string
	RemoteWikiStatus  string
	RemoteTrustLevel  string
	RemoteEntityURL   string
}

type FederationLinkListParams struct {
	WikiID        uuid.UUID
	LocalEntityID *uuid.UUID
	RemoteWikiID  *uuid.UUID
	Status        string
	Query         string
	Cursor        string
	Limit         int
}

type FederationLinkPage struct {
	Items      []FederationLinkView
	NextCursor *string
}

type federationCursor struct {
	UpdatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

func (s *Service) RegisterFederatedWiki(
	ctx context.Context,
	params RegisterFederatedWikiParams,
) (*FederatedWiki, error) {
	normalized, err := normalizeFederatedWiki(params)
	if err != nil {
		return nil, err
	}
	var result *FederatedWiki
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.pages.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		id, err := s.ids.New()
		if err != nil {
			return err
		}
		normalized.ID = id
		normalized.CreatedBy = params.ActorID
		err = s.repo.q(tx).QueryRow(ctx, `INSERT INTO federated_wiki
			(id,wiki_id,wiki_key,display_name,base_url,entity_url_template,
			 trust_level,status,metadata_json,created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			RETURNING created_at,updated_at`,
			normalized.ID, normalized.WikiID, normalized.WikiKey,
			normalized.DisplayName, normalized.BaseURL,
			normalized.EntityURLTemplate, normalized.TrustLevel,
			normalized.Status, normalized.Metadata, normalized.CreatedBy,
		).Scan(&normalized.CreatedAt, &normalized.UpdatedAt)
		if err != nil {
			if isFederationUniqueViolation(err) {
				return ErrFederatedWikiExists
			}
			return err
		}
		if err := s.insertFederationAudit(
			ctx, tx, params.ActorID, "federated_wiki.registered",
			"federated_wiki", normalized.ID, map[string]any{
				"wiki_key":    normalized.WikiKey,
				"trust_level": normalized.TrustLevel,
				"status":      normalized.Status,
			},
		); err != nil {
			return err
		}
		copy := normalized
		result = &copy
		return nil
	})
	return result, err
}

func (s *Service) UpdateFederatedWiki(
	ctx context.Context,
	wikiID, remoteWikiID uuid.UUID,
	params UpdateFederatedWikiParams,
) (*FederatedWiki, error) {
	normalized, err := normalizeFederatedWiki(RegisterFederatedWikiParams{
		WikiID: wikiID, WikiKey: "placeholder",
		DisplayName: params.DisplayName, BaseURL: params.BaseURL,
		EntityURLTemplate: params.EntityURLTemplate,
		TrustLevel:        params.TrustLevel, Status: params.Status,
		Metadata: params.Metadata, ActorID: params.ActorID,
	})
	if err != nil {
		return nil, err
	}
	var result *FederatedWiki
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.pages.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		current, err := s.getFederatedWiki(ctx, tx, remoteWikiID)
		if err != nil {
			return err
		}
		if current.WikiID != wikiID {
			return ErrFederatedWikiNotFound
		}
		err = s.repo.q(tx).QueryRow(ctx, `UPDATE federated_wiki SET
			display_name=$3,base_url=$4,entity_url_template=$5,
			trust_level=$6,status=$7,metadata_json=$8,updated_at=now()
			WHERE id=$1 AND wiki_id=$2
			RETURNING updated_at`,
			remoteWikiID, wikiID, normalized.DisplayName,
			normalized.BaseURL, normalized.EntityURLTemplate,
			normalized.TrustLevel, normalized.Status, normalized.Metadata,
		).Scan(&current.UpdatedAt)
		if err != nil {
			return err
		}
		current.DisplayName = normalized.DisplayName
		current.BaseURL = normalized.BaseURL
		current.EntityURLTemplate = normalized.EntityURLTemplate
		current.TrustLevel = normalized.TrustLevel
		current.Status = normalized.Status
		current.Metadata = normalized.Metadata
		if err := s.insertFederationAudit(
			ctx, tx, params.ActorID, "federated_wiki.updated",
			"federated_wiki", current.ID, map[string]any{
				"wiki_key":    current.WikiKey,
				"trust_level": current.TrustLevel,
				"status":      current.Status,
			},
		); err != nil {
			return err
		}
		result = current
		return nil
	})
	return result, err
}

func (s *Service) ListFederatedWikis(
	ctx context.Context,
	wikiID uuid.UUID,
	includeDisabled bool,
) ([]FederatedWiki, error) {
	rows, err := s.repo.q(nil).Query(ctx, `SELECT
		id,wiki_id,wiki_key,display_name,base_url,entity_url_template,
		trust_level,status,metadata_json,created_by,created_at,updated_at
		FROM federated_wiki
		WHERE wiki_id=$1 AND ($2 OR status='active')
		ORDER BY display_name,wiki_key,id`,
		wikiID, includeDisabled,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []FederatedWiki{}
	for rows.Next() {
		item, err := scanFederatedWiki(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *item)
	}
	return result, rows.Err()
}

func (s *Service) CreateFederationLink(
	ctx context.Context,
	params CreateFederationLinkParams,
) (*FederationLinkView, error) {
	normalized, err := normalizeFederationLink(params)
	if err != nil {
		return nil, err
	}
	var linkID uuid.UUID
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.pages.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		entity, err := s.lockActiveEntity(ctx, tx, params.LocalEntityID)
		if err != nil {
			return err
		}
		if entity.WikiID != params.WikiID {
			return ErrEntityNotFound
		}
		remote, err := s.getFederatedWiki(ctx, tx, params.RemoteWikiID)
		if err != nil {
			return err
		}
		if remote.WikiID != params.WikiID {
			return ErrFederatedWikiNotFound
		}
		if remote.Status != FederatedWikiActive {
			return ErrFederatedWikiDisabled
		}
		linkID, err = s.ids.New()
		if err != nil {
			return err
		}
		_, err = s.repo.q(tx).Exec(ctx, `INSERT INTO entity_federation_link
			(id,wiki_id,local_entity_id,remote_wiki_id,remote_entity_id,
			 remote_canonical_key,remote_label,relation_type,
			 verification_status,status,metadata_json,created_by,updated_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'active',$10,$11,$11)`,
			linkID, params.WikiID, params.LocalEntityID,
			params.RemoteWikiID, normalized.RemoteEntityID,
			normalized.RemoteCanonicalKey, normalized.RemoteLabel,
			normalized.RelationType, normalized.VerificationStatus,
			normalized.Metadata, params.ActorID,
		)
		if err != nil {
			if isFederationUniqueViolation(err) {
				return ErrFederationLinkExists
			}
			return err
		}
		if err := s.repo.TouchEntity(ctx, tx, params.LocalEntityID); err != nil {
			return err
		}
		return s.insertFederationAudit(
			ctx, tx, params.ActorID, "entity_federation.linked",
			"entity_federation_link", linkID, map[string]any{
				"local_entity_id":     params.LocalEntityID,
				"remote_wiki_id":      params.RemoteWikiID,
				"remote_entity_id":    normalized.RemoteEntityID,
				"relation_type":       normalized.RelationType,
				"verification_status": normalized.VerificationStatus,
			},
		)
	})
	if err != nil {
		return nil, err
	}
	return s.getFederationLinkView(ctx, nil, params.WikiID, linkID)
}

func (s *Service) UpdateFederationLink(
	ctx context.Context,
	wikiID, linkID uuid.UUID,
	params UpdateFederationLinkParams,
) (*FederationLinkView, error) {
	normalized, err := normalizeFederationLink(CreateFederationLinkParams{
		RemoteEntityID:     "placeholder",
		RemoteCanonicalKey: params.RemoteCanonicalKey,
		RemoteLabel:        params.RemoteLabel, RelationType: params.RelationType,
		VerificationStatus: params.VerificationStatus,
		Metadata:           params.Metadata,
	})
	if err != nil {
		return nil, err
	}
	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = FederationLinkActive
	}
	if status != FederationLinkActive && status != FederationLinkDeprecated {
		return nil, ErrInvalidFederation
	}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.pages.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		current, err := s.getFederationLink(ctx, tx, linkID, true)
		if err != nil {
			return err
		}
		if current.WikiID != wikiID {
			return ErrFederationLinkNotFound
		}
		if _, err := s.lockActiveEntity(ctx, tx, current.LocalEntityID); err != nil {
			return err
		}
		remote, err := s.getFederatedWiki(ctx, tx, current.RemoteWikiID)
		if err != nil {
			return err
		}
		if status == FederationLinkActive && remote.Status != FederatedWikiActive {
			return ErrFederatedWikiDisabled
		}
		tag, err := s.repo.q(tx).Exec(ctx, `UPDATE entity_federation_link SET
			remote_canonical_key=$3,remote_label=$4,relation_type=$5,
			verification_status=$6,status=$7,metadata_json=$8,
			updated_by=$9,updated_at=now()
			WHERE id=$1 AND wiki_id=$2`,
			linkID, wikiID, normalized.RemoteCanonicalKey,
			normalized.RemoteLabel, normalized.RelationType,
			normalized.VerificationStatus, status, normalized.Metadata,
			params.ActorID,
		)
		if err != nil {
			if isFederationUniqueViolation(err) {
				return ErrFederationLinkExists
			}
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrFederationLinkNotFound
		}
		if err := s.repo.TouchEntity(ctx, tx, current.LocalEntityID); err != nil {
			return err
		}
		return s.insertFederationAudit(
			ctx, tx, params.ActorID, "entity_federation.updated",
			"entity_federation_link", linkID, map[string]any{
				"local_entity_id":     current.LocalEntityID,
				"remote_wiki_id":      current.RemoteWikiID,
				"relation_type":       normalized.RelationType,
				"verification_status": normalized.VerificationStatus,
				"status":              status,
			},
		)
	})
	if err != nil {
		return nil, err
	}
	return s.getFederationLinkView(ctx, nil, wikiID, linkID)
}

func (s *Service) ListFederationLinks(
	ctx context.Context,
	params FederationLinkListParams,
) (*FederationLinkPage, error) {
	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = FederationLinkActive
	}
	if status != FederationLinkActive &&
		status != FederationLinkDeprecated && status != "all" {
		return nil, ErrInvalidFederation
	}
	if params.Limit <= 0 {
		params.Limit = DefaultFederationPageSize
	}
	if params.Limit > MaxFederationPageSize {
		params.Limit = MaxFederationPageSize
	}
	var afterTime *time.Time
	var afterID uuid.UUID
	if params.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(params.Cursor)
		if err != nil {
			return nil, ErrInvalidFederation
		}
		var cursor federationCursor
		if err := json.Unmarshal(raw, &cursor); err != nil ||
			cursor.UpdatedAt.IsZero() || cursor.ID == uuid.Nil {
			return nil, ErrInvalidFederation
		}
		afterTime, afterID = &cursor.UpdatedAt, cursor.ID
	}
	query := strings.TrimSpace(params.Query)
	if len(query) > 255 {
		return nil, ErrInvalidFederation
	}
	pattern := ""
	if query != "" {
		pattern = likePattern(strings.ToLower(query))
	}
	rows, err := s.repo.q(nil).Query(ctx, federationLinkViewSelect+`
		WHERE l.wiki_id=$1
		  AND ($2='all' OR l.status=$2)
		  AND ($3::uuid IS NULL OR l.local_entity_id=$3)
		  AND ($4::uuid IS NULL OR l.remote_wiki_id=$4)
		  AND ($5='' OR lower(l.remote_entity_id) ILIKE $5 ESCAPE '\'
		       OR lower(l.remote_label) ILIKE $5 ESCAPE '\'
		       OR lower(l.remote_canonical_key) ILIKE $5 ESCAPE '\'
		       OR lower(local_entity.canonical_key) ILIKE $5 ESCAPE '\'
		       OR lower(COALESCE(local_label.label,'')) ILIKE $5 ESCAPE '\')
		  AND ($6::timestamptz IS NULL OR (l.updated_at,l.id)<($6,$7))
		ORDER BY l.updated_at DESC,l.id DESC
		LIMIT $8`,
		params.WikiID, status, params.LocalEntityID, params.RemoteWikiID,
		pattern, afterTime, afterID, params.Limit+1,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &FederationLinkPage{
		Items: make([]FederationLinkView, 0, params.Limit),
	}
	for rows.Next() {
		item, err := scanFederationLinkView(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > params.Limit {
		last := result.Items[params.Limit-1]
		raw, _ := json.Marshal(federationCursor{
			UpdatedAt: last.UpdatedAt, ID: last.ID,
		})
		cursor := base64.RawURLEncoding.EncodeToString(raw)
		result.NextCursor = &cursor
		result.Items = result.Items[:params.Limit]
	}
	return result, nil
}

func (s *Service) ListEntityFederationLinks(
	ctx context.Context,
	wikiID, entityID uuid.UUID,
	status string,
) ([]FederationLinkView, error) {
	entity, err := s.repo.GetEntityByID(ctx, nil, entityID)
	if err != nil || entity.WikiID != wikiID {
		return nil, ErrEntityNotFound
	}
	page, err := s.ListFederationLinks(ctx, FederationLinkListParams{
		WikiID: wikiID, LocalEntityID: &entityID,
		Status: status, Limit: MaxFederationPageSize,
	})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func normalizeFederatedWiki(
	params RegisterFederatedWikiParams,
) (FederatedWiki, error) {
	key := strings.ToLower(strings.TrimSpace(params.WikiKey))
	if !federationKeyPattern.MatchString(key) {
		return FederatedWiki{}, ErrInvalidFederation
	}
	name, err := displayLabel(params.DisplayName)
	if err != nil {
		return FederatedWiki{}, ErrInvalidFederation
	}
	baseURL, template, err := normalizeFederationURLs(
		params.BaseURL, params.EntityURLTemplate,
	)
	if err != nil {
		return FederatedWiki{}, err
	}
	trust := strings.TrimSpace(params.TrustLevel)
	if trust == "" {
		trust = FederationTrustReference
	}
	switch trust {
	case FederationTrustUntrusted, FederationTrustReference, FederationTrustTrusted:
	default:
		return FederatedWiki{}, ErrInvalidFederation
	}
	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = FederatedWikiActive
	}
	if status != FederatedWikiActive && status != FederatedWikiDisabled {
		return FederatedWiki{}, ErrInvalidFederation
	}
	metadata, err := normalizeFederationMetadata(params.Metadata)
	if err != nil {
		return FederatedWiki{}, err
	}
	return FederatedWiki{
		WikiID: params.WikiID, WikiKey: key, DisplayName: name,
		BaseURL: baseURL, EntityURLTemplate: template,
		TrustLevel: trust, Status: status, Metadata: metadata,
	}, nil
}

func normalizeFederationLink(
	params CreateFederationLinkParams,
) (EntityFederationLink, error) {
	remoteID := strings.TrimSpace(params.RemoteEntityID)
	if remoteID == "" || len(remoteID) > 512 || strings.ContainsAny(remoteID, "\r\n\x00") {
		return EntityFederationLink{}, ErrInvalidFederation
	}
	remoteKey := strings.TrimSpace(params.RemoteCanonicalKey)
	if len(remoteKey) > 255 || strings.ContainsAny(remoteKey, "\r\n\x00") {
		return EntityFederationLink{}, ErrInvalidFederation
	}
	remoteLabel := strings.TrimSpace(params.RemoteLabel)
	if remoteLabel != "" {
		var err error
		remoteLabel, err = displayLabel(remoteLabel)
		if err != nil {
			return EntityFederationLink{}, ErrInvalidFederation
		}
	}
	relation := strings.TrimSpace(params.RelationType)
	if relation == "" {
		relation = FederationSameAs
	}
	switch relation {
	case FederationSameAs, FederationBroader, FederationNarrower, FederationRelated:
	default:
		return EntityFederationLink{}, ErrInvalidFederation
	}
	verification := strings.TrimSpace(params.VerificationStatus)
	if verification == "" {
		verification = FederationUnverified
	}
	switch verification {
	case FederationUnverified, FederationHumanVerified, FederationDisputed:
	default:
		return EntityFederationLink{}, ErrInvalidFederation
	}
	metadata, err := normalizeFederationMetadata(params.Metadata)
	if err != nil {
		return EntityFederationLink{}, err
	}
	return EntityFederationLink{
		RemoteEntityID: remoteID, RemoteCanonicalKey: remoteKey,
		RemoteLabel: remoteLabel, RelationType: relation,
		VerificationStatus: verification, Metadata: metadata,
	}, nil
}

func normalizeFederationURLs(baseRaw, templateRaw string) (string, string, error) {
	base, err := url.Parse(strings.TrimSpace(baseRaw))
	if err != nil || base.Scheme == "" || base.Host == "" ||
		(base.Scheme != "http" && base.Scheme != "https") ||
		base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return "", "", ErrInvalidFederation
	}
	base.Path = strings.TrimRight(base.Path, "/")
	baseURL := strings.TrimRight(base.String(), "/")
	template := strings.TrimSpace(templateRaw)
	if !strings.Contains(template, "{entity_id}") {
		return "", "", ErrInvalidFederation
	}
	probe, err := url.Parse(strings.ReplaceAll(template, "{entity_id}", "probe"))
	if err != nil || probe.Scheme != base.Scheme || probe.Host != base.Host ||
		probe.User != nil || probe.Fragment != "" {
		return "", "", ErrInvalidFederation
	}
	return baseURL, template, nil
}

func normalizeFederationMetadata(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if len(raw) > 64<<10 {
		return nil, ErrInvalidFederation
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, ErrInvalidFederation
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalidFederation
	}
	return normalized, nil
}

func (s *Service) getFederatedWiki(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
) (*FederatedWiki, error) {
	item, err := scanFederatedWiki(s.repo.q(tx).QueryRow(ctx, `SELECT
		id,wiki_id,wiki_key,display_name,base_url,entity_url_template,
		trust_level,status,metadata_json,created_by,created_at,updated_at
		FROM federated_wiki WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFederatedWikiNotFound
	}
	return item, err
}

func scanFederatedWiki(row pgx.Row) (*FederatedWiki, error) {
	var item FederatedWiki
	if err := row.Scan(
		&item.ID, &item.WikiID, &item.WikiKey, &item.DisplayName,
		&item.BaseURL, &item.EntityURLTemplate, &item.TrustLevel,
		&item.Status, &item.Metadata, &item.CreatedBy,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) getFederationLink(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
	lock bool,
) (*EntityFederationLink, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	var item EntityFederationLink
	err := s.repo.q(tx).QueryRow(ctx, `SELECT
		id,wiki_id,local_entity_id,remote_wiki_id,remote_entity_id,
		remote_canonical_key,remote_label,relation_type,verification_status,
		status,metadata_json,created_by,updated_by,created_at,updated_at
		FROM entity_federation_link WHERE id=$1`+suffix, id).Scan(
		&item.ID, &item.WikiID, &item.LocalEntityID, &item.RemoteWikiID,
		&item.RemoteEntityID, &item.RemoteCanonicalKey, &item.RemoteLabel,
		&item.RelationType, &item.VerificationStatus, &item.Status,
		&item.Metadata, &item.CreatedBy, &item.UpdatedBy,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFederationLinkNotFound
	}
	return &item, err
}

const federationLinkViewSelect = `SELECT
	l.id,l.wiki_id,l.local_entity_id,l.remote_wiki_id,l.remote_entity_id,
	l.remote_canonical_key,l.remote_label,l.relation_type,
	l.verification_status,l.status,l.metadata_json,l.created_by,l.updated_by,
	l.created_at,l.updated_at,
	local_entity.canonical_key,COALESCE(local_label.label,local_entity.canonical_key),
	local_entity.status,remote.wiki_key,remote.display_name,remote.status,
	remote.trust_level,remote.entity_url_template
	FROM entity_federation_link l
	JOIN entity local_entity ON local_entity.id=l.local_entity_id
	JOIN federated_wiki remote ON remote.id=l.remote_wiki_id
	LEFT JOIN LATERAL (
	  SELECT el.label FROM entity_label el
	  WHERE el.entity_id=local_entity.id
	  ORDER BY el.is_primary DESC,el.language,el.label LIMIT 1
	) local_label ON true`

func (s *Service) getFederationLinkView(
	ctx context.Context,
	tx pgx.Tx,
	wikiID, linkID uuid.UUID,
) (*FederationLinkView, error) {
	item, err := scanFederationLinkView(s.repo.q(tx).QueryRow(ctx,
		federationLinkViewSelect+` WHERE l.wiki_id=$1 AND l.id=$2`,
		wikiID, linkID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFederationLinkNotFound
	}
	return item, err
}

func scanFederationLinkView(row pgx.Row) (*FederationLinkView, error) {
	var item FederationLinkView
	var template string
	if err := row.Scan(
		&item.ID, &item.WikiID, &item.LocalEntityID, &item.RemoteWikiID,
		&item.RemoteEntityID, &item.RemoteCanonicalKey, &item.RemoteLabel,
		&item.RelationType, &item.VerificationStatus, &item.Status,
		&item.Metadata, &item.CreatedBy, &item.UpdatedBy,
		&item.CreatedAt, &item.UpdatedAt, &item.LocalCanonicalKey,
		&item.LocalLabel, &item.LocalStatus, &item.RemoteWikiKey,
		&item.RemoteWikiName, &item.RemoteWikiStatus,
		&item.RemoteTrustLevel, &template,
	); err != nil {
		return nil, err
	}
	item.RemoteEntityURL = strings.ReplaceAll(
		template, "{entity_id}", url.PathEscape(item.RemoteEntityID),
	)
	return &item, nil
}

func (s *Service) insertFederationAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	eventType, aggregateType string,
	aggregateID uuid.UUID,
	payload map[string]any,
) error {
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.repo.q(tx).Exec(ctx, `INSERT INTO audit_event
		(id,actor_id,event_type,aggregate_type,aggregate_id,payload_json)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		auditID, actorID, eventType, aggregateType, aggregateID, raw,
	)
	return err
}

func isFederationUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}
