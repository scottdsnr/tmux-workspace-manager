package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type settingsField int

const (
	fieldBaseDir settingsField = iota
	fieldWindowIndex
	fieldPaneIndex
	fieldEditor
	fieldConfirmDown
	fieldUseEmoji
	fieldCount
)

type settingsModel struct {
	focus       settingsField
	baseDir     textinput.Model
	windowIndex textinput.Model
	paneIndex   textinput.Model
	editor      textinput.Model
	confirmDown bool
	useEmoji    bool
	err         string
}

func newSettingsInput(val string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(val)
	ti.CharLimit = 200
	ti.Width = 40
	return ti
}

func newSettingsModel() *settingsModel {
	cur := settings
	m := &settingsModel{
		baseDir:     newSettingsInput(cur.BaseDir),
		windowIndex: newSettingsInput(strconv.Itoa(cur.WindowBaseIndex)),
		paneIndex:   newSettingsInput(strconv.Itoa(cur.PaneBaseIndex)),
		editor:      newSettingsInput(cur.Editor),
		confirmDown: cur.ConfirmDown,
		useEmoji:    cur.UseEmoji,
	}
	m.baseDir.Focus()
	return m
}

func (m *settingsModel) Init() tea.Cmd { return textinput.Blink }

func (m *settingsModel) blurAll() {
	m.baseDir.Blur()
	m.windowIndex.Blur()
	m.paneIndex.Blur()
	m.editor.Blur()
}

func (m *settingsModel) focusCurrent() tea.Cmd {
	switch m.focus {
	case fieldBaseDir:
		return m.baseDir.Focus()
	case fieldWindowIndex:
		return m.windowIndex.Focus()
	case fieldPaneIndex:
		return m.paneIndex.Focus()
	case fieldEditor:
		return m.editor.Focus()
	}
	return nil
}

func (m *settingsModel) save() (*settingsModel, tea.Cmd) {
	wi, err := strconv.Atoi(strings.TrimSpace(m.windowIndex.Value()))
	if err != nil {
		m.err = "window_base_index must be a whole number"
		return m, nil
	}
	pi, err := strconv.Atoi(strings.TrimSpace(m.paneIndex.Value()))
	if err != nil {
		m.err = "pane_base_index must be a whole number"
		return m, nil
	}

	newSettings := Settings{
		BaseDir:         strings.TrimSpace(m.baseDir.Value()),
		WindowBaseIndex: wi,
		PaneBaseIndex:   pi,
		Editor:          strings.TrimSpace(m.editor.Value()),
		ConfirmDown:     m.confirmDown,
		UseEmoji:        m.useEmoji,
	}
	if newSettings.BaseDir == "" {
		newSettings.BaseDir = "~"
	}
	if newSettings.Editor == "" {
		newSettings.Editor = "nano"
	}

	if err := saveSettings(newSettings); err != nil {
		m.err = err.Error()
		return m, nil
	}
	settings = newSettings
	return m, func() tea.Msg { return screenFinishedMsg{} }
}

func (m *settingsModel) Update(msg tea.Msg) (*settingsModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			return m, func() tea.Msg { return screenFinishedMsg{} }
		case "tab", "down":
			m.blurAll()
			m.focus = (m.focus + 1) % fieldCount
			return m, m.focusCurrent()
		case "shift+tab", "up":
			m.blurAll()
			m.focus = (m.focus - 1 + fieldCount) % fieldCount
			return m, m.focusCurrent()
		case " ":
			switch m.focus {
			case fieldConfirmDown:
				m.confirmDown = !m.confirmDown
				return m, nil
			case fieldUseEmoji:
				m.useEmoji = !m.useEmoji
				return m, nil
			}
		case "enter":
			return m.save()
		}
	}

	var cmd tea.Cmd
	switch m.focus {
	case fieldBaseDir:
		m.baseDir, cmd = m.baseDir.Update(msg)
	case fieldWindowIndex:
		m.windowIndex, cmd = m.windowIndex.Update(msg)
	case fieldPaneIndex:
		m.paneIndex, cmd = m.paneIndex.Update(msg)
	case fieldEditor:
		m.editor, cmd = m.editor.Update(msg)
	}
	return m, cmd
}

func (m *settingsModel) View() string {
	checkbox := func(v bool) string {
		if v {
			return successStyle.Render("[x]")
		}
		return subtleStyle.Render("[ ]")
	}
	row := func(label string, field settingsField, content string) string {
		l := "  " + fieldLabel.Render(label)
		if m.focus == field {
			l = focusedFieldLabel.Render("▸ " + label)
		}
		return fmt.Sprintf("%-28s %s", l, content)
	}

	lines := []string{
		titleStyle.Render("Settings"),
		"",
		row("base_dir", fieldBaseDir, m.baseDir.View()),
		row("window_base_index", fieldWindowIndex, m.windowIndex.View()),
		row("pane_base_index", fieldPaneIndex, m.paneIndex.View()),
		row("editor", fieldEditor, m.editor.View()),
		row("confirm_down", fieldConfirmDown, checkbox(m.confirmDown)+subtleStyle.Render("  space to toggle")),
		row("use_emoji", fieldUseEmoji, checkbox(m.useEmoji)+subtleStyle.Render("  space to toggle")),
		"",
	}
	if m.err != "" {
		lines = append(lines, errorStyle.Render(m.err), "")
	}
	lines = append(lines, subtleStyle.Render("tab/shift+tab move   space toggle   enter save   esc cancel"))
	return "\n" + panelStyle.Render(strings.Join(lines, "\n")) + "\n"
}
