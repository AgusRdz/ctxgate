package hookio

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- ReadStdinJSON / readJSON ---

func TestReadJSON_ValidInput(t *testing.T) {
	r := strings.NewReader(`{"tool_name":"Read","tool_use_id":"abc","tool_input":{"file_path":"/tmp/x.py"}}`)
	got, err := readJSON(r, 500*time.Millisecond, MaxBytes)
	require.NoError(t, err)
	require.Equal(t, "Read", got["tool_name"])
	require.Equal(t, "abc", got["tool_use_id"])
}

func TestReadJSON_EmptyInput(t *testing.T) {
	r := strings.NewReader("")
	got, err := readJSON(r, 500*time.Millisecond, MaxBytes)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestReadJSON_InvalidJSON(t *testing.T) {
	r := strings.NewReader("not json at all")
	got, err := readJSON(r, 500*time.Millisecond, MaxBytes)
	require.NoError(t, err) // parse errors are swallowed — hook must not crash
	require.Empty(t, got)
}

func TestReadJSON_Timeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()

	start := time.Now()
	got, err := readJSON(pr, 50*time.Millisecond, MaxBytes)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, ErrTimeout)
	require.Empty(t, got)
	require.Less(t, elapsed, 200*time.Millisecond)
}

func TestReadJSON_MaxBytesLimit(t *testing.T) {
	// 20-byte limit truncates the JSON so it won't parse — expect empty map, no error.
	r := strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":"/very/long/path"}}`)
	got, err := readJSON(r, 500*time.Millisecond, 20)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestReadJSON_InvalidUTF8(t *testing.T) {
	// Construct JSON with invalid UTF-8 bytes in a string value.
	// After replacement the JSON should still parse; the key must be present.
	raw := "{\"key\":\"\xff\xfe\"}"
	r := strings.NewReader(raw)
	got, err := readJSON(r, 500*time.Millisecond, MaxBytes)
	require.NoError(t, err)
	_, ok := got["key"]
	require.True(t, ok)
}

func TestReadJSON_NestedObject(t *testing.T) {
	r := strings.NewReader(`{"tool_input":{"file_path":"/tmp/a.go","offset":10,"limit":50}}`)
	got, err := readJSON(r, 500*time.Millisecond, MaxBytes)
	require.NoError(t, err)
	inner, ok := got["tool_input"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/tmp/a.go", inner["file_path"])
}

// --- EmitPreToolResponse ---

func hookOutput(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &m))
	inner, ok := m["hookSpecificOutput"].(map[string]any)
	require.True(t, ok, "missing hookSpecificOutput")
	return inner
}

func TestEmitPreToolResponse_DenyWithReason(t *testing.T) {
	out := captureStdout(t, func() {
		EmitPreToolResponse("deny", "file too large", "")
	})

	inner := hookOutput(t, out)
	require.Equal(t, "PreToolUse", inner["hookEventName"])
	require.Equal(t, "deny", inner["permissionDecision"])
	require.Equal(t, "file too large", inner["permissionDecisionReason"])
	_, hasCtx := inner["additionalContext"]
	require.False(t, hasCtx)
}

func TestEmitPreToolResponse_DenyWithReasonAndContext(t *testing.T) {
	out := captureStdout(t, func() {
		EmitPreToolResponse("deny", "file unchanged", "structure map text here")
	})

	inner := hookOutput(t, out)
	require.Equal(t, "deny", inner["permissionDecision"])
	require.Equal(t, "file unchanged", inner["permissionDecisionReason"])
	require.Equal(t, "structure map text here", inner["additionalContext"])
}

func TestEmitPreToolResponse_AdditionalContextOnly(t *testing.T) {
	out := captureStdout(t, func() {
		EmitPreToolResponse("", "", "delta context info")
	})

	inner := hookOutput(t, out)
	require.Equal(t, "PreToolUse", inner["hookEventName"])
	_, hasDeny := inner["permissionDecision"]
	require.False(t, hasDeny, "no permissionDecision for empty decision")
	require.Equal(t, "delta context info", inner["additionalContext"])
}

func TestEmitPreToolResponse_AllEmpty(t *testing.T) {
	out := captureStdout(t, func() {
		EmitPreToolResponse("", "", "")
	})
	require.Empty(t, out)
}

func TestEmitPreToolResponse_UnknownDecision(t *testing.T) {
	// unknown decision is treated as non-deny — should still emit with additionalContext
	out := captureStdout(t, func() {
		EmitPreToolResponse("unknown", "r", "ctx")
	})
	inner := hookOutput(t, out)
	_, hasDeny := inner["permissionDecision"]
	require.False(t, hasDeny)
	require.Equal(t, "r", inner["permissionDecisionReason"])
	require.Equal(t, "ctx", inner["additionalContext"])
}

// --- EmitPostToolResponse ---

func TestEmitPostToolResponse(t *testing.T) {
	out := captureStdout(t, func() {
		EmitPostToolResponse("summary: 3 files changed")
	})
	require.Equal(t, "summary: 3 files changed", out)
}

// --- EmitSilent ---

func TestEmitSilent(t *testing.T) {
	exitCode := -1
	old := osExit
	osExit = func(code int) { exitCode = code }
	defer func() { osExit = old }()

	EmitSilent()
	require.Equal(t, 0, exitCode)
}

// captureStdout swaps os.Stdout with a pipe, runs fn, returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)

	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}
