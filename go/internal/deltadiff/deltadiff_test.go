package deltadiff_test

import (
	"strings"
	"testing"

	"github.com/agusrdz/ctxgate/internal/deltadiff"
	"github.com/stretchr/testify/require"
)

// --- IsEligible ---

func TestIsEligible_CodeExtensions(t *testing.T) {
	eligible := []string{
		"main.go", "app.py", "index.js", "style.css", "schema.sql",
		"config.yaml", "data.json", "notes.md", "build.sh", "module.rs",
		"Main.java", "lib.ts", "view.tsx", "component.vue", "page.svelte",
		"infra.tf", "config.hcl",
	}
	for _, f := range eligible {
		require.True(t, deltadiff.IsEligible(f), "expected eligible: %s", f)
	}
}

func TestIsEligible_SpecialNames(t *testing.T) {
	names := []string{
		"Makefile", "makefile", "Dockerfile", "dockerfile",
		"Gemfile", "Rakefile", "Procfile", "Jenkinsfile",
		".gitignore", ".dockerignore",
	}
	for _, f := range names {
		require.True(t, deltadiff.IsEligible(f), "expected eligible: %s", f)
	}
}

func TestIsEligible_EnvExcluded(t *testing.T) {
	excluded := []string{".env", ".env.local", ".env.production", ".envrc"}
	for _, f := range excluded {
		require.False(t, deltadiff.IsEligible(f), "expected ineligible: %s", f)
	}
}

func TestIsEligible_BinaryExtensions(t *testing.T) {
	ineligible := []string{"image.png", "archive.zip", "binary.exe", "font.woff2", "data.pb"}
	for _, f := range ineligible {
		require.False(t, deltadiff.IsEligible(f), "expected ineligible: %s", f)
	}
}

func TestIsEligible_PathWithDirectory(t *testing.T) {
	require.True(t, deltadiff.IsEligible("/home/user/project/main.go"))
	require.False(t, deltadiff.IsEligible("/home/user/.env"))
	require.False(t, deltadiff.IsEligible("/home/user/.env.local"))
}

// --- ContentHash ---

func TestContentHash_Deterministic(t *testing.T) {
	h1 := deltadiff.ContentHash([]byte("hello world"))
	h2 := deltadiff.ContentHash([]byte("hello world"))
	require.Equal(t, h1, h2)
}

func TestContentHash_DifferentContent(t *testing.T) {
	h1 := deltadiff.ContentHash([]byte("hello"))
	h2 := deltadiff.ContentHash([]byte("world"))
	require.NotEqual(t, h1, h2)
}

func TestContentHash_HexFormat(t *testing.T) {
	h := deltadiff.ContentHash([]byte("test"))
	require.Len(t, h, 64, "SHA-256 hex should be 64 chars")
	for _, c := range h {
		require.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"expected lowercase hex, got %c", c)
	}
}

func TestContentHash_EmptyInput(t *testing.T) {
	h := deltadiff.ContentHash([]byte{})
	require.Len(t, h, 64)
}

// --- ComputeDelta ---

func TestComputeDelta_IdenticalContent(t *testing.T) {
	_, _, ok := deltadiff.ComputeDelta("same\n", "same\n", "file.go")
	require.False(t, ok, "identical content should return ok=false")
}

func TestComputeDelta_SimpleAddition(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nline2\nnewline\nline3\n"

	delta, stats, ok := deltadiff.ComputeDelta(old, new, "file.go")
	require.True(t, ok)
	require.Equal(t, 1, stats.Added)
	require.Equal(t, 0, stats.Removed)
	require.Contains(t, delta, "+newline")
	require.Contains(t, delta, "file.go: 1 lines changed (+1/-0)")
}

func TestComputeDelta_SimpleDeletion(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nline3\n"

	delta, stats, ok := deltadiff.ComputeDelta(old, new, "file.go")
	require.True(t, ok)
	require.Equal(t, 0, stats.Added)
	require.Equal(t, 1, stats.Removed)
	require.Contains(t, delta, "-line2")
}

func TestComputeDelta_Modification(t *testing.T) {
	old := "line1\noldline\nline3\n"
	new := "line1\nnewline\nline3\n"

	delta, stats, ok := deltadiff.ComputeDelta(old, new, "file.py")
	require.True(t, ok)
	require.Equal(t, 1, stats.Added)
	require.Equal(t, 1, stats.Removed)
	require.Contains(t, delta, "-oldline")
	require.Contains(t, delta, "+newline")
}

func TestComputeDelta_HeaderFormat(t *testing.T) {
	old := "a\nb\nc\n"
	new := "a\nX\nc\n"

	delta, _, ok := deltadiff.ComputeDelta(old, new, "test.go")
	require.True(t, ok)

	lines := strings.Split(delta, "\n")
	require.Equal(t, "test.go: 2 lines changed (+1/-1)", lines[0])
	require.Equal(t, "--- a/test.go", lines[1])
	require.Equal(t, "+++ b/test.go", lines[2])
}

func TestComputeDelta_OneLineContext(t *testing.T) {
	old := "A\nB\nC\nD\nE\n"
	new := "A\nX\nC\nD\nE\n"

	delta, _, ok := deltadiff.ComputeDelta(old, new, "f.go")
	require.True(t, ok)
	// With n=1 context, only A (before) and C (after) should appear as context.
	require.Contains(t, delta, " A\n")
	require.Contains(t, delta, "-B\n")
	require.Contains(t, delta, "+X\n")
	require.Contains(t, delta, " C\n")
	// D and E are beyond context range — should NOT appear.
	require.NotContains(t, delta, " D\n")
	require.NotContains(t, delta, " E\n")
}

func TestComputeDelta_MaxDeltaLines_OldExceeds(t *testing.T) {
	old := strings.Repeat("line\n", deltadiff.MaxDeltaLines+1)
	new := "different\n"
	_, _, ok := deltadiff.ComputeDelta(old, new, "big.go")
	require.False(t, ok, "old file exceeding MaxDeltaLines should return ok=false")
}

func TestComputeDelta_MaxDeltaLines_NewExceeds(t *testing.T) {
	old := "short\n"
	new := strings.Repeat("line\n", deltadiff.MaxDeltaLines+1)
	_, _, ok := deltadiff.ComputeDelta(old, new, "big.go")
	require.False(t, ok, "new file exceeding MaxDeltaLines should return ok=false")
}

func TestComputeDelta_MaxDeltaChars(t *testing.T) {
	// Generate a diff that would exceed MaxDeltaChars.
	var oldLines, newLines []string
	for i := range 100 {
		oldLines = append(oldLines, "old-line-with-lots-of-content-to-exceed-char-limit\n")
		newLines = append(newLines, "new-line-with-lots-of-content-to-exceed-char-limit\n")
		_ = i
	}
	old := strings.Join(oldLines, "")
	new := strings.Join(newLines, "")

	_, _, ok := deltadiff.ComputeDelta(old, new, "large.go")
	require.False(t, ok, "diff exceeding MaxDeltaChars should return ok=false")
}

func TestComputeDelta_TwoSeparateHunks(t *testing.T) {
	// Two changes far apart should produce two @@ hunks.
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "stable\n"
	}
	old := strings.Join(lines, "")

	modified := make([]string, 20)
	copy(modified, lines)
	modified[1] = "changed-early\n"
	modified[17] = "changed-late\n"
	new := strings.Join(modified, "")

	delta, stats, ok := deltadiff.ComputeDelta(old, new, "two_hunks.go")
	require.True(t, ok)
	require.Equal(t, 2, stats.Added)
	require.Equal(t, 2, stats.Removed)
	hunkCount := 0
	for _, line := range strings.Split(delta, "\n") {
		if strings.HasPrefix(line, "@@ ") {
			hunkCount++
		}
	}
	require.Equal(t, 2, hunkCount, "expected 2 hunks")
}

func TestComputeDelta_UnifiedDiffFormat(t *testing.T) {
	old := "func foo() {\n\treturn 1\n}\n"
	new := "func foo() {\n\treturn 42\n}\n"

	delta, _, ok := deltadiff.ComputeDelta(old, new, "main.go")
	require.True(t, ok)
	require.Contains(t, delta, "@@")
	require.Contains(t, delta, "---")
	require.Contains(t, delta, "+++")
	require.Contains(t, delta, "-\treturn 1")
	require.Contains(t, delta, "+\treturn 42")
}
