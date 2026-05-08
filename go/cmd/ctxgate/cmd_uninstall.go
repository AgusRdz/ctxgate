package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newUninstallCmd())
}

func newUninstallCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove ctxgate hooks, PATH entry, and binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstall(dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	return cmd
}

func runUninstall(dryRun bool) error {
	exe, err := resolvedExePath()
	if err != nil {
		return fmt.Errorf("resolve binary: %w", err)
	}

	settingsPath := claudeSettingsPath()
	settings, err := loadSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	removed := uninstallRemoveHooks(settings, exe)

	if dryRun {
		fmt.Printf("[ctxgate] dry-run — binary:   %s\n", exe)
		fmt.Printf("[ctxgate] dry-run — settings: %s\n", settingsPath)
		if len(removed) == 0 {
			fmt.Println("[ctxgate] no ctxgate hooks found")
		} else {
			for _, h := range removed {
				fmt.Printf("[ctxgate]   - %s\n", h)
			}
		}
		fmt.Printf("[ctxgate] would remove PATH entry for: %s\n", filepath.Dir(filepath.FromSlash(exe)))
		fmt.Printf("[ctxgate] would remove binary: %s\n", exe)
		return nil
	}

	// 1. Remove hooks from settings.json
	if len(removed) > 0 {
		if err := writeSettings(settingsPath, settings); err != nil {
			return fmt.Errorf("write settings: %w", err)
		}
		fmt.Printf("[ctxgate] removed %d hook(s) from %s\n", len(removed), settingsPath)
		for _, h := range removed {
			fmt.Printf("[ctxgate]   - %s\n", h)
		}
	} else {
		fmt.Println("[ctxgate] no ctxgate hooks found in settings.json")
	}

	// 2. Remove install dir from PATH
	installDir := filepath.FromSlash(filepath.Dir(exe))
	if pathErr := uninstallRemoveFromPath(installDir); pathErr != nil {
		fmt.Printf("[ctxgate] warning: could not remove PATH entry: %v\n", pathErr)
	} else {
		fmt.Printf("[ctxgate] removed %s from PATH\n", installDir)
	}

	// 3. Remove binary (and install dir if empty)
	nativeExe := filepath.FromSlash(exe)
	if binErr := os.Remove(nativeExe); binErr != nil {
		// Windows: running process cannot delete itself — schedule deletion via cmd.exe.
		if runtime.GOOS == "windows" {
			uninstallScheduleWindowsDelete(installDir)
			fmt.Printf("[ctxgate] binary removal scheduled on exit: %s\n", installDir)
		} else {
			fmt.Printf("[ctxgate] warning: could not remove binary (delete manually): %s\n", nativeExe)
		}
	} else {
		os.Remove(installDir) // best-effort — only succeeds if the dir is now empty
		fmt.Printf("[ctxgate] removed %s\n", nativeExe)
	}

	fmt.Println("[ctxgate] restart Claude Code for hook changes to take effect")
	return nil
}

// uninstallRemoveHooks strips every hook group whose commands reference exe.
// Returns "event — subcommand" strings for each removed entry.
func uninstallRemoveHooks(settings map[string]any, exe string) []string {
	hooksAny, _ := settings["hooks"]
	hooksMap, _ := hooksAny.(map[string]any)
	if hooksMap == nil {
		return nil
	}

	// ctxgateHookDefs quotes the exe path: `"<exe>" subcommand`
	prefix := `"` + exe + `"`

	var removed []string

	for event, eventAny := range hooksMap {
		eventSlice, _ := eventAny.([]any)
		var kept []any
		for _, groupAny := range eventSlice {
			group, _ := groupAny.(map[string]any)
			if group == nil {
				kept = append(kept, groupAny)
				continue
			}
			hooksListAny, _ := group["hooks"]
			hooksList, _ := hooksListAny.([]any)
			isCtxgate := false
			for _, hAny := range hooksList {
				h, _ := hAny.(map[string]any)
				if cmd, _ := h["command"].(string); strings.HasPrefix(cmd, prefix) {
					removed = append(removed, fmt.Sprintf("%s — %s", event, shortCmd(cmd)))
					isCtxgate = true
				}
			}
			if !isCtxgate {
				kept = append(kept, groupAny)
			}
		}
		if len(kept) == 0 {
			delete(hooksMap, event)
		} else {
			hooksMap[event] = kept
		}
	}

	if len(hooksMap) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooksMap
	}

	return removed
}

func uninstallRemoveFromPath(installDir string) error {
	if runtime.GOOS == "windows" {
		return uninstallRemoveFromPathWindows(installDir)
	}
	return uninstallRemoveFromPathUnix(installDir)
}

// uninstallRemoveFromPathWindows removes installDir from the User-scoped PATH via PowerShell.
func uninstallRemoveFromPathWindows(installDir string) error {
	script := fmt.Sprintf(`
$target = '%s'
$current = [System.Environment]::GetEnvironmentVariable('PATH','User')
$entries = $current -split ';' | Where-Object { $_.TrimEnd('\') -ne $target.TrimEnd('\') }
$new = $entries -join ';'
if ($new -ne $current) {
    [System.Environment]::SetEnvironmentVariable('PATH', $new, 'User')
}
`, strings.ReplaceAll(installDir, "'", "''"))

	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// uninstallRemoveFromPathUnix removes the ctxgate PATH lines from shell profiles.
func uninstallRemoveFromPathUnix(installDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	profiles := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".profile"),
	}
	var lastErr error
	for _, p := range profiles {
		if err := uninstallRemovePathLines(p, installDir); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// uninstallRemovePathLines rewrites profile, dropping lines that reference installDir
// or the preceding "# ctxgate" comment.
func uninstallRemovePathLines(profilePath, installDir string) error {
	data, err := os.ReadFile(profilePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	skip := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "# ctxgate" {
			skip = true // drop this comment and the next matching export
			continue
		}
		if skip && strings.Contains(line, installDir) {
			skip = false
			continue
		}
		skip = false
		out = append(out, line)
	}

	// Avoid a spurious trailing newline change when nothing was removed.
	if len(out) == len(lines) {
		return nil
	}

	return os.WriteFile(profilePath, []byte(strings.Join(out, "\n")), 0o644)
}

// uninstallScheduleWindowsDelete spawns a detached cmd.exe that waits for this
// process to exit, then removes the install directory.
func uninstallScheduleWindowsDelete(installDir string) {
	// The ping trick gives enough delay without requiring a sleep binary.
	bat := fmt.Sprintf(`ping -n 3 127.0.0.1 >nul && rd /s /q "%s"`, installDir)
	cmd := exec.Command("cmd", "/c", "start", "/min", "", "cmd", "/c", bat)
	_ = cmd.Start() // fire and forget
}
