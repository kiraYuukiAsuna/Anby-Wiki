package wikicli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/coder/websocket"
	"github.com/google/uuid"
)

type CollaborationMessage struct {
	Type         string         `json:"type"`
	UpdateID     string         `json:"update_id,omitempty"`
	DataBase64   string         `json:"data_base64,omitempty"`
	Cursor       map[string]any `json:"cursor,omitempty"`
	UpToSequence int64          `json:"up_to_sequence,omitempty"`
	StateBase64  string         `json:"state_base64,omitempty"`
	Compact      bool           `json:"compact,omitempty"`
}

type collaborationFrame struct {
	Kind       string `json:"kind"`
	Sequence   int64  `json:"sequence"`
	DataBase64 string `json:"data_base64"`
	SizeBytes  int    `json:"size_bytes"`
}

type collaborationRecovery struct {
	DocumentID     uuid.UUID            `json:"document_id"`
	LatestSequence int64                `json:"latest_sequence"`
	Snapshots      []collaborationFrame `json:"snapshots"`
	Updates        []collaborationFrame `json:"updates"`
}

type collaborationResult struct {
	PageID          uuid.UUID            `json:"page_id"`
	ClientID        uuid.UUID            `json:"client_id"`
	DocumentID      uuid.UUID            `json:"document_id"`
	LatestSequence  int64                `json:"latest_sequence"`
	Snapshots       []collaborationFrame `json:"snapshots"`
	Updates         []collaborationFrame `json:"updates"`
	Acknowledgments []any                `json:"acknowledgements"`
}

func (a *App) runCollaboration(
	ctx context.Context,
	input Input,
) (Result, int) {
	_, config, err := a.runtimeConfig(input)
	if err != nil {
		return failure(input.Action, "config_invalid", err.Error(), nil), 2
	}
	if config.Token == "" {
		return failure(input.Action, "not_authenticated", "no CLI token is configured", nil), 2
	}
	pageID, err := uuid.Parse(strings.TrimSpace(input.PageID))
	if err != nil || pageID == uuid.Nil {
		return failure(input.Action, "validation_failed", "page_id must be a UUID", nil), 2
	}
	clientID := uuid.New()
	if strings.TrimSpace(input.ClientID) != "" {
		clientID, err = uuid.Parse(strings.TrimSpace(input.ClientID))
		if err != nil || clientID == uuid.Nil {
			return failure(input.Action, "validation_failed", "client_id must be a UUID", nil), 2
		}
	}
	if input.LastSequence < 0 {
		return failure(input.Action, "validation_failed", "last_sequence must be non-negative", nil), 2
	}
	for index := range input.Messages {
		if err := validateCollaborationMessage(input.Messages[index]); err != nil {
			return failure(
				input.Action, "validation_failed",
				fmt.Sprintf("messages[%d]: %v", index, err), nil,
			), 2
		}
	}
	timeout, err := requestTimeout(input.TimeoutSeconds)
	if err != nil {
		return failure(input.Action, "validation_failed", err.Error(), nil), 2
	}
	socketURL, origin, err := collaborationURL(
		config.BaseURL, pageID, clientID, input.LastSequence,
	)
	if err != nil {
		return failure(input.Action, "validation_failed", err.Error(), nil), 2
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	headers := http.Header{
		"Authorization": []string{"Bearer " + config.Token},
		"Origin":        []string{origin},
	}
	conn, response, err := websocket.Dial(
		runCtx, socketURL, &websocket.DialOptions{HTTPHeader: headers},
	)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		return failure(
			input.Action, "collaboration_connect_failed", err.Error(),
			map[string]any{"http_status": status},
		), 1
	}
	defer conn.Close(websocket.StatusNormalClosure, "agent CLI complete")
	conn.SetReadLimit(17 << 20)

	recovery, err := readCollaborationRecovery(runCtx, conn)
	if err != nil {
		return failure(input.Action, "collaboration_recovery_failed", err.Error(), nil), 1
	}
	result := collaborationResult{
		PageID: pageID, ClientID: clientID,
		DocumentID: recovery.DocumentID, LatestSequence: recovery.LatestSequence,
		Snapshots: recovery.Snapshots, Updates: recovery.Updates,
		Acknowledgments: []any{},
	}
	for _, message := range input.Messages {
		acknowledgment, sequence, err := sendCollaborationMessage(
			runCtx, conn, message,
		)
		if err != nil {
			return failure(
				input.Action, "collaboration_message_failed", err.Error(),
				map[string]any{
					"document_id":     result.DocumentID,
					"latest_sequence": result.LatestSequence,
				},
			), 1
		}
		if acknowledgment != nil {
			result.Acknowledgments = append(result.Acknowledgments, acknowledgment)
		}
		if sequence > result.LatestSequence {
			result.LatestSequence = sequence
		}
	}
	return success(input.Action, result), 0
}

func validateCollaborationMessage(message CollaborationMessage) error {
	switch message.Type {
	case "update":
		updateID, err := uuid.Parse(message.UpdateID)
		if err != nil || updateID == uuid.Nil {
			return errors.New("update_id must be a UUID")
		}
		value, err := base64.StdEncoding.DecodeString(message.DataBase64)
		if err != nil || len(value) == 0 {
			return errors.New("data_base64 must contain a non-empty update")
		}
	case "presence":
		if len(message.Cursor) == 0 {
			return errors.New("cursor must be a non-empty object")
		}
		raw, err := json.Marshal(map[string]any{
			"type": "presence", "cursor": message.Cursor,
		})
		if err != nil || len(raw) > 4<<10 {
			return errors.New("presence exceeds 4 KiB")
		}
	case "snapshot":
		if message.UpToSequence < 0 {
			return errors.New("up_to_sequence must be non-negative")
		}
		value, err := base64.StdEncoding.DecodeString(message.StateBase64)
		if err != nil || len(value) == 0 || len(value) > collaboration.MaxSnapshotBytes {
			return errors.New("state_base64 must contain at most 16 MiB")
		}
	default:
		return errors.New("type must be update, presence, or snapshot")
	}
	return nil
}

func collaborationURL(
	baseURL string,
	pageID, clientID uuid.UUID,
	lastSequence int64,
) (string, string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", "", err
	}
	scheme := "ws"
	if base.Scheme == "https" {
		scheme = "wss"
	}
	socket := url.URL{
		Scheme: scheme,
		Host:   base.Host,
		Path:   fmt.Sprintf("/api/v1/pages/%s/collaboration", pageID),
	}
	query := socket.Query()
	query.Set("client_id", clientID.String())
	query.Set("last_sequence", fmt.Sprintf("%d", lastSequence))
	socket.RawQuery = query.Encode()
	origin := (&url.URL{Scheme: base.Scheme, Host: base.Host}).String()
	return socket.String(), origin, nil
}

func readCollaborationRecovery(
	ctx context.Context,
	conn *websocket.Conn,
) (collaborationRecovery, error) {
	result := collaborationRecovery{
		Snapshots: []collaborationFrame{}, Updates: []collaborationFrame{},
	}
	for {
		messageType, payload, err := conn.Read(ctx)
		if err != nil {
			return result, err
		}
		if messageType == websocket.MessageBinary {
			frame, err := decodeCollaborationFrame(payload)
			if err != nil {
				return result, err
			}
			if frame.Kind == "snapshot" {
				result.Snapshots = append(result.Snapshots, frame)
			} else {
				result.Updates = append(result.Updates, frame)
			}
			continue
		}
		var envelope struct {
			Type           string    `json:"type"`
			DocumentID     uuid.UUID `json:"document_id"`
			LatestSequence int64     `json:"latest_sequence"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return result, err
		}
		switch envelope.Type {
		case "hello":
			result.DocumentID = envelope.DocumentID
			result.LatestSequence = envelope.LatestSequence
		case "ready":
			result.LatestSequence = envelope.LatestSequence
			if result.DocumentID == uuid.Nil {
				return result, errors.New("ready arrived before hello")
			}
			return result, nil
		}
	}
}

func sendCollaborationMessage(
	ctx context.Context,
	conn *websocket.Conn,
	message CollaborationMessage,
) (acknowledgment any, sequence int64, err error) {
	switch message.Type {
	case "presence":
		raw, _ := json.Marshal(map[string]any{
			"type": "presence", "cursor": message.Cursor,
		})
		err = conn.Write(ctx, websocket.MessageText, raw)
		return map[string]any{"type": "presence_sent"}, 0, err
	case "update":
		updateID := uuid.MustParse(message.UpdateID)
		value, _ := base64.StdEncoding.DecodeString(message.DataBase64)
		frame := append(append([]byte{}, updateID[:]...), value...)
		if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
			return nil, 0, err
		}
		for {
			messageType, payload, err := conn.Read(ctx)
			if err != nil {
				return nil, 0, err
			}
			if messageType != websocket.MessageBinary {
				continue
			}
			serverFrame, err := decodeCollaborationFrame(payload)
			if err != nil {
				return nil, 0, err
			}
			if serverFrame.Kind == "update" &&
				serverFrame.DataBase64 == message.DataBase64 {
				return serverFrame, serverFrame.Sequence, nil
			}
		}
	case "snapshot":
		raw, _ := json.Marshal(map[string]any{
			"type": "snapshot", "up_to_sequence": message.UpToSequence,
			"state": message.StateBase64, "compact": message.Compact,
		})
		if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
			return nil, 0, err
		}
		for {
			messageType, payload, err := conn.Read(ctx)
			if err != nil {
				return nil, 0, err
			}
			if messageType != websocket.MessageText {
				continue
			}
			var envelope struct {
				Type         string `json:"type"`
				UpToSequence int64  `json:"up_to_sequence"`
			}
			if json.Unmarshal(payload, &envelope) == nil &&
				envelope.Type == "snapshot_saved" {
				return map[string]any{
					"type":           "snapshot_saved",
					"up_to_sequence": envelope.UpToSequence,
				}, envelope.UpToSequence, nil
			}
		}
	default:
		return nil, 0, errors.New("unsupported collaboration message")
	}
}

func decodeCollaborationFrame(raw []byte) (collaborationFrame, error) {
	kind, sequence, payload, err := collaboration.DecodeServerFrame(raw)
	if err != nil {
		return collaborationFrame{}, err
	}
	label := "update"
	if kind == collaboration.FrameSnapshot {
		label = "snapshot"
	}
	return collaborationFrame{
		Kind: label, Sequence: sequence,
		DataBase64: base64.StdEncoding.EncodeToString(payload),
		SizeBytes:  len(payload),
	}, nil
}
