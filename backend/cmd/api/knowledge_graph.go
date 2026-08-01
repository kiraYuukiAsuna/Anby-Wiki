package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/projection"
)

type entityGraphNodeResponse struct {
	ID             uuid.UUID `json:"id"`
	CanonicalKey   string    `json:"canonical_key"`
	Status         string    `json:"status"`
	EntityTypeKey  string    `json:"entity_type_key"`
	EntityTypeName string    `json:"entity_type_name"`
	Label          string    `json:"label"`
	Language       string    `json:"language"`
	Description    string    `json:"description"`
	Depth          int       `json:"depth"`
}

type entityGraphEdgeResponse struct {
	ClaimID            uuid.UUID `json:"claim_id"`
	SubjectEntityID    uuid.UUID `json:"subject_entity_id"`
	TargetEntityID     uuid.UUID `json:"target_entity_id"`
	PropertyID         uuid.UUID `json:"property_id"`
	PropertyKey        string    `json:"property_key"`
	PropertyName       string    `json:"property_name"`
	Rank               string    `json:"rank"`
	VerificationStatus string    `json:"verification_status"`
	ClaimCreatedAt     time.Time `json:"claim_created_at"`
	ProjectedAt        time.Time `json:"projected_at"`
}

type entityGraphResponse struct {
	RootID              uuid.UUID                 `json:"root_id"`
	Direction           string                    `json:"direction"`
	PropertyKey         string                    `json:"property_key"`
	RequestedDepth      int                       `json:"requested_depth"`
	ReachedDepth        int                       `json:"reached_depth"`
	Nodes               []entityGraphNodeResponse `json:"nodes"`
	Edges               []entityGraphEdgeResponse `json:"edges"`
	Truncated           bool                      `json:"truncated"`
	ProjectionUpdatedAt *time.Time                `json:"projection_updated_at,omitempty"`
}

type entityGraphRebuildResponse struct {
	Subjects int `json:"subjects"`
	Edges    int `json:"edges"`
}

func (a *KnowledgeReadAPI) getEntityGraph(
	w http.ResponseWriter,
	r *http.Request,
) {
	entityID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	depth, ok := graphInteger(
		w, r, "depth", projection.DefaultGraphDepth,
		1, projection.MaxGraphDepth,
	)
	if !ok {
		return
	}
	maxNodes, ok := graphInteger(
		w, r, "max_nodes", projection.DefaultGraphNodes,
		2, projection.MaxGraphNodes,
	)
	if !ok {
		return
	}
	if a.entityGraph == nil {
		httpx.WriteError(
			w, r, http.StatusInternalServerError,
			httpx.CodeInternal, "实体图查询未装配",
		)
		return
	}
	result, err := a.entityGraph.Query(
		r.Context(), projection.EntityGraphQuery{
			WikiID: a.wikiID, RootID: entityID,
			Direction:   r.URL.Query().Get("direction"),
			PropertyKey: r.URL.Query().Get("property_key"),
			Depth:       depth, MaxNodes: maxNodes,
		},
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	response := entityGraphResponse{
		RootID: result.RootID, Direction: result.Direction,
		PropertyKey:         result.PropertyKey,
		RequestedDepth:      result.RequestedDepth,
		ReachedDepth:        result.ReachedDepth,
		Nodes:               make([]entityGraphNodeResponse, len(result.Nodes)),
		Edges:               make([]entityGraphEdgeResponse, len(result.Edges)),
		Truncated:           result.Truncated,
		ProjectionUpdatedAt: result.ProjectionUpdatedAt,
	}
	for index, node := range result.Nodes {
		response.Nodes[index] = entityGraphNodeResponse{
			ID: node.ID, CanonicalKey: node.CanonicalKey, Status: node.Status,
			EntityTypeKey: node.EntityTypeKey, EntityTypeName: node.EntityTypeName,
			Label: node.Label, Language: node.Language,
			Description: node.Description, Depth: node.Depth,
		}
	}
	for index, edge := range result.Edges {
		response.Edges[index] = entityGraphEdgeResponse{
			ClaimID:         edge.ClaimID,
			SubjectEntityID: edge.SubjectEntityID,
			TargetEntityID:  edge.TargetEntityID,
			PropertyID:      edge.PropertyID, PropertyKey: edge.PropertyKey,
			PropertyName: edge.PropertyName, Rank: edge.Rank,
			VerificationStatus: edge.VerificationStatus,
			ClaimCreatedAt:     edge.ClaimCreatedAt,
			ProjectedAt:        edge.ProjectedAt,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (a *KnowledgeReadAPI) rebuildEntityGraph(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	if a.authorization == nil || a.entityGraph == nil {
		a.writeError(w, r, governance.ErrPermissionDenied)
		return
	}
	if err := a.authorization.Check(
		r.Context(), actorID, a.wikiID, governance.ActionManage, nil,
	); err != nil {
		a.writeError(w, r, err)
		return
	}
	result, err := a.entityGraph.RebuildWiki(r.Context(), a.wikiID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, entityGraphRebuildResponse{
		Subjects: result.Subjects, Edges: result.Edges,
	})
}

func graphInteger(
	w http.ResponseWriter,
	r *http.Request,
	name string,
	fallback, minimum, maximum int,
) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		httpx.WriteError(
			w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			name+" 必须是 "+strconv.Itoa(minimum)+" 到 "+
				strconv.Itoa(maximum)+" 的整数",
		)
		return 0, false
	}
	return value, true
}
