package contextignore

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/agusrdz/ctxgate/internal/runtimeenv"
)

const maxPatterns = 200

// Matcher holds compiled patterns from .contextignore files.
type Matcher struct {
	patterns []string
}

// Load reads .contextignore from projectDir and the user's runtime home dir and returns a Matcher.
// Missing files are silently ignored.
func Load(projectDir string) (*Matcher, error) {
	var patterns []string

	patterns = appendIgnoreFile(patterns, filepath.Join(projectDir, ".contextignore"))
	patterns = appendIgnoreFile(patterns, filepath.Join(runtimeenv.RuntimeHome(), ".contextignore"))

	if len(patterns) > maxPatterns {
		patterns = patterns[:maxPatterns]
	}

	return &Matcher{patterns: patterns}, nil
}

// Match returns true if filePath matches any loaded pattern.
// Tests both the full path and just the filename (mirrors Python fnmatch behavior).
// Uses path.Match (not filepath.Match) for cross-platform glob behavior.
func (m *Matcher) Match(filePath string) bool {
	if len(m.patterns) == 0 {
		return false
	}
	normalized := filepath.ToSlash(filePath)
	base := path.Base(normalized)
	for _, pat := range m.patterns {
		if ok, _ := path.Match(pat, normalized); ok {
			return true
		}
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
	}
	return false
}

func appendIgnoreFile(patterns []string, filePath string) []string {
	f, err := os.Open(filePath)
	if err != nil {
		return patterns
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}
