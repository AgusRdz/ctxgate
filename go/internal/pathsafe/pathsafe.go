package pathsafe

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var sessionIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// IsSafeSubdir returns true if candidate is an existing, non-symlink directory
// that resolves to a path under base (after symlink resolution on both sides).
func IsSafeSubdir(candidate, base string) bool {
	info, err := os.Lstat(candidate)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	baseResolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseResolved, resolved)
	if err != nil {
		return false
	}
	// "." means candidate == base (not a strict subdir); ".." prefix means outside.
	return rel != "." && !strings.HasPrefix(rel, "..")
}

// ResolveStrictUnderHome resolves rawPath and verifies it resides under the
// user's home directory. Rejects symlinks that escape home and non-existent paths.
func ResolveStrictUnderHome(rawPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(rawPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path %q: %w", rawPath, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	homeResolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		homeResolved = home
	}
	rel, err := filepath.Rel(homeResolved, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q escapes home directory", rawPath)
	}
	return resolved, nil
}

// SanitizeSessionID returns id if it matches ^[a-zA-Z0-9_-]+$, otherwise "unknown".
func SanitizeSessionID(id string) string {
	if id != "" && sessionIDRe.MatchString(id) {
		return id
	}
	return "unknown"
}
