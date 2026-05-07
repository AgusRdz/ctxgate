#Requires -Version 5.1
<#
.SYNOPSIS
    ctxgate installer for Windows.
.DESCRIPTION
    Downloads the latest ctxgate release from GitHub, verifies SHA256 and Ed25519
    signature, then installs the binary to $env:LOCALAPPDATA\Programs\ctxgate
    (or $env:CTXGATE_INSTALL_DIR if set).
.EXAMPLE
    Invoke-Expression (Invoke-WebRequest https://raw.githubusercontent.com/AgusRdz/ctxgate/main/install.ps1).Content
#>

$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
$Repo           = 'AgusRdz/ctxgate'
$BinaryName     = 'ctxgate-windows-amd64.exe'
$PublicKeyUrl   = "https://raw.githubusercontent.com/$Repo/main/go/public_key.pem"
$ApiUrl         = "https://api.github.com/repos/$Repo/releases/latest"
$BaseDownload   = "https://github.com/$Repo/releases/download"

# ---------------------------------------------------------------------------
# Color helpers
# ---------------------------------------------------------------------------
$SupportsColor = $Host.UI.SupportsVirtualTerminal

function Write-Info  { param([string]$Msg)
    if ($SupportsColor) { Write-Host "`e[32m[ctxgate]`e[0m $Msg" }
    else                { Write-Host "[ctxgate] $Msg" }
}
function Write-Warn  { param([string]$Msg)
    if ($SupportsColor) { Write-Host "`e[33m[ctxgate] warning:`e[0m $Msg" -ForegroundColor Yellow }
    else                { Write-Host "[ctxgate] warning: $Msg" }
}
function Write-Fail  { param([string]$Msg)
    if ($SupportsColor) { Write-Host "`e[31m[ctxgate] error:`e[0m $Msg" -ForegroundColor Red }
    else                { Write-Host "[ctxgate] error: $Msg" }
    exit 1
}

# ---------------------------------------------------------------------------
# Architecture check — only amd64 supported
# ---------------------------------------------------------------------------
$OsArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
if ($OsArch -ne [System.Runtime.InteropServices.Architecture]::X64) {
    Write-Fail "Unsupported architecture: $OsArch. Only X64 (amd64) is supported at this time."
}
Write-Info "Platform: windows/amd64"

# ---------------------------------------------------------------------------
# Install dir
# ---------------------------------------------------------------------------
if ($env:CTXGATE_INSTALL_DIR) {
    $InstallDir = $env:CTXGATE_INSTALL_DIR
} else {
    $InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\ctxgate'
}
Write-Info "Install dir: $InstallDir"

# ---------------------------------------------------------------------------
# openssl check (ships with Git for Windows)
# ---------------------------------------------------------------------------
if (-not (Get-Command openssl -ErrorAction SilentlyContinue)) {
    Write-Fail "'openssl' not found in PATH. Install Git for Windows (https://git-scm.com) which bundles openssl, then retry."
}

# ---------------------------------------------------------------------------
# Fetch latest release tag
# ---------------------------------------------------------------------------
Write-Info "Fetching latest release tag..."
try {
    $Release = Invoke-RestMethod -Uri $ApiUrl -Headers @{ 'User-Agent' = 'ctxgate-installer' }
} catch {
    Write-Fail "Failed to fetch latest release from GitHub API: $_"
}
$Tag = $Release.tag_name
if (-not $Tag) { Write-Fail "Could not parse tag_name from GitHub API response." }
Write-Info "Latest release: $Tag"

$DownloadBase = "$BaseDownload/$Tag"

# ---------------------------------------------------------------------------
# Download artifacts to TEMP
# ---------------------------------------------------------------------------
$Tmp = $env:TEMP

$BinaryPath   = Join-Path $Tmp $BinaryName
$ChecksumsPath    = Join-Path $Tmp 'ctxgate_checksums.txt'
$SigHexPath       = Join-Path $Tmp 'ctxgate_checksums.txt.sig'
$SigBinPath       = Join-Path $Tmp 'ctxgate_checksums.txt.sig.bin'
$PublicKeyPath    = Join-Path $Tmp 'ctxgate_public_key.pem'

function Fetch { param([string]$Url, [string]$Dest)
    Write-Info "Downloading $(Split-Path $Dest -Leaf)..."
    try {
        Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
    } catch {
        Write-Fail "Download failed for $Url : $_"
    }
}

Fetch "$DownloadBase/$BinaryName"        $BinaryPath
Fetch "$DownloadBase/checksums.txt"      $ChecksumsPath
Fetch "$DownloadBase/checksums.txt.sig"  $SigHexPath
Fetch $PublicKeyUrl                      $PublicKeyPath

# ---------------------------------------------------------------------------
# Cleanup helper — runs on success AND on Write-Fail (exit)
# ---------------------------------------------------------------------------
function Remove-TempFiles {
    foreach ($f in @($BinaryPath, $ChecksumsPath, $SigHexPath, $SigBinPath, $PublicKeyPath)) {
        if (Test-Path $f) { Remove-Item $f -Force -ErrorAction SilentlyContinue }
    }
}

# ---------------------------------------------------------------------------
# Verify SHA256
# ---------------------------------------------------------------------------
Write-Info "Verifying SHA256 checksum..."

$ChecksumContent = Get-Content $ChecksumsPath -Raw
# Find the line for our binary (format: "<hex>  <filename>")
$MatchLine = ($ChecksumContent -split "`n") | Where-Object { $_ -match [regex]::Escape($BinaryName) } | Select-Object -First 1
if (-not $MatchLine) {
    Remove-TempFiles
    Write-Fail "No checksum entry found for '$BinaryName' in checksums.txt."
}

$ExpectedHash = ($MatchLine.Trim() -split '\s+')[0].ToUpper()
$ActualHash   = (Get-FileHash -Path $BinaryPath -Algorithm SHA256).Hash.ToUpper()

if ($ActualHash -ne $ExpectedHash) {
    Remove-TempFiles
    Write-Fail "SHA256 mismatch!`n  Expected: $ExpectedHash`n  Actual:   $ActualHash`nThe downloaded binary may be corrupted or tampered with."
}
Write-Info "SHA256 checksum OK."

# ---------------------------------------------------------------------------
# Verify Ed25519 signature
# ---------------------------------------------------------------------------
Write-Info "Verifying Ed25519 signature..."

# The .sig file is hex-encoded (xxd -p -c 256 output). Decode hex → binary.
$HexString = (Get-Content $SigHexPath -Raw).Trim() -replace '\s', ''
if ($HexString.Length % 2 -ne 0) {
    Remove-TempFiles
    Write-Fail "Signature hex string has odd length — file may be corrupted."
}

$SigBytes = [byte[]]::new($HexString.Length / 2)
for ($i = 0; $i -lt $HexString.Length; $i += 2) {
    $SigBytes[$i / 2] = [Convert]::ToByte($HexString.Substring($i, 2), 16)
}
[System.IO.File]::WriteAllBytes($SigBinPath, $SigBytes)

$opensslArgs = @(
    'pkeyutl', '-verify',
    '-pubin',
    '-inkey', $PublicKeyPath,
    '-rawin',
    '-in', $ChecksumsPath,
    '-sigfile', $SigBinPath
)

$result = & openssl @opensslArgs 2>&1
if ($LASTEXITCODE -ne 0) {
    Remove-TempFiles
    Write-Fail "Ed25519 signature verification FAILED. The release artifacts may have been tampered with.`n$result"
}
Write-Info "Signature OK."

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
$DestPath = Join-Path $InstallDir 'ctxgate.exe'
Copy-Item -Path $BinaryPath -Destination $DestPath -Force
Write-Info "Installed ctxgate $Tag to $DestPath"

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
Remove-TempFiles

# ---------------------------------------------------------------------------
# PATH registration
# ---------------------------------------------------------------------------
$CurrentPath = [System.Environment]::GetEnvironmentVariable('PATH', 'User')
if ($CurrentPath -notlike "*$InstallDir*") {
    $NewPath = if ($CurrentPath) { "$CurrentPath;$InstallDir" } else { $InstallDir }
    [System.Environment]::SetEnvironmentVariable('PATH', $NewPath, 'User')
    $env:PATH = "$env:PATH;$InstallDir"
    Write-Info "Added $InstallDir to PATH (effective immediately)."
} else {
    Write-Info "$InstallDir already in PATH."
}

# ---------------------------------------------------------------------------
# Wire hooks into Claude Code settings.json
# ---------------------------------------------------------------------------
try {
    & $DestPath init
} catch {
    Write-Warn "Hook wiring skipped: $_. Run 'ctxgate init' manually."
}

if ($SupportsColor) {
    Write-Host "`e[32m[ctxgate] Done! Run: ctxgate version`e[0m"
} else {
    Write-Host "[ctxgate] Done! Run: ctxgate version"
}
