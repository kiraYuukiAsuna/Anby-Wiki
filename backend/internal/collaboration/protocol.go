package collaboration

import (
	"encoding/binary"
	"errors"

	"github.com/google/uuid"
)

const (
	FrameSnapshot byte = 1
	FrameUpdate   byte = 2

	serverFrameHeader = 9
	clientFrameHeader = 16
)

var ErrInvalidFrame = errors.New("collaboration: invalid websocket frame")

// EncodeServerFrame prefixes an opaque Yjs payload with kind and durable
// server sequence. The payload is never interpreted by the Go service.
func EncodeServerFrame(kind byte, sequence int64, payload []byte) ([]byte, error) {
	if (kind != FrameSnapshot && kind != FrameUpdate) || sequence < 0 || len(payload) == 0 {
		return nil, ErrInvalidFrame
	}
	frame := make([]byte, serverFrameHeader+len(payload))
	frame[0] = kind
	binary.BigEndian.PutUint64(frame[1:serverFrameHeader], uint64(sequence))
	copy(frame[serverFrameHeader:], payload)
	return frame, nil
}

func DecodeServerFrame(frame []byte) (kind byte, sequence int64, payload []byte, err error) {
	if len(frame) <= serverFrameHeader {
		return 0, 0, nil, ErrInvalidFrame
	}
	kind = frame[0]
	if kind != FrameSnapshot && kind != FrameUpdate {
		return 0, 0, nil, ErrInvalidFrame
	}
	sequence = int64(binary.BigEndian.Uint64(frame[1:serverFrameHeader]))
	return kind, sequence, append([]byte(nil), frame[serverFrameHeader:]...), nil
}

// DecodeClientUpdate reads a UUID idempotency key followed by opaque Yjs data.
func DecodeClientUpdate(frame []byte) (uuid.UUID, []byte, error) {
	if len(frame) <= clientFrameHeader {
		return uuid.Nil, nil, ErrInvalidFrame
	}
	updateID, err := uuid.FromBytes(frame[:clientFrameHeader])
	if err != nil || updateID == uuid.Nil {
		return uuid.Nil, nil, ErrInvalidFrame
	}
	return updateID, append([]byte(nil), frame[clientFrameHeader:]...), nil
}
