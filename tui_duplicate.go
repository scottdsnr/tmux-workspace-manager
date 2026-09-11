package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// duplicateModel is a one-field prompt for cloning an existing workspace
// profile under a new alias — handy for near-identical projects that would
// otherwise mean re-running the whole wizard.
type duplicateModel struct {
	source     string
	aliasInput textinput.Model
	err        string
}

func newDuplicateModel(source string) *duplicateModel {
	m := &duplicateModel{source: source}
	m.aliasInput = newSettingsInput(source + "-copy")
	m.aliasInput.Focus()
	m.aliasInput.CursorEnd()
	return m
}

func (m *duplicateModel) Init() tea.Cmd { return textinput.Blink }

func (m *duplicateModel) Update(msg tea.Msg) (*duplicateModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "ctrl+c":
			return m, func() tea.Msg { return screenFinishedMsg{} }
		case "enter":
			alias := strings.ToLower(strings.TrimSpace(m.aliasInput.Value()))
			if errStr := validateAlias(alias); errStr != "" {
				m.err = "invalid alias: " + errStr
				return m, nil
			}
			if alias == m.source {
				m.err = "pick a different alias than the source"
				return m, nil
			}
			if findConfigPath(alias) != "" {
				m.err = fmt.Sprintf("'%s' already exists", alias)
				return m, nil
			}
			raw, err := loadConfigRawErr(m.source)
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			if _, err := saveConfigRaw(alias, raw); err != nil {
				m.err = "failed to save: " + err.Error()
				return m, nil
			}
			return m, func() tea.Msg { return screenFinishedMsg{} }
		}
	}
	var cmd tea.Cmd
	m.aliasInput, cmd = m.aliasInput.Update(msg)
	return m, cmd
}

func (m *duplicateModel) View() string {
	body := titleStyle.Render(fmt.Sprintf("Duplicate '%s'", m.source)) + "\n\n" +
		fieldLabel.Render("new alias") + "\n" + m.aliasInput.View() + "\n\n" +
		subtleStyle.Render("enter confirm   esc cancel")
	if m.err != "" {
		body += "\n\n" + errorStyle.Render(m.err)
	}
	return "\n" + panelStyle.Render(body) + "\n"
}
