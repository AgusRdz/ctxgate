package deltadiff

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	MaxDeltaChars        = 1500
	MaxDeltaLines        = 2000
	MaxContentCacheBytes = 50 * 1024
)

// DeltaStats holds line-level add/remove counts from a diff.
type DeltaStats struct {
	Added   int
	Removed int
}

// eligibleExts is the set of file extensions eligible for delta mode.
var eligibleExts = map[string]bool{
	".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".rb": true, ".rs": true, ".go": true, ".java": true, ".kt": true,
	".swift": true, ".c": true, ".cpp": true, ".h": true, ".hpp": true,
	".cs": true, ".php": true, ".sh": true, ".bash": true, ".zsh": true,
	".fish": true, ".yaml": true, ".yml": true, ".toml": true, ".json": true,
	".xml": true, ".html": true, ".css": true, ".scss": true, ".less": true,
	".sql": true, ".md": true, ".txt": true, ".cfg": true, ".ini": true,
	".vue": true, ".svelte": true, ".astro": true, ".ex": true, ".exs": true,
	".erl": true, ".hs": true, ".lua": true, ".r": true, ".jl": true,
	".dart": true, ".scala": true, ".clj": true, ".tf": true, ".hcl": true,
	".dockerfile": true, ".makefile": true,
}

// eligibleNames is the set of extension-less basenames eligible for delta mode.
var eligibleNames = map[string]bool{
	"makefile": true, "dockerfile": true, "gemfile": true,
	"rakefile": true, "procfile": true, "jenkinsfile": true,
	".gitignore": true, ".dockerignore": true,
}

// IsEligible returns true if filePath's extension supports delta diffing.
// .env files are always excluded (credential safety, SEC-F8).
func IsEligible(filePath string) bool {
	name := strings.ToLower(filepath.Base(filePath))
	if strings.HasPrefix(name, ".env") {
		return false
	}
	if eligibleNames[name] {
		return true
	}
	return eligibleExts[strings.ToLower(filepath.Ext(filePath))]
}

// ContentHash returns a hex-encoded SHA-256 digest of content.
func ContentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}

// ComputeDelta computes a unified diff between oldContent and newContent.
// Returns ok=false when content is identical, either input exceeds MaxDeltaLines,
// or the resulting diff text exceeds MaxDeltaChars.
func ComputeDelta(oldContent, newContent, filename string) (deltaText string, stats DeltaStats, ok bool) {
	if oldContent == newContent {
		return "", DeltaStats{}, false
	}

	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	if len(oldLines) > MaxDeltaLines || len(newLines) > MaxDeltaLines {
		return "", DeltaStats{}, false
	}

	edits := lcsEdits(oldLines, newLines)

	var added, removed int
	for _, e := range edits {
		switch e.kind {
		case editInsert:
			added++
		case editDelete:
			removed++
		}
	}

	if added+removed == 0 {
		return "", DeltaStats{}, false
	}

	header := fmt.Sprintf("%s: %d lines changed (+%d/-%d)\n", filename, added+removed, added, removed)
	body := formatUnified(edits, "a/"+filename, "b/"+filename, 1)
	delta := header + body

	if len(delta) > MaxDeltaChars {
		return "", DeltaStats{}, false
	}

	return delta, DeltaStats{Added: added, Removed: removed}, true
}

// --- internal diff machinery ---

type editKind uint8

const (
	editEqual  editKind = iota
	editInsert          // present in new, not old
	editDelete          // present in old, not new
)

type diffLine struct {
	kind editKind
	text string
}

// lcsEdits computes the minimal edit list between a (old) and b (new) via LCS.
// Memory: O(len(a) × len(b)) — guarded by MaxDeltaLines before call.
func lcsEdits(a, b []string) []diffLine {
	m, n := len(a), len(b)

	// Build LCS DP table.
	dp := make([][]int32, m+1)
	for i := range dp {
		dp[i] = make([]int32, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack (edits collected in reverse, then flipped).
	edits := make([]diffLine, 0, m+n)
	i, j := m, n
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			edits = append(edits, diffLine{editEqual, a[i-1]})
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			edits = append(edits, diffLine{editInsert, b[j-1]})
			j--
		default:
			edits = append(edits, diffLine{editDelete, a[i-1]})
			i--
		}
	}

	for l, r := 0, len(edits)-1; l < r; l, r = l+1, r-1 {
		edits[l], edits[r] = edits[r], edits[l]
	}
	return edits
}

// formatUnified renders edits as a unified diff with ctx lines of context.
func formatUnified(edits []diffLine, fromFile, toFile string, ctx int) string {
	n := len(edits)
	if n == 0 {
		return ""
	}

	// Mark which positions are "in context" (within ctx of any change).
	inCtx := make([]bool, n)
	for i, e := range edits {
		if e.kind == editEqual {
			continue
		}
		lo := i - ctx
		if lo < 0 {
			lo = 0
		}
		hi := i + ctx
		if hi >= n {
			hi = n - 1
		}
		for j := lo; j <= hi; j++ {
			inCtx[j] = true
		}
	}

	// Pre-compute cumulative old/new line numbers.
	oldPos := make([]int, n+1)
	newPos := make([]int, n+1)
	oldPos[0] = 1
	newPos[0] = 1
	for i, e := range edits {
		oldPos[i+1] = oldPos[i]
		newPos[i+1] = newPos[i]
		switch e.kind {
		case editEqual:
			oldPos[i+1]++
			newPos[i+1]++
		case editDelete:
			oldPos[i+1]++
		case editInsert:
			newPos[i+1]++
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", fromFile, toFile)

	i := 0
	for i < n {
		if !inCtx[i] {
			i++
			continue
		}

		// Collect the full hunk extent.
		hunkStart := i
		hunkEnd := i + 1
		for hunkEnd < n && inCtx[hunkEnd] {
			hunkEnd++
		}

		// Count old/new lines in this hunk for the @@ header.
		oldCount, newCount := 0, 0
		for j := hunkStart; j < hunkEnd; j++ {
			switch edits[j].kind {
			case editEqual:
				oldCount++
				newCount++
			case editDelete:
				oldCount++
			case editInsert:
				newCount++
			}
		}

		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n",
			oldPos[hunkStart], oldCount,
			newPos[hunkStart], newCount)

		for j := hunkStart; j < hunkEnd; j++ {
			switch edits[j].kind {
			case editEqual:
				sb.WriteByte(' ')
			case editDelete:
				sb.WriteByte('-')
			case editInsert:
				sb.WriteByte('+')
			}
			sb.WriteString(edits[j].text)
		}

		i = hunkEnd
	}

	return sb.String()
}

// splitLines splits s into lines, preserving line endings.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	for s != "" {
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, s[:idx+1])
		s = s[idx+1:]
	}
	return lines
}
