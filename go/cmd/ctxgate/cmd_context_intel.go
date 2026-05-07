package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/agusrdz/ctxgate/internal/hookio"
	"github.com/agusrdz/ctxgate/internal/pluginenv"
	"github.com/agusrdz/ctxgate/internal/sessionstore"
	"github.com/spf13/cobra"
)

const (
	intelOutputThreshold = 8192 // only summarize outputs >= 8K chars
	intelSummaryCap      = 600  // max chars per summary
	intelCooldownWindow  = 300  // 5 minutes in seconds
	intelCooldownMax     = 3    // max summaries per window
	intelMaxDecisions    = 10
	intelPruneThreshold  = 30
	intelPruneKeep       = 20
	intelWindowSize      = 10
)

var (
	intelPathRE = regexp.MustCompile(
		`(?:^|[\s"':=])(/[\w./-]{3,120}(?:\.\w{1,10})?)`,
	)
	intelErrorRE = regexp.MustCompile(
		`(?m)^.*(?:error|Error|ERROR|FAIL|FAILED|panic|exception|Exception` +
			`|TypeError|ValueError|KeyError|ImportError|ModuleNotFoundError` +
			`|SyntaxError|RuntimeError|AttributeError|NameError|OSError` +
			`|FileNotFoundError|PermissionError|ConnectionError` +
			`|traceback|Traceback).*$`,
	)
	intelWarnRE = regexp.MustCompile(
		`(?m)^.*(?:warning|Warning|WARNING|WARN|deprecated|DEPRECATED).*$`,
	)
	intelErrorTypeRE = regexp.MustCompile(
		`(TypeError|ValueError|KeyError|ImportError|ModuleNotFoundError` +
			`|SyntaxError|RuntimeError|AttributeError|NameError|OSError` +
			`|FileNotFoundError|PermissionError|ConnectionError` +
			`|Error|FAIL|FAILED|panic|exception|Exception` +
			`|warning|Warning|WARNING|WARN|deprecated)`,
	)
	intelDecisionRE = regexp.MustCompile(
		`(?i)\b(chose|decided|because|instead of|went with|going with|switched to|` +
			`prefer|better to|should use|will use|picking|opting for|let's use|` +
			`using .+ over|settled on|sticking with)\b`,
	)
	intelSentenceSplitRE = regexp.MustCompile(`[.!?\n]+`)
	intelSafeToolNameRE  = regexp.MustCompile(`[^A-Za-z0-9._:/-]`)

	// Tool bucket sets
	intelEditTools  = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}
	intelReadTools  = map[string]bool{"Read": true, "Glob": true, "Grep": true}
	intelAgentTools = map[string]bool{"Agent": true, "TaskCreate": true, "TaskUpdate": true, "TaskGet": true, "TaskList": true}

	// Bash sub-classifiers
	intelInfraBashRE  = regexp.MustCompile(`\b(?:systemctl|nginx|docker|kubectl|service|daemon|launchctl|brew|apt|apt-get|yum|dnf|pacman)\b`)
	intelGitWriteRE   = regexp.MustCompile(`\bgit\s+(?:push|pull|merge|rebase|cherry-pick|tag)\b`)
	intelInstallRE    = regexp.MustCompile(`\b(?:pip|npm|pnpm|yarn|bun|cargo|go)\s+(?:install|add|update|upgrade)\b`)
)

func init() {
	rootCmd.AddCommand(newContextIntelCmd())
}

func newContextIntelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context-intel",
		Short: "PostToolUse[Bash|Read|Grep|Glob|mcp__.*] hook — emit context intelligence events",
		RunE: func(cmd *cobra.Command, args []string) error {
			runContextIntel()
			return nil
		},
	}
}

func runContextIntel() {
	hookInput, err := hookio.ReadStdinJSON(hookio.MaxBytes)
	if err != nil && len(hookInput) == 0 {
		return
	}

	toolName, _ := hookInput["tool_name"].(string)
	toolUseID, _ := hookInput["tool_use_id"].(string)
	toolResponse, _ := hookInput["tool_response"].(string)
	sessionID, _ := hookInput["session_id"].(string)

	if sessionID == "" {
		return
	}

	snapshotDir := pluginenv.ResolveSnapshotDir()
	store, storeErr := sessionstore.Open(sessionID, snapshotDir)
	if storeErr != nil {
		return
	}
	defer store.Close()

	// Activity tracking on every tool call.
	var command string
	if toolInput, ok := hookInput["tool_input"].(map[string]any); ok {
		if cmd, ok := toolInput["command"].(string); ok {
			command = cmd
		}
	}
	hasError := intelHasErrorSignals(toolResponse)
	bucket := intelClassifyTool(toolName, command)
	_ = store.InsertActivityLog(toolName, bucket, hasError)
	intelPruneAndUpdateMode(store, toolName)

	// Decision extraction on outputs > 500 chars.
	if len(toolResponse) > 500 {
		intelExtractDecisions(toolResponse, store)
	}

	// Context intel summary only for large outputs.
	if toolUseID == "" || len(toolResponse) < intelOutputThreshold {
		return
	}

	// Cooldown check via DB.
	cutoff := float64(time.Now().Unix() - intelCooldownWindow)
	count, _ := store.CountContextIntelEventsSince(cutoff)
	if count >= intelCooldownMax {
		return
	}

	summary := intelSummarizeOutput(toolName, toolResponse)
	_ = store.InsertContextIntelEvent(toolName, toolUseID, summary, len(toolResponse))
}

// ---------------------------------------------------------------------------
// Activity tracking helpers
// ---------------------------------------------------------------------------

func intelClassifyTool(toolName, command string) string {
	if intelEditTools[toolName] {
		return "edit"
	}
	if intelReadTools[toolName] {
		return "read"
	}
	if intelAgentTools[toolName] {
		return "agent"
	}
	if strings.HasPrefix(toolName, "mcp__") {
		return "mcp"
	}
	if toolName == "Bash" {
		if intelInfraBashRE.MatchString(command) {
			return "bash_infra"
		}
		if intelGitWriteRE.MatchString(command) {
			return "bash_git"
		}
		if intelInstallRE.MatchString(command) {
			return "bash_install"
		}
		return "bash_other"
	}
	if toolName == "WebSearch" || toolName == "WebFetch" {
		return "web"
	}
	return "other"
}

func intelPruneAndUpdateMode(store *sessionstore.SessionStore, _ string) {
	mode := store.DetectCurrentMode(intelWindowSize, intelPruneThreshold, intelPruneKeep)
	_ = store.SetMeta("current_mode", mode)
}

// ---------------------------------------------------------------------------
// Decision extraction
// ---------------------------------------------------------------------------

func intelExtractDecisions(text string, store *sessionstore.SessionStore) {
	if len(text) < 50 {
		return
	}
	sample := text
	if len(sample) > 5000 {
		sample = sample[:5000]
	}
	if !intelDecisionRE.MatchString(sample) {
		return
	}

	rawSentences := intelSentenceSplitRE.Split(sample, -1)
	var newDecisions []string
	for _, s := range rawSentences {
		s = strings.TrimSpace(s)
		if len(s) < 20 || len(s) > 200 {
			continue
		}
		if intelDecisionRE.MatchString(s) {
			if len(s) > 150 {
				s = s[:150]
			}
			newDecisions = append(newDecisions, s)
			if len(newDecisions) >= 3 {
				break
			}
		}
	}
	if len(newDecisions) == 0 {
		return
	}

	existing := []string{}
	raw, _ := store.GetMeta("session_decisions")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &existing)
	}
	if len(existing) >= intelMaxDecisions {
		return
	}
	seen := map[string]bool{}
	for _, d := range existing {
		seen[d] = true
	}
	for _, d := range newDecisions {
		if !seen[d] {
			existing = append(existing, d)
			if len(existing) >= intelMaxDecisions {
				break
			}
		}
	}
	if data, err := json.Marshal(existing); err == nil {
		_ = store.SetMeta("session_decisions", string(data))
	}
}

// ---------------------------------------------------------------------------
// Summary extraction
// ---------------------------------------------------------------------------

func intelSummarizeOutput(toolName, output string) string {
	toolName = intelSafeToolNameRE.ReplaceAllString(toolName, "")
	if len(toolName) > 64 {
		toolName = toolName[:64]
	}

	lines := strings.Split(output, "\n")
	lineCount := len(lines)
	charCount := len(output)

	var parts []string
	parts = append(parts, strings.Join([]string{toolName, ": ", itoa(lineCount), " lines, ", itoa(charCount), " chars"}, ""))

	paths := intelExtractPaths(output)
	if len(paths) > 0 {
		end := min(5, len(paths))
		parts = append(parts, "Files: "+strings.Join(paths[:end], ", "))
		if len(paths) > 5 {
			parts = append(parts, "  +"+itoa(len(paths)-5)+" more paths")
		}
	}

	signals := intelExtractSignals(output)
	for i, s := range signals {
		if i >= 4 {
			break
		}
		parts = append(parts, s)
	}

	if lineCount > 20 && len(signals) == 0 {
		parts = append(parts, "Output: "+itoa(lineCount)+" lines, no errors detected")
	}

	summary := strings.Join(parts, "\n")
	if len(summary) > intelSummaryCap {
		return summary[:intelSummaryCap]
	}
	return summary
}

func intelExtractPaths(text string) []string {
	sample := text
	if len(sample) > 50000 {
		sample = sample[:50000]
	}
	matches := intelPathRE.FindAllStringSubmatch(sample, -1)
	seen := map[string]bool{}
	var paths []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		p := m[1]
		if seen[p] || strings.HasPrefix(p, "/dev/") || strings.HasPrefix(p, "/proc/") {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
		if len(paths) >= 10 {
			break
		}
	}
	return paths
}

func intelExtractSignals(text string) []string {
	sample := text
	if len(sample) > 50000 {
		sample = sample[:50000]
	}

	var signals []string
	seen := map[string]bool{}

	errors := intelErrorRE.FindAllString(sample, -1)
	for _, e := range errors {
		sanitized := intelSanitizeSignal(e, "ERR")
		if !seen[sanitized] {
			seen[sanitized] = true
			signals = append(signals, sanitized)
			if len(signals) >= 5 {
				break
			}
		}
	}

	warns := intelWarnRE.FindAllString(sample, -1)
	for _, w := range warns {
		sanitized := intelSanitizeSignal(w, "WARN")
		if !seen[sanitized] {
			seen[sanitized] = true
			signals = append(signals, sanitized)
			if len(signals) >= 8 {
				break
			}
		}
	}

	return signals
}

func intelSanitizeSignal(rawLine, prefix string) string {
	typeStr := "unknown"
	if m := intelErrorTypeRE.FindString(rawLine); m != "" {
		typeStr = m
	}
	pathStr := ""
	if pm := intelPathRE.FindStringSubmatch(rawLine); len(pm) >= 2 {
		pathStr = " in " + pm[1]
	}
	return prefix + ": " + typeStr + pathStr
}

func intelHasErrorSignals(text string) bool {
	if len(text) < 10 {
		return false
	}
	sample := text
	if len(sample) > 20000 {
		sample = sample[:20000]
	}
	return intelErrorRE.MatchString(sample)
}

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
