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

func tmuxRun(args []string, capture bool) string {
	if dryRun {
		fmt.Printf("   %s[dry-run] tmux %s\n", sym("dry"), shellJoin(args[1:]))
		return ""
	}
	if capture {
		return strings.TrimSpace(tmuxQuery(args).stdout)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Run()
	return ""
}

func hasSession(name string) bool {
	return tmuxQuery([]string{"tmux", "has-session", "-t", name}).code == 0
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
	return tmuxRun([]string{"tmux", "new-session", "-d", "-s", session, "-n", winName, "-c", cwd, "-P", "-F", "#{pane_id}"}, true)
}

func tmuxNewWindow(session, prevWinName, winName, cwd string) string {
	return tmuxRun([]string{"tmux", "new-window", "-a", "-t", session + ":" + prevWinName, "-n", winName, "-c", cwd,
		"-P", "-F", "#{pane_id}"}, true)
}

func tmuxSplitWindow(paneID, cwd string) string {
	return tmuxRun([]string{"tmux", "split-window", "-h", "-t", paneID, "-c", cwd, "-P", "-F", "#{pane_id}"}, true)
}

func tmuxSendKeys(paneID, command string) {
	tmuxRun([]string{"tmux", "send-keys", "-t", paneID, command, "C-m"}, false)
}

func tmuxSendCtrlC(paneID string) {
	tmuxRun([]string{"tmux", "send-keys", "-t", paneID, "C-c"}, false)
}

func tmuxSetOption(session, name string, value interface{}, windowOption bool) {
	args := []string{"tmux", "set-option"}
	if windowOption {
		args = append(args, "-w")
	}
	args = append(args, "-t", session, name, fmt.Sprintf("%v", value))
	tmuxRun(args, false)
}

func tmuxRenumberWindows(session string) {
	tmuxRun([]string{"tmux", "move-window", "-r", "-t", session}, false)
}

func tmuxKillSession(session string) {
	tmuxRun([]string{"tmux", "kill-session", "-t", session}, false)
}

func tmuxAttach(session string) {
	if dryRun {
		fmt.Printf("   %s[dry-run] would attach to session '%s'\n", sym("dry"), session)
		return
	}
	cmd := exec.Command("tmux", "attach-session", "-t", session)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Run()
}
