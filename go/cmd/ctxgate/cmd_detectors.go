package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newDetectorsCmd())
}

// Phase 4
func newDetectorsCmd() *cobra.Command {
	var sessionID string

	cmd := &cobra.Command{
		Use:   "detectors",
		Short: "Run waste detectors against a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "session ID to analyze")

	return cmd
}
