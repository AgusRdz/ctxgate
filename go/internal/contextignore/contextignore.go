package contextignore

// Matcher holds compiled patterns from .contextignore files.
type Matcher struct {
	patterns []string
}

// Load reads .contextignore from projectDir and the user's home dir and returns a Matcher.
func Load(projectDir string) (*Matcher, error) {
	panic("not implemented")
}

// Match returns true if path matches any loaded pattern.
// Uses path.Match (not filepath.Match) for cross-platform glob behavior.
func (m *Matcher) Match(path string) bool {
	panic("not implemented")
}
