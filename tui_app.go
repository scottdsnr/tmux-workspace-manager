package main

import (
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// Messages a child screen sends to ask the router to switch screens or
// signal completion. Bubble Tea sub-models can't reach into their parent
// directly, so navigation flows through these Cmd-produced messages instead.
type (
	pushWizardMsg struct {
		create bool
		alias  string
	}
	pushSettingsMsg     struct{}
	pushValidateMsg     struct{ alias string }
	pushUpMsg           struct{ alias string }
	pushDownConfirmMsg  struct{ alias, title string }
	pushDownProgressMsg struct{ alias string }

	// screenFinishedMsg signals a child screen is done. If exec is set, the
	// router hands the terminal to it via tea.ExecProcess (suspending the
	// Bubble Tea renderer) before returning to the dashboard or quitting.
	screenFinishedMsg struct{ exec *exec.Cmd }
	execFinishedMsg   struct{ err error }
)

type screenKind int

const (
	screenBoot screenKind = iota
	screenDashboard
	screenProgress
	screenConfirm
	screenWizard
	screenSettings
	screenValidate
)

// appModel is the root router. In "embedded" mode (bare invocation or
// `list`) it always returns to the dashboard when a child screen finishes;
// otherwise (a direct CLI verb like `up <alias>`) it quits once the single
// requested screen is done.
type appModel struct {
	embedded      bool
	screen        screenKind
	bootCmd       tea.Cmd
	width, height int

	dashboard dashboardModel
	progress  *progressModel
	confirm   *confirmModel
	wizard    *wizardModel
	settings  *settingsModel
	validate  *validateModel
}

func newDashboardApp() appModel {
	return appModel{embedded: true, screen: screenDashboard, dashboard: newDashboardModel()}
}

func newDirectApp(bootCmd tea.Cmd) appModel {
	return appModel{embedded: false, screen: screenBoot, bootCmd: bootCmd}
}

func (m appModel) Init() tea.Cmd {
	if m.screen == screenDashboard {
		return m.dashboard.Init()
	}
	return m.bootCmd
}

func (m appModel) toDashboard() (tea.Model, tea.Cmd) {
	if !m.embedded {
		return m, tea.Quit
	}
	m.screen = screenDashboard
	m.progress, m.confirm, m.wizard, m.settings, m.validate = nil, nil, nil, nil, nil
	m.dashboard = newDashboardModel()
	initCmd := m.dashboard.Init()
	if m.width > 0 {
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, tea.Batch(initCmd, cmd)
	}
	return m, initCmd
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Only the dashboard's list actually reflows on size; other screens
		// render as fixed-size boxes. Forwarding this to a zero-value
		// (never-initialized) dashboardModel in direct/boot mode would panic
		// inside bubbles/list, so only forward when it's live.
		if m.screen == screenDashboard {
			var cmd tea.Cmd
			m.dashboard, cmd = m.dashboard.Update(msg)
			return m, cmd
		}
		return m, nil

	case pushWizardMsg:
		m.screen = screenWizard
		m.wizard = newWizardModel(msg.create, msg.alias)
		return m, m.wizard.Init()

	case pushSettingsMsg:
		m.screen = screenSettings
		m.settings = newSettingsModel()
		return m, m.settings.Init()

	case pushValidateMsg:
		m.screen = screenValidate
		m.validate = newValidateModel(msg.alias)
		return m, m.validate.Init()

	case pushUpMsg:
		m.screen = screenProgress
		m.progress = newUpProgressModel(msg.alias)
		return m, m.progress.Init()

	case pushDownConfirmMsg:
		m.screen = screenConfirm
		m.confirm = newConfirmModel(msg.alias, msg.title)
		return m, m.confirm.Init()

	case pushDownProgressMsg:
		m.screen = screenProgress
		m.progress = newDownProgressModel(msg.alias)
		return m, m.progress.Init()

	case screenFinishedMsg:
		if msg.exec != nil {
			cmd := msg.exec
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return execFinishedMsg{err: err} })
		}
		return m.toDashboard()

	case execFinishedMsg:
		return m.toDashboard()
	}

	switch m.screen {
	case screenDashboard:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd
	case screenProgress:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd
	case screenConfirm:
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		return m, cmd
	case screenWizard:
		var cmd tea.Cmd
		m.wizard, cmd = m.wizard.Update(msg)
		return m, cmd
	case screenSettings:
		var cmd tea.Cmd
		m.settings, cmd = m.settings.Update(msg)
		return m, cmd
	case screenValidate:
		var cmd tea.Cmd
		m.validate, cmd = m.validate.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m appModel) View() string {
	switch m.screen {
	case screenDashboard:
		return m.dashboard.View()
	case screenProgress:
		if m.progress != nil {
			return m.progress.View()
		}
	case screenConfirm:
		if m.confirm != nil {
			return m.confirm.View()
		}
	case screenWizard:
		if m.wizard != nil {
			return m.wizard.View()
		}
	case screenSettings:
		if m.settings != nil {
			return m.settings.View()
		}
	case screenValidate:
		if m.validate != nil {
			return m.validate.View()
		}
	}
	return ""
}

// newUpProgressModel builds the progress screen for `up`: on success (or
// "already running"), pressing any key hands the terminal to tmux attach.
func newUpProgressModel(alias string) *progressModel {
	job := func(report reportFunc) error { return runUp(alias, false, report) }
	onContinue := func(err error) tea.Cmd {
		if err != nil {
			return func() tea.Msg { return screenFinishedMsg{} }
		}
		return func() tea.Msg { return screenFinishedMsg{exec: attachExecCmd(alias)} }
	}
	title := "Building '" + alias + "'"
	hint := "press any key to attach"
	return newProgressModel(title, job, hint, onContinue)
}

// newDownProgressModel builds the progress screen for `down`.
func newDownProgressModel(alias string) *progressModel {
	job := func(report reportFunc) error { return runDown(alias, false, false, report) }
	onContinue := func(error) tea.Cmd {
		return func() tea.Msg { return screenFinishedMsg{} }
	}
	title := "Tearing down '" + alias + "'"
	return newProgressModel(title, job, "", onContinue)
}

// attachExecCmd builds the exec.Cmd that jumps into alias's session,
// suitable for handing to tea.ExecProcess via screenFinishedMsg: attach if
// we're in a plain terminal, switch-client (near-instant, non-blocking) if
// this TUI is itself already running inside tmux.
func attachExecCmd(alias string) *exec.Cmd {
	args := attachArgs(alias)
	return exec.Command(args[0], args[1:]...)
}

func runTUIProgram(m tea.Model) {
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fatal("%sTUI error: %v", sym("error"), err)
	}
}

// runDashboardTUI launches the full interactive dashboard app.
func runDashboardTUI() {
	runTUIProgram(newDashboardApp())
}

// runUpTUI launches straight into the up-progress screen for alias, then
// quits once it (and any resulting attach) finishes.
func runUpTUI(alias string) {
	runTUIProgram(newDirectApp(func() tea.Msg { return pushUpMsg{alias: alias} }))
}

// runDownTUI launches straight into the down flow for alias: a confirmation
// screen (unless skipped), then progress, then quits.
func runDownTUI(alias string, assumeYes bool) {
	if !hasSession(alias) {
		fmt.Printf("%sNo active session found for alias '%s'\n", sym("info"), alias)
		return
	}
	if assumeYes || !settings.ConfirmDown {
		runTUIProgram(newDirectApp(func() tea.Msg { return pushDownProgressMsg{alias: alias} }))
		return
	}
	config, _ := loadConfigTypedErr(alias)
	title := config.ProjectNameDisplay
	if title == "" {
		title = alias
	}
	runTUIProgram(newDirectApp(func() tea.Msg { return pushDownConfirmMsg{alias: alias, title: title} }))
}

// runWizardTUI launches straight into the create/edit wizard, then quits.
func runWizardTUI(create bool, alias string) {
	runTUIProgram(newDirectApp(func() tea.Msg { return pushWizardMsg{create: create, alias: alias} }))
}

// runSettingsTUI launches straight into the settings form, then quits.
func runSettingsTUI() {
	runTUIProgram(newDirectApp(func() tea.Msg { return pushSettingsMsg{} }))
}
