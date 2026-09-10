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

func hasSession(name string) bool {
	return tmuxQuery([]string{"tmux", "has-session", "-t", name}).code == 0
}

// sessionAttached reports whether any client currently has session open.
// Used to distinguish "running, and someone's looking at it" from "running
// in the background" in status output.
func sessionAttached(session string) bool {
	res := tmuxQuery([]string{"tmux", "display-message", "-p", "-t", session, "#{session_attached}"})
	if res.code != 0 {
		return false
	}
	return strings.TrimSpace(res.stdout) != "0"
}

// windowCount returns how many windows currently exist in session (which can
// drift from the profile's window count if one was closed or added by hand).
func windowCount(session string) int {
	res := tmuxQuery([]string{"tmux", "list-windows", "-t", session, "-F", "#{window_index}"})
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
	res := tmuxQuery([]string{"tmux", "list-panes", "-t", sessionName + ":" + winName, "-F", "#{pane_id}"})
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
	return tmuxRun([]string{"tmux", "new-window", "-a", "-t", session + ":" + prevWinName, "-n", winName, "-c", cwd,
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

func tmuxSetOption(session, name string, value interface{}, windowOption bool) {
	args := []string{"tmux", "set-option"}
	if windowOption {
		args = append(args, "-w")
	}
	args = append(args, "-t", session, name, fmt.Sprintf("%v", value))
	tmuxRun(args)
}

func tmuxRenumberWindows(session string) {
	tmuxRun([]string{"tmux", "move-window", "-r", "-t", session})
}

func tmuxKillSession(session string) {
	tmuxRun([]string{"tmux", "kill-session", "-t", session})
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
		return []string{"tmux", "switch-client", "-t", session}
	}
	return []string{"tmux", "attach-session", "-t", session}
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
