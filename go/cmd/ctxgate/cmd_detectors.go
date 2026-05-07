package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agusrdz/ctxgate/internal/detectors"
	"github.com/agusrdz/ctxgate/internal/pluginenv"
	"github.com/agusrdz/ctxgate/internal/sessionstore"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newDetectorsCmd())
}

func newDetectorsCmd() *cobra.Command {
	var sessionID string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "detectors",
		Short: "Run waste detectors against the current session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDetectors(sessionID, asJSON)
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "session ID to analyse (default: from env)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit findings as JSON")
	return cmd
}

func runDetectors(sessionID string, asJSON bool) error {
	if sessionID == "" {
		sessionID = os.Getenv("CLAUDE_SESSION_ID")
	}

	snapshotDir := pluginenv.ResolveSnapshotDir()

	data := detectors.SessionData{SessionID: sessionID}

	// Load from session store if we have a session ID.
	if sessionID != "" {
		store, err := sessionstore.Open(sessionID, snapshotDir)
		if err == nil {
			defer store.Close()

			// Activity log.
			actRows, _ := store.GetActivityLog(50)
			for _, r := range actRows {
				data.ActivityLog = append(data.ActivityLog, detectors.ActivityEntry{
					ToolName:   r.ToolName,
					ToolBucket: r.ToolBucket,
					HasError:   r.HasError,
					Timestamp:  r.Timestamp,
				})
			}

			// File reads.
			fileRows, _ := store.GetAllFileEntries()
			for _, e := range fileRows {
				data.FileReads = append(data.FileReads, detectors.FileRead{
					FilePath: e.FilePath,
					Tokens:   e.TokensEst,
				})
			}

			// Tool outputs.
			outRows, _ := store.GetRecentToolOutputs(20)
			for _, o := range outRows {
				data.ToolOutputs = append(data.ToolOutputs, detectors.ToolOutputItem{
					ToolName:        o.ToolName,
					OutputTokensEst: o.OutputTokensEst,
					CommandOrPath:   o.CommandOrPath,
				})
			}
		}
	}

	// Load CLAUDE.md for cache_instability detector.
	if raw, err := os.ReadFile(filepath.Join(".", "CLAUDE.md")); err == nil {
		data.ClaudeMDContent = string(raw)
	}

	findings := detectors.RunAll(data)
	triaged := detectors.Triage(findings)
	if len(triaged) == 0 {
		triaged = findings
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		type jsonFinding struct {
			Name          string  `json:"name"`
			Confidence    float64 `json:"confidence"`
			SavingsTokens int     `json:"savings_tokens"`
			Evidence      string  `json:"evidence"`
			Suggestion    string  `json:"suggestion"`
		}
		var out []jsonFinding
		for _, f := range triaged {
			out = append(out, jsonFinding{f.Name, f.Confidence, f.SavingsTokens, f.Evidence, f.Suggestion})
		}
		return enc.Encode(out)
	}

	if len(triaged) == 0 {
		fmt.Println("[ctxgate] No significant waste patterns detected.")
		return nil
	}
	for _, f := range triaged {
		fmt.Printf("[%s] %.0f%% confidence, ~%d tokens\n  %s\n  -> %s\n\n",
			f.Name, f.Confidence*100, f.SavingsTokens, f.Evidence, f.Suggestion)
	}
	return nil
}
