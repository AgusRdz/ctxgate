package detectors

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ActivityEntry is one row from the activity_log table.
type ActivityEntry struct {
	ToolName   string
	ToolBucket string
	HasError   bool
	Timestamp  float64
}

// ToolOutputItem is one row from the tool_outputs table.
type ToolOutputItem struct {
	ToolName        string
	OutputTokensEst int
	CommandOrPath   string
}

// FileRead represents a file read event in the session.
type FileRead struct {
	FilePath  string
	Tokens    int
	Timestamp float64
}

// ToolCall represents a single tool invocation in the session (legacy field, keep for compat).
type ToolCall struct {
	ToolName  string
	Input     string
	Output    string
	HasError  bool
	Timestamp float64
}

// Message represents a user or assistant message (legacy, keep for compat).
type Message struct {
	Role      string
	Content   string
	Timestamp float64
}

// SessionData is the input to all detectors — sourced from the session store.
type SessionData struct {
	SessionID       string
	ActivityLog     []ActivityEntry  // from activity_log table
	ToolOutputs     []ToolOutputItem // from tool_outputs table
	FileReads       []FileRead       // from file_reads table
	ClaudeMDContent string           // for cache_instability: content of CLAUDE.md
	// Legacy fields retained for API compat
	ToolCalls []ToolCall
	Messages  []Message
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

// RunAll executes all detectors and returns combined findings sorted by confidence desc.
// Detectors that panic are silently skipped.
func RunAll(data SessionData) []Finding {
	var findings []Finding
	for _, d := range AllDetectors {
		func() {
			defer func() { recover() }()
			results := d.Fn(data)
			for _, f := range results {
				if f.Confidence > 0.3 {
					findings = append(findings, f)
				}
			}
		}()
	}
	// Sort by confidence desc.
	for i := 0; i < len(findings); i++ {
		for j := i + 1; j < len(findings); j++ {
			if findings[j].Confidence > findings[i].Confidence {
				findings[i], findings[j] = findings[j], findings[i]
			}
		}
	}
	return findings
}

// Triage filters findings to those with SavingsTokens > 5000.
func Triage(findings []Finding) []Finding {
	var out []Finding
	for _, f := range findings {
		if f.SavingsTokens > 5000 {
			out = append(out, f)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Detector 1: retry_churn — same tool repeated 3+ times after errors
// ---------------------------------------------------------------------------

func detectRetryChurn(data SessionData) []Finding {
	if len(data.ActivityLog) == 0 {
		return nil
	}

	// Count consecutive error-retry sequences per tool.
	toolAttempts := map[string]int{}
	for i := 1; i < len(data.ActivityLog); i++ {
		prev := data.ActivityLog[i-1]
		curr := data.ActivityLog[i]
		if prev.HasError && curr.ToolName == prev.ToolName {
			toolAttempts[curr.ToolName]++
		}
	}

	var findings []Finding
	for toolName, count := range toolAttempts {
		if count >= 3 {
			estTokens := count * 3000
			findings = append(findings, Finding{
				Name:          "retry_churn",
				Confidence:    0.8,
				SavingsTokens: estTokens,
				Evidence:      toolName + " retried " + itoa(count) + " times after errors",
				Suggestion: "Stop and diagnose after 2 failures instead of retrying. " +
					itoa(count) + " retries of " + toolName + " wasted ~" + itoa(estTokens) + " tokens.",
			})
		}
	}
	return findings
}

// ---------------------------------------------------------------------------
// Detector 2: tool_cascade — 4+ consecutive tool errors
// ---------------------------------------------------------------------------

func detectToolCascade(data SessionData) []Finding {
	if len(data.ActivityLog) == 0 {
		return nil
	}

	var streaks []int
	consecutive := 0
	for _, entry := range data.ActivityLog {
		if entry.HasError {
			consecutive++
		} else {
			if consecutive >= 4 {
				streaks = append(streaks, consecutive)
			}
			consecutive = 0
		}
	}
	if consecutive >= 4 {
		streaks = append(streaks, consecutive)
	}

	var findings []Finding
	for _, streakLen := range streaks {
		estTokens := streakLen * 2500
		findings = append(findings, Finding{
			Name:          "tool_cascade",
			Confidence:    0.7,
			SavingsTokens: estTokens,
			Evidence:      itoa(streakLen) + " consecutive tool errors in a row",
			Suggestion: "A cascade of " + itoa(streakLen) + " tool errors burned ~" + itoa(estTokens) +
				" tokens. Break error chains early: diagnose the root cause after 2 failures.",
		})
	}
	return findings
}

// ---------------------------------------------------------------------------
// Detector 3: looping — needs user message content; not available from store
// ---------------------------------------------------------------------------

func detectLooping(data SessionData) []Finding {
	// Cannot detect without user message content — session store doesn't have it.
	return nil
}

// ---------------------------------------------------------------------------
// Detector 4: overpowered — needs model_usage; not in session store
// ---------------------------------------------------------------------------

func detectOverpowered(data SessionData) []Finding {
	return nil
}

// ---------------------------------------------------------------------------
// Detector 5: weak_model — needs model_usage; not in session store
// ---------------------------------------------------------------------------

func detectWeakModel(data SessionData) []Finding {
	return nil
}

// ---------------------------------------------------------------------------
// Detector 6: bad_decomposition — needs JSONL user messages; return nil
// ---------------------------------------------------------------------------

func detectBadDecomposition(data SessionData) []Finding {
	return nil
}

// ---------------------------------------------------------------------------
// Detector 7: wasteful_thinking — needs thinking tokens from JSONL; return nil
// ---------------------------------------------------------------------------

func detectWastefulThinking(data SessionData) []Finding {
	return nil
}

// ---------------------------------------------------------------------------
// Detector 8: output_waste — detect large tool outputs
// ---------------------------------------------------------------------------

func detectOutputWaste(data SessionData) []Finding {
	if len(data.ToolOutputs) == 0 {
		return nil
	}

	var largeCount, largeWaste int
	const verboseThreshold = 2000

	for _, o := range data.ToolOutputs {
		if o.OutputTokensEst > verboseThreshold {
			largeCount++
			largeWaste += o.OutputTokensEst - verboseThreshold
		}
	}

	if largeCount < 3 || largeWaste < 5000 {
		return nil
	}

	return []Finding{{
		Name:          "output_waste",
		Confidence:    0.6,
		SavingsTokens: largeWaste,
		Evidence:      itoa(largeCount) + " tool outputs exceeded " + itoa(verboseThreshold) + " tokens (~" + itoa(largeWaste) + " excess tokens)",
		Suggestion: "Found " + itoa(largeCount) + " large tool outputs. ~" + itoa(largeWaste) +
			" tokens could be saved with more targeted tool calls or by using offset/limit on Read.",
	}}
}

// ---------------------------------------------------------------------------
// Detector 9: cache_instability — detect timestamps in CLAUDE.md prefix
// ---------------------------------------------------------------------------

var cacheInstabilityPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}`),
	regexp.MustCompile(`(?:Mon|Tue|Wed|Thu|Fri|Sat|Sun),?\s+\w+\s+\d{1,2}`),
	regexp.MustCompile(`Updated:\s*\d{4}-\d{2}-\d{2}`),
	regexp.MustCompile(`Last (?:updated|modified|synced):\s*\d`),
	regexp.MustCompile(`(?:^|>?\s*)As of \d{4}-\d{2}`),
}

var dynamicSectionMarkers = []string{
	"TOKEN_OPTIMIZER:MODEL_ROUTING",
	"AUTO-GENERATED",
	"DO NOT EDIT",
	"Generated by",
	"Synced from",
}

func detectCacheInstability(data SessionData) []Finding {
	content := data.ClaudeMDContent
	if content == "" {
		// Try to read CLAUDE.md from cwd.
		if raw, err := os.ReadFile("CLAUDE.md"); err == nil {
			content = string(raw)
		}
	}
	if len(content) < 200 {
		return nil
	}

	lines := strings.Split(content, "\n")
	prefixCutoff := len(lines) * 6 / 10

	var findings []Finding

	// Signal 1: timestamps in first 60%.
	var earlyTimestamps [][2]int // [lineNum, patIdx]
	for i, line := range lines[:prefixCutoff] {
		for _, pat := range cacheInstabilityPatterns {
			if pat.MatchString(line) {
				earlyTimestamps = append(earlyTimestamps, [2]int{i + 1, 0})
				break
			}
		}
	}
	if len(earlyTimestamps) > 0 {
		firstLine := earlyTimestamps[0][0]
		charsAfter := 0
		for _, ln := range lines[firstLine:] {
			charsAfter += len(ln)
		}
		wastedTokens := charsAfter / 4
		if wastedTokens > 500 {
			findings = append(findings, Finding{
				Name:          "cache_instability",
				Confidence:    0.75,
				SavingsTokens: wastedTokens,
				Evidence:      "Timestamps in CLAUDE.md prefix invalidate prompt cache for ~" + itoa(wastedTokens) + " tokens of stable content",
				Suggestion:    "Move timestamped content to the bottom of CLAUDE.md. Anthropic's prompt cache is prefix-based.",
			})
		}
	}

	// Signal 2: auto-generated markers in first 60%.
	var dynamicSections [][2]interface{}
	for i, line := range lines[:prefixCutoff] {
		for _, marker := range dynamicSectionMarkers {
			if strings.Contains(line, marker) {
				dynamicSections = append(dynamicSections, [2]interface{}{i + 1, marker})
				break
			}
		}
	}
	if len(dynamicSections) > 0 {
		firstLine := dynamicSections[0][0].(int)
		charsAfter := 0
		for _, ln := range lines[firstLine:] {
			charsAfter += len(ln)
		}
		wastedTokens := charsAfter / 4
		if wastedTokens > 500 {
			findings = append(findings, Finding{
				Name:          "cache_instability",
				Confidence:    0.7,
				SavingsTokens: wastedTokens,
				Evidence:      "Auto-generated sections in CLAUDE.md prefix break prompt cache stability",
				Suggestion:    "Move auto-generated sections to the end of CLAUDE.md.",
			})
		}
	}

	return findings
}

// ---------------------------------------------------------------------------
// Detector 10: pdf_ingestion — detect binary/expensive file reads
// ---------------------------------------------------------------------------

var expensiveBinaryExts = map[string]bool{
	".pdf": true, ".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".bmp": true, ".webp": true,
	".docx": true, ".xlsx": true, ".pptx": true,
}

var documentExts = map[string]bool{
	".pdf": true, ".docx": true, ".xlsx": true, ".pptx": true,
}

func detectPDFIngestion(data SessionData) []Finding {
	var findings []Finding
	for _, fr := range data.FileReads {
		ext := strings.ToLower(filepath.Ext(fr.FilePath))
		if !expensiveBinaryExts[ext] {
			continue
		}
		estTokens := fr.Tokens
		if estTokens == 0 {
			// Estimate from extension type.
			if documentExts[ext] {
				estTokens = 2500 // ~1MB document
			} else {
				estTokens = 1500
			}
		}
		suggestion := "Extract text first (`pdftotext`), read specific pages (pages: param), or summarize externally."
		if ext != ".pdf" {
			suggestion = "Consider extracting text content first, or describe what you need from this file."
		}
		findings = append(findings, Finding{
			Name:          "pdf_ingestion",
			Confidence:    0.9,
			SavingsTokens: estTokens,
			Evidence:      ext + " file read: " + filepath.Base(fr.FilePath) + " (~" + itoa(estTokens) + " tokens)",
			Suggestion:    suggestion,
		})
	}
	return findings
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
