# ctxgate

Single Go binary hook runtime for the ctxgate Claude Code plugin. Zero Python dependency, <20ms cold start.

## Install

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/AgusRdz/ctxgate/main/install.sh | bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/AgusRdz/ctxgate/main/install.ps1 | iex
```

Override install directory: `CTXGATE_INSTALL_DIR=/usr/local/bin bash install.sh`

## Verify

Each release ships `checksums.txt` (SHA-256), `checksums.txt.sig` (Ed25519, hex-encoded), and a SLSA build provenance attestation. Public key: `go/public_key.pem`.

```bash
# checksum
sha256sum -c checksums.txt --ignore-missing

# signature
xxd -r -p checksums.txt.sig > checksums.txt.sig.bin
openssl pkeyutl -verify -pubin -inkey public_key.pem -rawin -in checksums.txt -sigfile checksums.txt.sig.bin

# SLSA provenance
gh attestation verify ctxgate-linux-amd64 --repo AgusRdz/ctxgate
```

## Hook integration

The plugin's `hooks/hooks.json` wires all 19 Claude Code hook invocations to the binary.

| Subcommand | Hook event |
|---|---|
| `read-cache` | PreToolUse[Read] |
| `read-cache --invalidate` | PostToolUse[Edit\|Write\|...] |
| `read-cache --clear` | PreCompact, CwdChanged |
| `bash-hook` | PreToolUse[Bash] |
| `bash-compress <cmd>` | invoked by bash-hook rewrite |
| `archive-result` | PostToolUse[Bash\|Read\|Glob\|Grep\|Agent\|mcp__.*] |
| `context-intel` | PostToolUse[Bash\|Read\|Grep\|Glob\|mcp__.*] |
| `measure <action>` | SessionStart, Stop, StopFailure, SessionEnd, PreCompact, UserPromptSubmit, PreToolUse[Agent\|Task] |

## Inspection CLI

```
ctxgate report health
ctxgate report trends [--days 30]
ctxgate report savings [--days 30]
ctxgate report compression-stats [--days 30]
ctxgate report list-checkpoints
ctxgate report checkpoint-stats [--days 7]
ctxgate detectors [--session ID]
ctxgate outline <file>
```

## Build from source

Requires Docker.

```bash
git clone https://github.com/AgusRdz/ctxgate
cd ctxgate
make build-windows   # or build-linux, build-darwin-amd64, build-darwin-arm64
# Output: go/dist/
```

## License

MIT
