package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agusrdz/ctxgate/internal/pluginenv"
	"github.com/agusrdz/ctxgate/internal/trends"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newReportCmd())
}

func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Inspect session data, trends, and checkpoints",
	}
	cmd.AddCommand(newReportHealthCmd())
	cmd.AddCommand(newReportTrendsCmd())
	cmd.AddCommand(newReportSavingsCmd())
	cmd.AddCommand(newReportCompressionStatsCmd())
	cmd.AddCommand(newReportListCheckpointsCmd())
	cmd.AddCommand(newReportCheckpointStatsCmd())
	return cmd
}

// ── helpers ────────────────────────────────────────────────────────────────

// commas formats n with thousand separators (e.g. 1234567 → "1,234,567").
func commas(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		s = s[1:]
	}
	var b strings.Builder
	offset := len(s) % 3
	for i, c := range s {
		if i != 0 && (i-offset)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	result := b.String()
	if n < 0 {
		return "-" + result
	}
	return result
}

// humanAge returns a short human-readable age string relative to now.
func humanAge(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// openReader opens trends.Reader or returns nil if db doesn't exist.
func openReader(snapshotDir string) (*trends.Reader, error) {
	dbPath := filepath.Join(snapshotDir, "trends.db")
	return trends.OpenReader(dbPath)
}

// listSessionDBs returns count of *.db files in snapshotDir excluding trends.db.
func listSessionDBs(snapshotDir string) int {
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		if e.Name() == "trends.db" {
			continue
		}
		count++
	}
	return count
}

// checkpointDir returns the path to the checkpoints directory.
func checkpointDir(snapshotDir string) string {
	return filepath.Join(snapshotDir, "checkpoints")
}

// listCheckpointFiles returns os.FileInfo for .md files in checkpoints dir, sorted newest first.
func listCheckpointFiles(cpDir string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(cpDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var infos []os.FileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ModTime().After(infos[j].ModTime())
	})
	return infos, nil
}

// parseTrigger extracts the trigger type from a checkpoint filename.
// Format: <sid>-<YYYYMMDD-HHMMSS>[-trigger].md
// Returns "auto" if trigger segment is unrecognized.
func parseTrigger(name string) string {
	// Strip .md
	base := strings.TrimSuffix(name, ".md")
	parts := strings.Split(base, "-")
	if len(parts) < 1 {
		return "auto"
	}
	last := parts[len(parts)-1]
	switch last {
	case "stop", "auto":
		return last
	case "failure":
		// Could be last segment of "stop-failure"
		if len(parts) >= 2 && parts[len(parts)-2] == "stop" {
			return "stop-failure"
		}
	}
	return "auto"
}

// ── health ─────────────────────────────────────────────────────────────────

func newReportHealthCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Show system health check",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReportHealth(asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit as JSON")
	return cmd
}

func runReportHealth(asJSON bool) error {
	snapshotDir := pluginenv.ResolveSnapshotDir()

	// Snapshot dir status.
	snapshotOK := false
	if _, err := os.Stat(snapshotDir); err == nil {
		snapshotOK = true
	}

	// Session DBs.
	sessionDBs := listSessionDBs(snapshotDir)

	// Checkpoints.
	cpDir := checkpointDir(snapshotDir)
	cpFiles, _ := listCheckpointFiles(cpDir)
	cpCount := len(cpFiles)
	var cpNewest time.Time
	if cpCount > 0 {
		cpNewest = cpFiles[0].ModTime()
	}

	// Trends DB.
	r, _ := openReader(snapshotDir)
	eventCount := 0
	trendsOK := false
	if r != nil {
		defer r.Close()
		eventCount = r.GetEventCount()
		trendsOK = true
	}

	// Binary path.
	binaryPath, _ := os.Executable()

	if asJSON {
		out := map[string]any{
			"snapshot_dir":   snapshotDir,
			"snapshot_ok":    snapshotOK,
			"session_dbs":    sessionDBs,
			"checkpoints":    cpCount,
			"trends_events":  eventCount,
			"trends_ok":      trendsOK,
			"binary":         binaryPath,
		}
		if !cpNewest.IsZero() {
			out["checkpoint_newest"] = cpNewest.UTC().Format(time.RFC3339)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	snapStatus := "[OK]"
	if !snapshotOK {
		snapStatus = "[MISSING]"
	}
	trendsStatus := "[OK]"
	if !trendsOK {
		trendsStatus = "[NO DATA]"
	}

	fmt.Println("  ctxgate health")
	fmt.Println("  ===========================")
	fmt.Printf("  Snapshot dir:   %-40s %s\n", snapshotDir, snapStatus)
	fmt.Printf("  Session DBs:    %s active\n", commas(sessionDBs))

	if cpCount > 0 && !cpNewest.IsZero() {
		fmt.Printf("  Checkpoints:    %s (newest: %s)\n", commas(cpCount), humanAge(cpNewest))
	} else {
		fmt.Printf("  Checkpoints:    %s\n", commas(cpCount))
	}

	fmt.Printf("  Trends DB:    %s events   %s\n", commas(eventCount), trendsStatus)
	fmt.Printf("  Binary:         %s\n", binaryPath)
	return nil
}

// ── trends ─────────────────────────────────────────────────────────────────

func newReportTrendsCmd() *cobra.Command {
	var days int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "trends",
		Short: "Show session trends from session_log",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReportTrends(days, asJSON)
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "number of days to include")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit as JSON")
	return cmd
}

func runReportTrends(days int, asJSON bool) error {
	snapshotDir := pluginenv.ResolveSnapshotDir()
	r, err := openReader(snapshotDir)
	if err != nil {
		return err
	}
	if r == nil {
		if asJSON {
			fmt.Println(`{"error":"no data yet"}`)
			return nil
		}
		fmt.Println("  No trends data yet.")
		return nil
	}
	defer r.Close()

	sessions, toolCalls, totalTokens, savedTokens, err := r.GetTotals(days)
	if err != nil {
		return err
	}
	daily, err := r.GetSessionTrends(days)
	if err != nil {
		return err
	}

	if asJSON {
		out := map[string]any{
			"days":         days,
			"sessions":     sessions,
			"tool_calls":   toolCalls,
			"total_tokens": totalTokens,
			"saved_tokens": savedTokens,
			"daily":        daily,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	pct := 0.0
	if totalTokens > 0 {
		pct = float64(savedTokens) / float64(totalTokens) * 100
	}

	fmt.Printf("  Session Trends  (%dd)\n", days)
	fmt.Println("  ===========================")
	fmt.Printf("  Sessions logged:   %s\n", commas(sessions))
	fmt.Printf("  Tool calls:     %s\n", commas(toolCalls))
	fmt.Printf("  Total tokens:   %s\n", commas(totalTokens))
	fmt.Printf("  Saved tokens:   %s  (%.1f%%)\n", commas(savedTokens), pct)

	if len(daily) > 0 {
		fmt.Println()
		fmt.Println("  Daily breakdown (most recent first):")
		for _, d := range daily {
			fmt.Printf("    %-12s  %s sessions   %s calls   %s tok   %s saved\n",
				d.Date,
				commas(d.Sessions),
				commas(d.ToolCalls),
				commas(d.TotalTokens),
				commas(d.SavedTokens),
			)
		}
	}
	return nil
}

// ── savings ────────────────────────────────────────────────────────────────

func newReportSavingsCmd() *cobra.Command {
	var days int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "savings",
		Short: "Show savings summary from savings_events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReportSavings(days, asJSON)
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "number of days to include")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit as JSON")
	return cmd
}

func runReportSavings(days int, asJSON bool) error {
	snapshotDir := pluginenv.ResolveSnapshotDir()
	r, err := openReader(snapshotDir)
	if err != nil {
		return err
	}
	if r == nil {
		if asJSON {
			fmt.Println(`{"error":"no data yet"}`)
			return nil
		}
		fmt.Println("  No savings data yet.")
		return nil
	}
	defer r.Close()

	summary, err := r.GetSavingsSummary(days)
	if err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"days":           days,
			"total_events":   summary.TotalEvents,
			"total_tokens":   summary.TotalTokens,
			"total_cost_usd": summary.TotalCostUSD,
			"by_type":        summary.ByType,
		})
	}

	fmt.Printf("  Savings Report  (%dd)\n", days)
	fmt.Println("  ===========================")
	fmt.Printf("  Total events:   %s\n", commas(summary.TotalEvents))
	fmt.Printf("  Tokens saved:   %s\n", commas(summary.TotalTokens))
	fmt.Printf("  Est. cost:      $%.2f\n", summary.TotalCostUSD)

	if len(summary.ByType) > 0 {
		fmt.Println()
		fmt.Println("  By source:")
		// Sort by tokens saved descending.
		type kv struct {
			key string
			ts  trends.TypeStat
		}
		var sorted []kv
		for k, v := range summary.ByType {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].ts.TokensSaved > sorted[j].ts.TokensSaved
		})
		for _, item := range sorted {
			fmt.Printf("    %-24s  %s events   %s tokens\n",
				item.key, commas(item.ts.Events), commas(item.ts.TokensSaved))
		}
	}
	return nil
}

// ── compression-stats ──────────────────────────────────────────────────────

func newReportCompressionStatsCmd() *cobra.Command {
	var days int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "compression-stats",
		Short: "Show compression breakdown by event type",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReportCompressionStats(days, asJSON)
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "number of days to include")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit as JSON")
	return cmd
}

func runReportCompressionStats(days int, asJSON bool) error {
	snapshotDir := pluginenv.ResolveSnapshotDir()
	r, err := openReader(snapshotDir)
	if err != nil {
		return err
	}
	if r == nil {
		if asJSON {
			fmt.Println(`{"error":"no data yet"}`)
			return nil
		}
		fmt.Println("  No compression data yet.")
		return nil
	}
	defer r.Close()

	summary, err := r.GetSavingsSummary(days)
	if err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"days":         days,
			"total_events": summary.TotalEvents,
			"total_tokens": summary.TotalTokens,
			"by_type":      summary.ByType,
		})
	}

	fmt.Printf("  Compression Stats  (%dd)\n", days)
	fmt.Println("  ===========================")
	fmt.Printf("  Total events:   %s\n", commas(summary.TotalEvents))
	fmt.Printf("  Tokens saved:   %s\n", commas(summary.TotalTokens))

	if len(summary.ByType) > 0 {
		fmt.Println()
		fmt.Println("  By type:")
		type kv struct {
			key string
			ts  trends.TypeStat
		}
		var sorted []kv
		for k, v := range summary.ByType {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].ts.TokensSaved > sorted[j].ts.TokensSaved
		})
		for _, item := range sorted {
			fmt.Printf("    %-24s  %s events   %s tokens saved\n",
				item.key, commas(item.ts.Events), commas(item.ts.TokensSaved))
		}
	}
	return nil
}

// ── list-checkpoints ───────────────────────────────────────────────────────

func newReportListCheckpointsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list-checkpoints",
		Short: "List checkpoint .md files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReportListCheckpoints(asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit as JSON")
	return cmd
}

func runReportListCheckpoints(asJSON bool) error {
	snapshotDir := pluginenv.ResolveSnapshotDir()
	cpDir := checkpointDir(snapshotDir)
	files, err := listCheckpointFiles(cpDir)
	if err != nil {
		return err
	}

	if asJSON {
		type cpEntry struct {
			Name    string `json:"name"`
			ModTime string `json:"mod_time"`
			Age     string `json:"age"`
		}
		var out []cpEntry
		for _, f := range files {
			out = append(out, cpEntry{
				Name:    f.Name(),
				ModTime: f.ModTime().UTC().Format(time.RFC3339),
				Age:     humanAge(f.ModTime()),
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("  Checkpoints  (%d found)\n", len(files))
	fmt.Println("  ===========================")
	if len(files) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for _, f := range files {
		fmt.Printf("    %-48s  %s\n", f.Name(), humanAge(f.ModTime()))
	}
	return nil
}

// ── checkpoint-stats ───────────────────────────────────────────────────────

func newReportCheckpointStatsCmd() *cobra.Command {
	var days int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "checkpoint-stats",
		Short: "Show checkpoint counts by trigger type",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReportCheckpointStats(days, asJSON)
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "number of days to include")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit as JSON")
	return cmd
}

func runReportCheckpointStats(days int, asJSON bool) error {
	snapshotDir := pluginenv.ResolveSnapshotDir()
	cpDir := checkpointDir(snapshotDir)
	files, err := listCheckpointFiles(cpDir)
	if err != nil {
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	byTrigger := make(map[string]int)
	total := 0
	var newest, oldest time.Time

	for _, f := range files {
		mt := f.ModTime()
		if mt.Before(cutoff) {
			continue
		}
		trigger := parseTrigger(f.Name())
		byTrigger[trigger]++
		total++
		if newest.IsZero() || mt.After(newest) {
			newest = mt
		}
		if oldest.IsZero() || mt.Before(oldest) {
			oldest = mt
		}
	}

	if asJSON {
		out := map[string]any{
			"days":       days,
			"total":      total,
			"by_trigger": byTrigger,
		}
		if !newest.IsZero() {
			out["newest"] = newest.UTC().Format(time.RFC3339)
			out["oldest"] = oldest.UTC().Format(time.RFC3339)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("  Checkpoint Stats  (%dd)\n", days)
	fmt.Println("  ===========================")
	fmt.Printf("  Total checkpoints:  %s\n", commas(total))

	if len(byTrigger) > 0 {
		fmt.Println("  By trigger:")
		// Sort by count descending.
		type kv struct {
			key   string
			count int
		}
		var sorted []kv
		for k, v := range byTrigger {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].count != sorted[j].count {
				return sorted[i].count > sorted[j].count
			}
			return sorted[i].key < sorted[j].key
		})
		for _, item := range sorted {
			fmt.Printf("    %-16s  %s\n", item.key, commas(item.count))
		}
	}

	if !newest.IsZero() {
		fmt.Printf("  Newest:  %s\n", humanAge(newest))
		fmt.Printf("  Oldest:   %s\n", humanAge(oldest))
	}
	return nil
}
