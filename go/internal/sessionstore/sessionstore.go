package sessionstore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agusrdz/ctxgate/internal/pathsafe"
	_ "modernc.org/sqlite"
)

const sessionDBMaxBytes = 50 * 1024 * 1024 // 50MB

const schema = `
CREATE TABLE IF NOT EXISTS file_reads (
    file_path TEXT PRIMARY KEY,
    mtime_ns INTEGER NOT NULL,
    size_bytes INTEGER NOT NULL,
    ranges_seen TEXT NOT NULL DEFAULT '[]',
    tokens_est INTEGER NOT NULL DEFAULT 0,
    read_count INTEGER NOT NULL DEFAULT 1,
    content_hash TEXT,
    last_access REAL NOT NULL,
    last_replacement_fingerprint TEXT DEFAULT '',
    last_replacement_type TEXT DEFAULT '',
    repeat_replacement_count INTEGER DEFAULT 0,
    last_structure_reason TEXT DEFAULT '',
    last_structure_confidence REAL DEFAULT 0.0
);

CREATE TABLE IF NOT EXISTS tool_outputs (
    tool_use_id TEXT PRIMARY KEY,
    tool_name TEXT NOT NULL,
    tool_type TEXT NOT NULL,
    command_or_path TEXT,
    output_hash TEXT NOT NULL,
    output_chars INTEGER NOT NULL,
    output_tokens_est INTEGER NOT NULL,
    compressed_preview TEXT,
    timestamp REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS command_outputs (
    command_hash TEXT PRIMARY KEY,
    command_text TEXT NOT NULL,
    output_hash TEXT NOT NULL,
    output_chars INTEGER NOT NULL,
    compressed_output TEXT,
    timestamp REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS cached_content (
    file_path TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    cached_at REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS session_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS context_intel_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tool_name TEXT NOT NULL,
    tool_use_id TEXT NOT NULL,
    summary TEXT NOT NULL,
    output_chars INTEGER NOT NULL,
    timestamp REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS activity_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tool_name TEXT NOT NULL,
    tool_bucket TEXT NOT NULL,
    has_error INTEGER NOT NULL DEFAULT 0,
    timestamp REAL NOT NULL
);
`

// FileEntry mirrors the file_reads table row.
type FileEntry struct {
	FilePath                   string
	MtimeNs                    int64
	SizeBytes                  int64
	RangesSeen                 string // JSON array
	TokensEst                  int
	ReadCount                  int
	ContentHash                string
	LastAccess                 float64
	LastReplacementFingerprint string
	LastReplacementType        string
	RepeatReplacementCount     int
	LastStructureReason        string
	LastStructureConfidence    float64
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

// IntelEventRow is a query result from GetIntelEvents.
type IntelEventRow struct {
	ToolName    string
	ToolUseID   string
	Summary     string
	OutputChars int
	Timestamp   float64
}

// ToolOutputRowFull is returned by GetRecentToolOutputs.
type ToolOutputRowFull struct {
	ToolUseID         string
	ToolName          string
	CommandOrPath     string
	OutputTokensEst   int
	CompressedPreview string
	Timestamp         float64
}

// SessionStore wraps a SQLite connection for a single session.
type SessionStore struct {
	db        *sql.DB
	dbPath    string
	sessionID string
}

func defaultSnapshotDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base, _ = os.UserHomeDir()
		}
		return filepath.Join(base, "ctxgate", "sessions")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "ctxgate", "sessions")
}

// Open opens (or creates) the session store for sessionID.
// Optional snapshotDir overrides the default data directory.
func Open(sessionID string, snapshotDir ...string) (*SessionStore, error) {
	sid := pathsafe.SanitizeSessionID(sessionID)

	dir := defaultSnapshotDir()
	if len(snapshotDir) > 0 && snapshotDir[0] != "" {
		dir = snapshotDir[0]
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("sessionstore: create dir: %w", err)
	}

	dbPath := filepath.Join(dir, sid+".db")

	// Touch the file first to establish 0600 permissions before SQLite opens it.
	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("sessionstore: touch db: %w", err)
	}
	f.Close()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sessionstore: open db: %w", err)
	}
	// One writer at a time within this process; WAL handles between-process concurrency.
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=50",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("sessionstore: %s: %w", pragma, err)
		}
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("sessionstore: schema: %w", err)
	}

	return &SessionStore{db: db, dbPath: dbPath, sessionID: sid}, nil
}

// Close releases the database connection.
func (s *SessionStore) Close() error {
	return s.db.Close()
}

// sizeExceeded returns true if the DB file has reached the 50MB cap.
func (s *SessionStore) sizeExceeded() bool {
	if s.dbPath == "" {
		return false
	}
	info, err := os.Stat(s.dbPath)
	if err != nil {
		return false
	}
	return info.Size() >= sessionDBMaxBytes
}

func absorbLocked(err error) error {
	if err != nil && strings.Contains(err.Error(), "database is locked") {
		return nil
	}
	return err
}

// GetFileEntry retrieves the file_reads row for filePath, or nil if not found.
func (s *SessionStore) GetFileEntry(filePath string) (*FileEntry, error) {
	row := s.db.QueryRow(`
		SELECT file_path, mtime_ns, size_bytes, ranges_seen, tokens_est, read_count,
		       content_hash, last_access, last_replacement_fingerprint, last_replacement_type,
		       repeat_replacement_count, last_structure_reason, last_structure_confidence
		FROM file_reads WHERE file_path = ?`, filePath)

	var e FileEntry
	var contentHash sql.NullString
	err := row.Scan(
		&e.FilePath, &e.MtimeNs, &e.SizeBytes, &e.RangesSeen, &e.TokensEst, &e.ReadCount,
		&contentHash, &e.LastAccess, &e.LastReplacementFingerprint, &e.LastReplacementType,
		&e.RepeatReplacementCount, &e.LastStructureReason, &e.LastStructureConfidence,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if contentHash.Valid {
		e.ContentHash = contentHash.String
	}
	e.FilePath = filePath
	return &e, nil
}

// UpsertFileEntry inserts or replaces the file_reads row for filePath.
func (s *SessionStore) UpsertFileEntry(filePath string, e *FileEntry) error {
	if s.sizeExceeded() {
		return nil
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO file_reads (
			file_path, mtime_ns, size_bytes, ranges_seen, tokens_est, read_count,
			content_hash, last_access, last_replacement_fingerprint, last_replacement_type,
			repeat_replacement_count, last_structure_reason, last_structure_confidence
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		filePath, e.MtimeNs, e.SizeBytes, e.RangesSeen, e.TokensEst, e.ReadCount,
		sqlNullString(e.ContentHash), e.LastAccess, e.LastReplacementFingerprint, e.LastReplacementType,
		e.RepeatReplacementCount, e.LastStructureReason, e.LastStructureConfidence,
	)
	return absorbLocked(err)
}

// DeleteFileEntry removes the file_reads row for filePath.
func (s *SessionStore) DeleteFileEntry(filePath string) error {
	_, err := s.db.Exec(`DELETE FROM file_reads WHERE file_path = ?`, filePath)
	return absorbLocked(err)
}

// ClearFileEntries deletes all rows from file_reads.
func (s *SessionStore) ClearFileEntries() error {
	_, err := s.db.Exec(`DELETE FROM file_reads`)
	return absorbLocked(err)
}

// GetAllFileEntries returns all rows from file_reads keyed by file_path.
func (s *SessionStore) GetAllFileEntries() (map[string]*FileEntry, error) {
	rows, err := s.db.Query(`
		SELECT file_path, mtime_ns, size_bytes, ranges_seen, tokens_est, read_count,
		       content_hash, last_access, last_replacement_fingerprint, last_replacement_type,
		       repeat_replacement_count, last_structure_reason, last_structure_confidence
		FROM file_reads`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*FileEntry)
	for rows.Next() {
		var e FileEntry
		var contentHash sql.NullString
		if err := rows.Scan(
			&e.FilePath, &e.MtimeNs, &e.SizeBytes, &e.RangesSeen, &e.TokensEst, &e.ReadCount,
			&contentHash, &e.LastAccess, &e.LastReplacementFingerprint, &e.LastReplacementType,
			&e.RepeatReplacementCount, &e.LastStructureReason, &e.LastStructureConfidence,
		); err != nil {
			return nil, err
		}
		if contentHash.Valid {
			e.ContentHash = contentHash.String
		}
		result[e.FilePath] = &e
	}
	return result, rows.Err()
}

// GetCachedContent retrieves the cached_content row for filePath, or nil if not found.
func (s *SessionStore) GetCachedContent(filePath string) (*CachedContent, error) {
	row := s.db.QueryRow(`
		SELECT file_path, content, content_hash, cached_at
		FROM cached_content WHERE file_path = ?`, filePath)

	var c CachedContent
	err := row.Scan(&c.FilePath, &c.Content, &c.ContentHash, &c.CachedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpsertCachedContent inserts or replaces a cached_content row.
func (s *SessionStore) UpsertCachedContent(filePath, content, hash string) error {
	if s.sizeExceeded() {
		return nil
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO cached_content (file_path, content, content_hash, cached_at)
		VALUES (?, ?, ?, ?)`,
		filePath, content, hash, unixNow(),
	)
	return absorbLocked(err)
}

// DeleteCachedContent removes the cached_content row for filePath.
func (s *SessionStore) DeleteCachedContent(filePath string) error {
	_, err := s.db.Exec(`DELETE FROM cached_content WHERE file_path = ?`, filePath)
	return absorbLocked(err)
}

// GetRecentFileReads returns the most recently accessed files with at least minReadCount reads,
// ordered by last_access descending.
func (s *SessionStore) GetRecentFileReads(limit, minReadCount int) ([]FileReadRow, error) {
	rows, err := s.db.Query(`
		SELECT file_path, read_count, tokens_est
		FROM file_reads
		WHERE read_count >= ?
		ORDER BY last_access DESC
		LIMIT ?`, minReadCount, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FileReadRow
	for rows.Next() {
		var r FileReadRow
		if err := rows.Scan(&r.FilePath, &r.ReadCount, &r.TokensEst); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetHighValueOutputs returns tool outputs with at least minTokens estimated tokens,
// ordered by output_tokens_est descending.
func (s *SessionStore) GetHighValueOutputs(minTokens, limit int) ([]ToolOutputRow, error) {
	rows, err := s.db.Query(`
		SELECT tool_use_id, tool_name, output_tokens_est, compressed_preview
		FROM tool_outputs
		WHERE output_tokens_est >= ?
		ORDER BY output_tokens_est DESC
		LIMIT ?`, minTokens, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ToolOutputRow
	for rows.Next() {
		var r ToolOutputRow
		var preview sql.NullString
		if err := rows.Scan(&r.ToolUseID, &r.ToolName, &r.OutputTokensEst, &preview); err != nil {
			return nil, err
		}
		if preview.Valid {
			r.CompressedPreview = preview.String
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// InsertToolOutput inserts a record into tool_outputs (INSERT OR IGNORE).
func (s *SessionStore) InsertToolOutput(toolUseID, toolName, toolType, commandOrPath, outputHash string, outputChars, outputTokensEst int, compressedPreview string) error {
	if s.sizeExceeded() {
		return nil
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO tool_outputs
		(tool_use_id, tool_name, tool_type, command_or_path,
		 output_hash, output_chars, output_tokens_est, compressed_preview, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		toolUseID, toolName, toolType, commandOrPath,
		outputHash, outputChars, outputTokensEst, compressedPreview, unixNow(),
	)
	return absorbLocked(err)
}

// GetMeta retrieves a value from session_meta by key, or "" if not found.
func (s *SessionStore) GetMeta(key string) (string, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM session_meta WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, absorbLocked(err)
}

// SetMeta inserts or replaces a key-value pair in session_meta.
func (s *SessionStore) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO session_meta (key, value) VALUES (?, ?)`, key, value)
	return absorbLocked(err)
}

// InsertContextIntelEvent appends a summary event to context_intel_events.
func (s *SessionStore) InsertContextIntelEvent(toolName, toolUseID, summary string, outputChars int) error {
	if s.sizeExceeded() {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO context_intel_events
		(tool_name, tool_use_id, summary, output_chars, timestamp)
		VALUES (?, ?, ?, ?, ?)`,
		toolName, toolUseID, summary, outputChars, unixNow(),
	)
	return absorbLocked(err)
}

// CountContextIntelEventsSince counts events logged after the given Unix timestamp.
func (s *SessionStore) CountContextIntelEventsSince(since float64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM context_intel_events WHERE timestamp > ?`, since).Scan(&n)
	if err != nil {
		return 0, absorbLocked(err)
	}
	return n, nil
}

// InsertActivityLog appends a row to activity_log.
func (s *SessionStore) InsertActivityLog(toolName, toolBucket string, hasError bool) error {
	if s.sizeExceeded() {
		return nil
	}
	errInt := 0
	if hasError {
		errInt = 1
	}
	_, err := s.db.Exec(`INSERT INTO activity_log (tool_name, tool_bucket, has_error, timestamp)
		VALUES (?, ?, ?, ?)`, toolName, toolBucket, errInt, unixNow())
	return absorbLocked(err)
}

// DetectCurrentMode queries the activity_log, prunes if needed, and returns
// the current session mode string ("code", "debug", "review", "infra", "general").
func (s *SessionStore) DetectCurrentMode(windowSize, pruneThreshold, pruneKeep int) string {
	rows, err := s.db.Query(
		`SELECT tool_bucket, has_error FROM activity_log ORDER BY id DESC LIMIT ?`, windowSize,
	)
	if err != nil {
		return "general"
	}
	type row struct {
		bucket   string
		hasError bool
	}
	var recent []row
	for rows.Next() {
		var r row
		var errInt int
		if rows.Scan(&r.bucket, &errInt) == nil {
			r.hasError = errInt != 0
			recent = append(recent, r)
		}
	}
	rows.Close()

	// Prune if needed.
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM activity_log`).Scan(&total)
	if total > pruneThreshold {
		_, _ = s.db.Exec(
			`DELETE FROM activity_log WHERE id NOT IN `+
				`(SELECT id FROM activity_log ORDER BY id DESC LIMIT ?)`, pruneKeep,
		)
	}

	if len(recent) < 3 {
		return "general"
	}

	editCount, readCount, infraCount, webCount, bashOther := 0, 0, 0, 0, 0
	hasRecentErrors := false
	for _, r := range recent {
		if r.hasError {
			hasRecentErrors = true
		}
		switch r.bucket {
		case "edit":
			editCount++
		case "read":
			readCount++
		case "bash_infra", "bash_git", "bash_install":
			infraCount++
		case "web":
			webCount++
		case "bash_other":
			bashOther++
		}
	}

	switch {
	case infraCount >= 3:
		return "infra"
	case hasRecentErrors && readCount >= 3 && editCount <= 1:
		return "debug"
	case editCount >= 4:
		return "code"
	case readCount >= 4 && editCount == 0:
		return "review"
	case webCount >= 3:
		return "review"
	case editCount >= 2 && (bashOther >= 2 || readCount >= 2):
		return "code"
	default:
		return "general"
	}
}

// unixNow returns the current time as a Unix timestamp (float64 seconds).
func unixNow() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

// CleanupOldStores deletes session DB files older than maxAgeHours in snapshotDir.
// Returns the number of files deleted.
func CleanupOldStores(snapshotDir string, maxAgeHours int) (int, error) {
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := time.Now().Add(-time.Duration(maxAgeHours) * time.Hour)
	deleted := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			base := filepath.Join(snapshotDir, e.Name())
			if os.Remove(base) == nil {
				deleted++
			}
			os.Remove(base + "-wal")
			os.Remove(base + "-shm")
		}
	}
	return deleted, nil
}

// GetIntelEvents returns recent context intel events, most recent first.
func (s *SessionStore) GetIntelEvents(limit int) ([]IntelEventRow, error) {
	rows, err := s.db.Query(
		`SELECT tool_name, tool_use_id, summary, output_chars, timestamp
		 FROM context_intel_events ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []IntelEventRow
	for rows.Next() {
		var r IntelEventRow
		if rows.Scan(&r.ToolName, &r.ToolUseID, &r.Summary, &r.OutputChars, &r.Timestamp) == nil {
			result = append(result, r)
		}
	}
	return result, nil
}

// GetOneTimeReads returns file_reads rows read exactly once, ordered by last_access desc.
func (s *SessionStore) GetOneTimeReads(limit int) ([]FileReadRow, error) {
	rows, err := s.db.Query(
		`SELECT file_path, read_count, tokens_est FROM file_reads
		 WHERE read_count = 1 ORDER BY last_access DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []FileReadRow
	for rows.Next() {
		var r FileReadRow
		if rows.Scan(&r.FilePath, &r.ReadCount, &r.TokensEst) == nil {
			result = append(result, r)
		}
	}
	return result, nil
}

// GetRecentToolOutputs returns recent tool_outputs rows, most recent first.
func (s *SessionStore) GetRecentToolOutputs(limit int) ([]ToolOutputRowFull, error) {
	rows, err := s.db.Query(
		`SELECT tool_use_id, tool_name, COALESCE(command_or_path,''), output_tokens_est,
		 COALESCE(compressed_preview,''), timestamp
		 FROM tool_outputs ORDER BY rowid DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ToolOutputRowFull
	for rows.Next() {
		var r ToolOutputRowFull
		if rows.Scan(&r.ToolUseID, &r.ToolName, &r.CommandOrPath, &r.OutputTokensEst,
			&r.CompressedPreview, &r.Timestamp) == nil {
			result = append(result, r)
		}
	}
	return result, nil
}

func sqlNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

