//go:build !darwin

package sbx

// defaultSocketPath is sandboxd's XDG state directory.
const defaultSocketPath = ".local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock"
