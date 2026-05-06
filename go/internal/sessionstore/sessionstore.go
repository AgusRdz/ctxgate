package sessionstore

import "database/sql"

// FileEntry mirrors the file_reads table row.
type FileEntry struct {
	FilePath                    string
	MtimeNs                     int64
	SizeBytes                   int64
	RangesSeen                  string // JSON array
	TokensEst                   int
	ReadCount                   int
	ContentHash                 string
	LastAccess                  float64
	LastReplacementFingerprint  string
	LastReplacementType         string
	RepeatReplacementCount      int
	LastStructureReason         string
	LastStructureConfidence     float64
}

// CachedContent mirrors the cached_content table row.
type CachedContent struct {
	FilePath    string
	Content     string
	ContentHash string
	CachedAt    float64
}

// FileReadRow is a query result row from GetRecentFileReads.
type FileReadRow struct {
	FilePath  string
	ReadCount int
	TokensEst int
}

// ToolOutputRow is a query result row from GetHighValueOutputs.
type ToolOutputRow struct {
	ToolUseID         string
	ToolName          string
	OutputTokensEst   int
	CompressedPreview string
}

// SessionStore wraps a SQLite connection for a single session.
type SessionStore struct {
	db        *sql.DB
	sessionID string
}

// Open opens (or creates) the session store for sessionID.
// Optional snapshotDir overrides the default data directory.
func Open(sessionID string, snapshotDir ...string) (*SessionStore, error) {
	panic("not implemented")
}

// Close releases the database connection.
func (s *SessionStore) Close() error {
	panic("not implemented")
}

// GetFileEntry retrieves the file_reads row for filePath, or nil if not found.
func (s *SessionStore) GetFileEntry(filePath string) (*FileEntry, error) {
	panic("not implemented")
}

// UpsertFileEntry inserts or replaces the file_reads row for filePath.
func (s *SessionStore) UpsertFileEntry(filePath string, e *FileEntry) error {
	panic("not implemented")
}

// DeleteFileEntry removes the file_reads row for filePath.
func (s *SessionStore) DeleteFileEntry(filePath string) error {
	panic("not implemented")
}

// ClearFileEntries deletes all rows from file_reads.
func (s *SessionStore) ClearFileEntries() error {
	panic("not implemented")
}

// GetAllFileEntries returns all rows from file_reads keyed by file_path.
func (s *SessionStore) GetAllFileEntries() (map[string]*FileEntry, error) {
	panic("not implemented")
}

// GetCachedContent retrieves the cached_content row for filePath, or nil if not found.
func (s *SessionStore) GetCachedContent(filePath string) (*CachedContent, error) {
	panic("not implemented")
}

// UpsertCachedContent inserts or replaces a cached_content row.
func (s *SessionStore) UpsertCachedContent(filePath, content, hash string) error {
	panic("not implemented")
}

// DeleteCachedContent removes the cached_content row for filePath.
func (s *SessionStore) DeleteCachedContent(filePath string) error {
	panic("not implemented")
}

// GetRecentFileReads returns the most recently accessed files with at least minReadCount reads.
func (s *SessionStore) GetRecentFileReads(limit, minReadCount int) ([]FileReadRow, error) {
	panic("not implemented")
}

// GetHighValueOutputs returns tool outputs with at least minTokens estimated tokens.
func (s *SessionStore) GetHighValueOutputs(minTokens, limit int) ([]ToolOutputRow, error) {
	panic("not implemented")
}

// CleanupOldStores deletes session DB files older than maxAgeHours in snapshotDir.
// Returns the number of files deleted.
func CleanupOldStores(snapshotDir string, maxAgeHours int) (int, error) {
	panic("not implemented")
}
