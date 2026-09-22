//go:build darwin

package sbx

// defaultSocketPath is sandboxd's state directory on macOS, as sbx v0.45 reports it.
const defaultSocketPath = "Library/Application Support/com.docker.sandboxes/sandboxes/sandboxd/sandboxd.sock"
