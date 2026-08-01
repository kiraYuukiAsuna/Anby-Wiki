package projection

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrGraphEntityNotFound   = errors.New("projection: graph root entity not found")
	ErrGraphPropertyNotFound = errors.New("projection: graph property not found")
	ErrInvalidGraphQuery     = errors.New("projection: invalid graph query")
)

const (
	GraphDirectionOutbound = "outbound"
	GraphDirectionInbound  = "inbound"
	GraphDirectionBoth     = "both"

	DefaultGraphDepth = 1
	MaxGraphDepth     = 3
	DefaultGraphNodes = 60
	MaxGraphNodes     = 100
	MaxGraphEdges     = 250
)

type EntityGraphQuery struct {
	WikiID      uuid.UUID
	RootID      uuid.UUID
	Direction   string
	PropertyKey string
	Depth       int
	MaxNodes    int
}

type EntityGraphNode struct {
	ID             uuid.UUID
	CanonicalKey   string
	Status         string
	EntityTypeKey  string
	EntityTypeName string
	Label          string
	Language       string
	Description    string
	Depth          int
}

type EntityGraphEdge struct {
	ClaimID            uuid.UUID
	SubjectEntityID    uuid.UUID
	TargetEntityID     uuid.UUID
	PropertyID         uuid.UUID
	PropertyKey        string
	PropertyName       string
	Rank               string
	VerificationStatus string
	ClaimCreatedAt     time.Time
	ProjectedAt        time.Time
}

type EntityGraphResult struct {
	RootID              uuid.UUID
	Direction           string
	PropertyKey         string
	RequestedDepth      int
	ReachedDepth        int
	Nodes               []EntityGraphNode
	Edges               []EntityGraphEdge
	Truncated           bool
	ProjectionUpdatedAt *time.Time
}

type EntityGraphRebuildResult struct {
	Subjects int
	Edges    int
}

// EntityGraphService owns only the disposable entity-edge projection. Claims
// remain authoritative and are never changed here.
type EntityGraphService struct {
	pool *pgxpool.Pool
}

func NewEntityGraphService(pool *pgxpool.Pool) *EntityGraphService {
	return &EntityGraphService{pool: pool}
}

// RebuildSubject replaces all outgoing edges for one subject from currently
// published entity-valued Claims. claim_id is the projection source marker.
func (s *EntityGraphService) RebuildSubject(
	ctx context.Context,
	subjectID uuid.UUID,
) (int, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("projection: begin entity graph rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		"anby:entity-graph:"+subjectID.String(),
	); err != nil {
		return 0, err
	}
	var wikiID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT wiki_id FROM entity WHERE id=$1`,
		subjectID,
	).Scan(&wikiID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrGraphEntityNotFound
		}
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM entity_edge_projection WHERE subject_entity_id=$1`,
		subjectID,
	); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, entityGraphInsertSQL+
		` WHERE c.subject_entity_id=$1 AND c.status='published'
		    AND c.target_entity_id IS NOT NULL
		    AND subject.wiki_id=$2 AND target.wiki_id=$2`,
		subjectID, wikiID,
	)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("projection: commit entity graph rebuild: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// RebuildWiki atomically replaces the complete Wiki graph projection.
func (s *EntityGraphService) RebuildWiki(
	ctx context.Context,
	wikiID uuid.UUID,
) (*EntityGraphRebuildResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("projection: begin full entity graph rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		"anby:entity-graph-wiki:"+wikiID.String(),
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM entity_edge_projection WHERE wiki_id=$1`,
		wikiID,
	); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, entityGraphInsertSQL+
		` WHERE c.status='published' AND c.target_entity_id IS NOT NULL
		    AND subject.wiki_id=$1 AND target.wiki_id=$1`,
		wikiID,
	)
	if err != nil {
		return nil, err
	}
	var subjects int
	if err := tx.QueryRow(ctx,
		`SELECT count(DISTINCT subject_entity_id)
		 FROM entity_edge_projection WHERE wiki_id=$1`,
		wikiID,
	).Scan(&subjects); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("projection: commit full entity graph rebuild: %w", err)
	}
	return &EntityGraphRebuildResult{
		Subjects: subjects,
		Edges:    int(tag.RowsAffected()),
	}, nil
}

const entityGraphInsertSQL = `
	INSERT INTO entity_edge_projection (
		claim_id,wiki_id,subject_entity_id,target_entity_id,
		property_id,property_key,property_name,rank,
		verification_status,claim_created_at,projected_at)
	SELECT c.id,subject.wiki_id,c.subject_entity_id,c.target_entity_id,
		p.id,p.property_key,p.name,c.rank,c.verification_status,c.created_at,now()
	FROM claim c
	JOIN property p ON p.id=c.property_id
	JOIN entity subject ON subject.id=c.subject_entity_id
	JOIN entity target ON target.id=c.target_entity_id`

func (s *EntityGraphService) Query(
	ctx context.Context,
	query EntityGraphQuery,
) (*EntityGraphResult, error) {
	query.Direction = strings.TrimSpace(query.Direction)
	if query.Direction == "" {
		query.Direction = GraphDirectionBoth
	}
	switch query.Direction {
	case GraphDirectionOutbound, GraphDirectionInbound, GraphDirectionBoth:
	default:
		return nil, ErrInvalidGraphQuery
	}
	query.PropertyKey = strings.TrimSpace(query.PropertyKey)
	if query.Depth == 0 {
		query.Depth = DefaultGraphDepth
	}
	if query.Depth < 1 || query.Depth > MaxGraphDepth {
		return nil, ErrInvalidGraphQuery
	}
	if query.MaxNodes == 0 {
		query.MaxNodes = DefaultGraphNodes
	}
	if query.MaxNodes < 2 || query.MaxNodes > MaxGraphNodes {
		return nil, ErrInvalidGraphQuery
	}
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM entity
		   WHERE id=$1 AND wiki_id=$2 AND status IN ('active','merged')
		 )`,
		query.RootID, query.WikiID,
	).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrGraphEntityNotFound
	}
	if query.PropertyKey != "" {
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM property WHERE property_key=$1)`,
			query.PropertyKey,
		).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrGraphPropertyNotFound
		}
	}

	depthByNode := map[uuid.UUID]int{query.RootID: 0}
	frontier := []uuid.UUID{query.RootID}
	edgeByClaim := make(map[uuid.UUID]EntityGraphEdge)
	result := &EntityGraphResult{
		RootID: query.RootID, Direction: query.Direction,
		PropertyKey: query.PropertyKey, RequestedDepth: query.Depth,
		Nodes: []EntityGraphNode{}, Edges: []EntityGraphEdge{},
	}
	for level := 1; level <= query.Depth && len(frontier) > 0; level++ {
		edges, overflow, err := s.graphEdges(
			ctx, query.WikiID, frontier, query.Direction, query.PropertyKey,
		)
		if err != nil {
			return nil, err
		}
		result.Truncated = result.Truncated || overflow
		next := make([]uuid.UUID, 0)
		for _, edge := range edges {
			if _, seen := edgeByClaim[edge.ClaimID]; seen {
				continue
			}
			candidates := graphCandidates(edge, frontier, query.Direction)
			canInclude := true
			for _, candidate := range candidates {
				if _, seen := depthByNode[candidate]; seen {
					continue
				}
				if len(depthByNode) >= query.MaxNodes {
					result.Truncated = true
					canInclude = false
					break
				}
				depthByNode[candidate] = level
				next = append(next, candidate)
			}
			if !canInclude {
				continue
			}
			edgeByClaim[edge.ClaimID] = edge
			if len(edgeByClaim) >= MaxGraphEdges {
				result.Truncated = true
				break
			}
		}
		if len(next) > 0 {
			result.ReachedDepth = level
		}
		frontier = uniqueUUIDs(next)
		if len(edgeByClaim) >= MaxGraphEdges {
			break
		}
	}

	nodeIDs := make([]uuid.UUID, 0, len(depthByNode))
	for nodeID := range depthByNode {
		nodeIDs = append(nodeIDs, nodeID)
	}
	nodes, err := s.graphNodes(ctx, query.WikiID, nodeIDs, depthByNode)
	if err != nil {
		return nil, err
	}
	result.Nodes = nodes
	result.Edges = make([]EntityGraphEdge, 0, len(edgeByClaim))
	for _, edge := range edgeByClaim {
		result.Edges = append(result.Edges, edge)
		if result.ProjectionUpdatedAt == nil ||
			edge.ProjectedAt.After(*result.ProjectionUpdatedAt) {
			updated := edge.ProjectedAt
			result.ProjectionUpdatedAt = &updated
		}
	}
	sort.Slice(result.Edges, func(i, j int) bool {
		left, right := result.Edges[i], result.Edges[j]
		if left.PropertyKey != right.PropertyKey {
			return left.PropertyKey < right.PropertyKey
		}
		if left.SubjectEntityID != right.SubjectEntityID {
			return left.SubjectEntityID.String() < right.SubjectEntityID.String()
		}
		if left.TargetEntityID != right.TargetEntityID {
			return left.TargetEntityID.String() < right.TargetEntityID.String()
		}
		return left.ClaimID.String() < right.ClaimID.String()
	})
	return result, nil
}

func (s *EntityGraphService) graphEdges(
	ctx context.Context,
	wikiID uuid.UUID,
	frontier []uuid.UUID,
	direction, propertyKey string,
) ([]EntityGraphEdge, bool, error) {
	var relation string
	switch direction {
	case GraphDirectionOutbound:
		relation = `subject_entity_id=ANY($2::uuid[])`
	case GraphDirectionInbound:
		relation = `target_entity_id=ANY($2::uuid[])`
	default:
		relation = `(subject_entity_id=ANY($2::uuid[])
		             OR target_entity_id=ANY($2::uuid[]))`
	}
	rows, err := s.pool.Query(ctx, `SELECT
		claim_id,subject_entity_id,target_entity_id,property_id,
		property_key,property_name,rank,verification_status,
		claim_created_at,projected_at
		FROM entity_edge_projection
		WHERE wiki_id=$1 AND `+relation+`
		  AND ($3='' OR property_key=$3)
		ORDER BY property_key,subject_entity_id,target_entity_id,claim_id
		LIMIT $4`,
		wikiID, frontier, propertyKey, MaxGraphEdges+1,
	)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	edges := make([]EntityGraphEdge, 0)
	overflow := false
	for rows.Next() {
		var edge EntityGraphEdge
		if err := rows.Scan(
			&edge.ClaimID, &edge.SubjectEntityID, &edge.TargetEntityID,
			&edge.PropertyID, &edge.PropertyKey, &edge.PropertyName,
			&edge.Rank, &edge.VerificationStatus, &edge.ClaimCreatedAt,
			&edge.ProjectedAt,
		); err != nil {
			return nil, false, err
		}
		if len(edges) == MaxGraphEdges {
			overflow = true
			continue
		}
		edges = append(edges, edge)
	}
	return edges, overflow, rows.Err()
}

func (s *EntityGraphService) graphNodes(
	ctx context.Context,
	wikiID uuid.UUID,
	nodeIDs []uuid.UUID,
	depthByNode map[uuid.UUID]int,
) ([]EntityGraphNode, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		e.id,e.canonical_key,e.status,et.type_key,et.name,
		COALESCE(label.label,e.canonical_key),
		COALESCE(label.language,''),COALESCE(label.description,'')
		FROM entity e
		JOIN entity_type et ON et.id=e.entity_type_id
		LEFT JOIN LATERAL (
		  SELECT el.label,el.language,el.description
		  FROM entity_label el WHERE el.entity_id=e.id
		  ORDER BY el.is_primary DESC,el.language,el.label LIMIT 1
		) label ON true
		WHERE e.wiki_id=$1 AND e.id=ANY($2::uuid[])`,
		wikiID, nodeIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := make([]EntityGraphNode, 0, len(nodeIDs))
	for rows.Next() {
		var node EntityGraphNode
		if err := rows.Scan(
			&node.ID, &node.CanonicalKey, &node.Status,
			&node.EntityTypeKey, &node.EntityTypeName,
			&node.Label, &node.Language, &node.Description,
		); err != nil {
			return nil, err
		}
		node.Depth = depthByNode[node.ID]
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Depth != nodes[j].Depth {
			return nodes[i].Depth < nodes[j].Depth
		}
		if nodes[i].Label != nodes[j].Label {
			return nodes[i].Label < nodes[j].Label
		}
		return nodes[i].ID.String() < nodes[j].ID.String()
	})
	return nodes, nil
}

func graphCandidates(
	edge EntityGraphEdge,
	frontier []uuid.UUID,
	direction string,
) []uuid.UUID {
	switch direction {
	case GraphDirectionOutbound:
		return []uuid.UUID{edge.TargetEntityID}
	case GraphDirectionInbound:
		return []uuid.UUID{edge.SubjectEntityID}
	default:
		frontierSet := make(map[uuid.UUID]bool, len(frontier))
		for _, id := range frontier {
			frontierSet[id] = true
		}
		candidates := make([]uuid.UUID, 0, 2)
		if !frontierSet[edge.SubjectEntityID] {
			candidates = append(candidates, edge.SubjectEntityID)
		}
		if !frontierSet[edge.TargetEntityID] {
			candidates = append(candidates, edge.TargetEntityID)
		}
		return candidates
	}
}

func uniqueUUIDs(values []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(values))
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
