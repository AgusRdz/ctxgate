package hookio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	StdinTimeout = 500 * time.Millisecond
	MaxBytes     = 1 * 1024 * 1024 // 1MB
)

var ErrTimeout = errors.New("stdin read timeout")

// osExit is a variable so tests can intercept os.Exit calls.
var osExit = os.Exit

// ReadStdinJSON reads up to maxBytes of JSON from stdin with a 500ms timeout.
// Returns an empty map on timeout, empty input, or parse failure — never blocks the hook.
func ReadStdinJSON(maxBytes int) (map[string]any, error) {
	return readJSON(os.Stdin, StdinTimeout, maxBytes)
}

// readJSON is the testable core: reads from r with the given timeout.
func readJSON(r io.Reader, timeout time.Duration, maxBytes int) (map[string]any, error) {
	ch := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(io.LimitReader(r, int64(maxBytes)))
		ch <- data
	}()

	var raw []byte
	select {
	case raw = <-ch:
	case <-time.After(timeout):
		return map[string]any{}, ErrTimeout
	}

	if len(raw) == 0 {
		return map[string]any{}, nil
	}

	// Replace invalid UTF-8 sequences before parsing (mirrors Python errors="replace").
	if !utf8.Valid(raw) {
		raw = []byte(strings.ToValidUTF8(string(raw), string(utf8.RuneError)))
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}, nil
	}
	return m, nil
}

// EmitPreToolResponse writes a hook decision to stdout using the Claude Code
// hookSpecificOutput format.
//
// decision="deny": sets permissionDecision="deny" in the output
// decision="":     no permissionDecision (additionalContext-only output)
// reason:          sets permissionDecisionReason (only if non-empty)
// additionalContext: sets additionalContext field (only if non-empty)
//
// If all three are empty, emits nothing.
func EmitPreToolResponse(decision, reason, additionalContext string) {
	if decision == "" && reason == "" && additionalContext == "" {
		return
	}

	inner := map[string]any{
		"hookEventName": "PreToolUse",
	}

	if decision == "deny" {
		inner["permissionDecision"] = "deny"
	}
	if reason != "" {
		inner["permissionDecisionReason"] = reason
	}
	if additionalContext != "" {
		inner["additionalContext"] = additionalContext
	}

	payload := map[string]any{
		"hookSpecificOutput": inner,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

// EmitPostToolResponse writes output text to stdout for PostToolUse hooks.
func EmitPostToolResponse(output string) {
	fmt.Fprint(os.Stdout, output)
}

// EmitSilent exits 0 with no stdout output — pass-through for the hook.
func EmitSilent() {
	osExit(0)
}
