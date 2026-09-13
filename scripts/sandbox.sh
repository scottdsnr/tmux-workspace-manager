#!/usr/bin/env bash
# Dev-only helper for testing twm without touching your real install.
#
# Isolation is two env vars:
#   HOME          -> twm's config dir is $HOME/.config/tmux-workspaces, so a
#                    fake HOME gives it its own settings/workspaces.
#   TMUX_TMPDIR   -> tmux itself picks its server socket from this, so a
#                    separate value means a separate tmux server; sandbox
#                    sessions can't collide with or attach to real ones even
#                    if you reuse the same alias.
#
# Usage:
#   scripts/sandbox.sh up               build the sandbox binary
#   scripts/sandbox.sh run [args...]    launch twm inside the sandbox
#   scripts/sandbox.sh down             kill the sandbox tmux server, delete everything
#   scripts/sandbox.sh status           show whether the sandbox tmux server is running
#
# Override the sandbox location with TWM_SANDBOX_DIR (default: /tmp/twm-sandbox).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SANDBOX_DIR="${TWM_SANDBOX_DIR:-/tmp/twm-sandbox}"
SANDBOX_HOME="$SANDBOX_DIR/home"
SANDBOX_TMUX_TMPDIR="$SANDBOX_DIR/tmux"
SANDBOX_BIN="$SANDBOX_DIR/twm-test"

info() { printf '==> %s\n' "$1"; }

build() {
    mkdir -p "$SANDBOX_HOME" "$SANDBOX_TMUX_TMPDIR"
    info "Building sandbox binary -> $SANDBOX_BIN"
    (cd "$ROOT" && go build -o "$SANDBOX_BIN" ./cmd/twm)
}

up() {
    build
    info "Sandbox ready:"
    echo "    HOME=$SANDBOX_HOME"
    echo "    TMUX_TMPDIR=$SANDBOX_TMUX_TMPDIR"
    info "Launch it with: $0 run"
}

run() {
    [ -x "$SANDBOX_BIN" ] || build
    exec env HOME="$SANDBOX_HOME" TMUX_TMPDIR="$SANDBOX_TMUX_TMPDIR" "$SANDBOX_BIN" "$@"
}

down() {
    if [ -d "$SANDBOX_TMUX_TMPDIR" ]; then
        info "Killing sandbox tmux server (if running)..."
        TMUX_TMPDIR="$SANDBOX_TMUX_TMPDIR" tmux kill-server 2>/dev/null || true
    fi
    info "Removing $SANDBOX_DIR"
    rm -rf "$SANDBOX_DIR"
}

status() {
    if [ -d "$SANDBOX_TMUX_TMPDIR" ] && TMUX_TMPDIR="$SANDBOX_TMUX_TMPDIR" tmux list-sessions 2>/dev/null; then
        :
    else
        echo "no sandbox tmux server running"
    fi
}

cmd="${1:-}"
[ $# -gt 0 ] && shift

case "$cmd" in
    up)     up ;;
    run)    run "$@" ;;
    down)   down ;;
    status) status ;;
    *)
        cat >&2 <<EOF
Usage: $0 {up|run|down|status}

  up      build the sandbox binary and create sandbox dirs
  run     launch twm inside the sandbox (builds first if needed)
  down    kill the sandbox tmux server and delete all sandbox files
  status  show whether the sandbox tmux server is running
EOF
        exit 1
        ;;
esac
