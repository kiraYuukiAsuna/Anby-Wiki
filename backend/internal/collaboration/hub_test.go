package collaboration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestBroadcastExcept(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	documentID := uuid.New()
	sender := hub.Subscribe(documentID)
	peer := hub.Subscribe(documentID)
	defer sender.Close()
	defer peer.Close()

	hub.BroadcastExcept(
		documentID,
		HubMessage{Data: []byte(`{"type":"presence"}`)},
		sender,
	)

	select {
	case <-sender.Messages():
		t.Fatal("sender received its own presence")
	default:
	}

	select {
	case message := <-peer.Messages():
		if string(message.Data) != `{"type":"presence"}` {
			t.Fatalf("peer received %q", message.Data)
		}
	default:
		t.Fatal("peer did not receive presence")
	}
}

func TestRedisHubAcrossInstances(t *testing.T) {
	redisURL := os.Getenv("COLLABORATION_E2E_REDIS_URL")
	if redisURL == "" {
		t.Skip("set COLLABORATION_E2E_REDIS_URL to an isolated Redis instance")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, err := NewRedisHub(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewRedisHub(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	isolatedOptions := *options
	isolatedOptions.DB = 0
	if options.DB == 0 {
		isolatedOptions.DB = 1
	}
	isolatedClient := redis.NewClient(&isolatedOptions)
	defer isolatedClient.Close()
	isolatedHub, err := NewRedisHub(ctx, isolatedClient)
	if err != nil {
		t.Fatal(err)
	}
	defer isolatedHub.Close()
	documentID := uuid.New()
	sender := first.Subscribe(documentID)
	peer := second.Subscribe(documentID)
	unrelated := second.Subscribe(uuid.New())
	isolated := isolatedHub.Subscribe(documentID)
	defer sender.Close()
	defer peer.Close()
	defer unrelated.Close()
	defer isolated.Close()
	first.BroadcastExcept(documentID, HubMessage{Data: []byte("presence")}, sender)
	select {
	case message := <-peer.Messages():
		if string(message.Data) != "presence" {
			t.Fatalf("unexpected broadcast: %q", message.Data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second instance did not receive presence")
	}
	select {
	case <-sender.Messages():
		t.Fatal("sender received an echo from Redis")
	case <-unrelated.Messages():
		t.Fatal("broadcast leaked into an unrelated document")
	case <-isolated.Messages():
		t.Fatal("broadcast leaked into another Redis database")
	default:
	}
	first.Broadcast(documentID, HubMessage{Binary: true, Data: []byte("update")})
	for _, subscription := range []*Subscription{sender, peer} {
		select {
		case message := <-subscription.Messages():
			if !message.Binary || string(message.Data) != "update" {
				t.Fatal("durable update framing changed")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("durable update did not reach both instances")
		}
	}
	second.Close()
	select {
	case _, ok := <-peer.Messages():
		if ok {
			t.Fatal("closed hub left its subscribers connected")
		}
	case <-time.After(time.Second):
		t.Fatal("closed hub did not disconnect subscribers")
	}
}

func TestSlowSubscriberDisconnects(t *testing.T) {
	hub := NewHub()
	documentID := uuid.New()
	subscription := hub.Subscribe(documentID)
	for range subscriptionBuffer + 1 {
		hub.Broadcast(documentID, HubMessage{Binary: true, Data: []byte("update")})
	}
	if hub.RoomSize(documentID) != 0 {
		t.Fatal("slow subscriber kept a connection with dropped updates")
	}
	for range subscription.Messages() {
	}
}
