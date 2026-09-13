package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// reportFunc receives one human-readable progress line at a time. The plain
// CLI path renders it with fmt.Println; the TUI progress screen sends it
// over a channel as a log line.
type reportFunc func(string)

// runUp ensures the tmux session for alias exists (building every
// window/pane if it doesn't), but never attaches — attaching takes over the
// real terminal, which only the caller (plain fmt/exec, or the TUI's
// tea.ExecProcess) is positioned to do safely.
func runUp(alias string, dry bool, report reportFunc) error {
	dryRun = dry

	raw, err := loadConfigRawErr(alias)
	if err != nil {
		return err
	}
	if errs := validateConfig(raw); len(errs) > 0 {
		return fmt.Errorf("invalid config:\n   - %s", strings.Join(errs, "\n   - "))
	}
	config, err := loadConfigTypedErr(alias)
	if err != nil {
		return err
	}

	projectDir := resolveProjectPath(config.ProjectPath, settings.BaseDir)
	sessionName := alias

	if hasSession(sessionName) {
		report(fmt.Sprintf("%sWorkspace '%s' is already running.", sym("wave"), sessionName))
		return nil
	}

	displayTitle := config.ProjectNameDisplay
	if displayTitle == "" {
		displayTitle = sessionName
	}
	report(fmt.Sprintf("%sBuilding '%s' workspace...", sym("rocket"), displayTitle))

	windowsList := config.Windows
	prevWinName := ""

	for winIdx, win := range windowsList {
		winName := win.Name
		winDir := resolveWindowDir(projectDir, win)
		report(fmt.Sprintf("   %sWindow %d/%d: %s", sym("package"), winIdx+1, len(windowsList), winName))

		panes := win.Panes
		firstPaneDir := winDir
		if len(panes) > 0 {
			firstPaneDir = resolvePaneDir(winDir, panes[0])
		}

		var currentPaneID string
		if winIdx == 0 {
			currentPaneID = tmuxNewSession(sessionName, winName, firstPaneDir)
			tmuxSetOption(sessionName, "base-index", settings.WindowBaseIndex, false)
			tmuxRenumberWindows(sessionName)
		} else {
			currentPaneID = tmuxNewWindow(sessionName, prevWinName, winName, firstPaneDir)
		}

		// pane-base-index is a per-window option: "-w -t session" only ever
		// touches the session's *current* window, so it has to be set again,
		// explicitly per window, right after each one is created.
		tmuxSetOption(sessionName+":"+winName, "pane-base-index", settings.PaneBaseIndex, true)

		if len(panes) > 0 {
			sendPaneCommand(currentPaneID, panes[0])
		}

		for _, pane := range panes[minInt(1, len(panes)):] {
			paneDir := resolvePaneDir(winDir, pane)
			currentPaneID = tmuxSplitWindow(currentPaneID, paneDir)
			sendPaneCommand(currentPaneID, pane)
		}

		if win.Layout != "" {
			tmuxSelectLayout(sessionName+":"+winName, win.Layout)
		}

		prevWinName = winName
	}

	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// sendPaneCommand sends a pane's env assignments (if any) and command to
// paneID as a single line, so the exports are visible to the command.
func sendPaneCommand(paneID string, pane Pane) {
	if cmd := buildPaneCommand(pane); cmd != "" {
		tmuxSendKeys(paneID, cmd)
	}
}

// buildPaneCommand combines a pane's env vars and command into the single
// shell line send-keys should type. Env vars are sorted for deterministic
// output (useful for --dry-run and tests).
func buildPaneCommand(pane Pane) string {
	if len(pane.Env) == 0 {
		return pane.Command
	}
	keys := make([]string, 0, len(pane.Env))
	for k := range pane.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("export")
	for _, k := range keys {
		b.WriteString(" " + k + "=" + shellQuote(pane.Env[k]))
	}
	if pane.Command != "" {
		b.WriteString(" && " + pane.Command)
	}
	return b.String()
}

// runTeardown runs each teardown command in projectDir. When attachTTY is
// true (the plain CLI path), commands get the real terminal, matching
// today's behavior. When false (running inside the TUI, which owns the
// terminal for its own rendering), output is captured and reported as log
// lines instead — teardown commands lose interactive stdin in that case,
// which is the right trade-off for commands that are meant to run
// unattended during a "down".
func runTeardown(commands []string, projectDir string, dry, attachTTY bool, report reportFunc) {
	if len(commands) == 0 {
		return
	}

	report(fmt.Sprintf("%sRunning teardown commands...", sym("broom")))
	for _, cmd := range commands {
		if cmd == "" {
			continue
		}
		if dry {
			report(fmt.Sprintf("   %s[dry-run] would run: %s", sym("dry"), cmd))
			continue
		}
		report(fmt.Sprintf(" -> %s", cmd))
		c := exec.Command("sh", "-c", cmd)
		c.Dir = projectDir

		var runErr error
		if attachTTY {
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			runErr = c.Run()
		} else {
			out, err := c.CombinedOutput()
			for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
				if line != "" {
					report("    " + line)
				}
			}
			runErr = err
		}
		if ee, ok := runErr.(*exec.ExitError); ok && ee.ExitCode() != 0 {
			report(fmt.Sprintf("    %sExited with code %d", sym("warn"), ee.ExitCode()))
		}
	}
	report(" -> Teardown complete.")
}

// runDown signals every pane to stop, runs teardown, then kills the
// session. Like runUp, it never prompts — confirmation is a UI decision the
// caller makes before calling this.
func runDown(alias string, dry, attachTTY bool, report reportFunc) error {
	dryRun = dry

	raw, err := loadConfigRawErr(alias)
	if err != nil {
		return err
	}
	if errs := validateConfig(raw); len(errs) > 0 {
		return fmt.Errorf("invalid config:\n   - %s", strings.Join(errs, "\n   - "))
	}
	config, err := loadConfigTypedErr(alias)
	if err != nil {
		return err
	}

	projectDir := resolveProjectPath(config.ProjectPath, settings.BaseDir)
	sessionName := alias

	if !hasSession(sessionName) {
		report(fmt.Sprintf("%sNo active session found for alias '%s'", sym("info"), sessionName))
		return nil
	}

	report(fmt.Sprintf("%sSafely bringing down workspace: %s...", sym("stop"), sessionName))

	selfPane := os.Getenv("TMUX_PANE")

	for _, win := range config.Windows {
		winName := win.Name
		exitCmd := win.OnStop
		paneIDs := listPaneIDs(sessionName, winName)

		if exitCmd != "" && len(paneIDs) > 0 {
			tmuxSendKeys(paneIDs[0], exitCmd)
			continue
		}

		for _, paneID := range paneIDs {
			if paneID == selfPane {
				continue // never interrupt the pane we are running in
			}
			tmuxSendCtrlC(paneID)
		}
	}

	if !dry {
		time.Sleep(500 * time.Millisecond) // give Ctrl-C / exit commands a moment to land before teardown runs
	}

	runTeardown(config.Teardown, projectDir, dry, attachTTY, report)

	report(fmt.Sprintf("   %sCleaning environment allocations...", sym("broom")))
	tmuxKillSession(sessionName)
	report(fmt.Sprintf("%sCompleted clean exit!", sym("check")))
	return nil
}

// attachWorkspace jumps straight into an already-running workspace's tmux
// session, skipping the build step (and, in the TUI, its progress screen)
// entirely. tmuxAttach itself picks attach-session vs. switch-client.
func attachWorkspace(alias string) {
	if !hasSession(alias) {
		fmt.Printf("%sNo active session found for alias '%s'\n", sym("info"), alias)
		return
	}
	tmuxAttach(alias)
}

// startWorkspace is the plain-text CLI path: build (if needed) and attach,
// printing progress directly rather than through a TUI.
func startWorkspace(alias string, dry bool) {
	err := runUp(alias, dry, func(line string) { fmt.Println(line) })
	if err != nil {
		errPrint("%sCannot start '%s' — %v", sym("error"), alias, err)
		os.Exit(1)
	}
	tmuxAttach(alias)
}

// stopWorkspace is the plain-text CLI path: confirm (unless skipped), then
// tear down, printing progress directly.
func stopWorkspace(alias string, dry bool, assumeYes bool) {
	raw, err := loadConfigRawErr(alias)
	if err != nil {
		fatal("%s%s", sym("error"), err)
	}
	if errs := validateConfig(raw); len(errs) > 0 {
		errPrint("%sCannot stop '%s' — invalid config:", sym("error"), alias)
		for _, e := range errs {
			errPrint("   - %s", e)
		}
		os.Exit(1)
	}

	if !hasSession(alias) {
		fmt.Printf("%sNo active session found for alias '%s'\n", sym("info"), alias)
		return
	}

	if !dry && !assumeYes && settings.ConfirmDown {
		config, _ := loadConfigTypedErr(alias)
		displayTitle := config.ProjectNameDisplay
		if displayTitle == "" {
			displayTitle = alias
		}
		answer := prompt(fmt.Sprintf("%sTear down '%s' (%s)? This runs teardown commands and kills the session. (y/N): ",
			sym("warn"), displayTitle, alias))
		if strings.ToLower(answer) != "y" {
			fmt.Println("Cancelled.")
			return
		}
	}

	if err := runDown(alias, dry, true, func(line string) { fmt.Println(line) }); err != nil {
		errPrint("%sCannot stop '%s' — %v", sym("error"), alias, err)
		os.Exit(1)
	}
}
