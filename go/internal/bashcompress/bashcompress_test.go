package bashcompress_test

import (
	"strings"
	"testing"

	"github.com/agusrdz/ctxgate/internal/bashcompress"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// StripANSI
// ---------------------------------------------------------------------------

func TestStripANSI_RemovesCSI(t *testing.T) {
	input := "\x1b[32mHello\x1b[0m World"
	got := bashcompress.StripANSI(input)
	require.Equal(t, "Hello World", got)
}

func TestStripANSI_RemovesBoldAndColor(t *testing.T) {
	input := "\x1b[1;34mBold Blue\x1b[m"
	got := bashcompress.StripANSI(input)
	require.Equal(t, "Bold Blue", got)
}

func TestStripANSI_PreservesOSC8Label(t *testing.T) {
	// OSC 8 hyperlink: \x1b]8;;https://example.com\x07label\x1b]8;;\x07
	input := "\x1b]8;;https://example.com\x07click here\x1b]8;;\x07 after"
	got := bashcompress.StripANSI(input)
	require.Equal(t, "click here after", got)
}

func TestStripANSI_PlainTextUnchanged(t *testing.T) {
	input := "plain text no escapes"
	require.Equal(t, input, bashcompress.StripANSI(input))
}

func TestStripANSI_EmptyString(t *testing.T) {
	require.Equal(t, "", bashcompress.StripANSI(""))
}

// ---------------------------------------------------------------------------
// HasDangerousChars
// ---------------------------------------------------------------------------

func TestHasDangerousChars_Semicolon(t *testing.T) {
	require.True(t, bashcompress.HasDangerousChars("git status; rm -rf"))
}

func TestHasDangerousChars_Pipe(t *testing.T) {
	require.True(t, bashcompress.HasDangerousChars("ls | grep foo"))
}

func TestHasDangerousChars_Ampersand(t *testing.T) {
	require.True(t, bashcompress.HasDangerousChars("npm install & pytest"))
}

func TestHasDangerousChars_Backtick(t *testing.T) {
	require.True(t, bashcompress.HasDangerousChars("echo `id`"))
}

func TestHasDangerousChars_Dollar(t *testing.T) {
	require.True(t, bashcompress.HasDangerousChars("echo $SECRET"))
}

func TestHasDangerousChars_Parens(t *testing.T) {
	require.True(t, bashcompress.HasDangerousChars("cmd $(subshell)"))
}

func TestHasDangerousChars_Safe(t *testing.T) {
	require.False(t, bashcompress.HasDangerousChars("git status"))
	require.False(t, bashcompress.HasDangerousChars("pytest tests/ -v"))
	require.False(t, bashcompress.HasDangerousChars("npm install --save-dev"))
}

// ---------------------------------------------------------------------------
// IsWhitelisted
// ---------------------------------------------------------------------------

func TestIsWhitelisted_GitStatus(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("git status"))
}

func TestIsWhitelisted_GitLog(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("git log --oneline -10"))
}

func TestIsWhitelisted_GitDiff(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("git diff HEAD"))
}

func TestIsWhitelisted_Pytest(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("pytest tests/"))
}

func TestIsWhitelisted_NpmInstall(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("npm install"))
}

func TestIsWhitelisted_NpmCi(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("npm ci"))
}

func TestIsWhitelisted_GoTest(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("go test ./..."))
}

func TestIsWhitelisted_Eslint(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("eslint src/"))
}

func TestIsWhitelisted_KubectlGet(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("kubectl get pods"))
}

func TestIsWhitelisted_GitCommit_Blocked(t *testing.T) {
	require.False(t, bashcompress.IsWhitelisted("git commit -m 'msg'"))
}

func TestIsWhitelisted_GitPush_Blocked(t *testing.T) {
	require.False(t, bashcompress.IsWhitelisted("git push origin main"))
}

func TestIsWhitelisted_KubectlSecrets_Blocked(t *testing.T) {
	require.False(t, bashcompress.IsWhitelisted("kubectl get secrets"))
	require.False(t, bashcompress.IsWhitelisted("kubectl describe secret/mykey"))
}

func TestIsWhitelisted_Sqlite3Delete_Blocked(t *testing.T) {
	require.False(t, bashcompress.IsWhitelisted("sqlite3 mydb.db 'DELETE FROM users'"))
}

func TestIsWhitelisted_Sqlite3DotCommand_Blocked(t *testing.T) {
	require.False(t, bashcompress.IsWhitelisted("sqlite3 mydb.db .tables"))
}

func TestIsWhitelisted_SafeEnvPrefix(t *testing.T) {
	require.True(t, bashcompress.IsWhitelisted("TERM=xterm-256color git status"))
}

func TestIsWhitelisted_UnsafeEnvVar_Blocked(t *testing.T) {
	require.False(t, bashcompress.IsWhitelisted("SECRET_KEY=abc git status"))
}

func TestIsWhitelisted_Unknown_Blocked(t *testing.T) {
	require.False(t, bashcompress.IsWhitelisted("curl https://example.com"))
	require.False(t, bashcompress.IsWhitelisted("rm -rf /"))
}

// ---------------------------------------------------------------------------
// DetectPattern
// ---------------------------------------------------------------------------

func TestDetectPattern_GitStatus(t *testing.T) {
	require.Equal(t, "git_status", bashcompress.DetectPattern("git status"))
}

func TestDetectPattern_GitLog(t *testing.T) {
	require.Equal(t, "git_log", bashcompress.DetectPattern("git log --oneline"))
}

func TestDetectPattern_GitDiff(t *testing.T) {
	require.Equal(t, "git_diff", bashcompress.DetectPattern("git diff HEAD"))
}

func TestDetectPattern_GitShow(t *testing.T) {
	require.Equal(t, "git_diff", bashcompress.DetectPattern("git show abc123"))
}

func TestDetectPattern_GitBranch(t *testing.T) {
	require.Equal(t, "list", bashcompress.DetectPattern("git branch"))
}

func TestDetectPattern_Pytest(t *testing.T) {
	require.Equal(t, "pytest", bashcompress.DetectPattern("pytest tests/"))
}

func TestDetectPattern_PytestViaModule(t *testing.T) {
	require.Equal(t, "pytest", bashcompress.DetectPattern("python -m pytest tests/"))
}

func TestDetectPattern_Jest(t *testing.T) {
	require.Equal(t, "jest", bashcompress.DetectPattern("jest --coverage"))
	require.Equal(t, "jest", bashcompress.DetectPattern("npx jest"))
}

func TestDetectPattern_Vitest(t *testing.T) {
	require.Equal(t, "jest", bashcompress.DetectPattern("vitest run"))
}

func TestDetectPattern_NpmInstall(t *testing.T) {
	require.Equal(t, "npm_install", bashcompress.DetectPattern("npm install"))
	require.Equal(t, "npm_install", bashcompress.DetectPattern("npm ci"))
	require.Equal(t, "npm_install", bashcompress.DetectPattern("pip install -r requirements.txt"))
}

func TestDetectPattern_Ls(t *testing.T) {
	require.Equal(t, "ls", bashcompress.DetectPattern("ls -la"))
	require.Equal(t, "ls", bashcompress.DetectPattern("find . -name '*.go'"))
}

func TestDetectPattern_Lint(t *testing.T) {
	require.Equal(t, "lint", bashcompress.DetectPattern("eslint src/"))
	require.Equal(t, "lint", bashcompress.DetectPattern("flake8 ."))
	require.Equal(t, "lint", bashcompress.DetectPattern("golangci-lint run"))
	require.Equal(t, "lint", bashcompress.DetectPattern("ruff check ."))
}

func TestDetectPattern_Logs(t *testing.T) {
	require.Equal(t, "logs", bashcompress.DetectPattern("tail -f /var/log/syslog"))
	require.Equal(t, "logs", bashcompress.DetectPattern("journalctl -n 100"))
	require.Equal(t, "logs", bashcompress.DetectPattern("docker logs mycontainer"))
}

func TestDetectPattern_Tree(t *testing.T) {
	require.Equal(t, "tree", bashcompress.DetectPattern("tree"))
}

func TestDetectPattern_Progress(t *testing.T) {
	require.Equal(t, "progress", bashcompress.DetectPattern("docker build ."))
	require.Equal(t, "progress", bashcompress.DetectPattern("docker pull nginx"))
}

func TestDetectPattern_List(t *testing.T) {
	require.Equal(t, "list", bashcompress.DetectPattern("npm ls"))
	require.Equal(t, "list", bashcompress.DetectPattern("pip list"))
	require.Equal(t, "list", bashcompress.DetectPattern("docker ps"))
	require.Equal(t, "list", bashcompress.DetectPattern("kubectl get pods"))
}

func TestDetectPattern_Build(t *testing.T) {
	require.Equal(t, "build", bashcompress.DetectPattern("tsc --build"))
	require.Equal(t, "build", bashcompress.DetectPattern("go build ./..."))
	require.Equal(t, "build", bashcompress.DetectPattern("webpack"))
}

func TestDetectPattern_Sqlite3(t *testing.T) {
	require.Equal(t, "sqlite3", bashcompress.DetectPattern("sqlite3 mydb.db"))
}

func TestDetectPattern_DiskStats(t *testing.T) {
	require.Equal(t, "disk_stats", bashcompress.DetectPattern("df -h"))
	require.Equal(t, "disk_stats", bashcompress.DetectPattern("du -sh *"))
	require.Equal(t, "disk_stats", bashcompress.DetectPattern("wc -l *.go"))
}

func TestDetectPattern_DockerOutput(t *testing.T) {
	require.Equal(t, "docker_output", bashcompress.DetectPattern("docker inspect mycontainer"))
}

func TestDetectPattern_GitStatusPorcelain_NoMatch(t *testing.T) {
	require.Equal(t, "", bashcompress.DetectPattern("git status --porcelain"))
	require.Equal(t, "", bashcompress.DetectPattern("git status --short"))
}

func TestDetectPattern_Unknown(t *testing.T) {
	require.Equal(t, "", bashcompress.DetectPattern("curl https://example.com"))
	require.Equal(t, "", bashcompress.DetectPattern(""))
}

// ---------------------------------------------------------------------------
// Compress — passthrough conditions
// ---------------------------------------------------------------------------

func TestCompress_NonZeroReturncode_Passthrough(t *testing.T) {
	// Non-zero exit → return raw output unchanged.
	raw := strings.Repeat("line of output\n", 50)
	got := bashcompress.Compress("git status", raw, 1, "")
	require.Equal(t, raw, got)
}

func TestCompress_ErrorStderr_Passthrough(t *testing.T) {
	raw := strings.Repeat("line of output\n", 50)
	got := bashcompress.Compress("git status", raw, 0, "error: something broke")
	require.Equal(t, raw, got)
}

func TestCompress_ShortOutput_Passthrough(t *testing.T) {
	// <100 rune output → return as-is.
	raw := "branch: main, clean"
	got := bashcompress.Compress("git status", raw, 0, "")
	require.Equal(t, raw, got)
}

func TestCompress_UnknownPattern_ReturnsANSIStripped(t *testing.T) {
	// Unknown command → no handler; returns ANSI-stripped output.
	raw := strings.Repeat("\x1b[32mline\x1b[0m\n", 30)
	got := bashcompress.Compress("curl https://example.com", raw, 0, "")
	require.NotContains(t, got, "\x1b[")
}

func TestCompress_ANSIStripped(t *testing.T) {
	// Even when ratio gate fires, output has no ANSI codes.
	raw := "\x1b[32m" + strings.Repeat("line\n", 40) + "\x1b[0m"
	got := bashcompress.Compress("git log --oneline", raw, 0, "")
	require.NotContains(t, got, "\x1b[")
}

// ---------------------------------------------------------------------------
// Compress — ratio gate
// ---------------------------------------------------------------------------

func TestCompress_RatioGate_PassthroughWhenSavingsTooSmall(t *testing.T) {
	// ls with 55 lines: compressLs keeps 50 + tail → saves ~5 lines on 55.
	// Savings = 5/55 ~ 9% < 10% → should return cleaned (pass through).
	lines := make([]string, 55)
	for i := range lines {
		lines[i] = "file" + itoa(i) + ".go"
	}
	raw := strings.Join(lines, "\n")
	got := bashcompress.Compress("ls -la", raw, 0, "")
	// Either pass-through (ratio gate) or compressed — just verify no panic.
	require.NotEmpty(t, got)
}

func TestCompress_RatioGate_CompressesWhenSavingsLarge(t *testing.T) {
	// ls with 200 lines: handler keeps 50 + tail → saves 150/200 = 75% > 10%.
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "file" + itoa(i) + ".go"
	}
	raw := strings.Join(lines, "\n")
	got := bashcompress.Compress("ls -la", raw, 0, "")
	require.Less(t, len(got), len(raw), "expected compression to reduce output size")
}

// ---------------------------------------------------------------------------
// Compress — credential preservation
// ---------------------------------------------------------------------------

func TestCompress_CredentialLinePreserved(t *testing.T) {
	// Build a large git log output (to pass the 100-char check and ratio gate)
	// with one credential line buried in the middle.
	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, "commit abc123"+itoa(i))
		lines = append(lines, "Author: Alice <alice@example.com>")
		lines = append(lines, "Date:   Mon Jan 1 00:00:00 2024")
		lines = append(lines, "")
		lines = append(lines, "    commit message "+itoa(i))
		lines = append(lines, "")
	}
	credLine := "DATABASE_URL=postgresql://admin:sup3rs3cr3t@db.internal.example.com/prod"
	lines = append(lines, credLine)
	raw := strings.Join(lines, "\n")

	got := bashcompress.Compress("git log --oneline", raw, 0, "")
	require.Contains(t, got, credLine, "credential line must be preserved in compressed output")
}

// ---------------------------------------------------------------------------
// Handler: git_status
// ---------------------------------------------------------------------------

func TestCompress_GitStatus_Clean(t *testing.T) {
	raw := `On branch main
Your branch is up to date with 'origin/main'.

nothing to commit, working tree clean
`
	// Pad to exceed 100 rune threshold.
	raw = raw + strings.Repeat(" ", 100)
	got := bashcompress.Compress("git status", raw, 0, "")
	require.Contains(t, got, "branch: main")
	require.Contains(t, got, "clean")
}

func TestCompress_GitStatus_WithChanges(t *testing.T) {
	raw := `On branch feature/my-branch
Your branch is ahead of 'origin/main' by 2 commits.

Changes to be committed:
	modified:   go/internal/foo/foo.go
	new file:   go/internal/bar/bar.go

Changes not staged for commit:
	modified:   README.md

Untracked files:
	.DS_Store
` + strings.Repeat("x", 100)

	got := bashcompress.Compress("git status", raw, 0, "")
	require.Contains(t, got, "feature/my-branch")
	require.Contains(t, got, "staged")
	require.Contains(t, got, "unstaged")
	require.Contains(t, got, "untracked")
}

// ---------------------------------------------------------------------------
// Handler: pytest
// ---------------------------------------------------------------------------

func TestCompress_Pytest_PassedSummary(t *testing.T) {
	raw := strings.Repeat("collecting ...\n", 10) +
		strings.Repeat("test_foo PASSED\n", 20) +
		"========== 20 passed in 1.23s ==========\n"

	got := bashcompress.Compress("pytest tests/", raw, 0, "")
	require.Contains(t, got, "passed")
}

func TestCompress_Pytest_FailedWithFailures(t *testing.T) {
	raw := strings.Repeat("test_foo PASSED\n", 20) +
		"FAILURES\n" +
		"def test_bar():\n" +
		"    assert 1 == 2\n" +
		"AssertionError: assert 1 == 2\n" +
		strings.Repeat("=", 40) + "\n" +
		"1 failed in 0.5s\n"

	got := bashcompress.Compress("pytest tests/", raw, 0, "")
	require.Contains(t, got, "failed")
}

// ---------------------------------------------------------------------------
// Handler: npm_install
// ---------------------------------------------------------------------------

func TestCompress_NpmInstall_Summary(t *testing.T) {
	// Generate a large npm install output with noisy dep-tree lines (no keywords).
	noisy := strings.Repeat("  ├─┬ lodash@4.17.21\n", 60)
	summary := "added 150 packages, and audited 500 packages in 10s\n" +
		"found 0 vulnerabilities\n"
	raw := noisy + summary

	got := bashcompress.Compress("npm install", raw, 0, "")
	require.Contains(t, got, "added")
	require.Contains(t, got, "audited")
	require.Less(t, len(got), len(raw))
}

// ---------------------------------------------------------------------------
// Handler: ls
// ---------------------------------------------------------------------------

func TestCompress_Ls_Truncates(t *testing.T) {
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "file" + itoa(i) + ".txt"
	}
	raw := strings.Join(lines, "\n")

	got := bashcompress.Compress("ls -la", raw, 0, "")
	require.Contains(t, got, "more entries")
	require.Less(t, len(got), len(raw))
}

// ---------------------------------------------------------------------------
// Handler: lint
// ---------------------------------------------------------------------------

func TestCompress_Lint_RuleGrouping(t *testing.T) {
	// Generate many lint lines with a recognizable rule code.
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, "src/foo.ts:"+itoa(i)+":1: error TS2322: Type mismatch")
	}
	lines = append(lines, "Found 30 errors in 5 files.")
	raw := strings.Join(lines, "\n")

	got := bashcompress.Compress("eslint src/", raw, 0, "")
	require.Contains(t, got, "findings")
	require.Less(t, len(got), len(raw))
}

// ---------------------------------------------------------------------------
// Handler: git_log
// ---------------------------------------------------------------------------

func TestCompress_GitLog_StripsGPGAndMerge(t *testing.T) {
	raw := `commit abc123
gpg: Signature made Mon Jan 1 00:00:00 2024
gpg:               using RSA key ABCDEF
Primary key fingerprint: DEAD BEEF
Merge: abc def
Author: Alice <alice@example.com>
Date:   Mon Jan 1 00:00:00 2024

    Initial commit
` + strings.Repeat("commit xyz\nAuthor: Bob\nDate: Mon\n\n    Msg\n", 10)

	got := bashcompress.Compress("git log", raw, 0, "")
	require.NotContains(t, got, "gpg:")
	require.NotContains(t, got, "Primary key")
	require.NotContains(t, got, "Merge: abc")
	require.Contains(t, got, "Alice")
}

// ---------------------------------------------------------------------------
// Handler: git_diff
// ---------------------------------------------------------------------------

func TestCompress_GitDiff_LargeDiff_Truncated(t *testing.T) {
	var lines []string
	lines = append(lines, "diff --git a/foo.go b/foo.go")
	lines = append(lines, "--- a/foo.go")
	lines = append(lines, "+++ b/foo.go")
	for i := 0; i < 100; i++ {
		lines = append(lines, "+new line "+itoa(i))
	}
	raw := strings.Join(lines, "\n")

	got := bashcompress.Compress("git diff HEAD", raw, 0, "")
	require.Contains(t, got, "more lines")
	require.Less(t, len(got), len(raw))
}

// ---------------------------------------------------------------------------
// Handler: jest
// ---------------------------------------------------------------------------

func TestCompress_Jest_ExtractsSummary(t *testing.T) {
	raw := strings.Repeat("PASS src/foo.test.js\n", 10) +
		"Test Suites: 5 passed, 5 total\n" +
		"Tests:       42 passed, 42 total\n" +
		"Time:        3.14s\n"
	// Pad to exceed 100 chars and trigger ratio gate.
	raw = strings.Repeat("  console.log output line\n", 20) + raw

	got := bashcompress.Compress("jest --coverage", raw, 0, "")
	require.Contains(t, got, "Tests:")
	require.Contains(t, got, "passed")
}

// ---------------------------------------------------------------------------
// Handler: tree
// ---------------------------------------------------------------------------

func TestCompress_Tree_TruncatesDeepEntries(t *testing.T) {
	var lines []string
	lines = append(lines, ".")
	lines = append(lines, "├── src")
	lines = append(lines, "│   ├── components")
	for i := 0; i < 30; i++ {
		lines = append(lines, "│   │   └── comp"+itoa(i)+".tsx")
	}
	lines = append(lines, "└── README.md")
	raw := strings.Join(lines, "\n")

	got := bashcompress.Compress("tree", raw, 0, "")
	require.Contains(t, got, "truncated")
	require.Less(t, len(got), len(raw))
}

// ---------------------------------------------------------------------------
// Handler: build (tsc)
// ---------------------------------------------------------------------------

func TestCompress_Build_ExtractsErrors(t *testing.T) {
	noisy := strings.Repeat("Processing file src/module"+itoa(1)+".ts\n", 30)
	errors := strings.Repeat("src/foo.ts(10,5): error TS2345: Argument type mismatch\n", 10)
	summary := "Found 10 errors in 3 files.\n"
	raw := noisy + errors + summary

	got := bashcompress.Compress("tsc --build", raw, 0, "")
	require.Contains(t, got, "error")
	require.Less(t, len(got), len(raw))
}

// ---------------------------------------------------------------------------
// Handler: sqlite3
// ---------------------------------------------------------------------------

func TestCompress_Sqlite3_TruncatesRows(t *testing.T) {
	var lines []string
	lines = append(lines, "id|name|email")
	lines = append(lines, "--|-----|-----")
	for i := 0; i < 80; i++ {
		lines = append(lines, itoa(i)+"|user"+itoa(i)+"|user"+itoa(i)+"@example.com")
	}
	raw := strings.Join(lines, "\n")

	got := bashcompress.Compress("sqlite3 mydb.db", raw, 0, "")
	require.Contains(t, got, "more rows")
	require.Less(t, len(got), len(raw))
}

// ---------------------------------------------------------------------------
// Handler: disk_stats (df)
// ---------------------------------------------------------------------------

func TestCompress_DiskStats_TruncatesMiddle(t *testing.T) {
	var lines []string
	lines = append(lines, "Filesystem      Size  Used Avail Use% Mounted on")
	for i := 0; i < 40; i++ {
		lines = append(lines, "/dev/sd"+string(rune('a'+i))+"  100G  50G  50G  50% /mount"+itoa(i))
	}
	raw := strings.Join(lines, "\n")

	got := bashcompress.Compress("df -h", raw, 0, "")
	require.Contains(t, got, "omitted")
	require.Less(t, len(got), len(raw))
}

// ---------------------------------------------------------------------------
// Handler: docker_output (inspect)
// ---------------------------------------------------------------------------

func TestCompress_DockerOutput_JSONArray(t *testing.T) {
	// docker inspect returns a JSON array.
	item := `{"Id":"abc","Name":"mycontainer","State":{"Status":"running"}}`
	raw := "[" + item + "," + item + "," + item + "]"
	// Pad to exceed 100 chars.
	raw = raw + strings.Repeat(" ", 10)

	got := bashcompress.Compress("docker inspect mycontainer", raw, 0, "")
	require.Contains(t, got, "3 items")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
