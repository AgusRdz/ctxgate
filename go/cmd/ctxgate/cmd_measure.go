package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newMeasureCmd())
}

func newMeasureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "measure <action>",
		Short: "Measure hook subcommands (session lifecycle)",
	}

	cmd.AddCommand(newMeasureEnsureHealthCmd())
	cmd.AddCommand(newMeasureQualityCacheCmd())
	cmd.AddCommand(newMeasureCompactCaptureCmd())
	cmd.AddCommand(newMeasureCompactRestoreCmd())
	cmd.AddCommand(newMeasureDynamicCompactInstructionsCmd())
	cmd.AddCommand(newMeasureCheckpointTriggerCmd())
	cmd.AddCommand(newMeasureSessionEndFlushCmd())

	return cmd
}

func newMeasureEnsureHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ensure-health",
		Short: "SessionStart — verify session store health",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
}

func newMeasureQualityCacheCmd() *cobra.Command {
	var force, warn bool

	cmd := &cobra.Command{
		Use:   "quality-cache",
		Short: "SessionStart / UserPromptSubmit — run quality cache check",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
	cmd.Flags().BoolVar(&force, "force", false, "force cache rebuild")
	cmd.Flags().BoolVar(&warn, "warn", false, "warn-only mode (UserPromptSubmit)")
	return cmd
}

func newMeasureCompactCaptureCmd() *cobra.Command {
	var trigger string

	cmd := &cobra.Command{
		Use:   "compact-capture",
		Short: "Stop / StopFailure / PreCompact — capture compaction snapshot",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
	cmd.Flags().StringVar(&trigger, "trigger", "auto", "trigger source: stop, stop-failure, auto")
	return cmd
}

func newMeasureCompactRestoreCmd() *cobra.Command {
	var newSessionOnly bool

	cmd := &cobra.Command{
		Use:   "compact-restore",
		Short: "SessionStart — restore compaction snapshot",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
	cmd.Flags().BoolVar(&newSessionOnly, "new-session-only", false, "only restore for brand-new sessions")
	return cmd
}

func newMeasureDynamicCompactInstructionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dynamic-compact-instructions",
		Short: "PreCompact — emit dynamic compaction instructions",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
}

func newMeasureCheckpointTriggerCmd() *cobra.Command {
	var milestone string

	cmd := &cobra.Command{
		Use:   "checkpoint-trigger",
		Short: "PreToolUse[Agent|Task] — trigger checkpoint if milestone reached",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
	cmd.Flags().StringVar(&milestone, "milestone", "", "milestone name (e.g. pre-fanout)")
	return cmd
}

func newMeasureSessionEndFlushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "session-end-flush",
		Short: "SessionEnd — flush session data and run detectors",
		RunE:  func(cmd *cobra.Command, args []string) error { return errors.New("not implemented") },
	}
}
