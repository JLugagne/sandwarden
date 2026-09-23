package app

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
)

//go:embed agents_default.md
var defaultAgentsFile string

const (
	agentsFileDir     = "agents"
	agentsFileName    = "AGENTS.md"
	sandboxAgentsFile = sandboxAgentsDir + "/" + agentsFileName
)

// AgentsDoc is the instructions file mounted read-only at
// /home/agent/.agents/AGENTS.md in every running sandbox. Default reports
// whether Content still matches the built-in template.
type AgentsDoc struct {
	Path    string `json:"path"`
	Target  string `json:"target"`
	Content string `json:"content"`
	Default bool   `json:"default"`
}

func (a *App) agentsFilePath() string {
	return filepath.Join(a.Fleet.Dir(), agentsFileDir, agentsFileName)
}

// AgentsFile returns the shared instructions file, writing the built-in
// template first when it does not exist yet.
func (a *App) AgentsFile(ctx context.Context) (AgentsDoc, error) {
	path, err := a.ensureAgentsFile()
	if err != nil {
		return AgentsDoc{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return AgentsDoc{}, errors.Join(errors.New("read agents file"), err)
	}
	return agentsDoc(path, string(raw)), nil
}

// SaveAgentsFile replaces the content of the shared instructions file. It
// rewrites the file in place: running sandboxes bind the file's inode, so a
// write-and-rename would leave them on the old content.
func (a *App) SaveAgentsFile(ctx context.Context, content string) (AgentsDoc, error) {
	path, err := a.ensureAgentsFile()
	if err != nil {
		return AgentsDoc{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return AgentsDoc{}, errors.Join(errors.New("write agents file"), err)
	}
	return agentsDoc(path, content), nil
}

// ResetAgentsFile restores the built-in template.
func (a *App) ResetAgentsFile(ctx context.Context) (AgentsDoc, error) {
	return a.SaveAgentsFile(ctx, defaultAgentsFile)
}

func (a *App) ensureAgentsFile() (string, error) {
	path := a.agentsFilePath()
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", errors.Join(errors.New("create agents dir"), err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, os.ErrExist) {
		return path, nil
	}
	if err != nil {
		return "", errors.Join(errors.New("create agents file"), err)
	}
	_, werr := file.WriteString(defaultAgentsFile)
	if cerr := file.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return "", errors.Join(errors.New("write agents file"), werr)
	}
	return path, nil
}

func agentsDoc(path, content string) AgentsDoc {
	return AgentsDoc{Path: path, Target: sandboxAgentsFile, Content: content, Default: content == defaultAgentsFile}
}

func (a *App) syncAgentsFile(ctx context.Context, name string) (int, []string) {
	path, err := a.ensureAgentsFile()
	if err != nil {
		return 0, []string{err.Error()}
	}
	live, err := a.Sbx.Mounts(ctx, name)
	if err != nil {
		return 0, []string{err.Error()}
	}
	current, err := a.releaseDriftedMount(ctx, name, live, path, sandboxAgentsFile, true)
	if err != nil {
		return 0, []string{agentsFileName + ": " + err.Error()}
	}
	if current {
		return 0, nil
	}
	if err := a.mountRef(ctx, name, mountRefFor(path, sandboxAgentsFile, true)); err != nil {
		return 0, []string{agentsFileName + ": " + err.Error()}
	}
	return 1, nil
}
