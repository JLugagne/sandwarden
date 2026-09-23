package sbx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	envSocketPath = "DOCKER_SANDBOXES_API"

	// childWaitDelay bounds how long a cancelled or exited sbx child may keep
	// its captured stdout/stderr pipes open (grandchildren inherit them).
	childWaitDelay = 2 * time.Second

	// MaxConcurrentCLI caps the short sbx CLI calls one Client runs at once, so
	// background sweeps cannot flood the daemon with parallel sessions.
	// Streaming commands (Exec, CreateSandbox, RunCLI) are not counted.
	MaxConcurrentCLI = 3
)

// Client talks to the local sandboxd daemon over its unix socket using the
// daemon's HTTP (REST) API.
type Client struct {
	http     *http.Client
	base     string
	cliSlots chan struct{}
}

// SocketPath resolves the sandboxd unix socket path. Precedence: DOCKER_SANDBOXES_API,
// then the platform default under the user's home directory.
func SocketPath() string {
	if p := os.Getenv(envSocketPath); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultSocketPath
	}
	return filepath.Join(home, defaultSocketPath)
}

// New builds a client bound to socketPath. An empty path resolves via SocketPath.
func New(socketPath string) *Client {
	if socketPath == "" {
		socketPath = SocketPath()
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		http:     &http.Client{Transport: tr},
		base:     "http://sandboxd",
		cliSlots: make(chan struct{}, MaxConcurrentCLI),
	}
}

// APIError is a non-2xx response from the daemon, carrying its JSON message.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("sandboxd returned HTTP %d", e.Status)
}

// IsNotFound reports whether err is a 404 from the daemon.
func IsNotFound(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.Status == http.StatusNotFound
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sandboxd request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp)
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func decodeError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &payload)
	if payload.Message == "" {
		payload.Message = strings.TrimSpace(string(raw))
	}
	return &APIError{Status: resp.StatusCode, Message: payload.Message}
}

// runCLI executes the sbx binary, optionally piping stdin and teeing combined
// output to a stream. It returns the captured output; on a non-zero exit the
// error carries the CLI's own message so callers can surface it directly.
func (c *Client) runCLI(ctx context.Context, stdin io.Reader, stream io.Writer, args ...string) (string, error) {
	release, err := c.acquireCLI(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	cmd := cliCommand(ctx, args...)
	var buf bytes.Buffer
	var out io.Writer = &buf
	if stream != nil {
		out = io.MultiWriter(&buf, stream)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	if stdin != nil {
		cmd.Stdin = stdin
	}
	runErr := cmd.Run()
	captured := buf.String()
	if runErr != nil {
		if ctx.Err() != nil {
			return captured, ctx.Err()
		}
		msg := strings.TrimSpace(captured)
		if msg == "" {
			msg = runErr.Error()
		}
		return captured, errors.New(msg)
	}
	return captured, nil
}

// decodeCLIJSON unmarshals into v the JSON value embedded in the combined sbx
// output raw. runCLI merges stderr into stdout, so CLI diagnostics (keychain
// fallbacks, WARN lines) usually precede the payload. It returns an error when
// raw holds no decodable JSON value.
func decodeCLIJSON(raw string, v any) error {
	for offset := 0; offset < len(raw); {
		line := raw[offset:]
		end := strings.IndexByte(line, '\n')
		if end >= 0 {
			line = line[:end]
		}
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var value json.RawMessage
			if err := json.NewDecoder(strings.NewReader(raw[offset:])).Decode(&value); err == nil {
				return json.Unmarshal(value, v)
			}
		}
		if end < 0 {
			break
		}
		offset += end + 1
	}
	return errors.New("no JSON value in output")
}

func (c *Client) acquireCLI(ctx context.Context) (func(), error) {
	if c.cliSlots == nil {
		return func() {}, nil
	}
	select {
	case c.cliSlots <- struct{}{}:
		return func() { <-c.cliSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func cliCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, BinaryPath(), args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = childWaitDelay
	return cmd
}

type inspectCacheKey struct{}

type inspectCache struct {
	mu      sync.Mutex
	details map[string]InspectDetail
}

// WithInspectCache returns a context under which InspectDetail and Mounts
// reuse one `sbx inspect` per sandbox. Mount changes made through the Client
// with that context drop the sandbox's entry; changes made elsewhere are not
// seen, so scope it to one short pass.
func WithInspectCache(ctx context.Context) context.Context {
	if _, ok := ctx.Value(inspectCacheKey{}).(*inspectCache); ok {
		return ctx
	}
	return context.WithValue(ctx, inspectCacheKey{}, &inspectCache{details: map[string]InspectDetail{}})
}

func cachedInspect(ctx context.Context, sandbox string) (InspectDetail, bool) {
	cache, ok := ctx.Value(inspectCacheKey{}).(*inspectCache)
	if !ok {
		return InspectDetail{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	detail, ok := cache.details[sandbox]
	return detail, ok
}

func storeInspect(ctx context.Context, sandbox string, detail InspectDetail) {
	if cache, ok := ctx.Value(inspectCacheKey{}).(*inspectCache); ok {
		cache.mu.Lock()
		cache.details[sandbox] = detail
		cache.mu.Unlock()
	}
}

func forgetInspect(ctx context.Context, sandbox string) {
	if cache, ok := ctx.Value(inspectCacheKey{}).(*inspectCache); ok {
		cache.mu.Lock()
		delete(cache.details, sandbox)
		cache.mu.Unlock()
	}
}
