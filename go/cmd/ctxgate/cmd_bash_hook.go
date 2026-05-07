package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/agusrdz/ctxgate/internal/bashcompress"
	"github.com/agusrdz/ctxgate/internal/hookio"
	"github.com/agusrdz/ctxgate/internal/pluginenv"
	"github.com/google/shlex"
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
			runBashHook()
			return nil
		},
	}
}

func runBashHook() {
	if !pluginenv.IsV5FlagEnabled("v5_bash_compress", "TOKEN_OPTIMIZER_BASH_COMPRESS", true, "") {
		hookio.EmitSilent()
		return
	}

	hookInput, err := hookio.ReadStdinJSON(hookio.MaxBytes)
	if err != nil && len(hookInput) == 0 {
		hookio.EmitSilent()
		return
	}

	toolName, _ := hookInput["tool_name"].(string)
	if toolName != "Bash" {
		hookio.EmitSilent()
		return
	}

	toolInput, _ := hookInput["tool_input"].(map[string]any)
	if toolInput == nil {
		hookio.EmitSilent()
		return
	}

	command, _ := toolInput["command"].(string)
	if command == "" {
		hookio.EmitSilent()
		return
	}

	// Categorical exclusion: shell metacharacters.
	if bashcompress.HasDangerousChars(command) {
		hookio.EmitSilent()
		return
	}

	// Whitelist check.
	if !bashcompress.IsWhitelisted(command) {
		hookio.EmitSilent()
		return
	}

	// Resolve ctxgate binary path from os.Executable (most reliable).
	exe, err := os.Executable()
	if err != nil || exe == "" {
		hookio.EmitSilent()
		return
	}

	// Tokenise and re-quote so paths with spaces survive the shell round-trip.
	tokens, err := shlex.Split(command)
	if err != nil || len(tokens) == 0 {
		hookio.EmitSilent()
		return
	}

	parts := make([]string, 0, len(tokens)+2)
	parts = append(parts, shellQuote(exe), "bash-compress")
	for _, t := range tokens {
		parts = append(parts, shellQuote(t))
	}
	rewritten := strings.Join(parts, " ")

	// Emit updatedInput to rewrite the Bash command.
	payload := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":    "PreToolUse",
			"permissionDecision": "allow",
			"updatedInput": map[string]any{
				"command": rewritten,
			},
		},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

// shellQuote returns s quoted for safe embedding in a POSIX shell command.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '/' || c == ':' || c == '@' || c == '+') {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
