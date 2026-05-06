package archive

// Result holds the outcome of archiving a tool output.
type Result struct {
	Archived bool
	Path     string
	Preview  string
}

// Handle processes a PostToolUse hook input and archives the tool output if warranted.
func Handle(hookInput map[string]any, snapshotDir string) (*Result, error) {
	panic("not implemented")
}
