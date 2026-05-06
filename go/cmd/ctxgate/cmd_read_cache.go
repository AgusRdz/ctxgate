package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newReadCacheCmd())
}

func newReadCacheCmd() *cobra.Command {
	var clear, invalidate bool

	cmd := &cobra.Command{
		Use:   "read-cache",
		Short: "PreToolUse[Read] hook — deduplicate and compress file reads",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}

	cmd.Flags().BoolVar(&clear, "clear", false, "clear file read cache (PreCompact / CwdChanged)")
	cmd.Flags().BoolVar(&invalidate, "invalidate", false, "invalidate stale entries (PostToolUse[Edit|Write|...])")

	return cmd
}
