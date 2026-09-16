package desktop

import (
	"context"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/app"
)

func TestBridgeResubscribesWhenHubDropsSlowSubscriber(t *testing.T) {
	hub := app.NewHub()
	d := &Desktop{app: &app.App{Hub: hub}}

	received := make(chan app.Event, 512)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Bridge(ctx, d, func(event app.Event) {
		time.Sleep(time.Millisecond)
		received <- event
	})

	awaitTopic(t, hub, received, "hello")

	for i := 0; i < 200; i++ {
		hub.Publish(app.EventFor("flood", nil))
	}
	time.Sleep(500 * time.Millisecond)

	awaitTopic(t, hub, received, "after")
}

func awaitTopic(t *testing.T, hub *app.Hub, received <-chan app.Event, topic app.Topic) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		hub.Publish(app.EventFor(topic, nil))
		select {
		case event := <-received:
			if event.Topic == topic {
				return
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf("no %q event received", topic)
}
