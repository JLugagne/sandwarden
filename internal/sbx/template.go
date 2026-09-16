package sbx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Template is one image in the sandbox runtime's template store
// (`sbx template ls --json`).
type Template struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Flavor     string `json:"flavor"`
	CreatedAt  string `json:"created_at"`
	Size       int64  `json:"size"`
}

// Reference returns the value `sbx create --template` and `sbx template rm`
// accept: the repository and tag when present, else the image ID.
func (t Template) Reference() string {
	if strings.TrimSpace(t.Repository) == "" || strings.TrimSpace(t.Tag) == "" {
		return t.ID
	}
	return t.Repository + ":" + t.Tag
}

// ListTemplates returns every template image of the local runtime.
func (c *Client) ListTemplates(ctx context.Context) ([]Template, error) {
	raw, err := c.runCLI(ctx, nil, nil, "template", "ls", "--json")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Images *[]Template `json:"images"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("decode template list: %w", err)
	}
	if payload.Images == nil {
		return nil, errors.New("unexpected template list output")
	}
	return *payload.Images, nil
}

// RemoveTemplate deletes a template image by tag or ID.
func (c *Client) RemoveTemplate(ctx context.Context, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return errors.New("template reference is required")
	}
	_, err := c.runCLI(ctx, nil, nil, "template", "rm", ref)
	return err
}
