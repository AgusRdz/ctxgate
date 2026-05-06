package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var quiet bool

var rootCmd = &cobra.Command{
	Use:           "ctxgate",
	Short:         "Claude Code hook runtime",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func main() {
	rootCmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "suppress non-essential output")

	rootCmd.AddCommand(newVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}
}
