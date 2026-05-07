package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/agusrdz/ctxgate/internal/structuremap"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newOutlineCmd())
}

func newOutlineCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "outline <file>",
		Short: "Print a bounded structure-map outline for a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOutline(args[0], asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the full structure-map payload as JSON")
	return cmd
}

func runOutline(filePath string, asJSON bool) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("[ctxgate] file not found: %s", filePath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("[ctxgate] not a file: %s", filePath)
	}

	result := structuremap.SummarizeCodeFile(filePath)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"file_path":              result.FilePath,
			"language":               result.Language,
			"replacement_type":       result.ReplacementType,
			"replacement_text":       result.ReplacementText,
			"reason":                 result.Reason,
			"confidence":             result.Confidence,
			"eligible":               result.Eligible,
			"replacement_tokens_est": result.ReplacementTokensEst,
			"line_count":             result.LineCount,
		})
	}

	fmt.Print(result.ReplacementText)
	return nil
}
