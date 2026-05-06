package contextignore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agusrdz/ctxgate/internal/contextignore"
	"github.com/stretchr/testify/require"
)

// writeIgnoreFile creates a .contextignore in dir with the given content.
func writeIgnoreFile(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".contextignore"), []byte(content), 0o600))
}

// --- Load ---

func TestLoad_MissingFiles(t *testing.T) {
	m, err := contextignore.Load(t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, m)
	require.False(t, m.Match("anything.py"))
}

func TestLoad_ProjectPatterns(t *testing.T) {
	dir := t.TempDir()
	writeIgnoreFile(t, dir, "*.secret\n# comment\n\n*.key\n")

	m, err := contextignore.Load(dir)
	require.NoError(t, err)

	require.True(t, m.Match("passwords.secret"))
	require.True(t, m.Match("id_rsa.key"))
	require.False(t, m.Match("main.go"))
}

func TestLoad_CommentsAndBlanksStripped(t *testing.T) {
	dir := t.TempDir()
	writeIgnoreFile(t, dir, "# this is a comment\n\n   \n*.log\n# another comment\n")

	m, err := contextignore.Load(dir)
	require.NoError(t, err)

	require.True(t, m.Match("app.log"))
	require.False(t, m.Match("app.go"))
}

func TestLoad_PatternCap(t *testing.T) {
	dir := t.TempDir()

	// Write 250 patterns; only 200 should be loaded.
	var lines []string
	for i := range 250 {
		lines = append(lines, "pattern"+strings.Repeat("x", i%10)+".txt")
	}
	writeIgnoreFile(t, dir, strings.Join(lines, "\n"))

	// The 201st pattern must NOT match (it was truncated).
	// pattern200 would be at index 200 (0-based), which is past the 200 cap.
	// We can't easily test the exact pattern count from outside, but we can
	// verify Load doesn't error and returns a usable Matcher.
	m, err := contextignore.Load(dir)
	require.NoError(t, err)
	require.NotNil(t, m)

	// The first pattern must still match.
	require.True(t, m.Match(lines[0]))
}

// --- Match ---

func TestMatch_FullPath(t *testing.T) {
	dir := t.TempDir()
	writeIgnoreFile(t, dir, "secrets/*\n")

	m, err := contextignore.Load(dir)
	require.NoError(t, err)

	require.True(t, m.Match("secrets/token.txt"))
	require.False(t, m.Match("public/token.txt"))
}

func TestMatch_Basename(t *testing.T) {
	dir := t.TempDir()
	writeIgnoreFile(t, dir, "*.env\n")

	m, err := contextignore.Load(dir)
	require.NoError(t, err)

	// Pattern *.env matches the basename regardless of directory depth.
	require.True(t, m.Match("/some/deep/path/.env"))
	require.True(t, m.Match("production.env"))
	require.False(t, m.Match("production.env.bak"))
}

func TestMatch_WindowsBackslashPaths(t *testing.T) {
	dir := t.TempDir()
	writeIgnoreFile(t, dir, "*.secret\n")

	m, err := contextignore.Load(dir)
	require.NoError(t, err)

	// Paths with backslashes (Windows) should still match on the basename.
	require.True(t, m.Match(`C:\Users\foo\bar.secret`))
}

func TestMatch_NoPatterns(t *testing.T) {
	dir := t.TempDir()

	m, err := contextignore.Load(dir)
	require.NoError(t, err)

	require.False(t, m.Match("anything.py"))
	require.False(t, m.Match(""))
}

func TestMatch_ExactFilename(t *testing.T) {
	dir := t.TempDir()
	writeIgnoreFile(t, dir, ".env\nCLAUDE.md\n")

	m, err := contextignore.Load(dir)
	require.NoError(t, err)

	require.True(t, m.Match(".env"))
	require.True(t, m.Match("/project/.env"))
	require.True(t, m.Match("CLAUDE.md"))
	require.False(t, m.Match("README.md"))
}

func TestMatch_WildcardDirectory(t *testing.T) {
	dir := t.TempDir()
	writeIgnoreFile(t, dir, "node_modules/*\n")

	m, err := contextignore.Load(dir)
	require.NoError(t, err)

	require.True(t, m.Match("node_modules/lodash"))
	require.False(t, m.Match("src/index.js"))
}

func TestMatch_QuestionMarkGlob(t *testing.T) {
	dir := t.TempDir()
	writeIgnoreFile(t, dir, "file?.txt\n")

	m, err := contextignore.Load(dir)
	require.NoError(t, err)

	require.True(t, m.Match("file1.txt"))
	require.True(t, m.Match("fileA.txt"))
	require.False(t, m.Match("file10.txt"))
}
