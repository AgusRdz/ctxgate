package measure

// EnsureHealth verifies that the session store and snapshot dir are healthy.
func EnsureHealth(snapshotDir string) error {
	panic("not implemented")
}

// QualityCache runs the quality cache check. force rebuilds; warn emits warnings only.
func QualityCache(snapshotDir string, force, warn bool) error {
	panic("not implemented")
}

// CompactCapture captures a compaction snapshot. trigger is "stop", "stop-failure", or "auto".
func CompactCapture(snapshotDir, trigger string) error {
	panic("not implemented")
}

// CompactRestore restores a compaction snapshot. newSessionOnly skips existing sessions.
func CompactRestore(snapshotDir string, newSessionOnly bool) error {
	panic("not implemented")
}

// DynamicCompactInstructions emits dynamic compaction instructions to stdout.
func DynamicCompactInstructions(hookInput map[string]any) error {
	panic("not implemented")
}

// CheckpointTrigger triggers a checkpoint if the milestone has been reached.
func CheckpointTrigger(hookInput map[string]any, milestone string) error {
	panic("not implemented")
}

// SessionEndFlush flushes session telemetry and runs async quality checks.
func SessionEndFlush(sessionID, snapshotDir string) error {
	panic("not implemented")
}
