package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agusrdz/ctxgate/internal/hookio"
	"github.com/agusrdz/ctxgate/internal/pathsafe"
	"github.com/agusrdz/ctxgate/internal/pluginenv"
	"github.com/agusrdz/ctxgate/internal/sessionstore"
	"github.com/agusrdz/ctxgate/internal/trends"
	"github.com/spf13/cobra"
)

const (
	archiveThreshold   = 4096
	archivePreviewSize = 1500
	archiveMaxSize     = 5 * 1024 * 1024
)

var validToolUseID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func init() {
	rootCmd.AddCommand(newArchiveResultCmd())
}

func newArchiveResultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archive-result",
		Short: "PostToolUse[Bash|Read|Glob|Grep|Agent|mcp__.*] hook — archive tool output",
		RunE: func(cmd *cobra.Command, args []string) error {
			runArchiveResult()
			return nil
		},
	}
}

func runArchiveResult() {
	hookInput, err := hookio.ReadStdinJSON(hookio.MaxBytes)
	if err != nil && len(hookInput) == 0 {
		return
	}

	toolName, _ := hookInput["tool_name"].(string)
	toolUseID, _ := hookInput["tool_use_id"].(string)
	toolResponse, _ := hookInput["tool_response"].(string)
	sessionID, _ := hookInput["session_id"].(string)

	if toolResponse == "" || len(toolResponse) < archiveThreshold {
		return
	}
	if toolUseID == "" || sessionID == "" {
		return
	}
	if !validToolUseID.MatchString(toolUseID) {
		return
	}

	snapshotDir := pluginenv.ResolveSnapshotDir()
	sid := pathsafe.SanitizeSessionID(sessionID)
	archiveDir := filepath.Join(snapshotDir, "tool-archive", sid)

	originalCharCount := len(toolResponse)
	truncated := originalCharCount > archiveMaxSize
	if truncated {
		toolResponse = toolResponse[:archiveMaxSize] +
			fmt.Sprintf("\n\n[TRUNCATED at 5MB. Original size: %d chars]", originalCharCount)
	}

	charCount := originalCharCount
	if truncated {
		charCount = archiveMaxSize
	}
	tokenEst := charCount / 4

	now := time.Now().UTC().Format(time.RFC3339)
	meta := map[string]any{
		"tool_name":      toolName,
		"tool_use_id":    toolUseID,
		"chars":          charCount,
		"original_chars": originalCharCount,
		"tokens_est":     tokenEst,
		"truncated":      truncated,
		"timestamp":      now,
		"archived_from":  "PostToolUse",
	}

	if mkErr := os.MkdirAll(archiveDir, 0o700); mkErr == nil {
		entryPath := filepath.Join(archiveDir, toolUseID+".json")
		full := make(map[string]any, len(meta)+1)
		for k, v := range meta {
			full[k] = v
		}
		full["response"] = toolResponse
		if data, jErr := json.Marshal(full); jErr == nil {
			_ = archiveWriteFile600(entryPath, data)
		}

		manifestPath := filepath.Join(archiveDir, "manifest.jsonl")
		if data, jErr := json.Marshal(meta); jErr == nil {
			_ = archiveAppendFile600(manifestPath, append(data, '\n'))
		}
	}

	toolType := strings.ToLower(toolName)
	if strings.Contains(toolName, "__") {
		toolType = "mcp"
	}
	cmdOrPath := toolName
	if toolInput, ok := hookInput["tool_input"].(map[string]any); ok {
		if cmd, ok := toolInput["command"].(string); ok && cmd != "" {
			cmdOrPath = cmd
		} else if fp, ok := toolInput["file_path"].(string); ok && fp != "" {
			cmdOrPath = fp
		}
	}
	if len(cmdOrPath) > 500 {
		cmdOrPath = cmdOrPath[:500]
	}

	sample := toolResponse
	if len(sample) > 10000 {
		sample = sample[:10000]
	}
	h := sha256.Sum256([]byte(sample))
	outputHash := fmt.Sprintf("%x", h[:8])

	store, storeErr := sessionstore.Open(sessionID, snapshotDir)
	if storeErr == nil {
		preview := toolResponse[:min(archivePreviewSize, len(toolResponse))]
		_ = store.InsertToolOutput(toolUseID, toolName, toolType, cmdOrPath, outputHash,
			charCount, tokenEst, preview)
		store.Close()
	}

	if strings.Contains(toolName, "__") {
		outputType := detectOutputType(toolResponse)
		preview := compressMCPPreview(toolResponse, outputType)
		suffix := ""
		if outputType != "text" {
			suffix = " (" + outputType + ")"
		}
		var replacement string
		if originalCharCount > archiveMaxSize {
			replacement = preview + fmt.Sprintf("\n\n[Full result archived (%d chars%s, truncated to 5MB).]", originalCharCount, suffix)
		} else {
			replacement = preview + fmt.Sprintf("\n\n[Full result archived (%d chars%s).]", charCount, suffix)
		}
		out := map[string]any{"updatedMCPToolOutput": replacement}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(out)

		previewTokens := len(preview) / 4
		if savedTokens := tokenEst - previewTokens; savedTokens > 0 {
			dbPath := filepath.Join(snapshotDir, "trends.db")
			if w, err := trends.NewWriter(dbPath); err == nil {
				_ = w.RecordSavings(trends.SavingsEvent{
					SessionID:   sessionID,
					ToolName:    "mcp-archive",
					RawTokens:   tokenEst,
					SavedTokens: savedTokens,
				})
				w.Close()
			}
		}
	}
}

func archiveWriteFile600(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	f.Close()
	return err
}

func archiveAppendFile600(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	f.Close()
	return err
}

func detectOutputType(text string) string {
	stripped := strings.TrimSpace(text)
	if len(stripped) > 0 && (stripped[0] == '{' || stripped[0] == '[') {
		sample := stripped
		if len(sample) > 100000 {
			sample = sample[:100000]
		}
		var v any
		if json.Unmarshal([]byte(sample), &v) == nil {
			return "json"
		}
	}
	lines := strings.Split(stripped, "\n")
	if len(lines) > 5 {
		check := lines
		if len(check) > 50 {
			check = check[:50]
		}
		pathLike := 0
		for _, ln := range check {
			if strings.ContainsAny(ln, "/\\") {
				pathLike++
			}
		}
		if pathLike > len(check)*6/10 {
			return "paths"
		}
		sepCount := 0
		for _, ln := range check {
			t := strings.TrimSpace(ln)
			if t == "" {
				continue
			}
			allSep := true
			for _, c := range t {
				if c != '-' && c != '=' && c != '|' && c != ' ' && c != '+' {
					allSep = false
					break
				}
			}
			if allSep {
				sepCount++
			}
		}
		if sepCount >= 1 {
			return "table"
		}
	}
	return "text"
}

func compressMCPPreview(text, outputType string) string {
	switch outputType {
	case "json":
		return compressMCPJSON(text)
	case "paths":
		return compressMCPPaths(text)
	case "table":
		return compressMCPTable(text)
	default:
		if len(text) > archivePreviewSize {
			return text[:archivePreviewSize]
		}
		return text
	}
}

func compressMCPJSON(text string) string {
	sample := text
	if len(sample) > 500000 {
		sample = sample[:500000]
	}
	var data any
	if json.Unmarshal([]byte(sample), &data) != nil {
		if len(text) > archivePreviewSize {
			return text[:archivePreviewSize]
		}
		return text
	}

	var parts []string
	switch v := data.(type) {
	case map[string]any:
		parts = append(parts, fmt.Sprintf("JSON object (%d keys):", len(v)))
		i := 0
		for key, val := range v {
			if i >= 15 {
				break
			}
			switch inner := val.(type) {
			case []any:
				parts = append(parts, fmt.Sprintf("  %s: [%d items]", key, len(inner)))
			case map[string]any:
				subkeys := make([]string, 0, 5)
				for sk := range inner {
					if len(subkeys) >= 5 {
						break
					}
					subkeys = append(subkeys, sk)
				}
				suffix := ""
				if len(inner) > 5 {
					suffix = "..."
				}
				parts = append(parts, fmt.Sprintf("  %s: {%s%s}", key, strings.Join(subkeys, ", "), suffix))
			case string:
				if len(inner) > 80 {
					parts = append(parts, fmt.Sprintf("  %s: %q", key, inner[:77]+"..."))
				} else {
					b, _ := json.Marshal(inner)
					s := string(b)
					if len(s) > 80 {
						s = s[:80]
					}
					parts = append(parts, fmt.Sprintf("  %s: %s", key, s))
				}
			default:
				b, _ := json.Marshal(val)
				s := string(b)
				if len(s) > 80 {
					s = s[:80]
				}
				parts = append(parts, fmt.Sprintf("  %s: %s", key, s))
			}
			i++
		}
		if len(v) > 15 {
			parts = append(parts, fmt.Sprintf("  ... (%d more keys)", len(v)-15))
		}
	case []any:
		parts = append(parts, fmt.Sprintf("JSON array (%d items):", len(v)))
		for idx, item := range v {
			if idx >= 5 {
				break
			}
			switch inner := item.(type) {
			case map[string]any:
				keys := make([]string, 0, 5)
				for k := range inner {
					if len(keys) >= 5 {
						break
					}
					keys = append(keys, k)
				}
				suffix := ""
				if len(inner) > 5 {
					suffix = "..."
				}
				parts = append(parts, fmt.Sprintf("  {%s%s}", strings.Join(keys, ", "), suffix))
			default:
				b, _ := json.Marshal(item)
				s := string(b)
				if len(s) > 80 {
					s = s[:80]
				}
				parts = append(parts, "  "+s)
			}
		}
		if len(v) > 5 {
			parts = append(parts, fmt.Sprintf("  ... (%d more items)", len(v)-5))
		}
	}

	result := strings.Join(parts, "\n")
	if len(result) > archivePreviewSize {
		return result[:archivePreviewSize]
	}
	return result
}

func compressMCPPaths(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	dirs := map[string]int{}
	for _, line := range lines {
		stripped := strings.TrimSpace(line)
		if idx := strings.LastIndex(stripped, "/"); idx >= 0 {
			dirs[stripped[:idx]]++
		}
	}

	type kv struct {
		k string
		v int
	}
	sorted := make([]kv, 0, len(dirs))
	for k, v := range dirs {
		sorted = append(sorted, kv{k, v})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].v > sorted[i].v {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	parts := []string{fmt.Sprintf("%d paths across %d directories:", len(lines), len(dirs))}
	for idx, d := range sorted {
		if idx >= 10 {
			break
		}
		parts = append(parts, fmt.Sprintf("  %s/ (%d files)", d.k, d.v))
	}
	if len(sorted) > 10 {
		parts = append(parts, fmt.Sprintf("  ... (%d more directories)", len(sorted)-10))
	}

	result := strings.Join(parts, "\n")
	if len(result) > archivePreviewSize {
		return result[:archivePreviewSize]
	}
	return result
}

func compressMCPTable(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	var result []string
	headerEnd := min(2, len(lines))
	result = append(result, lines[:headerEnd]...)
	rest := lines[headerEnd:]
	var nonEmpty []string
	for _, ln := range rest {
		if strings.TrimSpace(ln) != "" {
			nonEmpty = append(nonEmpty, ln)
		}
	}
	end := min(10, len(nonEmpty))
	result = append(result, nonEmpty[:end]...)
	if len(nonEmpty) > 10 {
		result = append(result, fmt.Sprintf("... (%d more rows, %d total)", len(nonEmpty)-10, len(nonEmpty)))
	}
	out := strings.Join(result, "\n")
	if len(out) > archivePreviewSize {
		return out[:archivePreviewSize]
	}
	return out
}
