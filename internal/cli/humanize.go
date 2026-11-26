package cli

import (
	"fmt"
	"time"
)

// humanizeAge renders how long ago at was, relative to now. A nil timestamp
// renders "-" so a column stays aligned when the daemon does not report it.
func humanizeAge(at *time.Time, now time.Time) string {
	if at == nil {
		return "-"
	}
	return humanizeAgeAt(at, now)
}

// humanizeAgeAt is humanizeAge with an injectable clock, for tests.
func humanizeAgeAt(at *time.Time, now time.Time) string {
	if at == nil {
		return "-"
	}
	diff := now.Sub(*at)
	if diff < 0 {
		return "just now"
	}
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff/time.Minute))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff/time.Hour))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff/(24*time.Hour)))
	case diff < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(diff/(7*24*time.Hour)))
	case diff < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(diff/(30*24*time.Hour)))
	default:
		return fmt.Sprintf("%dy ago", int(diff/(365*24*time.Hour)))
	}
}
