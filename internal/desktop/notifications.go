package desktop

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// notificationBackend is the part of the Wails notification service used here.
type notificationBackend interface {
	ServiceStartup(ctx context.Context, options application.ServiceOptions) error
	ServiceShutdown() error
	SendNotification(options notifications.NotificationOptions) error
}

// Notifier is the Wails service that owns the platform notification backend
// and exposes a single Notify operation to the frontend. A backend that fails
// to start disables notifications instead of the application.
type Notifier struct {
	service notificationBackend
	startup error
}

// NewNotifier builds the notification service wrapper.
func NewNotifier() *Notifier {
	return &Notifier{service: notifications.New()}
}

// ServiceName identifies the service in Wails logs.
func (n *Notifier) ServiceName() string { return "Notifier" }

// ServiceStartup connects the platform notification backend.
func (n *Notifier) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	n.startup = n.service.ServiceStartup(ctx, options)
	if n.startup != nil {
		log.Printf("desktop notifications disabled: %v", n.startup)
	}
	return nil
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
	if n.startup != nil {
		return fmt.Errorf("desktop notifications are unavailable: %w", n.startup)
	}
	return n.service.SendNotification(notifications.NotificationOptions{
		ID:    fmt.Sprintf("sandwarden-%d", time.Now().UnixNano()),
		Title: title,
		Body:  body,
	})
}
