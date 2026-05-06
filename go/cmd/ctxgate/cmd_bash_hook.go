package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newBashHookCmd())
}

func newBashHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bash-hook",
		Short: "PreToolUse[Bash] hook — whitelist check and output rewrite",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
}
