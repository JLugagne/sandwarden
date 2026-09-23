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
	CheckNotificationAuthorization() (bool, error)
	RequestNotificationAuthorization() (bool, error)
}

// Notifier is the Wails service that owns the platform notification backend
// and exposes Notify, Status and RequestAuthorization to the frontend. A
// backend that fails to start disables notifications instead of the
// application.
type Notifier struct {
	service notificationBackend
	startup error
}

// NotificationStatus tells the settings page whether notifications can be
// delivered. Available is false when the platform backend did not start (on
// macOS: the binary runs outside sandwarden.app, which has no bundle
// identifier); Reason then carries the backend's error.
type NotificationStatus struct {
	Available  bool   `json:"available"`
	Authorized bool   `json:"authorized"`
	Reason     string `json:"reason,omitempty"`
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
		return errors.Join(errors.New("desktop notifications are unavailable"), n.startup)
	}
	if authorized, err := n.service.CheckNotificationAuthorization(); err != nil {
		return errors.Join(errors.New("check notification authorization"), err)
	} else if !authorized {
		return errNotAuthorized
	}
	return n.service.SendNotification(notifications.NotificationOptions{
		ID:    fmt.Sprintf("sandwarden-%d", time.Now().UnixNano()),
		Title: title,
		Body:  body,
	})
}

var errNotAuthorized = errors.New("desktop notifications are not authorized; allow sandwarden in the system notification settings")

// Status reports whether notifications can be delivered right now. It never
// prompts the user.
func (n *Notifier) Status() NotificationStatus {
	if n.startup != nil {
		return NotificationStatus{Reason: n.startup.Error()}
	}
	authorized, err := n.service.CheckNotificationAuthorization()
	if err != nil {
		return NotificationStatus{Available: true, Reason: err.Error()}
	}
	status := NotificationStatus{Available: true, Authorized: authorized}
	if !authorized {
		status.Reason = errNotAuthorized.Error()
	}
	return status
}

// RequestAuthorization asks the platform for permission to notify, showing the
// system prompt on macOS the first time. It returns an error when the backend
// did not start or the request itself failed; a refusal is reported through
// the returned status, not as an error.
func (n *Notifier) RequestAuthorization() (NotificationStatus, error) {
	if n.startup != nil {
		return n.Status(), errors.Join(errors.New("desktop notifications are unavailable"), n.startup)
	}
	if _, err := n.service.RequestNotificationAuthorization(); err != nil {
		return n.Status(), errors.Join(errors.New("request notification authorization"), err)
	}
	return n.Status(), nil
}
