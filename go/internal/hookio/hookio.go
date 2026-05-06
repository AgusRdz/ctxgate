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

// EmitPreToolResponse writes a block or inject decision to stdout.
//
// decision="block":  {"type":"block","reason":"<reason>","comparison":<additionalContext as JSON object>}
// decision="inject": {"type":"inject","replacement":"<additionalContext>","reason":"<reason>"}
func EmitPreToolResponse(decision, reason, additionalContext string) {
	var payload map[string]any

	switch decision {
	case "block":
		comparison := map[string]any{}
		if additionalContext != "" {
			_ = json.Unmarshal([]byte(additionalContext), &comparison)
		}
		payload = map[string]any{
			"type":       "block",
			"reason":     reason,
			"comparison": comparison,
		}
	case "inject":
		payload = map[string]any{
			"type":        "inject",
			"replacement": additionalContext,
			"reason":      reason,
		}
	default:
		return
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
