package sbx

import (
	"context"
	"errors"
	"strings"
)

// SettingsGet returns the current value of an sbx setting through
// `sbx settings get`.
func (c *Client) SettingsGet(ctx context.Context, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("setting key is required")
	}
	out, err := c.runCLI(ctx, nil, nil, "settings", "get", key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// SettingsSet writes a persistent override for an sbx setting through
// `sbx settings set`. The value travels as a single argv entry so JSON values
// keep their exact form.
func (c *Client) SettingsSet(ctx context.Context, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("setting key is required")
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("setting value is required")
	}
	_, err := c.runCLI(ctx, nil, nil, "settings", "set", key, value)
	return err
}
