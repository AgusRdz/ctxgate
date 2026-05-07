package trends

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS savings_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL,
    event_type TEXT NOT NULL,
    tokens_saved INTEGER DEFAULT 0,
    cost_saved_usd REAL DEFAULT 0.0,
    session_id TEXT,
    detail TEXT
);

CREATE TABLE IF NOT EXISTS session_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL,
    session_id TEXT,
    total_tokens INTEGER DEFAULT 0,
    saved_tokens INTEGER DEFAULT 0,
    tool_call_count INTEGER DEFAULT 0
);
`

// SavingsEvent records a single token savings observation.
type SavingsEvent struct {
	SessionID   string
	ToolName    string
	RawTokens   int
	SavedTokens int
	Timestamp   float64
}

// SessionSnapshot records aggregate session statistics.
type SessionSnapshot struct {
	SessionID     string
	TotalTokens   int
	SavedTokens   int
	ToolCallCount int
	Timestamp     float64
}

// Writer appends savings events and session snapshots to the trends database.
type Writer struct {
	db *sql.DB
}

// NewWriter opens (or creates) the trends database at the given path.
func NewWriter(dbPath string) (*Writer, error) {
	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("trends: touch db: %w", err)
	}
	f.Close()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("trends: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")
	db.Exec("PRAGMA synchronous=NORMAL")

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("trends: schema: %w", err)
	}
	return &Writer{db: db}, nil
}

// Close closes the trends database.
func (w *Writer) Close() error {
	return w.db.Close()
}

// RecordSavings appends a savings event.
func (w *Writer) RecordSavings(e SavingsEvent) error {
	ts := time.Unix(int64(e.Timestamp), 0).UTC().Format(time.RFC3339)
	if e.Timestamp == 0 {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := w.db.Exec(
		`INSERT INTO savings_events (timestamp, event_type, tokens_saved, cost_saved_usd, session_id, detail)
		 VALUES (?, ?, ?, 0.0, ?, ?)`,
		ts, e.ToolName, e.SavedTokens, e.SessionID,
		fmt.Sprintf("raw=%d saved=%d", e.RawTokens, e.SavedTokens),
	)
	return err
}

// RecordSnapshot appends a session snapshot.
func (w *Writer) RecordSnapshot(s SessionSnapshot) error {
	ts := time.Unix(int64(s.Timestamp), 0).UTC().Format(time.RFC3339)
	if s.Timestamp == 0 {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := w.db.Exec(
		`INSERT INTO session_log (timestamp, session_id, total_tokens, saved_tokens, tool_call_count)
		 VALUES (?, ?, ?, ?, ?)`,
		ts, s.SessionID, s.TotalTokens, s.SavedTokens, s.ToolCallCount,
	)
	return err
}
