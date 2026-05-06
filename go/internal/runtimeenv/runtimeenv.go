package runtimeenv

// DetectRuntime returns "claude" or "codex" based on environment.
func DetectRuntime() string {
	panic("not implemented")
}

// RuntimeHome returns the runtime-specific home directory (~/.claude or ~/.codex).
func RuntimeHome() string {
	panic("not implemented")
}

// ClaudeHome returns ~/.claude regardless of active runtime.
func ClaudeHome() string {
	panic("not implemented")
}
