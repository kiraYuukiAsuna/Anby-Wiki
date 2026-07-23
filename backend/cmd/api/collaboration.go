package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

const (
	maxPresenceBytes = 4096
	socketWriteLimit = 5 * time.Second
)

type CollaborationAPI struct {
	service       *collaboration.Service
	authorization *governance.AuthorizationService
	hub           *collaboration.Hub
	wikiID        uuid.UUID
}

func NewCollaborationAPI(
	service *collaboration.Service,
	authorization *governance.AuthorizationService,
	hub *collaboration.Hub,
	wikiID uuid.UUID,
) *CollaborationAPI {
	return &CollaborationAPI{
		service: service, authorization: authorization, hub: hub, wikiID: wikiID,
	}
}

func (a *CollaborationAPI) connect(w http.ResponseWriter, r *http.Request) {
	principal, ok := authdomain.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "需要登录")
		return
	}
	pageID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "页面 ID 非法")
		return
	}
	clientID, err := uuid.Parse(r.URL.Query().Get("client_id"))
	if err != nil || clientID == uuid.Nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "client_id 非法")
		return
	}
	lastSequence, err := parseLastSequence(r.URL.Query().Get("last_sequence"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "last_sequence 非法")
		return
	}
	if err := a.authorization.Check(
		r.Context(), principal.ActorID, a.wikiID, governance.ActionEdit, &pageID,
	); err != nil {
		serviceError(w, r, err)
		return
	}
	document, err := a.service.Open(r.Context(), pageID, principal.ActorID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(collaboration.MaxUpdateBytes + 16)

	subscription := a.hub.Subscribe(document.ID)
	defer subscription.Close()
	recovery, err := a.service.LoadSince(r.Context(), document.ID, lastSequence)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "recovery failed")
		return
	}
	if err := writeRecovery(r.Context(), conn, recovery); err != nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	var writers sync.WaitGroup
	writers.Add(1)
	go func() {
		defer writers.Done()
		defer cancel()
		a.writeBroadcasts(ctx, conn, subscription.Messages())
	}()
	defer func() {
		cancel()
		writers.Wait()
	}()

	for {
		messageType, value, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if err := a.authorization.Check(
			ctx, principal.ActorID, a.wikiID, governance.ActionEdit, &pageID,
		); err != nil {
			_ = conn.Close(websocket.StatusPolicyViolation, "edit permission revoked")
			return
		}
		switch messageType {
		case websocket.MessageBinary:
			updateID, updateBytes, err := collaboration.DecodeClientUpdate(value)
			if err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid update frame")
				return
			}
			update, err := a.service.Append(
				ctx, document.ID, principal.ActorID, clientID, updateID, updateBytes,
			)
			if err != nil {
				_ = conn.Close(websocket.StatusInternalError, "persist update failed")
				return
			}
			frame, err := collaboration.EncodeServerFrame(
				collaboration.FrameUpdate, update.Sequence, update.Bytes,
			)
			if err != nil {
				_ = conn.Close(websocket.StatusInternalError, "encode update failed")
				return
			}
			a.hub.Broadcast(document.ID, collaboration.HubMessage{Binary: true, Data: frame})
		case websocket.MessageText:
			presence, err := presenceMessage(value, principal.ActorID)
			if err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid presence")
				return
			}
			a.hub.Broadcast(document.ID, collaboration.HubMessage{Data: presence})
		}
	}
}

func parseLastSequence(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("invalid sequence")
	}
	return value, nil
}

func writeRecovery(ctx context.Context, conn *websocket.Conn, recovery collaboration.Recovery) error {
	hello, err := json.Marshal(map[string]any{
		"type":             "hello",
		"document_id":      recovery.Document.ID,
		"page_id":          recovery.Document.PageID,
		"base_revision_id": recovery.Document.BaseRevisionID,
		"latest_sequence":  recovery.Document.LatestSequence,
		"schema_version":   recovery.Document.SchemaVersion,
		"crdt_codec":       recovery.Document.CRDTCodec,
	})
	if err != nil {
		return err
	}
	if err := writeSocket(ctx, conn, websocket.MessageText, hello); err != nil {
		return err
	}
	if recovery.Snapshot != nil {
		frame, err := collaboration.EncodeServerFrame(
			collaboration.FrameSnapshot,
			recovery.Snapshot.UpToSequence,
			recovery.Snapshot.State,
		)
		if err != nil {
			return err
		}
		if err := writeSocket(ctx, conn, websocket.MessageBinary, frame); err != nil {
			return err
		}
	}
	for _, update := range recovery.Updates {
		frame, err := collaboration.EncodeServerFrame(
			collaboration.FrameUpdate, update.Sequence, update.Bytes,
		)
		if err != nil {
			return err
		}
		if err := writeSocket(ctx, conn, websocket.MessageBinary, frame); err != nil {
			return err
		}
	}
	ready, err := json.Marshal(map[string]any{
		"type":            "ready",
		"latest_sequence": recovery.Document.LatestSequence,
	})
	if err != nil {
		return err
	}
	return writeSocket(ctx, conn, websocket.MessageText, ready)
}

func (a *CollaborationAPI) writeBroadcasts(
	ctx context.Context,
	conn *websocket.Conn,
	messages <-chan collaboration.HubMessage,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-messages:
			if !ok {
				return
			}
			messageType := websocket.MessageText
			if message.Binary {
				messageType = websocket.MessageBinary
			}
			if err := writeSocket(ctx, conn, messageType, message.Data); err != nil {
				return
			}
		}
	}
}

func writeSocket(
	ctx context.Context,
	conn *websocket.Conn,
	messageType websocket.MessageType,
	value []byte,
) error {
	writeCtx, cancel := context.WithTimeout(ctx, socketWriteLimit)
	defer cancel()
	return conn.Write(writeCtx, messageType, value)
}

func presenceMessage(value []byte, actorID uuid.UUID) ([]byte, error) {
	if len(value) == 0 || len(value) > maxPresenceBytes {
		return nil, errors.New("presence size")
	}
	var input struct {
		Type   string          `json:"type"`
		Cursor json.RawMessage `json:"cursor"`
	}
	if err := json.Unmarshal(value, &input); err != nil || input.Type != "presence" ||
		len(input.Cursor) == 0 || !json.Valid(input.Cursor) {
		return nil, errors.New("presence shape")
	}
	output, err := json.Marshal(struct {
		Type    string          `json:"type"`
		ActorID uuid.UUID       `json:"actor_id"`
		Cursor  json.RawMessage `json:"cursor"`
	}{
		Type: "presence", ActorID: actorID, Cursor: input.Cursor,
	})
	if err != nil {
		return nil, fmt.Errorf("encode presence: %w", err)
	}
	return output, nil
}
