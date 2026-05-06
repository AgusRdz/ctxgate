package pathsafe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- SanitizeSessionID ---

func TestSanitizeSessionID_ValidAlphanumeric(t *testing.T) {
	require.Equal(t, "abc123", SanitizeSessionID("abc123"))
}

func TestSanitizeSessionID_ValidWithDashUnderscore(t *testing.T) {
	require.Equal(t, "sess-abc_XYZ-001", SanitizeSessionID("sess-abc_XYZ-001"))
}

func TestSanitizeSessionID_Empty(t *testing.T) {
	require.Equal(t, "unknown", SanitizeSessionID(""))
}

func TestSanitizeSessionID_PathTraversal(t *testing.T) {
	require.Equal(t, "unknown", SanitizeSessionID("../etc/passwd"))
}

func TestSanitizeSessionID_Slash(t *testing.T) {
	require.Equal(t, "unknown", SanitizeSessionID("abc/def"))
}

func TestSanitizeSessionID_Space(t *testing.T) {
	require.Equal(t, "unknown", SanitizeSessionID("abc def"))
}

func TestSanitizeSessionID_NullByte(t *testing.T) {
	require.Equal(t, "unknown", SanitizeSessionID("abc\x00def"))
}

// --- IsSafeSubdir ---

func TestIsSafeSubdir_ValidSubdir(t *testing.T) {
	base := t.TempDir()
	child := filepath.Join(base, "child")
	require.NoError(t, os.Mkdir(child, 0o700))
	require.True(t, IsSafeSubdir(child, base))
}

func TestIsSafeSubdir_BaseItself(t *testing.T) {
	base := t.TempDir()
	// base == candidate: not a strict subdir
	require.False(t, IsSafeSubdir(base, base))
}

func TestIsSafeSubdir_NotExist(t *testing.T) {
	base := t.TempDir()
	require.False(t, IsSafeSubdir(filepath.Join(base, "nope"), base))
}

func TestIsSafeSubdir_File(t *testing.T) {
	base := t.TempDir()
	f := filepath.Join(base, "file.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	require.False(t, IsSafeSubdir(f, base))
}

func TestIsSafeSubdir_Symlink(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	link := filepath.Join(base, "link")
	require.NoError(t, os.Mkdir(real, 0o700))
	require.NoError(t, os.Symlink(real, link))
	require.False(t, IsSafeSubdir(link, base))
}

func TestIsSafeSubdir_OutsideBase(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir() // separate temp dir
	require.False(t, IsSafeSubdir(outside, base))
}

// --- ResolveStrictUnderHome ---

func TestResolveStrictUnderHome_ValidPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// Create a real subdir under home's temp equivalent — use a subdir of TempDir
	// that is under the user's home if possible, otherwise skip.
	// For CI portability: use a path we know exists under home.
	claudeDir := filepath.Join(home, ".claude")
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		t.Skip("~/.claude does not exist on this machine")
	}
	resolved, err := ResolveStrictUnderHome(claudeDir)
	require.NoError(t, err)
	require.NotEmpty(t, resolved)
}

func TestResolveStrictUnderHome_NonExistentPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	_, err = ResolveStrictUnderHome(filepath.Join(home, "does-not-exist-xyz"))
	require.Error(t, err)
}

func TestResolveStrictUnderHome_OutsideHome(t *testing.T) {
	outside := t.TempDir()
	home, _ := os.UserHomeDir()
	// Only run if temp dir is actually outside home (normally true).
	rel, _ := filepath.Rel(home, outside)
	if len(rel) < 2 || rel[:2] != ".." {
		t.Skip("TempDir is under home on this machine")
	}
	_, err := ResolveStrictUnderHome(outside)
	require.Error(t, err)
}
