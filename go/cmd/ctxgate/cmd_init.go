package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agusrdz/ctxgate/internal/runtimeenv"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newStatusCmd())
}

// hookEntry is one command in a hook group.
type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// hookGroup is one matcher+hooks block inside a settings.json event array.
type hookGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

// ctxgateHookDefs returns the full hook map keyed by event name, using exe as the
// binary path (forward-slash, quoted in the command string).
func ctxgateHookDefs(exe string) map[string][]hookGroup {
	q := func(sub string) string {
		return fmt.Sprintf(`"%s" %s`, exe, sub)
	}
	return map[string][]hookGroup{
		"PreToolUse": {
			{Matcher: "Read", Hooks: []hookEntry{{Type: "command", Command: q("read-cache --quiet")}}},
			{Matcher: "Bash", Hooks: []hookEntry{{Type: "command", Command: q("bash-hook --quiet")}}},
			{Matcher: "Agent|Task", Hooks: []hookEntry{{Type: "command", Command: q("measure checkpoint-trigger --milestone pre-fanout --quiet")}}},
		},
		"PreCompact": {
			{Hooks: []hookEntry{{Type: "command", Command: q("measure dynamic-compact-instructions --quiet")}}},
			{Hooks: []hookEntry{{Type: "command", Command: q("measure compact-capture --trigger auto --quiet")}}},
			{Hooks: []hookEntry{{Type: "command", Command: q("read-cache --clear --quiet")}}},
		},
		"SessionStart": {
			{Hooks: []hookEntry{{Type: "command", Command: q("measure ensure-health"), Async: true, Timeout: 15000}}},
			{Hooks: []hookEntry{{Type: "command", Command: q("measure quality-cache --force --quiet")}}},
			{Matcher: "compact", Hooks: []hookEntry{{Type: "command", Command: q("measure compact-restore")}}},
			{Hooks: []hookEntry{{Type: "command", Command: q("measure compact-restore --new-session-only")}}},
		},
		"Stop": {
			{Hooks: []hookEntry{{Type: "command", Command: q("measure compact-capture --trigger stop --quiet")}}},
		},
		"SessionEnd": {
			{Hooks: []hookEntry{{Type: "command", Command: q("measure session-end-flush"), Async: true, Timeout: 60000}}},
		},
		"StopFailure": {
			{Hooks: []hookEntry{{Type: "command", Command: q("measure compact-capture --trigger stop-failure --quiet")}}},
		},
		"UserPromptSubmit": {
			{Hooks: []hookEntry{{Type: "command", Command: q("measure quality-cache --warn --quiet")}}},
		},
		"PostToolUse": {
			{Matcher: "Bash|Read|Glob|Grep|Agent|mcp__.*", Hooks: []hookEntry{{Type: "command", Command: q("archive-result --quiet")}}},
			{Matcher: "Bash|Read|Grep|Glob|mcp__.*", Hooks: []hookEntry{{Type: "command", Command: q("context-intel --quiet")}}},
			{Matcher: "Edit|Write|MultiEdit|NotebookEdit", Hooks: []hookEntry{{Type: "command", Command: q("read-cache --invalidate --quiet")}}},
		},
		"PostCompact": {
			{Hooks: []hookEntry{{Type: "command", Command: q("measure quality-cache --force --quiet")}}},
		},
		"CwdChanged": {
			{Hooks: []hookEntry{{Type: "command", Command: q("read-cache --clear --quiet")}}},
		},
	}
}

func newInitCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Wire ctxgate hooks into Claude Code settings.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	return cmd
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show ctxgate hook wiring status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitStatus()
		},
	}
}

func claudeSettingsPath() string {
	return filepath.Join(runtimeenv.ClaudeHome(), "settings.json")
}

func resolvedExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Resolve symlinks so we get the real installed path.
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	// Claude Code runs on Git Bash on Windows — use forward slashes.
	if runtime.GOOS == "windows" {
		exe = strings.ReplaceAll(exe, `\`, `/`)
	}
	return exe, nil
}

func runInit(dryRun bool) error {
	exe, err := resolvedExePath()
	if err != nil {
		return fmt.Errorf("resolve binary: %w", err)
	}

	settingsPath := claudeSettingsPath()
	settings, err := loadSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	defs := ctxgateHookDefs(exe)
	added := mergeHooks(settings, defs)

	if dryRun {
		fmt.Printf("[ctxgate] dry-run — binary: %s\n", exe)
		fmt.Printf("[ctxgate] settings: %s\n", settingsPath)
		if len(added) == 0 {
			fmt.Println("[ctxgate] all hooks already present — nothing to add")
		} else {
			for _, ev := range added {
				fmt.Printf("[ctxgate]   + %s\n", ev)
			}
		}
		return nil
	}

	if len(added) == 0 {
		fmt.Println("[ctxgate] all hooks already present")
		return nil
	}

	if err := writeSettings(settingsPath, settings); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	fmt.Printf("[ctxgate] hooks wired in %s\n", settingsPath)
	for _, ev := range added {
		fmt.Printf("[ctxgate]   + %s\n", ev)
	}
	fmt.Println("[ctxgate] restart Claude Code for changes to take effect")
	return nil
}

func runInitStatus() error {
	exe, err := resolvedExePath()
	if err != nil {
		return fmt.Errorf("resolve binary: %w", err)
	}

	settingsPath := claudeSettingsPath()
	settings, err := loadSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	defs := ctxgateHookDefs(exe)

	fmt.Printf("[ctxgate] binary:   %s\n", exe)
	fmt.Printf("[ctxgate] settings: %s\n", settingsPath)
	fmt.Println()

	allPresent := true
	for event, groups := range defs {
		existing := existingCommands(settings, event)
		for _, g := range groups {
			for _, h := range g.Hooks {
				if commandPresent(existing, h.Command) {
					fmt.Printf("  [ok] %s — %s\n", event, shortCmd(h.Command))
				} else {
					fmt.Printf("  [--] %s — %s\n", event, shortCmd(h.Command))
					allPresent = false
				}
			}
		}
	}

	fmt.Println()
	if allPresent {
		fmt.Println("[ctxgate] all hooks wired")
	} else {
		fmt.Println("[ctxgate] run 'ctxgate init' to wire missing hooks")
	}
	return nil
}

// loadSettings reads settings.json as a generic map, creating it if missing.
func loadSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func writeSettings(path string, settings map[string]any) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".ctxgate.tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// mergeHooks adds any missing ctxgate hook groups to settings["hooks"].
// Returns a list of "event — command" strings for the added entries.
func mergeHooks(settings map[string]any, defs map[string][]hookGroup) []string {
	hooksAny, _ := settings["hooks"]
	hooksMap, _ := hooksAny.(map[string]any)
	if hooksMap == nil {
		hooksMap = map[string]any{}
	}

	var added []string

	for event, groups := range defs {
		existing := existingCommands(hooksMap, event)

		for _, g := range groups {
			for _, h := range g.Hooks {
				if commandPresent(existing, h.Command) {
					continue
				}
				// Build a new group map to append.
				newGroup := map[string]any{
					"hooks": []any{hookEntryToMap(h)},
				}
				if g.Matcher != "" {
					newGroup["matcher"] = g.Matcher
				}

				eventAny, _ := hooksMap[event]
				eventSlice, _ := eventAny.([]any)
				hooksMap[event] = append(eventSlice, newGroup)
				existing = append(existing, h.Command)
				added = append(added, fmt.Sprintf("%s — %s", event, shortCmd(h.Command)))
			}
		}
	}

	settings["hooks"] = hooksMap
	return added
}

func hookEntryToMap(h hookEntry) map[string]any {
	m := map[string]any{
		"type":    h.Type,
		"command": h.Command,
	}
	if h.Async {
		m["async"] = true
	}
	if h.Timeout > 0 {
		m["timeout"] = h.Timeout
	}
	return m
}

// existingCommands returns all command strings registered for an event.
func existingCommands(hooksMap map[string]any, event string) []string {
	eventAny, _ := hooksMap[event]
	eventSlice, _ := eventAny.([]any)
	var cmds []string
	for _, groupAny := range eventSlice {
		group, _ := groupAny.(map[string]any)
		if group == nil {
			continue
		}
		hooksAny, _ := group["hooks"]
		hooks, _ := hooksAny.([]any)
		for _, hAny := range hooks {
			h, _ := hAny.(map[string]any)
			if cmd, _ := h["command"].(string); cmd != "" {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

func commandPresent(existing []string, cmd string) bool {
	for _, c := range existing {
		if c == cmd {
			return true
		}
	}
	return false
}

func shortCmd(cmd string) string {
	// Strip the quoted exe path, show only the subcommand.
	if i := strings.Index(cmd, `" `); i != -1 {
		return cmd[i+2:]
	}
	return cmd
}
