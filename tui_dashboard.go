package main

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type workspaceItem struct{ workspaceEntry }

func (i workspaceItem) FilterValue() string { return i.alias + " " + i.displayName }

type workspaceDelegate struct{}

func (d workspaceDelegate) Height() int                         { return 1 }
func (d workspaceDelegate) Spacing() int                        { return 0 }
func (d workspaceDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d workspaceDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(workspaceItem)
	if !ok {
		return
	}

	dot := inactiveDotStyle.Render(glyphInactive)
	status := subtleStyle.Render("inactive")
	name := it.displayName
	switch {
	case it.hasError:
		dot = errorStyle.Render(glyphCross)
		status = errorStyle.Render("error")
		name = it.errMsg
	case it.active:
		dot = activeDotStyle.Render(glyphActive)
		state := "detached"
		if it.attached {
			state = "attached"
		}
		status = successStyle.Render(fmt.Sprintf("%s %d/%d win", state, it.windowsAlive, it.windowsTotal))
	}

	row := fmt.Sprintf("%s %-12s %-28s %s", dot, it.alias, name, status)
	if index == m.Index() {
		row = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("▸ " + row)
	} else {
		row = "  " + row
	}
	fmt.Fprint(w, row)
}

type dashboardModel struct {
	list list.Model
}

func newDashboardModel() dashboardModel {
	entries := gatherWorkspaceEntries()
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = workspaceItem{e}
	}

	l := list.New(items, workspaceDelegate{}, 0, 0)
	l.Title = "tmux workspaces"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	if len(items) == 0 {
		l.SetShowFilter(false)
		l.SetFilteringEnabled(false)
	}

	return dashboardModel{list: l}
}

func (m dashboardModel) Init() tea.Cmd { return nil }

func (m dashboardModel) selected() (workspaceItem, bool) {
	it, ok := m.list.SelectedItem().(workspaceItem)
	return it, ok
}

func (m dashboardModel) Update(msg tea.Msg) (dashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := msg.Height - lipgloss.Height(m.helpView()) - 2
		if h < 3 {
			h = 3
		}
		m.list.SetSize(msg.Width-4, h)
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() != list.Filtering {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "r":
				nm := newDashboardModel()
				nm.list.SetSize(m.list.Width(), m.list.Height())
				return nm, nil
			case "n":
				return m, func() tea.Msg { return pushWizardMsg{create: true} }
			case "c":
				return m, func() tea.Msg { return pushSettingsMsg{} }
			case "enter", "o":
				if it, ok := m.selected(); ok && !it.hasError {
					if it.active {
						// Already running: jump straight in rather than
						// running the up-progress screen just to report
						// "already running" before attaching anyway.
						return m, func() tea.Msg { return screenFinishedMsg{exec: attachExecCmd(it.alias)} }
					}
					return m, func() tea.Msg { return pushUpMsg{alias: it.alias} }
				}
				return m, nil
			case "d":
				if it, ok := m.selected(); ok && !it.hasError && it.active {
					return m, func() tea.Msg {
						return pushDownConfirmMsg{alias: it.alias, title: it.displayName}
					}
				}
				return m, nil
			case "e":
				if it, ok := m.selected(); ok {
					return m, func() tea.Msg { return pushWizardMsg{alias: it.alias} }
				}
				return m, nil
			case "v":
				if it, ok := m.selected(); ok {
					return m, func() tea.Msg { return pushValidateMsg{alias: it.alias} }
				}
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m dashboardModel) helpView() string {
	return helpBarStyle.Render(
		"enter/o up   d down   e edit   n new   v validate   c settings   / filter   r refresh   q quit",
	)
}

func (m dashboardModel) View() string {
	if len(m.list.Items()) == 0 {
		return "\n" + panelStyle.Render(
			subtleStyle.Render(fmt.Sprintf("No workspaces found. Create .yml profiles inside: %s", configDir))+
				"\n\n"+subtleStyle.Render("[n] new workspace   [q] quit"),
		) + "\n"
	}
	return m.list.View() + m.helpView()
}
