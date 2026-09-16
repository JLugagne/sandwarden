package sbx

import (
	"context"
	"testing"
)

func TestSettingsGetTrimsCLIOutput(t *testing.T) {
	logPath := newStubSbx(t, " [\"docker.io/\"]\n")
	client := New("/nonexistent.sock")

	value, err := client.SettingsGet(context.Background(), "kit.allowedSources")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if value != `["docker.io/"]` {
		t.Fatalf("unexpected value %q", value)
	}
	calls := stubCalls(t, logPath)
	if len(calls) != 1 || calls[0] != "settings get kit.allowedSources" {
		t.Fatalf("unexpected calls %v", calls)
	}
}

func TestSettingsSetPassesJSONAsSingleArg(t *testing.T) {
	logPath := newStubSbx(t, "")
	client := New("/nonexistent.sock")

	value := `["docker.io/","github.com/Acme/kit-repo"]`
	if err := client.SettingsSet(context.Background(), "kit.allowedSources", value); err != nil {
		t.Fatalf("set: %v", err)
	}
	calls := stubCalls(t, logPath)
	want := "settings set kit.allowedSources " + value
	if len(calls) != 1 || calls[0] != want {
		t.Fatalf("want %q, got %v", want, calls)
	}
}

func TestSettingsRequireKeyAndValue(t *testing.T) {
	newStubSbx(t, "")
	client := New("/nonexistent.sock")

	if _, err := client.SettingsGet(context.Background(), "  "); err == nil {
		t.Fatal("expected get to reject an empty key")
	}
	if err := client.SettingsSet(context.Background(), "", "x"); err == nil {
		t.Fatal("expected set to reject an empty key")
	}
	if err := client.SettingsSet(context.Background(), "kit.allowedSources", " "); err == nil {
		t.Fatal("expected set to reject an empty value")
	}
}
