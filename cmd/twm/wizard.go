package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var stdinReader = bufio.NewReader(os.Stdin)

func prompt(msg string) string {
	fmt.Print(msg)
	line, err := stdinReader.ReadString('\n')
	if err != nil {
		fmt.Println()
		fatal("%sInput closed; aborting.", sym("error"))
	}
	return strings.TrimSpace(line)
}

// promptEnv asks for a single line of KEY=value assignments, re-prompting
// until it parses. current is offered as the default, so hitting Enter
// keeps what's already set; typing "none" clears it.
func promptEnv(label string, current map[string]string) map[string]string {
	shown := formatEnvAssignments(current)
	if shown == "" {
		shown = "(none)"
	}
	for {
		line := prompt(fmt.Sprintf("%s [%s] (KEY=value, space separated; 'none' to clear): ", label, shown))
		if line == "" {
			return current
		}
		if strings.ToLower(line) == "none" {
			return nil
		}
		env, errStr := parseEnvAssignments(line)
		if errStr != "" {
			fmt.Printf("  %s%s\n", sym("error"), errStr)
			continue
		}
		return env
	}
}

func validateWorkspaceCmd(alias string) {
	raw, _ := loadConfigRaw(alias)
	errs := validateConfig(raw)
	if len(errs) > 0 {
		errPrint("%s'%s' has %d problem(s):", sym("error"), alias, len(errs))
		for _, e := range errs {
			errPrint("   - %s", e)
		}
		os.Exit(1)
	}
	windows, _ := asInterfaceList(raw["windows"])
	fmt.Printf("%s'%s' looks valid (%d window(s)).\n", sym("check"), alias, len(windows))
}

// workspaceEntry is a summarized workspace profile, used both by the plain
// text `list` renderer and the dashboard TUI.
type workspaceEntry struct {
	alias, displayName string
	active             bool
	attached           bool // a client currently has the session open (only meaningful if active)
	windowsAlive       int  // windows currently in the session (only meaningful if active)
	windowsTotal       int  // windows defined in the profile (only meaningful if active)
	hasError           bool
	errMsg             string
}

// statusText renders a workspaceEntry's status for both the plain `list`
// output and the dashboard: "inactive", or "ACTIVE (attached|detached, n/m
// windows)" — the window count surfaces drift between what the profile
// defines and what's actually running (one closed by hand, say).
func statusText(e workspaceEntry) string {
	if !e.active {
		return "inactive"
	}
	state := "detached"
	if e.attached {
		state = "attached"
	}
	return fmt.Sprintf("ACTIVE (%s, %d/%d windows)", state, e.windowsAlive, e.windowsTotal)
}

// gatherWorkspaceEntries reads every profile out of the consolidated
// workspaces file (plus any not-yet-migrated standalone profile) and
// returns a summary of each, sorted by alias. It performs no output side
// effects, so it's safe to call from both the plain and TUI paths.
func gatherWorkspaceEntries() []workspaceEntry {
	doc, err := loadWorkspacesDoc()
	if err != nil {
		return []workspaceEntry{{alias: "workspaces.yml", hasError: true, errMsg: err.Error()}}
	}

	var names []string
	for alias := range doc {
		names = append(names, alias)
	}
	legacy := legacyWorkspaceFiles()
	for alias := range legacy {
		if _, ok := doc[alias]; !ok {
			names = append(names, alias)
		}
	}
	sort.Strings(names)

	var results []workspaceEntry
	for _, alias := range names {
		cfg, ok := doc[alias]
		if !ok {
			// Not migrated yet: read straight from its standalone file so it
			// still shows up in `list`, but flag it so the user knows to run
			// `upgrade`.
			data, err := os.ReadFile(filepath.Join(configDir, legacy[alias]))
			if err != nil {
				results = append(results, workspaceEntry{alias: alias, hasError: true, errMsg: err.Error()})
				continue
			}
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				results = append(results, workspaceEntry{alias: alias, hasError: true, errMsg: err.Error()})
				continue
			}
		}
		displayName := "Unnamed Project"
		if dn, ok := cfg["project_name_display"].(string); ok && dn != "" {
			displayName = dn
		}
		if _, migrated := doc[alias]; !migrated {
			displayName += " (run `upgrade` to migrate)"
		}
		entry := workspaceEntry{alias: alias, displayName: displayName, active: hasSession(alias)}
		if entry.active {
			entry.attached = sessionAttached(alias)
			entry.windowsAlive = windowCount(alias)
			if windows, ok := asInterfaceList(cfg["windows"]); ok {
				entry.windowsTotal = len(windows)
			}
		}
		results = append(results, entry)
	}
	return results
}

func listWorkspaces() {
	results := gatherWorkspaceEntries()
	if len(results) == 0 {
		fmt.Printf("%sNo workspaces found. Create one with '%s create <alias>', or add it to %s\n", sym("info"), progName(), workspacesPath)
		return
	}

	aliasW := 6
	nameW := 12
	for _, r := range results {
		if len(r.alias) > aliasW {
			aliasW = len(r.alias)
		}
		if !r.hasError && len(r.displayName) > nameW {
			nameW = len(r.displayName)
		}
	}

	fmt.Printf("%sAvailable Project Workspaces:\n", sym("list"))
	fmt.Println(strings.Repeat("-", aliasW+nameW+20))
	for _, r := range results {
		if r.hasError {
			fmt.Printf("   %s%s | Error parsing json config: %s\n", sym("error"), padRight(r.alias, aliasW), r.errMsg)
			continue
		}
		glyph := sym("inactive")
		if r.active {
			glyph = sym("active")
		}
		status := glyph + statusText(r)
		fmt.Printf("   %s | %s | %s\n", padRight(r.alias, aliasW), padRight(r.displayName, nameW), status)
	}
	fmt.Println(strings.Repeat("-", aliasW+nameW+20))
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

func editWorkspaceRaw(alias string) {
	if _, err := loadConfigRawErr(alias); err != nil {
		fatal("%s%s", sym("error"), err)
	}
	fmt.Printf("%sAll workspaces share one file — look for the '%s:' section.\n", sym("info"), alias)
	openInEditor(workspacesPath)
}

func editWorkspaceInteractive(alias string) {
	raw, _ := loadConfigRaw(alias)

	fmt.Printf("\n%sInteractive Editor for Profile: '%s' %s\n", sym("wave"), alias, sym("wave"))
	fmt.Println(strings.Repeat("-", 50))

	currentTitle, _ := raw["project_name_display"].(string)
	if currentTitle == "" {
		currentTitle = strings.ToUpper(alias)
	}
	newTitle := prompt(fmt.Sprintf("%sProject Title [%s]: ", sym("label"), currentTitle))
	if newTitle != "" {
		raw["project_name_display"] = newTitle
	}

	currentPath, _ := raw["project_path"].(string)
	for {
		newPath := prompt(fmt.Sprintf("%sProject Path, relative to %s [%s]: ", sym("folder"), settings.BaseDir, currentPath))
		if newPath == "" {
			break
		}
		resolved := resolveProjectPath(newPath, settings.BaseDir)
		if _, err := os.Stat(resolved); err != nil {
			if strings.ToLower(prompt(fmt.Sprintf("%s'%s' does not exist. Use it anyway? (y/N): ", sym("warn"), resolved))) == "y" {
				raw["project_path"] = newPath
				break
			}
			continue
		}
		raw["project_path"] = newPath
		break
	}

	fmt.Printf("\n%sWorkspace-wide environment (exported in every pane of every window):\n", sym("label"))
	if env := promptEnv("  Workspace env", stringMapFromRaw(raw["env"])); len(env) > 0 {
		raw["env"] = env
	} else {
		delete(raw, "env")
	}

	fmt.Printf("\n%sReviewing Window and Pane Configurations...\n", sym("package"))
	var updatedWindows []Window

	existingWindows, _ := asInterfaceList(raw["windows"])
	for idx, wRaw := range existingWindows {
		w, _ := wRaw.(map[string]interface{})
		fmt.Printf("\n--- Window #%d ---\n", idx+1)
		winName, _ := w["name"].(string)
		if winName == "" {
			winName = fmt.Sprintf("win-%d", idx+1)
		}
		newWinName := prompt(fmt.Sprintf("  Name [%s] (Type 'DELETE' to remove window): ", winName))

		if strings.ToUpper(newWinName) == "DELETE" {
			fmt.Printf("  %sRemoving window '%s'\n", sym("trash"), winName)
			continue
		}

		finalWinName := winName
		if newWinName != "" {
			finalWinName = newWinName
		}

		currentWinPath, _ := w["path"].(string)
		pathPrompt := currentWinPath
		if pathPrompt == "" {
			pathPrompt = "(project path)"
		}
		newWinPath := prompt(fmt.Sprintf("  Working dir override [%s]: ", pathPrompt))
		finalWinPath := currentWinPath
		if newWinPath != "" {
			finalWinPath = newWinPath
		}

		winEnv := promptEnv("  Window env (adds to workspace env)", stringMapFromRaw(w["env"]))

		currentOnStop, _ := w["on_stop"].(string)
		onStopPrompt := currentOnStop
		if onStopPrompt == "" {
			onStopPrompt = "(Ctrl-C)"
		}
		newOnStop := prompt(fmt.Sprintf("  Graceful exit command on 'down' [%s]: ", onStopPrompt))
		finalOnStop := currentOnStop
		if newOnStop != "" {
			finalOnStop = newOnStop
		}

		var updatedPanes []Pane
		existingPanes, _ := asInterfaceList(w["panes"])
		for pIdx, pRaw := range existingPanes {
			p, _ := pRaw.(map[string]interface{})
			currentCmd, _ := p["command"].(string)
			cmdPrompt := currentCmd
			if cmdPrompt == "" {
				cmdPrompt = "(shell)"
			}
			newCmd := prompt(fmt.Sprintf("    %sPane %d Command [%s]: ", sym("arrow"), pIdx+1, cmdPrompt))
			finalCmd := currentCmd
			if newCmd != "" {
				finalCmd = newCmd
			}
			currentPanePath, _ := p["path"].(string)
			paneEnv := promptEnv(fmt.Sprintf("    %sPane %d env (adds to window env)", sym("arrow"), pIdx+1), stringMapFromRaw(p["env"]))
			updatedPanes = append(updatedPanes, Pane{Command: finalCmd, Path: currentPanePath, Env: paneEnv})
		}

		for {
			addMore := strings.ToLower(prompt(fmt.Sprintf("    %sAdd an additional pane to window '%s'? (y/N): ", sym("plus"), finalWinName)))
			if addMore != "y" {
				break
			}
			extraCmd := prompt(fmt.Sprintf("      %sPane %d Command (blank for shell): ", sym("arrow"), len(updatedPanes)+1))
			updatedPanes = append(updatedPanes, Pane{Command: extraCmd})
		}

		updatedWindows = append(updatedWindows, Window{Name: finalWinName, Path: finalWinPath, OnStop: finalOnStop, Env: winEnv, Panes: updatedPanes})
	}

	for {
		addWin := strings.ToLower(prompt(fmt.Sprintf("\n%sAdd an entirely new window to this profile? (y/N): ", sym("plus"))))
		if addWin != "y" {
			break
		}
		newName := prompt(fmt.Sprintf("  Window #%d Name: ", len(updatedWindows)+1))
		if newName == "" {
			continue
		}

		newWinPath := prompt("  Working dir override, relative to project path (blank = project path): ")
		newWinEnv := promptEnv("  Window env (adds to workspace env)", nil)

		var newPanes []Pane
		paneCounter := 1
		for {
			newCmd := prompt(fmt.Sprintf("    %sPane %d Command (blank for shell): ", sym("arrow"), paneCounter))
			newPanes = append(newPanes, Pane{Command: newCmd})
			if strings.ToLower(prompt(fmt.Sprintf("    %sAdd another side-by-side pane split? (y/N): ", sym("plus")))) != "y" {
				break
			}
			paneCounter++
		}

		newOnStop := prompt("  Graceful exit command on 'down' (blank = Ctrl-C): ")

		updatedWindows = append(updatedWindows, Window{Name: newName, Path: newWinPath, OnStop: newOnStop, Env: newWinEnv, Panes: newPanes})
	}

	raw["windows"] = updatedWindows

	existingTeardown := teardownFromRaw(raw["teardown"])
	fmt.Printf("\n%sTeardown commands (blocking, run before the session is killed):\n", sym("broom"))
	if len(existingTeardown) > 0 {
		for i, cmd := range existingTeardown {
			fmt.Printf("   %d. %s\n", i+1, cmd)
		}
	} else {
		fmt.Println("   (none set)")
	}

	updatedTeardown := existingTeardown
	if strings.ToLower(prompt("   Replace teardown commands? (y/N): ")) == "y" {
		updatedTeardown = []string{}
		for {
			cmd := prompt(fmt.Sprintf("   %sTeardown command #%d (blank to finish): ", sym("arrow"), len(updatedTeardown)+1))
			if cmd == "" {
				break
			}
			updatedTeardown = append(updatedTeardown, cmd)
		}
	}
	raw["teardown"] = updatedTeardown

	newProjectPath, _ := raw["project_path"].(string)
	errs := validateTypedConfig(newProjectPath, updatedWindows)
	if len(errs) > 0 {
		errPrint("%sNot saving — config is invalid:", sym("error"))
		for _, e := range errs {
			errPrint("   - %s", e)
		}
		os.Exit(1)
	}

	savedPath, err := saveConfigRaw(alias, raw)
	if err != nil {
		fatal("%sFailed to save %s: %v", sym("error"), alias, err)
	}
	fmt.Printf("\n%sProfile '%s' updated successfully! (%s)\n", sym("check"), alias, savedPath)
}

func teardownFromRaw(v interface{}) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []interface{}:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	default:
		return nil
	}
}

func createWorkspaceWizard(presetAlias string) {
	fmt.Printf("%sInteractive TMUX Workspace Creator %s\n", sym("sparkle"), sym("sparkle"))
	fmt.Println(strings.Repeat("-", 40))

	alias := strings.ToLower(strings.TrimSpace(presetAlias))
	for {
		if aliasErr := validateAlias(alias); aliasErr == "" {
			break
		} else if alias != "" {
			fmt.Printf("%sInvalid alias (%s).\n", sym("error"), aliasErr)
		}
		alias = strings.ToLower(prompt(fmt.Sprintf("%sEnter short workspace alias (e.g., am): ", sym("label"))))
	}

	if findConfigPath(alias) != "" {
		overwrite := strings.ToLower(prompt(fmt.Sprintf("%sAlias '%s' already exists. Overwrite? (y/N): ", sym("warn"), alias)))
		if overwrite != "y" {
			fmt.Printf("%sOperation cancelled.\n", sym("error"))
			os.Exit(0)
		}
	}

	displayName := prompt(fmt.Sprintf("%sEnter friendly project title: ", sym("label")))
	if displayName == "" {
		displayName = strings.ToUpper(alias)
	}

	var rawPath string
	for {
		rawPath = prompt(fmt.Sprintf("%sProject directory (relative to %s, or absolute/~) [.]: ", sym("folder"), settings.BaseDir))
		if rawPath == "" {
			rawPath = "."
		}
		resolved := resolveProjectPath(rawPath, settings.BaseDir)
		if _, err := os.Stat(resolved); err != nil {
			if strings.ToLower(prompt(fmt.Sprintf("%s'%s' does not exist. Create it now? (y/N): ", sym("warn"), resolved))) == "y" {
				os.MkdirAll(resolved, 0755)
				break
			}
			continue
		}
		break
	}

	fmt.Printf("\n%sWorkspace-wide environment: exported in every pane of every window.\n", sym("label"))
	projectEnv := promptEnv("   Workspace env", nil)

	var windows []Window
	fmt.Printf("\n%sLet's configure your windows and panes...\n", sym("tool"))

	for {
		winName := prompt(fmt.Sprintf("\n%sWindow #%d Name (or hit Enter to finish): ", sym("package"), len(windows)+1))
		if winName == "" {
			if len(windows) == 0 {
				fmt.Printf("%sYou must create at least one window layout.\n", sym("error"))
				continue
			}
			break
		}

		winPath := prompt(fmt.Sprintf("   %sWorking dir override, relative to project path (blank = project path): ", sym("folder")))
		winEnv := promptEnv(fmt.Sprintf("   %sWindow env (adds to workspace env)", sym("label")), nil)

		var panes []Pane
		paneCount := 1
		for {
			cmd := prompt(fmt.Sprintf("   %sPane %d command (blank for shell): ", sym("arrow"), paneCount))
			panes = append(panes, Pane{Command: cmd})
			if strings.ToLower(prompt(fmt.Sprintf("   %sAdd an additional side-by-side pane split? (y/N): ", sym("plus")))) != "y" {
				break
			}
			paneCount++
		}

		onStop := prompt(fmt.Sprintf("   %sGraceful exit command on 'down' instead of Ctrl-C (blank = none, e.g. /exit): ", sym("stop")))

		windows = append(windows, Window{Name: winName, Path: winPath, OnStop: onStop, Env: winEnv, Panes: panes})
	}

	var teardown []string
	fmt.Printf("\n%sTeardown commands (run and waited on before the session is killed).\n", sym("broom"))
	for {
		cmd := prompt(fmt.Sprintf("   %sTeardown command #%d (blank to finish): ", sym("arrow"), len(teardown)+1))
		if cmd == "" {
			break
		}
		teardown = append(teardown, cmd)
	}
	if teardown == nil {
		teardown = []string{}
	}

	workspaceData := map[string]interface{}{
		"project_name_display": displayName,
		"project_path":         rawPath,
		"windows":              windows,
		"teardown":             teardown,
	}
	if len(projectEnv) > 0 {
		workspaceData["env"] = projectEnv
	}

	savedPath, err := saveConfigRaw(alias, workspaceData)
	if err != nil {
		fatal("%sFailed to save %s: %v", sym("error"), alias, err)
	}
	fmt.Printf("\n%sWorkspace '%s' saved to: %s\n", sym("check"), alias, savedPath)
}
