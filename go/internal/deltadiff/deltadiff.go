package deltadiff

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

// IsEligible returns true if filePath's extension supports delta diffing.
func IsEligible(filePath string) bool {
	panic("not implemented")
}

// ContentHash returns a short SHA256 hex digest of content.
func ContentHash(content []byte) string {
	panic("not implemented")
}

// ComputeDelta computes a unified diff between oldContent and newContent.
// Returns ok=false if the delta exceeds MaxDeltaChars or MaxDeltaLines.
func ComputeDelta(oldContent, newContent, filename string) (deltaText string, stats DeltaStats, ok bool) {
	panic("not implemented")
}
