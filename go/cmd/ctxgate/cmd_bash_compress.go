package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newBashCompressCmd())
}

func newBashCompressCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bash-compress <cmd...>",
		Short: "Compress bash command output (invoked by bash-hook rewrite chain)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
}
