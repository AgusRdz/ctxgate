package pluginenv

// ResolvePluginDataDir returns the ctxgate data directory.
// Result is cached via sync.Once — safe to call on every hook invocation.
func ResolvePluginDataDir() (string, error) {
	panic("not implemented")
}

// ResolveSnapshotDir returns the session snapshot directory.
func ResolveSnapshotDir() string {
	panic("not implemented")
}

// IsV5FlagEnabled checks a v5 feature flag from env or installed_plugins.json.
func IsV5FlagEnabled(flagName, envVar string, defaultVal bool, envTruthyValue string) bool {
	panic("not implemented")
}
