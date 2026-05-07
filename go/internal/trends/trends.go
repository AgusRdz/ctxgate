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

// Reader provides read-only access to the trends database.
type Reader struct {
	db *sql.DB
}

// OpenReader opens the trends database for reading.
// Returns nil, nil if the file does not exist yet (no data).
func OpenReader(dbPath string) (*Reader, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=1000")
	return &Reader{db: db}, nil
}

// Close closes the reader.
func (r *Reader) Close() error { return r.db.Close() }

// TypeStat holds per-event-type aggregates for savings events.
type TypeStat struct {
	Events      int
	TokensSaved int
	CostUSD     float64
}

// SavingsSummary holds aggregate savings data over a date range.
type SavingsSummary struct {
	TotalEvents  int
	TotalTokens  int
	TotalCostUSD float64
	ByType       map[string]TypeStat
}

// TrendDay holds per-day aggregates from session_log.
type TrendDay struct {
	Date        string
	Sessions    int
	ToolCalls   int
	TotalTokens int
	SavedTokens int
}

// GetSavingsSummary returns aggregate savings data for the last N days.
func (r *Reader) GetSavingsSummary(days int) (SavingsSummary, error) {
	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)
	summary := SavingsSummary{ByType: make(map[string]TypeStat)}
	rows, err := r.db.Query(
		`SELECT event_type,
		        COUNT(*),
		        COALESCE(SUM(tokens_saved), 0),
		        COALESCE(SUM(cost_saved_usd), 0)
		 FROM savings_events WHERE timestamp >= ?
		 GROUP BY event_type ORDER BY SUM(tokens_saved) DESC`, cutoff)
	if err != nil {
		return summary, err
	}
	defer rows.Close()
	for rows.Next() {
		var typ string
		var count, tokens int
		var cost float64
		if err := rows.Scan(&typ, &count, &tokens, &cost); err != nil {
			continue
		}
		summary.TotalEvents += count
		summary.TotalTokens += tokens
		summary.TotalCostUSD += cost
		summary.ByType[typ] = TypeStat{Events: count, TokensSaved: tokens, CostUSD: cost}
	}
	return summary, nil
}

// GetTotals returns high-level session_log totals for the last N days.
func (r *Reader) GetTotals(days int) (sessions, toolCalls, totalTokens, savedTokens int, err error) {
	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)
	row := r.db.QueryRow(
		`SELECT COUNT(*),
		        COALESCE(SUM(tool_call_count), 0),
		        COALESCE(SUM(total_tokens), 0),
		        COALESCE(SUM(saved_tokens), 0)
		 FROM session_log WHERE timestamp >= ?`, cutoff)
	err = row.Scan(&sessions, &toolCalls, &totalTokens, &savedTokens)
	return
}

// GetSessionTrends returns per-day session_log aggregates for the last N days.
func (r *Reader) GetSessionTrends(days int) ([]TrendDay, error) {
	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)
	rows, err := r.db.Query(
		`SELECT substr(timestamp, 1, 10) AS day,
		        COUNT(*),
		        COALESCE(SUM(tool_call_count), 0),
		        COALESCE(SUM(total_tokens), 0),
		        COALESCE(SUM(saved_tokens), 0)
		 FROM session_log WHERE timestamp >= ?
		 GROUP BY day ORDER BY day DESC LIMIT 30`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrendDay
	for rows.Next() {
		var d TrendDay
		if err := rows.Scan(&d.Date, &d.Sessions, &d.ToolCalls, &d.TotalTokens, &d.SavedTokens); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// GetEventCount returns total number of savings events in the database.
func (r *Reader) GetEventCount() int {
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM savings_events`).Scan(&n)
	return n
}
