package cli

import (
	"testing"
	"time"
)

func TestHumanizeAge(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time {
		t := now.Add(-d)
		return &t
	}
	future := now.Add(time.Hour)

	cases := []struct {
		name string
		at   *time.Time
		want string
	}{
		{name: "unknown", at: nil, want: "-"},
		{name: "just now", at: ago(30 * time.Second), want: "just now"},
		{name: "minutes", at: ago(5 * time.Minute), want: "5m ago"},
		{name: "hours", at: ago(3 * time.Hour), want: "3h ago"},
		{name: "days", at: ago(3 * 24 * time.Hour), want: "3d ago"},
		{name: "weeks", at: ago(2 * 7 * 24 * time.Hour), want: "2w ago"},
		{name: "months", at: ago(90 * 24 * time.Hour), want: "3mo ago"},
		{name: "years", at: ago(400 * 24 * time.Hour), want: "1y ago"},
		{name: "future clamps to just now", at: &future, want: "just now"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := humanizeAgeAt(tc.at, now); got != tc.want {
				t.Errorf("humanizeAgeAt(%v) = %q, want %q", tc.at, got, tc.want)
			}
		})
	}
}
