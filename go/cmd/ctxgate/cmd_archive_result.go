package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newArchiveResultCmd())
}

func newArchiveResultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archive-result",
		Short: "PostToolUse[Bash|Read|Glob|Grep|Agent|mcp__.*] hook — archive tool output",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
}
