package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

type wizardStep int

const (
	stepAlias wizardStep = iota
	stepTitle
	stepPath
	stepWindows
	stepTeardown
	stepReview
)

type wizardWindowDraft struct {
	name, path, onStop string
	panes              []string
}

// wizardModel drives the create/edit flow as a stack of steps, with two
// small nested "list editors" (windows, and each window's panes/teardown
// commands) rather than separate Bubble Tea sub-programs.
type wizardModel struct {
	editing bool
	alias   string
	step    wizardStep
	err     string

	width, height int

	aliasInput textinput.Model
	titleInput textinput.Model
	pathInput  textinput.Model

	overwriteConfirming bool
	pathConfirming      bool
	pathWarning         string

	windows      []wizardWindowDraft
	windowCursor int

	winEditing    bool
	winIsNew      bool
	winIndex      int
	winFocus      int // 0=name 1=path 2=on_stop 3=panes
	winNameInput  textinput.Model
	winPathInput  textinput.Model
	winStopInput  textinput.Model
	winPanes      []string
	paneCursor    int
	addingPane    bool
	paneEditIndex int
	paneInput     textinput.Model

	teardown          []string
	teardownCursor    int
	addingTeardown    bool
	teardownEditIndex int
	teardownInput     textinput.Model
}

func newWizardModel(create bool, alias string) *wizardModel {
	m := &wizardModel{editing: !create}
	m.aliasInput = newSettingsInput("")
	m.titleInput = newSettingsInput("")
	m.pathInput = newSettingsInput("")

	if create {
		m.aliasInput.SetValue(strings.ToLower(strings.TrimSpace(alias)))
		m.step = stepAlias
		m.aliasInput.Focus()
		return m
	}

	m.alias = alias
	raw, err := loadConfigRawErr(alias)
	if err != nil {
		m.editing = false
		m.step = stepAlias
		m.aliasInput.Focus()
		m.err = err.Error()
		return m
	}

	if dn, ok := raw["project_name_display"].(string); ok {
		m.titleInput.SetValue(dn)
	}
	if pp, ok := raw["project_path"].(string); ok {
		m.pathInput.SetValue(pp)
	}
	windowsRaw, _ := asInterfaceList(raw["windows"])
	for _, wRaw := range windowsRaw {
		w, _ := wRaw.(map[string]interface{})
		name, _ := w["name"].(string)
		path, _ := w["path"].(string)
		onStop, _ := w["on_stop"].(string)
		var panes []string
		panesRaw, _ := asInterfaceList(w["panes"])
		for _, pRaw := range panesRaw {
			p, _ := pRaw.(map[string]interface{})
			cmd, _ := p["command"].(string)
			panes = append(panes, cmd)
		}
		if len(panes) == 0 {
			panes = []string{""}
		}
		m.windows = append(m.windows, wizardWindowDraft{name: name, path: path, onStop: onStop, panes: panes})
	}
	m.teardown = teardownFromRaw(raw["teardown"])
	m.step = stepTitle
	m.titleInput.Focus()
	return m
}

func (m *wizardModel) Init() tea.Cmd { return textinput.Blink }

func (m *wizardModel) Update(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, func() tea.Msg { return screenFinishedMsg{} }
		case "ctrl+r":
			if cmd := m.rawEditCmd(); cmd != nil {
				return m, cmd
			}
			return m, nil
		}
	}

	switch {
	case m.overwriteConfirming:
		return m.updateOverwriteConfirm(msg)
	case m.pathConfirming:
		return m.updatePathConfirm(msg)
	case m.winEditing:
		return m.updateWindowEdit(msg)
	}

	switch m.step {
	case stepAlias:
		return m.updateAlias(msg)
	case stepTitle:
		return m.updateTitle(msg)
	case stepPath:
		return m.updatePath(msg)
	case stepWindows:
		return m.updateWindowsList(msg)
	case stepTeardown:
		return m.updateTeardown(msg)
	case stepReview:
		return m.updateReview(msg)
	}
	return m, nil
}

func (m *wizardModel) updateAlias(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			return m, func() tea.Msg { return screenFinishedMsg{} }
		case "enter":
			alias := strings.ToLower(strings.TrimSpace(m.aliasInput.Value()))
			if errStr := validateAlias(alias); errStr != "" {
				m.err = "invalid alias: " + errStr
				return m, nil
			}
			m.err = ""
			m.alias = alias
			if findConfigPath(alias) != "" {
				m.overwriteConfirming = true
				return m, nil
			}
			m.step = stepTitle
			return m, m.titleInput.Focus()
		}
	}
	var cmd tea.Cmd
	m.aliasInput, cmd = m.aliasInput.Update(msg)
	return m, cmd
}

func (m *wizardModel) updateOverwriteConfirm(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "y", "Y":
			m.overwriteConfirming = false
			m.step = stepTitle
			return m, m.titleInput.Focus()
		case "n", "N", "esc":
			m.overwriteConfirming = false
		}
	}
	return m, nil
}

func (m *wizardModel) updateTitle(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			if m.editing {
				return m, func() tea.Msg { return screenFinishedMsg{} }
			}
			m.step = stepAlias
			return m, m.aliasInput.Focus()
		case "enter":
			m.step = stepPath
			return m, m.pathInput.Focus()
		}
	}
	var cmd tea.Cmd
	m.titleInput, cmd = m.titleInput.Update(msg)
	return m, cmd
}

func (m *wizardModel) goToWindows() (*wizardModel, tea.Cmd) {
	m.step = stepWindows
	if m.windowCursor >= len(m.windows) {
		m.windowCursor = 0
	}
	return m, nil
}

func (m *wizardModel) updatePath(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.step = stepTitle
			return m, m.titleInput.Focus()
		case "enter":
			raw := strings.TrimSpace(m.pathInput.Value())
			if raw == "" {
				raw = "."
				m.pathInput.SetValue(raw)
			}
			resolved := resolveProjectPath(raw, settings.BaseDir)
			if _, err := os.Stat(resolved); err != nil {
				m.pathWarning = resolved
				m.pathConfirming = true
				return m, nil
			}
			return m.goToWindows()
		}
	}
	var cmd tea.Cmd
	m.pathInput, cmd = m.pathInput.Update(msg)
	return m, cmd
}

func (m *wizardModel) updatePathConfirm(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "c", "C":
			os.MkdirAll(resolveProjectPath(strings.TrimSpace(m.pathInput.Value()), settings.BaseDir), 0755)
			m.pathConfirming = false
			return m.goToWindows()
		case "u", "U":
			m.pathConfirming = false
			return m.goToWindows()
		case "esc", "n", "N":
			m.pathConfirming = false
		}
	}
	return m, nil
}

func (m *wizardModel) updateWindowsList(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.step = stepPath
			return m, m.pathInput.Focus()
		case "up", "k":
			if m.windowCursor > 0 {
				m.windowCursor--
			}
		case "down", "j":
			if m.windowCursor < len(m.windows)-1 {
				m.windowCursor++
			}
		case "a":
			m.startWindowEdit(-1)
		case "enter":
			if len(m.windows) > 0 {
				m.startWindowEdit(m.windowCursor)
			}
		case "d", "x":
			if len(m.windows) > 0 {
				m.windows = append(m.windows[:m.windowCursor], m.windows[m.windowCursor+1:]...)
				if m.windowCursor >= len(m.windows) && m.windowCursor > 0 {
					m.windowCursor--
				}
			}
		case "n":
			if len(m.windows) == 0 {
				m.err = "add at least one window first"
				return m, nil
			}
			m.err = ""
			m.step = stepTeardown
			m.teardownCursor = 0
		}
	}
	return m, nil
}

func (m *wizardModel) startWindowEdit(idx int) {
	m.winEditing = true
	m.winFocus = 0
	m.paneCursor = 0
	m.addingPane = false
	m.err = ""

	if idx < 0 {
		m.winIsNew = true
		m.winIndex = -1
		m.winNameInput = newSettingsInput("")
		m.winPathInput = newSettingsInput("")
		m.winStopInput = newSettingsInput("")
		m.winPanes = nil
	} else {
		m.winIsNew = false
		m.winIndex = idx
		d := m.windows[idx]
		m.winNameInput = newSettingsInput(d.name)
		m.winPathInput = newSettingsInput(d.path)
		m.winStopInput = newSettingsInput(d.onStop)
		m.winPanes = append([]string{}, d.panes...)
		if len(m.winPanes) == 0 {
			m.winPanes = []string{""}
		}
	}
	m.winNameInput.Focus()
}

func (m *wizardModel) blurWindowFields() {
	m.winNameInput.Blur()
	m.winPathInput.Blur()
	m.winStopInput.Blur()
}

func (m *wizardModel) focusWindowField() tea.Cmd {
	switch m.winFocus {
	case 0:
		return m.winNameInput.Focus()
	case 1:
		return m.winPathInput.Focus()
	case 2:
		return m.winStopInput.Focus()
	}
	return nil
}

func (m *wizardModel) saveWindowEdit() (*wizardModel, tea.Cmd) {
	name := strings.TrimSpace(m.winNameInput.Value())
	if name == "" {
		m.err = "window name is required"
		m.winFocus = 0
		return m, m.winNameInput.Focus()
	}
	if len(m.winPanes) == 0 {
		m.err = "add at least one pane before saving"
		m.winFocus = 3
		return m, nil
	}
	draft := wizardWindowDraft{
		name:   name,
		path:   strings.TrimSpace(m.winPathInput.Value()),
		onStop: strings.TrimSpace(m.winStopInput.Value()),
		panes:  append([]string{}, m.winPanes...),
	}
	if m.winIsNew {
		m.windows = append(m.windows, draft)
		m.windowCursor = len(m.windows) - 1
	} else {
		m.windows[m.winIndex] = draft
	}
	m.winEditing = false
	m.err = ""
	return m, nil
}

func (m *wizardModel) updateWindowEdit(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if m.addingPane {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "enter":
				val := strings.TrimSpace(m.paneInput.Value())
				if m.paneEditIndex >= 0 {
					m.winPanes[m.paneEditIndex] = val
				} else {
					m.winPanes = append(m.winPanes, val)
					m.paneCursor = len(m.winPanes) - 1
				}
				m.addingPane = false
				return m, nil
			case "esc":
				m.addingPane = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.paneInput, cmd = m.paneInput.Update(msg)
		return m, cmd
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.winEditing = false
			return m, nil
		case "s":
			return m.saveWindowEdit()
		case "tab":
			m.blurWindowFields()
			m.winFocus = (m.winFocus + 1) % 4
			return m, m.focusWindowField()
		case "shift+tab":
			m.blurWindowFields()
			m.winFocus = (m.winFocus - 1 + 4) % 4
			return m, m.focusWindowField()
		}

		if m.winFocus == 3 {
			switch key.String() {
			case "up", "k":
				if m.paneCursor > 0 {
					m.paneCursor--
				}
			case "down", "j":
				if m.paneCursor < len(m.winPanes)-1 {
					m.paneCursor++
				}
			case "a":
				m.addingPane = true
				m.paneEditIndex = -1
				m.paneInput = newSettingsInput("")
				m.paneInput.Focus()
			case "enter":
				if len(m.winPanes) > 0 {
					m.addingPane = true
					m.paneEditIndex = m.paneCursor
					m.paneInput = newSettingsInput(m.winPanes[m.paneCursor])
					m.paneInput.Focus()
				}
			case "d", "x":
				if len(m.winPanes) > 0 {
					m.winPanes = append(m.winPanes[:m.paneCursor], m.winPanes[m.paneCursor+1:]...)
					if m.paneCursor >= len(m.winPanes) && m.paneCursor > 0 {
						m.paneCursor--
					}
				}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.winFocus {
	case 0:
		m.winNameInput, cmd = m.winNameInput.Update(msg)
	case 1:
		m.winPathInput, cmd = m.winPathInput.Update(msg)
	case 2:
		m.winStopInput, cmd = m.winStopInput.Update(msg)
	}
	return m, cmd
}

func (m *wizardModel) updateTeardown(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if m.addingTeardown {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "enter":
				val := strings.TrimSpace(m.teardownInput.Value())
				if val != "" {
					if m.teardownEditIndex >= 0 {
						m.teardown[m.teardownEditIndex] = val
					} else {
						m.teardown = append(m.teardown, val)
						m.teardownCursor = len(m.teardown) - 1
					}
				}
				m.addingTeardown = false
				return m, nil
			case "esc":
				m.addingTeardown = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.teardownInput, cmd = m.teardownInput.Update(msg)
		return m, cmd
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.step = stepWindows
			return m, nil
		case "up", "k":
			if m.teardownCursor > 0 {
				m.teardownCursor--
			}
		case "down", "j":
			if m.teardownCursor < len(m.teardown)-1 {
				m.teardownCursor++
			}
		case "a":
			m.addingTeardown = true
			m.teardownEditIndex = -1
			m.teardownInput = newSettingsInput("")
			m.teardownInput.Focus()
		case "enter":
			if len(m.teardown) > 0 {
				m.addingTeardown = true
				m.teardownEditIndex = m.teardownCursor
				m.teardownInput = newSettingsInput(m.teardown[m.teardownCursor])
				m.teardownInput.Focus()
			}
		case "d", "x":
			if len(m.teardown) > 0 {
				m.teardown = append(m.teardown[:m.teardownCursor], m.teardown[m.teardownCursor+1:]...)
				if m.teardownCursor >= len(m.teardown) && m.teardownCursor > 0 {
					m.teardownCursor--
				}
			}
		case "n":
			m.step = stepReview
		}
	}
	return m, nil
}

func (m *wizardModel) updateReview(msg tea.Msg) (*wizardModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.step = stepTeardown
		case "s":
			return m.saveWorkspace()
		}
	}
	return m, nil
}

// buildRawConfig assembles the workspace map from the wizard's current
// draft state, in the same shape saveConfigRaw expects. It's shared by
// saveWorkspace, the live preview pane, and the raw-edit escape hatch, so
// all three always agree on what the draft currently looks like.
func (m *wizardModel) buildRawConfig() map[string]interface{} {
	var windows []Window
	for _, d := range m.windows {
		var panes []Pane
		for _, p := range d.panes {
			panes = append(panes, Pane{Command: p})
		}
		windows = append(windows, Window{Name: d.name, Path: d.path, OnStop: d.onStop, Panes: panes})
	}
	projectPath := strings.TrimSpace(m.pathInput.Value())
	if projectPath == "" {
		projectPath = "."
	}

	displayName := strings.TrimSpace(m.titleInput.Value())
	if displayName == "" {
		displayName = strings.ToUpper(m.alias)
	}

	raw := map[string]interface{}{
		"project_name_display": displayName,
		"project_path":         projectPath,
		"windows":              windows,
		"teardown":             append([]string{}, m.teardown...),
	}
	if m.editing {
		if existing, err := loadConfigRawErr(m.alias); err == nil {
			for k, v := range existing {
				if _, known := raw[k]; !known {
					raw[k] = v
				}
			}
		}
	}
	return raw
}

func (m *wizardModel) saveWorkspace() (*wizardModel, tea.Cmd) {
	raw := m.buildRawConfig()
	windows, _ := raw["windows"].([]Window)
	projectPath, _ := raw["project_path"].(string)

	if errs := validateTypedConfig(projectPath, windows); len(errs) > 0 {
		m.err = "cannot save: " + strings.Join(errs, "; ")
		return m, nil
	}

	if _, err := saveConfigRaw(m.alias, raw); err != nil {
		m.err = "failed to save: " + err.Error()
		return m, nil
	}
	return m, func() tea.Msg { return screenFinishedMsg{} }
}

// rawEditCmd saves the current draft (even if incomplete) and hands the
// terminal to $EDITOR on the resulting file, as an escape hatch out of the
// step-by-step flow. It requires an alias, since there's nowhere to save to
// before one has been chosen.
func (m *wizardModel) rawEditCmd() tea.Cmd {
	if strings.TrimSpace(m.alias) == "" {
		m.err = "choose an alias before editing raw YAML"
		return nil
	}
	path, err := saveConfigRaw(m.alias, m.buildRawConfig())
	if err != nil {
		m.err = "failed to save: " + err.Error()
		return nil
	}
	alias := m.alias
	return func() tea.Msg { return screenFinishedMsg{exec: rawEditExecCmd(path), validateAfterExec: alias} }
}

// previewYAML renders the current draft exactly as saveWorkspace would
// write it, for the live preview pane.
func (m *wizardModel) previewYAML() string {
	node, err := encodeOrdered(m.buildRawConfig(), topLevelOrder)
	if err != nil {
		return err.Error()
	}
	data, err := yaml.Marshal(node)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func (m *wizardModel) View() string {
	// Every viewXxx below wraps its panel in a leading/trailing blank line
	// on its own, which is fine rendered alone but would misalign the two
	// panels by a row when joined side by side, so normalize before (and
	// re-add after) combining them.
	main := strings.Trim(m.viewMain(), "\n")
	if preview := m.viewPreview(main); preview != "" {
		return "\n" + lipgloss.JoinHorizontal(lipgloss.Top, main, preview) + "\n"
	}
	return "\n" + main + "\n"
}

func (m *wizardModel) viewMain() string {
	switch {
	case m.overwriteConfirming:
		return m.viewOverwriteConfirm()
	case m.pathConfirming:
		return m.viewPathConfirm()
	case m.winEditing:
		return m.viewWindowEdit()
	}
	switch m.step {
	case stepAlias:
		return m.viewAlias()
	case stepTitle:
		return m.viewSimpleField("Project title", "title", m.titleInput, "enter continue   esc back")
	case stepPath:
		return m.viewSimpleField("Project path", fmt.Sprintf("path, relative to %s", settings.BaseDir), m.pathInput, "enter continue   esc back")
	case stepWindows:
		return m.viewWindowsList()
	case stepTeardown:
		return m.viewTeardown()
	case stepReview:
		return m.viewReview()
	}
	return ""
}

// viewPreview renders a live YAML preview pane alongside main, sized to
// whatever room is left on the terminal. It returns "" when there isn't
// enough width to be worth showing (a narrow terminal keeps the plain
// single-panel layout).
func (m *wizardModel) viewPreview(main string) string {
	if m.width <= 0 {
		return ""
	}
	avail := m.width - lipgloss.Width(main) - 2
	if avail < 40 {
		return ""
	}

	alias := m.alias
	if alias == "" {
		alias = "<alias>"
	}
	body := titleStyle.Render("Preview") + "\n" +
		subtleStyle.Render(alias+".yml") + "\n\n" +
		strings.TrimRight(m.previewYAML(), "\n") + "\n\n" +
		subtleStyle.Render("^R edit raw yaml")

	contentWidth := avail - 2
	if contentWidth > 64 {
		contentWidth = 64 // wide terminals still get a readable column, not a stretched one
	}
	style := panelStyle
	if contentWidth > 0 {
		style = style.Width(contentWidth)
	}
	return style.Render(body)
}

func (m *wizardModel) viewAlias() string {
	title := "New workspace"
	if m.err != "" && !m.editing {
		title = "New workspace"
	}
	body := titleStyle.Render(title) + "\n\n" +
		fieldLabel.Render("alias") + "\n" + m.aliasInput.View() + "\n\n" +
		subtleStyle.Render("enter continue   esc cancel")
	if m.err != "" {
		body += "\n\n" + errorStyle.Render(m.err)
	}
	return "\n" + panelStyle.Render(body) + "\n"
}

func (m *wizardModel) viewSimpleField(title, label string, input textinput.Model, help string) string {
	body := titleStyle.Render(title) + "\n\n" +
		fieldLabel.Render(label) + "\n" + input.View() + "\n\n" +
		subtleStyle.Render(help+"   ^R raw edit")
	if m.err != "" {
		body += "\n\n" + errorStyle.Render(m.err)
	}
	return "\n" + panelStyle.Render(body) + "\n"
}

func (m *wizardModel) viewOverwriteConfirm() string {
	body := warnStyle.Render(sym("warn")+"Alias exists") + "\n\n" +
		fmt.Sprintf("'%s' already has a profile. Overwrite it?", m.alias) + "\n\n" +
		subtleStyle.Render("[y] overwrite   [n/esc] cancel")
	return "\n" + panelStyle.Render(body) + "\n"
}

func (m *wizardModel) viewPathConfirm() string {
	body := warnStyle.Render(sym("warn")+"Path does not exist") + "\n\n" +
		m.pathWarning + "\n\n" +
		subtleStyle.Render("[c] create it   [u] use anyway   [esc] back")
	return "\n" + panelStyle.Render(body) + "\n"
}

func (m *wizardModel) viewWindowsList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Windows (%d)", len(m.windows))))
	b.WriteString("\n\n")
	if len(m.windows) == 0 {
		b.WriteString(subtleStyle.Render("  no windows yet — press 'a' to add one"))
		b.WriteString("\n")
	}
	for i, w := range m.windows {
		cursor := "  "
		summary := fmt.Sprintf("%s (%d pane(s))", w.name, len(w.panes))
		if i == m.windowCursor {
			cursor = accentStyle.Render("▸ ")
			summary = accentStyle.Render(summary)
		}
		b.WriteString(cursor + summary + "\n")
	}
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err) + "\n\n")
	}
	b.WriteString(subtleStyle.Render("a add   enter edit   d delete   n next: teardown   esc back   ^R raw edit"))
	return "\n" + panelStyle.Render(strings.TrimRight(b.String(), "\n")) + "\n"
}

func (m *wizardModel) viewWindowEdit() string {
	label := func(i int, text string) string {
		if m.winFocus == i && !m.addingPane {
			return focusedFieldLabel.Render("▸ " + text)
		}
		return "  " + fieldLabel.Render(text)
	}

	var b strings.Builder
	title := "Add window"
	if !m.winIsNew {
		title = "Edit window"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("%-26s %s\n", label(0, "name"), m.winNameInput.View()))
	b.WriteString(fmt.Sprintf("%-26s %s\n", label(1, "path (optional)"), m.winPathInput.View()))
	b.WriteString(fmt.Sprintf("%-26s %s\n", label(2, "on_stop (optional)"), m.winStopInput.View()))
	b.WriteString("\n")

	panesLabel := "  panes"
	if m.winFocus == 3 {
		panesLabel = focusedFieldLabel.Render("▸ panes")
	}
	b.WriteString(panesLabel + "\n")
	if len(m.winPanes) == 0 && !m.addingPane {
		b.WriteString("    " + subtleStyle.Render("no panes yet — press 'a' to add one") + "\n")
	}
	for i, p := range m.winPanes {
		cursor := "    "
		text := p
		if text == "" {
			text = "(shell)"
		}
		if m.winFocus == 3 && i == m.paneCursor && !m.addingPane {
			cursor = accentStyle.Render("  ▸ ")
			text = accentStyle.Render(text)
		} else {
			text = subtleStyle.Render(text)
		}
		b.WriteString(cursor + text + "\n")
	}
	if m.addingPane {
		b.WriteString("    " + m.paneInput.View() + "\n")
	}
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err) + "\n\n")
	}

	help := "tab next field   s save window   esc cancel"
	switch {
	case m.addingPane:
		help = "enter confirm   esc cancel"
	case m.winFocus == 3:
		help = "a add pane   enter edit pane   d delete pane   tab next field   s save window   esc cancel"
	}
	b.WriteString(subtleStyle.Render(help))
	return "\n" + panelStyle.Render(strings.TrimRight(b.String(), "\n")) + "\n"
}

func (m *wizardModel) viewTeardown() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Teardown commands (%d)", len(m.teardown))))
	b.WriteString("\n\n")
	if len(m.teardown) == 0 {
		b.WriteString(subtleStyle.Render("  none set — press 'a' to add one"))
		b.WriteString("\n")
	}
	for i, t := range m.teardown {
		cursor := "  "
		text := t
		if i == m.teardownCursor && !m.addingTeardown {
			cursor = accentStyle.Render("▸ ")
			text = accentStyle.Render(text)
		}
		b.WriteString(cursor + text + "\n")
	}
	if m.addingTeardown {
		b.WriteString("  " + m.teardownInput.View() + "\n")
	}
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err) + "\n\n")
	}
	help := "a add   enter edit   d delete   n next: review   esc back   ^R raw edit"
	if m.addingTeardown {
		help = "enter confirm   esc cancel"
	}
	b.WriteString(subtleStyle.Render(help))
	return "\n" + panelStyle.Render(strings.TrimRight(b.String(), "\n")) + "\n"
}

func (m *wizardModel) viewReview() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Review"))
	b.WriteString("\n\n")

	title := strings.TrimSpace(m.titleInput.Value())
	if title == "" {
		title = strings.ToUpper(m.alias)
	}
	path := strings.TrimSpace(m.pathInput.Value())
	if path == "" {
		path = "."
	}
	b.WriteString(fmt.Sprintf("alias:  %s\n", m.alias))
	b.WriteString(fmt.Sprintf("title:  %s\n", title))
	b.WriteString(fmt.Sprintf("path:   %s\n\n", path))

	b.WriteString(fmt.Sprintf("windows (%d):\n", len(m.windows)))
	for wi, w := range m.windows {
		branch, cont := "├─", "│  "
		if wi == len(m.windows)-1 {
			branch, cont = "└─", "   "
		}
		line := fmt.Sprintf("  %s %s", branch, w.name)
		if w.onStop != "" {
			line += subtleStyle.Render("  (on_stop: " + w.onStop + ")")
		}
		b.WriteString(line + "\n")
		for pi, p := range w.panes {
			pBranch := "├─"
			if pi == len(w.panes)-1 {
				pBranch = "└─"
			}
			text := p
			if text == "" {
				text = "(shell)"
			}
			b.WriteString(subtleStyle.Render(fmt.Sprintf("  %s%s %s", cont, pBranch, text)) + "\n")
		}
	}

	b.WriteString(fmt.Sprintf("\nteardown (%d):\n", len(m.teardown)))
	for _, t := range m.teardown {
		b.WriteString("  - " + t + "\n")
	}
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err) + "\n\n")
	}
	b.WriteString(subtleStyle.Render("s save   esc back   ^R raw edit"))
	return "\n" + panelStyle.Render(strings.TrimRight(b.String(), "\n")) + "\n"
}
