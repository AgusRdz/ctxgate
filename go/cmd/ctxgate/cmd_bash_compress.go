package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/agusrdz/ctxgate/internal/bashcompress"
	"github.com/spf13/cobra"
)

const maxCompressBytes = 5 * 1024 * 1024

func init() {
	rootCmd.AddCommand(newBashCompressCmd())
}

func newBashCompressCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bash-compress <cmd...>",
		Short: "Compress bash command output (invoked by bash-hook rewrite chain)",
		Args:  cobra.MinimumNArgs(1),
		// Run (not RunE) so we can call os.Exit to propagate the subprocess exit code.
		Run: func(cmd *cobra.Command, args []string) {
			runBashCompress(args)
		},
	}
}

func runBashCompress(args []string) {
	commandStr := strings.Join(args, " ")

	proc := exec.Command(args[0], args[1:]...) //nolint:gosec
	var stdout, stderr bytes.Buffer
	proc.Stdout = &stdout
	proc.Stderr = &stderr

	runErr := proc.Run()
	exitCode := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			fmt.Fprintf(os.Stdout, "[bash-compress: failed to run %q: %v]\n", args[0], runErr)
			os.Exit(1)
		}
	}

	rawOutput := stdout.String() + stderr.String()

	// 5MB bypass: never compress enormous outputs.
	if len(rawOutput) > maxCompressBytes {
		fmt.Print(rawOutput)
		os.Exit(exitCode)
	}

	compressed := bashcompress.Compress(commandStr, rawOutput, exitCode, stderr.String())

	fmt.Print(compressed)
	os.Exit(exitCode)
}
