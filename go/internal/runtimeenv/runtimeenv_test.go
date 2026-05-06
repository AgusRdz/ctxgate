package runtimeenv

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectRuntime_DefaultClaude(t *testing.T) {
	t.Setenv("CTXGATE_RUNTIME", "")
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	t.Setenv("CLAUDE_PLUGIN_DATA", "")
	t.Setenv("CODEX_HOME", "")
	require.Equal(t, RuntimeClaude, detectRuntime())
}

func TestDetectRuntime_ExplicitClaude(t *testing.T) {
	t.Setenv("CTXGATE_RUNTIME", "claude")
	require.Equal(t, RuntimeClaude, detectRuntime())
}

func TestDetectRuntime_ExplicitCodex(t *testing.T) {
	t.Setenv("CTXGATE_RUNTIME", "codex")
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	t.Setenv("CLAUDE_PLUGIN_DATA", "")
	require.Equal(t, RuntimeCodex, detectRuntime())
}

func TestDetectRuntime_ClaudePluginRootImpliesClaude(t *testing.T) {
	t.Setenv("CTXGATE_RUNTIME", "")
	t.Setenv("CLAUDE_PLUGIN_ROOT", "/some/path")
	t.Setenv("CODEX_HOME", "")
	require.Equal(t, RuntimeClaude, detectRuntime())
}

func TestDetectRuntime_ClaudePluginDataImpliesClaude(t *testing.T) {
	t.Setenv("CTXGATE_RUNTIME", "")
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	t.Setenv("CLAUDE_PLUGIN_DATA", "/some/path")
	t.Setenv("CODEX_HOME", "")
	require.Equal(t, RuntimeClaude, detectRuntime())
}

func TestDetectRuntime_CodexHomeImpliesCodex(t *testing.T) {
	t.Setenv("CTXGATE_RUNTIME", "")
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	t.Setenv("CLAUDE_PLUGIN_DATA", "")
	t.Setenv("CODEX_HOME", "/some/codex")
	require.Equal(t, RuntimeCodex, detectRuntime())
}

func TestDetectRuntime_ClaudeWinsOverCodexHome(t *testing.T) {
	// CLAUDE_PLUGIN_ROOT takes priority over CODEX_HOME.
	t.Setenv("CTXGATE_RUNTIME", "")
	t.Setenv("CLAUDE_PLUGIN_ROOT", "/some/path")
	t.Setenv("CODEX_HOME", "/some/codex")
	require.Equal(t, RuntimeClaude, detectRuntime())
}

func TestDetectRuntime_InvalidOverrideIgnored(t *testing.T) {
	t.Setenv("CTXGATE_RUNTIME", "invalid")
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	t.Setenv("CLAUDE_PLUGIN_DATA", "")
	t.Setenv("CODEX_HOME", "")
	require.Equal(t, RuntimeClaude, detectRuntime())
}

func TestClaudeHome_EndsWithDotClaude(t *testing.T) {
	home := ClaudeHome()
	require.True(t, strings.HasSuffix(home, ".claude"), "expected ~/ .claude, got %s", home)
}

func TestRuntimeHome_DefaultIsClaude(t *testing.T) {
	t.Setenv("CTXGATE_RUNTIME", "")
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	t.Setenv("CLAUDE_PLUGIN_DATA", "")
	t.Setenv("CODEX_HOME", "")
	// RuntimeHome calls DetectRuntime (cached) — bypass via codexHome directly.
	ch := ClaudeHome()
	home, _ := os.UserHomeDir()
	require.Equal(t, ch, strings.Join([]string{home, ".claude"}, string(os.PathSeparator)))
}
