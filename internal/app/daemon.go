package app

import "context"

// StartDaemon starts the sandboxd daemon and refreshes the sandbox views.
func (a *App) StartDaemon(ctx context.Context) error {
	if err := a.Sbx.StartDaemon(ctx); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	a.requestReconcile()
	return nil
}
