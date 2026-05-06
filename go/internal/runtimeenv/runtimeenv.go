package runtimeenv

import (
	"os"
	"path/filepath"
	"sync"
)

const (
	RuntimeClaude = "claude"
	RuntimeCodex  = "codex"

	envRuntimeOverride = "CTXGATE_RUNTIME"
	envCodexHome       = "CODEX_HOME"
)

// claudePluginEnvs: presence of any of these implies Claude Code is the active runtime.
var claudePluginEnvs = []string{"CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA"}

var (
	runtimeOnce   sync.Once
	runtimeResult string
)

// DetectRuntime returns "claude" or "codex". Result is cached after first call.
//
// Priority:
//  1. CTXGATE_RUNTIME env var (explicit override)
//  2. CLAUDE_PLUGIN_ROOT or CLAUDE_PLUGIN_DATA present → "claude"
//  3. CODEX_HOME present → "codex"
//  4. Default: "claude"
func DetectRuntime() string {
	runtimeOnce.Do(func() {
		runtimeResult = detectRuntime()
	})
	return runtimeResult
}

// detectRuntime is the uncached core, called directly in tests.
func detectRuntime() string {
	if override := os.Getenv(envRuntimeOverride); override == RuntimeClaude || override == RuntimeCodex {
		return override
	}
	for _, env := range claudePluginEnvs {
		if os.Getenv(env) != "" {
			return RuntimeClaude
		}
	}
	if os.Getenv(envCodexHome) != "" {
		return RuntimeCodex
	}
	return RuntimeClaude
}

// ClaudeHome returns ~/.claude unconditionally.
func ClaudeHome() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// codexHome returns ~/.codex, or CODEX_HOME when it is a safe directory under the user's home.
func codexHome() string {
	home, _ := os.UserHomeDir()
	fallback := filepath.Join(home, ".codex")

	raw := os.Getenv(envCodexHome)
	if raw == "" {
		return fallback
	}
	candidate := filepath.Clean(raw)
	// Must be an existing directory, not a symlink, under the user's home.
	if info, err := os.Lstat(candidate); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fallback
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return fallback
	}
	homeResolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		return fallback
	}
	rel, err := filepath.Rel(homeResolved, resolved)
	if err != nil || len(rel) >= 2 && rel[:2] == ".." {
		return fallback
	}
	return resolved
}

// RuntimeHome returns the home directory for the active runtime.
func RuntimeHome() string {
	if DetectRuntime() == RuntimeCodex {
		return codexHome()
	}
	return ClaudeHome()
}
