package trends

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
type Writer struct{}

// NewWriter opens (or creates) the trends database at the given path.
func NewWriter(dbPath string) (*Writer, error) {
	panic("not implemented")
}

// Close closes the trends database.
func (w *Writer) Close() error {
	panic("not implemented")
}

// RecordSavings appends a savings event.
func (w *Writer) RecordSavings(e SavingsEvent) error {
	panic("not implemented")
}

// RecordSnapshot appends a session snapshot.
func (w *Writer) RecordSnapshot(s SessionSnapshot) error {
	panic("not implemented")
}
