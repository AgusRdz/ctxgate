package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/agusrdz/ctxgate/internal/contextignore"
	"github.com/agusrdz/ctxgate/internal/deltadiff"
	"github.com/agusrdz/ctxgate/internal/hookio"
	"github.com/agusrdz/ctxgate/internal/pluginenv"
	"github.com/agusrdz/ctxgate/internal/sessionstore"
	"github.com/agusrdz/ctxgate/internal/structuremap"
	"github.com/spf13/cobra"
)

const (
	minStructureConfidence    = 0.75
	maxAdditionalContextChars = 2600
	reasonOnlyTokensEst       = 10
	maxRangesSeen             = 20
)

var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".ico": true, ".webp": true, ".svg": true,
	".pdf": true, ".wasm": true, ".zip": true, ".tar": true, ".gz": true,
	".bz2": true, ".7z": true, ".rar": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".o": true, ".a": true,
	".mp3": true, ".mp4": true, ".wav": true, ".avi": true, ".mov": true, ".mkv": true,
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true,
	".pyc": true, ".pyo": true, ".class": true, ".jar": true,
	".sqlite": true, ".db": true, ".sqlite3": true,
}

func init() {
	rootCmd.AddCommand(newReadCacheCmd())
}

func newReadCacheCmd() *cobra.Command {
	var clear, invalidate bool

	cmd := &cobra.Command{
		Use:   "read-cache",
		Short: "PreToolUse[Read] hook — deduplicate and compress file reads",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case clear:
				return runReadCacheClear()
			case invalidate:
				return runReadCacheInvalidate()
			default:
				return runReadCacheHook()
			}
		},
	}

	cmd.Flags().BoolVar(&clear, "clear", false, "clear file read cache (PreCompact / CwdChanged)")
	cmd.Flags().BoolVar(&invalidate, "invalidate", false, "invalidate stale entries (PostToolUse[Edit|Write|...])")

	return cmd
}

// ---------------------------------------------------------------------------
// Mode 1: hook mode (no flags) — PreToolUse[Read]
// ---------------------------------------------------------------------------

func runReadCacheHook() error {
	hookInput, err := hookio.ReadStdinJSON(hookio.MaxBytes)
	if err != nil && len(hookInput) == 0 {
		hookio.EmitSilent()
		return nil
	}

	toolName, _ := hookInput["tool_name"].(string)
	if toolName != "Read" {
		hookio.EmitSilent()
		return nil
	}

	toolInput, _ := hookInput["tool_input"].(map[string]any)
	if toolInput == nil {
		hookio.EmitSilent()
		return nil
	}

	rawPath, _ := toolInput["file_path"].(string)
	if rawPath == "" {
		hookio.EmitSilent()
		return nil
	}

	filePath, err := filepath.Abs(rawPath)
	if err != nil {
		hookio.EmitSilent()
		return nil
	}

	sessionID := sessionIDFrom(hookInput)
	offset := toInt(toolInput["offset"])
	limit := toInt(toolInput["limit"])

	// Check if read-cache is enabled.
	if os.Getenv("TOKEN_OPTIMIZER_READ_CACHE") == "0" {
		hookio.EmitSilent()
		return nil
	}

	mode := os.Getenv("TOKEN_OPTIMIZER_READ_CACHE_MODE")
	if mode == "" {
		mode = "soft_block"
	}

	// .contextignore check.
	m, _ := contextignore.Load(".")
	if m != nil && m.Match(filePath) {
		base := filepath.Base(filePath)
		hookio.EmitPreToolResponse("deny", "Blocked by .contextignore: "+base, "")
		return nil
	}

	// Binary extension check.
	if binaryExtensions[filepath.Ext(filePath)] {
		hookio.EmitSilent()
		return nil
	}

	// Open SessionStore.
	snapshotDir := pluginenv.ResolveSnapshotDir()
	store, err := sessionstore.Open(sessionID, snapshotDir)
	if err != nil {
		hookio.EmitSilent()
		return nil
	}
	defer store.Close()

	entry, err := store.GetFileEntry(filePath)
	if err != nil {
		hookio.EmitSilent()
		return nil
	}

	nowF := unixNowFloat()

	// Delta mode flag.
	deltaEnabled := pluginenv.IsV5FlagEnabled("v5_delta_mode", "TOKEN_OPTIMIZER_READ_CACHE_DELTA", true, "")

	if entry == nil {
		// First read.
		info, statErr := os.Stat(filePath)
		var mtimeNs, sizeBytes int64
		if statErr == nil {
			mtimeNs = info.ModTime().UnixNano()
			sizeBytes = info.Size()
		}

		e := &sessionstore.FileEntry{
			FilePath:   filePath,
			MtimeNs:    mtimeNs,
			SizeBytes:  sizeBytes,
			RangesSeen: encodeRanges([][2]int{{offset, limit}}),
			TokensEst:  int(sizeBytes / 4),
			ReadCount:  1,
			LastAccess: nowF,
		}

		// Cache content for delta if eligible.
		if deltaEnabled && offset == 0 && limit == 0 && deltadiff.IsEligible(filePath) {
			if statErr == nil && sizeBytes <= deltadiff.MaxContentCacheBytes {
				content, readErr := os.ReadFile(filePath)
				if readErr == nil {
					hash := deltadiff.ContentHash(content)
					e.ContentHash = hash
					_ = store.UpsertCachedContent(filePath, string(content), hash)
				}
			}
		}

		_ = store.UpsertFileEntry(filePath, e)
		hookio.EmitSilent()
		return nil
	}

	// Subsequent read.
	entry.ReadCount++
	entry.LastAccess = nowF

	info, statErr := os.Stat(filePath)
	if statErr != nil {
		_ = store.DeleteFileEntry(filePath)
		hookio.EmitSilent()
		return nil
	}

	newMtimeNs := info.ModTime().UnixNano()
	newSizeBytes := info.Size()
	mtimeMatch := newMtimeNs == entry.MtimeNs
	sizeMatch := newSizeBytes == entry.SizeBytes

	ranges := parseRanges(entry.RangesSeen)
	rangeCovered := isRangeCovered(ranges, offset, limit)

	if !(mtimeMatch && sizeMatch && rangeCovered) {
		// File changed or range not seen — try delta first.
		if deltaEnabled && offset == 0 && limit == 0 && !mtimeMatch && entry.ContentHash != "" {
			cached, cacheErr := store.GetCachedContent(filePath)
			if cacheErr == nil && cached != nil {
				newContent, readErr := os.ReadFile(filePath)
				if readErr == nil {
					newHash := deltadiff.ContentHash(newContent)
					if newHash != entry.ContentHash {
						delta, _, ok := deltadiff.ComputeDelta(cached.Content, string(newContent), filepath.Base(filePath))
						if ok {
							// Update entry with new state.
							entry.MtimeNs = newMtimeNs
							entry.SizeBytes = newSizeBytes
							entry.ContentHash = newHash
							entry.RangesSeen = encodeRanges([][2]int{{0, 0}})
							resetReplacementState(entry)
							_ = store.UpsertFileEntry(filePath, entry)
							_ = store.UpsertCachedContent(filePath, string(newContent), newHash)

							hookio.EmitPreToolResponse("deny", "", delta)
							return nil
						}
					}
				}
			}
		}

		// No delta path — update entry, allow the read.
		entry.MtimeNs = newMtimeNs
		entry.SizeBytes = newSizeBytes
		if !rangeCovered {
			ranges = appendRange(ranges, [2]int{offset, limit})
			entry.RangesSeen = encodeRanges(ranges)
		}
		resetReplacementState(entry)
		_ = store.UpsertFileEntry(filePath, entry)
		hookio.EmitSilent()
		return nil
	}

	// Range covered — redundant read.
	if mode == "shadow" || mode == "warn" {
		if mode == "warn" {
			fmt.Fprintf(os.Stderr, "[ctxgate] read-cache: redundant read of %s (mode=warn)\n", filepath.Base(filePath))
		}
		_ = store.UpsertFileEntry(filePath, entry)
		hookio.EmitSilent()
		return nil
	}

	// soft_block or block: try structure map.
	eligible := false
	var summary structuremap.StructureMapResult
	tokensEst := entry.TokensEst
	if tokensEst == 0 {
		tokensEst = int(entry.SizeBytes / 4)
	}

	if structuremap.IsStructureSupported(filePath) {
		content, readErr := os.ReadFile(filePath)
		if readErr == nil {
			summary = structuremap.SummarizeCodeSource(
				string(content), filePath,
				offset, limit,
				tokensEst, int(entry.SizeBytes),
			)
			eligible = summary.Eligible && summary.Confidence >= minStructureConfidence
		}
	}

	if eligible {
		netSaved := tokensEst - summary.ReplacementTokensEst - reasonOnlyTokensEst
		if netSaved < 0 {
			netSaved = 0
		}

		fingerprint := summary.Fingerprint
		if fingerprint == entry.LastReplacementFingerprint && entry.LastReplacementFingerprint != "" {
			entry.RepeatReplacementCount++
		} else {
			entry.RepeatReplacementCount = 1
		}
		entry.LastReplacementFingerprint = fingerprint
		entry.LastReplacementType = summary.ReplacementType
		entry.LastStructureReason = summary.Reason
		entry.LastStructureConfidence = summary.Confidence

		_ = store.UpsertFileEntry(filePath, entry)

		if entry.RepeatReplacementCount == 1 {
			// First occurrence: full structure message as additionalContext.
			msg := buildStructureMessage(filePath, summary, netSaved)
			if len(msg) > maxAdditionalContextChars {
				msg = msg[:maxAdditionalContextChars]
			}
			hookio.EmitPreToolResponse("deny", "", msg)
		} else {
			// Repeat: reason-only.
			hookio.EmitPreToolResponse("deny", buildReasonOnlyMessage(filePath), "")
		}
		return nil
	}

	// Not eligible.
	if mode == "soft_block" {
		_ = store.UpsertFileEntry(filePath, entry)
		hookio.EmitSilent()
	} else {
		// block mode: deny without structure.
		_ = store.UpsertFileEntry(filePath, entry)
		hookio.EmitPreToolResponse("deny", buildReasonOnlyMessage(filePath), "")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Mode 2: --clear flag
// ---------------------------------------------------------------------------

func runReadCacheClear() error {
	hookInput, _ := hookio.ReadStdinJSON(hookio.MaxBytes)
	sessionID := sessionIDFrom(hookInput)

	snapshotDir := pluginenv.ResolveSnapshotDir()
	store, err := sessionstore.Open(sessionID, snapshotDir)
	if err != nil {
		hookio.EmitSilent()
		return nil
	}
	defer store.Close()

	_ = store.ClearFileEntries()
	_, _ = sessionstore.CleanupOldStores(snapshotDir, 48)
	hookio.EmitSilent()
	return nil
}

// ---------------------------------------------------------------------------
// Mode 3: --invalidate flag
// ---------------------------------------------------------------------------

var invalidateToolNames = map[string]bool{
	"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true,
}

func runReadCacheInvalidate() error {
	hookInput, _ := hookio.ReadStdinJSON(hookio.MaxBytes)

	toolName, _ := hookInput["tool_name"].(string)
	if !invalidateToolNames[toolName] {
		hookio.EmitSilent()
		return nil
	}

	toolInput, _ := hookInput["tool_input"].(map[string]any)
	if toolInput == nil {
		hookio.EmitSilent()
		return nil
	}

	rawPath, _ := toolInput["file_path"].(string)
	if rawPath == "" {
		hookio.EmitSilent()
		return nil
	}

	filePath, err := filepath.Abs(rawPath)
	if err != nil {
		hookio.EmitSilent()
		return nil
	}

	sessionID := sessionIDFrom(hookInput)
	snapshotDir := pluginenv.ResolveSnapshotDir()

	store, err := sessionstore.Open(sessionID, snapshotDir)
	if err != nil {
		hookio.EmitSilent()
		return nil
	}
	defer store.Close()

	_ = store.DeleteFileEntry(filePath)
	_ = store.DeleteCachedContent(filePath)
	hookio.EmitSilent()
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sessionIDFrom(hookInput map[string]any) string {
	if v, ok := hookInput["agent_id"].(string); ok && v != "" {
		return v
	}
	if v, ok := hookInput["session_id"].(string); ok && v != "" {
		return v
	}
	return "unknown"
}

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

func parseRanges(s string) [][2]int {
	if s == "" || s == "[]" {
		return nil
	}
	var raw [][]int
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil
	}
	result := make([][2]int, 0, len(raw))
	for _, r := range raw {
		if len(r) == 2 {
			result = append(result, [2]int{r[0], r[1]})
		}
	}
	return result
}

func encodeRanges(ranges [][2]int) string {
	if len(ranges) == 0 {
		return "[]"
	}
	b, err := json.Marshal(ranges)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// isRangeCovered returns true if [offset, limit] is already covered by ranges.
// [0,0] means "entire file" and covers everything.
func isRangeCovered(ranges [][2]int, offset, limit int) bool {
	for _, r := range ranges {
		// [0,0] covers everything.
		if r[0] == 0 && r[1] == 0 {
			return true
		}
		// Exact match.
		if r[0] == offset && r[1] == limit {
			return true
		}
		// The requested range [0,0] (whole file) is only covered if stored [0,0].
		// For ranged reads: check if stored range subsumes the request.
		if limit > 0 && r[1] > 0 {
			reqEnd := offset + limit
			storedEnd := r[0] + r[1]
			if offset >= r[0] && reqEnd <= storedEnd {
				return true
			}
		}
	}
	return false
}

func appendRange(ranges [][2]int, r [2]int) [][2]int {
	ranges = append(ranges, r)
	if len(ranges) > maxRangesSeen {
		ranges = ranges[len(ranges)-maxRangesSeen:]
	}
	return ranges
}

func buildStructureMessage(filePath string, repl structuremap.StructureMapResult, netSaved int) string {
	base := filepath.Base(filePath)
	return base + " is unchanged (already read this session).\n" +
		"Using " + repl.ReplacementType + " view (~" + strconv.Itoa(netSaved) + " tokens saved).\n" +
		"You can still Edit this file directly, or Read a specific range (offset/limit) for full content.\n\n" +
		repl.ReplacementText
}

func buildReasonOnlyMessage(filePath string) string {
	base := filepath.Base(filePath)
	return "[Token Optimizer] " + base + " is unchanged, already in context, and was already " +
		"summarized in this session. You can still Edit this file or Read a specific range."
}

func resetReplacementState(e *sessionstore.FileEntry) {
	e.LastReplacementFingerprint = ""
	e.LastReplacementType = ""
	e.RepeatReplacementCount = 0
	e.LastStructureReason = ""
	e.LastStructureConfidence = 0.0
}

func unixNowFloat() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}
