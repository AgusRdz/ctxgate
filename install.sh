#!/usr/bin/env bash
set -euo pipefail

# ctxgate installer
# Usage: curl -fsSL https://raw.githubusercontent.com/AgusRdz/ctxgate/main/install.sh | bash
# Override install dir: CTXGATE_INSTALL_DIR=/usr/local/bin bash install.sh

REPO="AgusRdz/ctxgate"
BINARY="ctxgate"
PUBLIC_KEY_URL="https://raw.githubusercontent.com/${REPO}/main/go/public_key.pem"
API_URL="https://api.github.com/repos/${REPO}/releases/latest"
BASE_DOWNLOAD_URL="https://github.com/${REPO}/releases/download"

# ---------------------------------------------------------------------------
# Color output — only when stdout is a tty
# ---------------------------------------------------------------------------
if [ -t 1 ]; then
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  RED='\033[0;31m'
  NC='\033[0m'
else
  GREEN='' YELLOW='' RED='' NC=''
fi

info()    { printf "${GREEN}[ctxgate]${NC} %s\n" "$*"; }
warn()    { printf "${YELLOW}[ctxgate] warning:${NC} %s\n" "$*" >&2; }
error()   { printf "${RED}[ctxgate] error:${NC} %s\n" "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Platform detection
# ---------------------------------------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH_RAW="$(uname -m)"

case "${OS}" in
  linux)  GOOS="linux" ;;
  darwin) GOOS="darwin" ;;
  mingw*|msys*|cygwin*)
    error "Windows detected. Please use install.ps1 instead:
  Invoke-Expression (Invoke-WebRequest https://raw.githubusercontent.com/${REPO}/main/install.ps1).Content"
    ;;
  *)
    error "Unsupported operating system: ${OS}"
    ;;
esac

case "${ARCH_RAW}" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *)
    error "Unsupported architecture: ${ARCH_RAW}. Only amd64 and arm64 are supported."
    ;;
esac

BINARY_NAME="${BINARY}-${GOOS}-${GOARCH}"
info "Platform: ${GOOS}/${GOARCH}"

# ---------------------------------------------------------------------------
# Install dir
# ---------------------------------------------------------------------------
INSTALL_DIR="${CTXGATE_INSTALL_DIR:-${HOME}/.local/bin}"
info "Install dir: ${INSTALL_DIR}"

# ---------------------------------------------------------------------------
# Dependency checks
# ---------------------------------------------------------------------------
for cmd in curl openssl xxd grep sha256sum; do
  command -v "${cmd}" >/dev/null 2>&1 || error "'${cmd}' is required but not found. Please install it and retry."
done

# ---------------------------------------------------------------------------
# Fetch latest release tag
# ---------------------------------------------------------------------------
info "Fetching latest release tag..."
TAG="$(curl -fsSL "${API_URL}" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
[ -n "${TAG}" ] || error "Failed to fetch latest release tag from GitHub API."
info "Latest release: ${TAG}"

DOWNLOAD_BASE="${BASE_DOWNLOAD_URL}/${TAG}"

# ---------------------------------------------------------------------------
# Download artifacts
# ---------------------------------------------------------------------------
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

info "Downloading ${BINARY_NAME}..."
curl -fsSL --output "${TMP_DIR}/${BINARY_NAME}" "${DOWNLOAD_BASE}/${BINARY_NAME}"

info "Downloading checksums.txt..."
curl -fsSL --output "${TMP_DIR}/checksums.txt" "${DOWNLOAD_BASE}/checksums.txt"

info "Downloading checksums.txt.sig..."
curl -fsSL --output "${TMP_DIR}/checksums.txt.sig" "${DOWNLOAD_BASE}/checksums.txt.sig"

info "Downloading public key..."
curl -fsSL --output "${TMP_DIR}/ctxgate_public_key.pem" "${PUBLIC_KEY_URL}"

# ---------------------------------------------------------------------------
# Verify SHA256
# ---------------------------------------------------------------------------
info "Verifying SHA256 checksum..."
CHECKSUM_LINE="$(grep "${BINARY_NAME}" "${TMP_DIR}/checksums.txt")"
[ -n "${CHECKSUM_LINE}" ] || error "No checksum entry found for ${BINARY_NAME} in checksums.txt."

# Run sha256sum check from TMP_DIR so relative paths in checksums.txt resolve
(
  cd "${TMP_DIR}"
  # Rewrite the checksums file to only contain the line for our binary,
  # using the bare filename (no path prefix) so sha256sum --check works.
  EXPECTED_HASH="$(echo "${CHECKSUM_LINE}" | awk '{print $1}')"
  echo "${EXPECTED_HASH}  ${BINARY_NAME}" | sha256sum --check --status
) || error "SHA256 checksum verification FAILED. The downloaded binary may be corrupted or tampered with."
info "SHA256 checksum OK."

# ---------------------------------------------------------------------------
# Verify Ed25519 signature
# ---------------------------------------------------------------------------
info "Verifying Ed25519 signature..."

# The .sig file is hex-encoded (xxd -p -c 256 format); decode it to binary.
xxd -r -p "${TMP_DIR}/checksums.txt.sig" > "${TMP_DIR}/checksums.txt.sig.bin"

openssl pkeyutl \
  -verify \
  -pubin \
  -inkey "${TMP_DIR}/ctxgate_public_key.pem" \
  -rawin \
  -in "${TMP_DIR}/checksums.txt" \
  -sigfile "${TMP_DIR}/checksums.txt.sig.bin" \
  >/dev/null 2>&1 \
  || error "Ed25519 signature verification FAILED. The release artifacts may have been tampered with."
info "Signature OK."

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------
mkdir -p "${INSTALL_DIR}"
install -m 755 "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY}"
info "Installed ${BINARY} ${TAG} to ${INSTALL_DIR}/${BINARY}"

# ---------------------------------------------------------------------------
# PATH registration
# ---------------------------------------------------------------------------
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*)
    info "Already in PATH."
    ;;
  *)
    # Detect shell profile
    case "${SHELL:-}" in
      */zsh)  PROFILE="${ZDOTDIR:-${HOME}}/.zshrc" ;;
      */bash) PROFILE="${HOME}/.bashrc" ;;
      *)
        if [ -f "${HOME}/.zshrc" ]; then
          PROFILE="${HOME}/.zshrc"
        else
          PROFILE="${HOME}/.bashrc"
        fi
        ;;
    esac

    EXPORT_LINE="export PATH=\"${INSTALL_DIR}:\${PATH}\""

    if ! grep -qF "${INSTALL_DIR}" "${PROFILE}" 2>/dev/null; then
      printf '\n# ctxgate\n%s\n' "${EXPORT_LINE}" >> "${PROFILE}"
      info "Added ${INSTALL_DIR} to PATH in ${PROFILE}"
    fi

    info "Activate in this session:"
    info "  source ${PROFILE}"
    ;;
esac

printf "${GREEN}[ctxgate] Done! Run: ctxgate version${NC}\n"
