package pluginenv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/agusrdz/ctxgate/internal/pathsafe"
	"github.com/agusrdz/ctxgate/internal/runtimeenv"
)

const (
	pluginName     = "ctxgate"
	maxConfigBytes = 1 * 1024 * 1024
)

// safeMktName allows only conservative chars in marketplace names to prevent
// path traversal via plugin registry keys.
var safeMktName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// pluginDataEnvVars returns env var names to check for plugin data dir, in priority order.
func pluginDataEnvVars() []string {
	if runtimeenv.DetectRuntime() == runtimeenv.RuntimeCodex {
		return []string{"CTXGATE_PLUGIN_DATA"}
	}
	return []string{"CLAUDE_PLUGIN_DATA", "CTXGATE_PLUGIN_DATA"}
}

var (
	pluginDirOnce sync.Once
	pluginDirVal  string
)

// ResolvePluginDataDir returns the ctxgate plugin-data directory.
// Cached after first call via sync.Once — safe for repeated hook invocations.
//
// Priority:
//  1. CLAUDE_PLUGIN_DATA / CTXGATE_PLUGIN_DATA env var
//  2. installed_plugins.json registry lookup for ctxgate@* entries
//  3. Glob fallback: most-recently-modified ctxgate-* dir under plugins/data/
//  4. Empty string (caller should fall back to legacy path)
func ResolvePluginDataDir() (string, error) {
	pluginDirOnce.Do(func() {
		pluginDirVal = resolvePluginDataDir()
	})
	if pluginDirVal == "" {
		return "", nil
	}
	return pluginDirVal, nil
}

func resolvePluginDataDir() string {
	runtimeHome := runtimeenv.RuntimeHome()
	pluginDataBase := filepath.Join(runtimeHome, "plugins", "data")

	// 1. Env var
	for _, envVar := range pluginDataEnvVars() {
		val := os.Getenv(envVar)
		if val == "" {
			continue
		}
		candidate := filepath.Clean(val)
		if pathsafe.IsSafeSubdir(candidate, pluginDataBase) {
			return candidate
		}
	}

	// 2. installed_plugins.json registry
	installedPlugins := filepath.Join(runtimeHome, "plugins", "installed_plugins.json")
	var candidates []string

	if registry := safeLoadJSON(installedPlugins); registry != nil {
		if plugins, ok := registry["plugins"].(map[string]any); ok {
			for key := range plugins {
				if !strings.HasPrefix(key, pluginName+"@") {
					continue
				}
				marketplace := strings.SplitN(key, "@", 2)[1]
				if !safeMktName.MatchString(marketplace) {
					continue
				}
				candidate := filepath.Join(pluginDataBase, pluginName+"-"+marketplace)
				if pathsafe.IsSafeSubdir(candidate, pluginDataBase) {
					candidates = append(candidates, candidate)
				}
			}
		}
	}

	// 3. Glob fallback
	if len(candidates) == 0 {
		entries, err := os.ReadDir(pluginDataBase)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() || !strings.HasPrefix(e.Name(), pluginName+"-") {
					continue
				}
				candidate := filepath.Join(pluginDataBase, e.Name())
				if pathsafe.IsSafeSubdir(candidate, pluginDataBase) {
					candidates = append(candidates, candidate)
				}
			}
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	// Sort by mtime descending, pick newest.
	best := candidates[0]
	bestMtime := int64(0)
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil {
			if info.ModTime().UnixNano() > bestMtime {
				bestMtime = info.ModTime().UnixNano()
				best = c
			}
		}
	}
	return best
}

// ResolveSnapshotDir returns the directory for session snapshots and caches.
func ResolveSnapshotDir() string {
	dir, _ := ResolvePluginDataDir()
	if dir != "" {
		return filepath.Join(dir, "data")
	}
	// Legacy fallback: ~/.claude/_backups/ctxgate/
	runtimeHome := runtimeenv.RuntimeHome()
	return filepath.Join(runtimeHome, "_backups", pluginName)
}

var (
	truthyEnv = map[string]bool{"1": true, "true": true, "yes": true, "on": true}
	falsyEnv  = map[string]bool{"0": true, "false": true, "no": true, "off": true, "": true}
)

// IsV5FlagEnabled checks a feature flag in priority order:
//  1. Environment variable (standard booleans, or exact match when envTruthyValue != "")
//  2. User config: <runtime-home>/ctxgate/config.json
//  3. Plugin-data config: <plugin-data>/config/config.json
//  4. defaultVal
func IsV5FlagEnabled(flagName, envVar string, defaultVal bool, envTruthyValue string) bool {
	if envVal, ok := os.LookupEnv(envVar); ok {
		if envTruthyValue != "" {
			return envVal == envTruthyValue
		}
		normalized := strings.ToLower(strings.TrimSpace(envVal))
		if truthyEnv[normalized] {
			return true
		}
		if falsyEnv[normalized] {
			return false
		}
		// Unrecognized value — fall through.
	}

	runtimeHome := runtimeenv.RuntimeHome()
	configPaths := []string{
		filepath.Join(runtimeHome, pluginName, "config.json"),
	}
	if dir, _ := ResolvePluginDataDir(); dir != "" {
		configPaths = append(configPaths, filepath.Join(dir, "config", "config.json"))
	}

	for _, path := range configPaths {
		cfg := safeLoadJSON(path)
		if cfg == nil {
			continue
		}
		if val, ok := cfg[flagName]; ok {
			switch v := val.(type) {
			case bool:
				return v
			case float64:
				return v != 0
			}
		}
	}

	return defaultVal
}

// safeLoadJSON reads and parses JSON from path with size guard.
// Returns nil on any failure — never panics.
func safeLoadJSON(path string) map[string]any {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxConfigBytes {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}
