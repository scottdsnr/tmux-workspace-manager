package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func startWorkspace(alias string, dry bool) {
	dryRun = dry

	raw, _ := loadConfigRaw(alias)
	if errs := validateConfig(raw); len(errs) > 0 {
		errPrint("%sCannot start '%s' — invalid config:", sym("error"), alias)
		for _, e := range errs {
			errPrint("   - %s", e)
		}
		os.Exit(1)
	}
	config := loadConfigTyped(alias)

	projectDir := resolveProjectPath(config.ProjectPath, settings.BaseDir)
	sessionName := alias

	if hasSession(sessionName) {
		fmt.Printf("%sWorkspace '%s' is already running. Attaching...\n", sym("wave"), sessionName)
		tmuxAttach(sessionName)
		return
	}

	displayTitle := config.ProjectNameDisplay
	if displayTitle == "" {
		displayTitle = sessionName
	}
	fmt.Printf("%sBuilding '%s' workspace...\n", sym("rocket"), displayTitle)

	windowsList := config.Windows
	prevWinName := ""

	for winIdx, win := range windowsList {
		winName := win.Name
		winDir := resolveWindowDir(projectDir, win)
		fmt.Printf("   %sWindow %d/%d: %s\n", sym("package"), winIdx+1, len(windowsList), winName)

		var currentPaneID string
		if winIdx == 0 {
			currentPaneID = tmuxNewSession(sessionName, winName, winDir)
			tmuxSetOption(sessionName, "base-index", settings.WindowBaseIndex, false)
			tmuxRenumberWindows(sessionName)
		} else {
			currentPaneID = tmuxNewWindow(sessionName, prevWinName, winName, winDir)
		}

		// pane-base-index is a per-window option: "-w -t session" only ever
		// touches the session's *current* window, so it has to be set again,
		// explicitly per window, right after each one is created.
		tmuxSetOption(sessionName+":"+winName, "pane-base-index", settings.PaneBaseIndex, true)

		panes := win.Panes
		firstCmd := ""
		if len(panes) > 0 {
			firstCmd = panes[0].Command
		}
		if firstCmd != "" {
			tmuxSendKeys(currentPaneID, firstCmd)
		}

		for _, pane := range panes[minInt(1, len(panes)):] {
			currentPaneID = tmuxSplitWindow(currentPaneID, winDir)
			if pane.Command != "" {
				tmuxSendKeys(currentPaneID, pane.Command)
			}
		}

		prevWinName = winName
	}

	tmuxAttach(sessionName)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func runTeardown(commands []string, projectDir string, dry bool) {
	if len(commands) == 0 {
		return
	}

	fmt.Printf("%sRunning teardown commands...\n", sym("broom"))
	for _, cmd := range commands {
		if cmd == "" {
			continue
		}
		if dry {
			fmt.Printf("   %s[dry-run] would run: %s\n", sym("dry"), cmd)
			continue
		}
		fmt.Printf(" -> %s\n", cmd)
		c := exec.Command("sh", "-c", cmd)
		c.Dir = projectDir
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		err := c.Run()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() != 0 {
				fmt.Printf("    %sExited with code %d\n", sym("warn"), ee.ExitCode())
			}
		}
	}
	fmt.Println(" -> Teardown complete.")
}

func stopWorkspace(alias string, dry bool, assumeYes bool) {
	dryRun = dry

	raw, _ := loadConfigRaw(alias)
	if errs := validateConfig(raw); len(errs) > 0 {
		errPrint("%sCannot stop '%s' — invalid config:", sym("error"), alias)
		for _, e := range errs {
			errPrint("   - %s", e)
		}
		os.Exit(1)
	}
	config := loadConfigTyped(alias)

	projectDir := resolveProjectPath(config.ProjectPath, settings.BaseDir)
	sessionName := alias

	if !hasSession(sessionName) {
		fmt.Printf("%sNo active session found for alias '%s'\n", sym("info"), sessionName)
		return
	}

	if !dry && !assumeYes && settings.ConfirmDown {
		displayTitle := config.ProjectNameDisplay
		if displayTitle == "" {
			displayTitle = sessionName
		}
		answer := prompt(fmt.Sprintf("%sTear down '%s' (%s)? This runs teardown commands and kills the session. (y/N): ",
			sym("warn"), displayTitle, sessionName))
		if strings.ToLower(answer) != "y" {
			fmt.Println("Cancelled.")
			return
		}
	}

	fmt.Printf("%sSafely bringing down workspace: %s...\n", sym("stop"), sessionName)

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

	runTeardown(config.Teardown, projectDir, dry)

	fmt.Printf("   %sCleaning environment allocations...\n", sym("broom"))
	tmuxKillSession(sessionName)
	fmt.Printf("%sCompleted clean exit!\n", sym("check"))
}
