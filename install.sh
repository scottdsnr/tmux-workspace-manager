#!/usr/bin/env bash
# Installer for twm.
#
# Downloads a prebuilt release binary — no Go, Python, or other runtime
# required on the target machine, just tmux itself.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/scottdsnr/tmux-workspace-manager/master/install.sh | bash
#
# Environment overrides:
#   TMUX_WORKSPACE_VERSION      release tag to install (e.g. v1.2.0)        [default: latest]
#   TMUX_WORKSPACE_INSTALL_DIR  directory to install the binary into        [default: ~/.local/bin]
#   TMUX_WORKSPACE_BIN_NAME     name of the installed command               [default: twm]

set -euo pipefail

REPO="scottdsnr/tmux-workspace-manager"
VERSION="${TMUX_WORKSPACE_VERSION:-latest}"
INSTALL_DIR="${TMUX_WORKSPACE_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="${TMUX_WORKSPACE_BIN_NAME:-twm}"
CONFIG_DIR="$HOME/.config/tmux-workspaces"

info()  { printf '==> %s\n' "$1"; }
warn()  { printf 'warning: %s\n' "$1" >&2; }
fail()  { printf 'error: %s\n' "$1" >&2; exit 1; }

if ! command -v tmux >/dev/null 2>&1; then
    warn "tmux was not found on PATH. Install it before using ${BIN_NAME}."
fi

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL "$1" -o "$2"; }
    fetch_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO "$2" "$1"; }
    fetch_stdout() { wget -qO- "$1"; }
else
    fail "Either curl or wget is required to install ${BIN_NAME}."
fi

case "$(uname -s)" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *) fail "Unsupported OS: $(uname -s). Only Linux and macOS have prebuilt binaries." ;;
esac

case "$(uname -m)" in
    x86_64|amd64)   ARCH="amd64" ;;
    aarch64|arm64)  ARCH="arm64" ;;
    *) fail "Unsupported architecture: $(uname -m). Only amd64 and arm64 have prebuilt binaries." ;;
esac

if [ "$VERSION" = "latest" ]; then
    info "Looking up the latest release of ${REPO}..."
    RELEASE_JSON="$(fetch_stdout "https://api.github.com/repos/${REPO}/releases/latest")"
    TAG="$(printf '%s\n' "$RELEASE_JSON" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
    [ -n "$TAG" ] || fail "Could not determine the latest release tag. Set TMUX_WORKSPACE_VERSION to install a specific version."
else
    TAG="$VERSION"
fi

ASSET="tmux-workspace-manager_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

info "Downloading ${ASSET} (${TAG})..."
fetch "${BASE_URL}/${ASSET}" "${WORK_DIR}/${ASSET}"

info "Verifying checksum..."
if fetch "${BASE_URL}/checksums.txt" "${WORK_DIR}/checksums.txt" 2>/dev/null; then
    EXPECTED="$(grep " ${ASSET}\$" "${WORK_DIR}/checksums.txt" | awk '{print $1}')"
    if [ -z "$EXPECTED" ]; then
        warn "No checksum entry found for ${ASSET}; skipping verification."
    else
        if command -v sha256sum >/dev/null 2>&1; then
            ACTUAL="$(sha256sum "${WORK_DIR}/${ASSET}" | awk '{print $1}')"
        elif command -v shasum >/dev/null 2>&1; then
            ACTUAL="$(shasum -a 256 "${WORK_DIR}/${ASSET}" | awk '{print $1}')"
        else
            warn "Neither sha256sum nor shasum found; skipping verification."
            ACTUAL="$EXPECTED"
        fi
        [ "$ACTUAL" = "$EXPECTED" ] || fail "Checksum mismatch for ${ASSET}: expected ${EXPECTED}, got ${ACTUAL}."
    fi
else
    warn "Could not fetch checksums.txt; skipping verification."
fi

info "Extracting..."
tar -xzf "${WORK_DIR}/${ASSET}" -C "${WORK_DIR}"

EXTRACTED_DIR="${WORK_DIR}/tmux-workspace-manager_${OS}_${ARCH}"
[ -f "${EXTRACTED_DIR}/twm" ] || fail "Downloaded archive did not contain the expected binary."

mkdir -p "$INSTALL_DIR" "$CONFIG_DIR"
TARGET="${INSTALL_DIR}/${BIN_NAME}"
cp "${EXTRACTED_DIR}/twm" "$TARGET"
chmod +x "$TARGET"

info "Installed to ${TARGET}"
info "Config directory: ${CONFIG_DIR}"

case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        warn "${INSTALL_DIR} is not on your PATH."
        printf '\n  Add this to your shell profile (e.g. ~/.bashrc or ~/.zshrc):\n\n' >&2
        printf '    export PATH="%s:$PATH"\n\n' "$INSTALL_DIR" >&2
        ;;
esac

info "Done. Try: ${BIN_NAME} create myproject"
