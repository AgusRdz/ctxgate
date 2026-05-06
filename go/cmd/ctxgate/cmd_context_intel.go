package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newContextIntelCmd())
}

func newContextIntelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context-intel",
		Short: "PostToolUse[Bash|Read|Grep|Glob|mcp__.*] hook — emit context intelligence events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
}
