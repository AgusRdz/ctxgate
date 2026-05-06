package pathsafe

// IsSafeSubdir returns true if candidate is a non-symlink subdirectory of base.
func IsSafeSubdir(candidate, base string) bool {
	panic("not implemented")
}

// ResolveStrictUnderHome resolves rawPath and verifies it resides under the user's home dir.
// Rejects symlinks that escape home.
func ResolveStrictUnderHome(rawPath string) (string, error) {
	panic("not implemented")
}

// SanitizeSessionID returns id if it matches ^[a-zA-Z0-9_-]+$, otherwise "unknown".
func SanitizeSessionID(id string) string {
	panic("not implemented")
}
