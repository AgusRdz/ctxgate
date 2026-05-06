package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newOutlineCmd())
}

// Phase 4
func newOutlineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "outline <file>",
		Short: "Print structural outline of a source file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
}
