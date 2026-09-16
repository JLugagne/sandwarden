package sbx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// EventTypeLifecycle, EventTypePorts and EventTypePolicy are the daemon's event
// type families accepted by GET /events?type=...
const (
	EventTypeLifecycle = "sandbox.lifecycle"
	EventTypePorts     = "sandbox.ports"
	EventTypePolicy    = "policy.network"
)

// StreamEvents reads the daemon's newline-delimited JSON event stream, invoking
// handle for each event until ctx is cancelled or handle returns an error.
// Passing no types subscribes to every family.
//
// The daemon ends an idle stream with a synthetic {"type":"sync"} event; callers
// should treat that as a keepalive and reconnect on return.
func (c *Client) StreamEvents(ctx context.Context, types []string, handle func(Event) error) error {
	q := url.Values{}
	for _, t := range types {
		q.Add("type", t)
	}
	path := "/events"
	if len(types) > 0 {
		path += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("subscribe events: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		ev.Raw = append(json.RawMessage(nil), line...)
		if err := handle(ev); err != nil {
			return err
		}
	}
	return scanner.Err()
}
