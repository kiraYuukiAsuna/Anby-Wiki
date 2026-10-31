package collaboration

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const subscriptionBuffer = 64
const broadcastChannel = "anbywiki:collaboration:v1"
const broadcastTimeout = 3 * time.Second

// Hub broadcasts persisted updates and ephemeral presence across API processes.
// PostgreSQL remains the reconnect source of truth.
type Hub struct {
	mu         sync.RWMutex
	rooms      map[uuid.UUID]map[*Subscription]struct{}
	redis      *redis.Client
	channel    string
	pubsub     *redis.PubSub
	instanceID uuid.UUID
	cancel     context.CancelFunc
	done       chan struct{}
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

// NewRedisHub waits for the subscription acknowledgement before accepting
// editors. PostgreSQL recovery covers crashes between commit and publish.
func NewRedisHub(ctx context.Context, client *redis.Client) (*Hub, error) {
	ctx, cancel := context.WithCancel(ctx)
	// Redis Pub/Sub is global across logical databases. Include the selected
	// database so isolated deployments cannot receive each other's updates.
	channel := broadcastChannel + ":" + strconv.Itoa(client.Options().DB)
	pubsub := client.Subscribe(ctx, channel)
	startCtx, startCancel := context.WithTimeout(ctx, broadcastTimeout)
	_, err := pubsub.Receive(startCtx)
	startCancel()
	if err != nil {
		cancel()
		_ = pubsub.Close()
		return nil, err
	}
	hub := NewHub()
	hub.pubsub = pubsub
	hub.channel = channel
	hub.redis, hub.instanceID, hub.cancel, hub.done = client, uuid.New(), cancel, make(chan struct{})
	go func() {
		defer close(hub.done)
		defer pubsub.Close()
		defer hub.disconnectAll()
		for ctx.Err() == nil {
			message, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				// Redis may have lost messages while disconnected. Force all
				// local editors through durable recovery before continuing.
				hub.disconnectAll()
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}
			var envelope broadcastEnvelope
			if json.Unmarshal([]byte(message.Payload), &envelope) == nil &&
				envelope.InstanceID != hub.instanceID && envelope.DocumentID != uuid.Nil {
				hub.broadcastLocal(envelope.DocumentID, envelope.Message, nil)
			}
		}
	}()
	return hub, nil
}

type broadcastEnvelope struct {
	InstanceID uuid.UUID
	DocumentID uuid.UUID
	Message    HubMessage
}

func (h *Hub) Close() {
	if h.cancel != nil {
		h.cancel()
		_ = h.pubsub.Close()
		<-h.done
	}
	h.disconnectAll()
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
	h.broadcast(documentID, message, nil)
}

// BroadcastExcept is used for ephemeral presence, which the sender already
// owns locally and does not need echoed back.
func (h *Hub) BroadcastExcept(
	documentID uuid.UUID,
	message HubMessage,
	except *Subscription,
) {
	h.broadcast(documentID, message, except)
}

func (h *Hub) broadcast(
	documentID uuid.UUID,
	message HubMessage,
	except *Subscription,
) {
	h.broadcastLocal(documentID, message, except)
	if h.redis == nil {
		return
	}
	value, err := json.Marshal(broadcastEnvelope{
		InstanceID: h.instanceID, DocumentID: documentID, Message: message,
	})
	if err != nil {
		h.disconnectAll()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), broadcastTimeout)
	defer cancel()
	if h.redis.Publish(ctx, h.channel, value).Err() != nil {
		h.disconnectAll()
	}
}

func (h *Hub) broadcastLocal(
	documentID uuid.UUID,
	message HubMessage,
	except *Subscription,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscription := range h.rooms[documentID] {
		if subscription == except {
			continue
		}
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

func (h *Hub) disconnectAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, room := range h.rooms {
		for subscription := range room {
			h.closeLocked(subscription)
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
