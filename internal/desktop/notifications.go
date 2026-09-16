package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Notifier is the Wails service that owns the platform notification backend
// and exposes a single Notify operation to the frontend. Wails starts it with
// the application, which connects the platform notifier (D-Bus on Linux).
type Notifier struct {
	service *notifications.NotificationService
}

// NewNotifier builds the notification service wrapper.
func NewNotifier() *Notifier {
	return &Notifier{service: notifications.New()}
}

// ServiceName identifies the service in Wails logs.
func (n *Notifier) ServiceName() string { return "Notifier" }

// ServiceStartup connects the platform notification backend.
func (n *Notifier) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return n.service.ServiceStartup(ctx, options)
}

// ServiceShutdown releases the platform notification backend.
func (n *Notifier) ServiceShutdown() error {
	return n.service.ServiceShutdown()
}

// Notify sends one native desktop notification.
func (n *Notifier) Notify(title, body string) error {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" && body == "" {
		return errors.New("notification title or body is required")
	}
	return n.service.SendNotification(notifications.NotificationOptions{
		ID:    fmt.Sprintf("sandwarden-%d", time.Now().UnixNano()),
		Title: title,
		Body:  body,
	})
}
