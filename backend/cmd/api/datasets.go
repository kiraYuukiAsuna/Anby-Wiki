package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/dataset"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type DatasetAPI struct {
	service *dataset.Service
	wikiID  uuid.UUID
}

func NewDatasetAPI(service *dataset.Service, wikiID uuid.UUID) *DatasetAPI {
	return &DatasetAPI{service: service, wikiID: wikiID}
}

type createDatasetRequest struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}

func (a *DatasetAPI) list(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.ListDatasets(
		r.Context(), a.wikiID, r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *DatasetAPI) create(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var request createDatasetRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := a.service.CreateDataset(r.Context(), dataset.CreateDatasetParams{
		WikiID: a.wikiID, Name: request.Name, Schema: request.Schema, ActorID: actorID,
	})
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, value)
}

func (a *DatasetAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := datasetPathID(w, r, "id")
	if !ok {
		return
	}
	value, err := a.service.GetDataset(r.Context(), a.wikiID, id)
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, value)
}

type recordRequest struct {
	EntityID *uuid.UUID      `json:"entity_id"`
	Values   json.RawMessage `json:"values"`
}

func (a *DatasetAPI) listRecords(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := datasetPathID(w, r, "id")
	if !ok {
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.ListRecords(
		r.Context(), a.wikiID, datasetID, r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *DatasetAPI) createRecord(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	datasetID, ok := datasetPathID(w, r, "id")
	if !ok {
		return
	}
	var request recordRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := a.service.CreateRecord(r.Context(), dataset.CreateRecordParams{
		WikiID: a.wikiID, DatasetID: datasetID, EntityID: request.EntityID,
		Values: request.Values, ActorID: actorID,
	})
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, value)
}

func (a *DatasetAPI) updateRecord(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	recordID, ok := datasetPathID(w, r, "id")
	if !ok {
		return
	}
	var request recordRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := a.service.UpdateRecord(
		r.Context(), a.wikiID, recordID, actorID, request.EntityID, request.Values,
	)
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, value)
}

type createViewRequest struct {
	ViewType string          `json:"view_type"`
	Name     string          `json:"name"`
	Config   json.RawMessage `json:"config"`
}

func (a *DatasetAPI) listViews(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := datasetPathID(w, r, "id")
	if !ok {
		return
	}
	result, err := a.service.ListViews(r.Context(), a.wikiID, datasetID)
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *DatasetAPI) createView(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	datasetID, ok := datasetPathID(w, r, "id")
	if !ok {
		return
	}
	var request createViewRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := a.service.CreateView(r.Context(), dataset.CreateViewParams{
		WikiID: a.wikiID, DatasetID: datasetID, ViewType: request.ViewType,
		Name: request.Name, Config: request.Config, ActorID: actorID,
	})
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, value)
}

func (a *DatasetAPI) getView(w http.ResponseWriter, r *http.Request) {
	viewID, ok := datasetPathID(w, r, "id")
	if !ok {
		return
	}
	value, err := a.service.GetView(r.Context(), a.wikiID, viewID)
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, value)
}

func (a *DatasetAPI) queryView(w http.ResponseWriter, r *http.Request) {
	viewID, ok := datasetPathID(w, r, "id")
	if !ok {
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.QueryView(
		r.Context(), a.wikiID, viewID, r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		datasetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func datasetPathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	value, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			"id 不是合法 UUID")
		return uuid.Nil, false
	}
	return value, true
}

func datasetError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, dataset.ErrNotFound),
		errors.Is(err, dataset.ErrRecordNotFound),
		errors.Is(err, dataset.ErrViewNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, dataset.ErrDuplicateName):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, dataset.ErrInvalidCursor):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
	case errors.Is(err, dataset.ErrInvalidDefinition),
		errors.Is(err, dataset.ErrInvalidRecord),
		errors.Is(err, dataset.ErrInvalidView):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity,
			httpx.CodeValidationFailed, err.Error())
	default:
		serviceError(w, r, err)
	}
}
