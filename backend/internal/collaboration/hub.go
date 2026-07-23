package collaboration

import (
	"sync"

	"github.com/google/uuid"
)

const subscriptionBuffer = 64

// Hub broadcasts persisted updates and ephemeral presence within one process.
// PostgreSQL remains the reconnect source of truth.
type Hub struct {
	mu    sync.RWMutex
	rooms map[uuid.UUID]map[*Subscription]struct{}
}

type Subscription struct {
	hub        *Hub
	documentID uuid.UUID
	messages   chan HubMessage
	closed     bool
}

type HubMessage struct {
	Binary bool
	Data   []byte
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[uuid.UUID]map[*Subscription]struct{})}
}

func (h *Hub) Subscribe(documentID uuid.UUID) *Subscription {
	subscription := &Subscription{
		hub: h, documentID: documentID, messages: make(chan HubMessage, subscriptionBuffer),
	}
	h.mu.Lock()
	if h.rooms[documentID] == nil {
		h.rooms[documentID] = make(map[*Subscription]struct{})
	}
	h.rooms[documentID][subscription] = struct{}{}
	h.mu.Unlock()
	return subscription
}

func (s *Subscription) Messages() <-chan HubMessage {
	return s.messages
}

func (s *Subscription) Close() {
	s.hub.mu.Lock()
	if !s.closed {
		s.hub.closeLocked(s)
	}
	s.hub.mu.Unlock()
}

// Broadcast disconnects slow subscribers instead of silently dropping a
// durable update. Their clients reconnect using the last server sequence.
func (h *Hub) Broadcast(documentID uuid.UUID, message HubMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscription := range h.rooms[documentID] {
		copied := HubMessage{Binary: message.Binary, Data: append([]byte(nil), message.Data...)}
		select {
		case subscription.messages <- copied:
		default:
			if !subscription.closed {
				h.closeLocked(subscription)
			}
		}
	}
}

func (h *Hub) closeLocked(subscription *Subscription) {
	subscription.closed = true
	room := h.rooms[subscription.documentID]
	delete(room, subscription)
	if len(room) == 0 {
		delete(h.rooms, subscription.documentID)
	}
	close(subscription.messages)
}

func (h *Hub) RoomSize(documentID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[documentID])
}
