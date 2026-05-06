package detectors

// SessionData is the input to all detectors — sourced from the session store.
type SessionData struct {
	SessionID  string
	ToolCalls  []ToolCall
	Messages   []Message
	FileReads  []FileRead
}

// ToolCall represents a single tool invocation in the session.
type ToolCall struct {
	ToolName  string
	Input     string
	Output    string
	HasError  bool
	Timestamp float64
}

// Message represents a user or assistant message in the session.
type Message struct {
	Role      string
	Content   string
	Timestamp float64
}

// FileRead represents a file read event in the session.
type FileRead struct {
	FilePath  string
	Tokens    int
	Timestamp float64
}

// Finding is a single detector result.
type Finding struct {
	Name          string
	Confidence    float64
	SavingsTokens int
	Evidence      string
	Suggestion    string
}

// DetectorFunc is the signature for all waste detectors.
type DetectorFunc func(data SessionData) []Finding

// AllDetectors is the registry of all 10 waste detectors.
var AllDetectors = []struct {
	Name string
	Fn   DetectorFunc
}{
	{"retry_churn", detectRetryChurn},
	{"tool_cascade", detectToolCascade},
	{"looping", detectLooping},
	{"overpowered", detectOverpowered},
	{"weak_model", detectWeakModel},
	{"bad_decomposition", detectBadDecomposition},
	{"wasteful_thinking", detectWastefulThinking},
	{"output_waste", detectOutputWaste},
	{"cache_instability", detectCacheInstability},
	{"pdf_ingestion", detectPDFIngestion},
}

// RunAll executes all detectors and returns combined findings.
func RunAll(data SessionData) []Finding {
	panic("not implemented")
}

// Triage filters findings to those with SavingsTokens > 5000.
func Triage(findings []Finding) []Finding {
	panic("not implemented")
}

func detectRetryChurn(data SessionData) []Finding       { panic("not implemented") }
func detectToolCascade(data SessionData) []Finding      { panic("not implemented") }
func detectLooping(data SessionData) []Finding          { panic("not implemented") }
func detectOverpowered(data SessionData) []Finding      { panic("not implemented") }
func detectWeakModel(data SessionData) []Finding        { panic("not implemented") }
func detectBadDecomposition(data SessionData) []Finding { panic("not implemented") }
func detectWastefulThinking(data SessionData) []Finding { panic("not implemented") }
func detectOutputWaste(data SessionData) []Finding      { panic("not implemented") }
func detectCacheInstability(data SessionData) []Finding { panic("not implemented") }
func detectPDFIngestion(data SessionData) []Finding     { panic("not implemented") }
