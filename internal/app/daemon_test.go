package app

import (
	"context"
	"testing"
)

func TestStartDaemonRunsCLIAndNotifies(t *testing.T) {
	ctx := context.Background()
	logPath := newSkillsStub(t)
	a, _ := newTestApp(t)

	events, unsubscribe := a.Hub.Subscribe()
	defer unsubscribe()

	if err := a.StartDaemon(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	if got := countCalls(skillCalls(t, logPath), "daemon start"); got != 1 {
		t.Fatalf("expected one 'daemon start' call, got %v", skillCalls(t, logPath))
	}
	waitForTopic(t, events, TopicSandboxes)
}
