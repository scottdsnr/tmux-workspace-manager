#!/usr/bin/env bash
# Installer for tmux-workspace.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/scotthellingsnm/tmux/master/install.sh | bash
#
# Environment overrides:
#   TMUX_WORKSPACE_REF          git ref (branch/tag/commit) to install from  [default: master]
#   TMUX_WORKSPACE_INSTALL_DIR  directory to install the binary into         [default: ~/.local/bin]
#   TMUX_WORKSPACE_BIN_NAME     name of the installed command                [default: tmux-workspace]

set -euo pipefail

REPO="scotthellingsnm/tmux"
REF="${TMUX_WORKSPACE_REF:-master}"
INSTALL_DIR="${TMUX_WORKSPACE_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="${TMUX_WORKSPACE_BIN_NAME:-tmux-workspace}"
SOURCE_URL="https://raw.githubusercontent.com/${REPO}/${REF}/manage-workspace.py"
CONFIG_DIR="$HOME/.config/tmux-workspaces"

info()  { printf '==> %s\n' "$1"; }
warn()  { printf 'warning: %s\n' "$1" >&2; }
fail()  { printf 'error: %s\n' "$1" >&2; exit 1; }

command -v python3 >/dev/null 2>&1 || fail "python3 is required but was not found on PATH."

if ! command -v tmux >/dev/null 2>&1; then
    warn "tmux was not found on PATH. Install it before using ${BIN_NAME}."
fi

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO "$2" "$1"; }
else
    fail "Either curl or wget is required to download ${BIN_NAME}."
fi

mkdir -p "$INSTALL_DIR" "$CONFIG_DIR"

TARGET="${INSTALL_DIR}/${BIN_NAME}"
TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT

info "Downloading ${BIN_NAME} (${REF}) from ${REPO}..."
fetch "$SOURCE_URL" "$TMP_FILE"

head -c 2 "$TMP_FILE" | grep -q '#!' || fail "Downloaded file does not look like a script; aborting."

mv "$TMP_FILE" "$TARGET"
chmod +x "$TARGET"
trap - EXIT

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
