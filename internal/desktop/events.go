package desktop

import (
	"context"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventName carries every hub envelope to the frontend. Wails events replace
// the old websocket transport; the envelope shape is unchanged, so the
// frontend store keeps consuming envelopes exactly as before.
const EventName = "hub:event"

func init() {
	application.RegisterEvent[app.Event](EventName)
}

// Bridge forwards hub envelopes to emit until ctx is cancelled. It is a
// package function, not a method, so it is never exposed as a binding; the
// emit callback keeps it testable without a running Wails application.
func Bridge(ctx context.Context, d *Desktop, emit func(app.Event)) {
	events, unsubscribe := d.app.Hub.Subscribe()
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			emit(event)
		}
	}
}
