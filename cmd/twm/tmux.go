package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// All mutating calls go through argv slices (never a shell string) so
// paths, aliases, and pane commands can't be misparsed or break out of
// quoting.

var dryRun bool

type cmdResult struct {
	stdout string
	code   int
}

func tmuxQuery(args []string) cmdResult {
	cmd := exec.Command(args[0], args[1:]...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	return cmdResult{stdout: out.String(), code: code}
}

// tmuxRun executes a tmux mutation/query. Output is always captured rather
// than attached to the real terminal — these calls print nothing on success,
// and keeping them off stdio means they're safe to run from a goroutine
// while a Bubble Tea program is actively rendering (bubbles/up/down progress
// screens run these mid-render).
func tmuxRun(args []string) string {
	if dryRun {
		fmt.Printf("   %s[dry-run] tmux %s\n", sym("dry"), shellJoin(args[1:]))
		return ""
	}
	return strings.TrimSpace(tmuxQuery(args).stdout)
}

// tmux resolves a bare "-t name" loosely: it tries an exact match first, but
// falls back to any session (or window) the name is a *prefix* of, and then
// to an fnmatch pattern. With neighbouring workspaces like "am" and
// "am-app" that silently crosses the wires — "tmux has-session -t am"
// succeeds while only "am-app" is running, so `up am` reports the wrong
// workspace as already running and attaches to its neighbour, and `down am`
// would tear that neighbour down. Prefixing a name with "=" forces tmux to
// match it literally, so every target we build below is an exact one.
//
// sessionTarget is for commands that take a target-session (has-session,
// kill-session, list-windows, move-window, attach-session, switch-client).
func sessionTarget(session string) string {
	return "=" + session
}

// sessionPaneTarget is for commands whose -t is resolved as a pane/window
// even when we only mean a session (display-message, set-option). A bare
// "=name" is read there as a literal pane name and matches nothing; the
// trailing ":" makes tmux resolve it as that session's current window.
func sessionPaneTarget(session string) string {
	return "=" + session + ":"
}

// windowTarget builds an exact "session:window" target — both halves need
// their own "=", since window names match by prefix too.
func windowTarget(session, window string) string {
	return "=" + session + ":=" + window
}

func hasSession(name string) bool {
	return tmuxQuery([]string{"tmux", "has-session", "-t", sessionTarget(name)}).code == 0
}

// sessionAttached reports whether any client currently has session open.
// Used to distinguish "running, and someone's looking at it" from "running
// in the background" in status output.
func sessionAttached(session string) bool {
	res := tmuxQuery([]string{"tmux", "display-message", "-p", "-t", sessionPaneTarget(session), "#{session_attached}"})
	// An unresolvable target isn't an error here — display-message still
	// exits 0 and just prints nothing — so empty output means "no such
	// session", not "attached".
	out := strings.TrimSpace(res.stdout)
	if res.code != 0 || out == "" {
		return false
	}
	return out != "0"
}

// windowCount returns how many windows currently exist in session (which can
// drift from the profile's window count if one was closed or added by hand).
func windowCount(session string) int {
	res := tmuxQuery([]string{"tmux", "list-windows", "-t", sessionTarget(session), "-F", "#{window_index}"})
	if res.code != 0 {
		return 0
	}
	out := strings.TrimSpace(res.stdout)
	if out == "" {
		return 0
	}
	return len(strings.Split(out, "\n"))
}

func listPaneIDs(sessionName, winName string) []string {
	res := tmuxQuery([]string{"tmux", "list-panes", "-t", windowTarget(sessionName, winName), "-F", "#{pane_id}"})
	var ids []string
	for _, line := range strings.Split(res.stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ids = append(ids, line)
		}
	}
	return ids
}

func tmuxNewSession(session, winName, cwd string) string {
	return tmuxRun([]string{"tmux", "new-session", "-d", "-s", session, "-n", winName, "-c", cwd, "-P", "-F", "#{pane_id}"})
}

func tmuxNewWindow(session, prevWinName, winName, cwd string) string {
	return tmuxRun([]string{"tmux", "new-window", "-a", "-t", windowTarget(session, prevWinName), "-n", winName, "-c", cwd,
		"-P", "-F", "#{pane_id}"})
}

func tmuxSplitWindow(paneID, cwd string) string {
	return tmuxRun([]string{"tmux", "split-window", "-h", "-t", paneID, "-c", cwd, "-P", "-F", "#{pane_id}"})
}

func tmuxSendKeys(paneID, command string) {
	tmuxRun([]string{"tmux", "send-keys", "-t", paneID, command, "C-m"})
}

func tmuxSendCtrlC(paneID string) {
	tmuxRun([]string{"tmux", "send-keys", "-t", paneID, "C-c"})
}

// tmuxSetSessionOption sets a session-scoped option (e.g. base-index) on
// session.
func tmuxSetSessionOption(session, name string, value interface{}) {
	tmuxRun([]string{"tmux", "set-option", "-t", sessionPaneTarget(session), name, fmt.Sprintf("%v", value)})
}

// tmuxSetWindowOption sets a window-scoped option (e.g. pane-base-index) on
// one specific window of session.
func tmuxSetWindowOption(session, winName, name string, value interface{}) {
	tmuxRun([]string{"tmux", "set-option", "-w", "-t", windowTarget(session, winName), name, fmt.Sprintf("%v", value)})
}

func tmuxRenumberWindows(session string) {
	tmuxRun([]string{"tmux", "move-window", "-r", "-t", sessionTarget(session)})
}

// tmuxSelectLayout applies a layout (a preset name like "tiled", or a
// literal tmux layout string) to every pane in session's winName window.
// Run after all of a window's panes have been created, since layouts are a
// pane-count-dependent arrangement.
func tmuxSelectLayout(session, winName, layout string) {
	tmuxRun([]string{"tmux", "select-layout", "-t", windowTarget(session, winName), layout})
}

func tmuxKillSession(session string) {
	tmuxRun([]string{"tmux", "kill-session", "-t", sessionTarget(session)})
}

// insideTmux reports whether this process is itself running inside a tmux
// client, which determines whether "jump into that session" should attach a
// new client (nesting tmux) or just switch the current client's view.
func insideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// attachArgs returns the tmux argv that puts session in front of the user:
// attach-session from a plain terminal, switch-client when already inside
// tmux, so opening one workspace from within another never nests sessions.
func attachArgs(session string) []string {
	if insideTmux() {
		return []string{"tmux", "switch-client", "-t", sessionTarget(session)}
	}
	return []string{"tmux", "attach-session", "-t", sessionTarget(session)}
}

func tmuxAttach(session string) {
	args := attachArgs(session)
	if dryRun {
		verb := "attach to"
		if insideTmux() {
			verb = "switch to"
		}
		fmt.Printf("   %s[dry-run] would %s session '%s'\n", sym("dry"), verb, session)
		return
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Run()
}
