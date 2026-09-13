package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// printValidateResult is the TTY `validate` path: colored text, no event
// loop needed for a one-shot pass/fail check. loadConfigRaw's fatal-on-error
// behavior is fine here since there's no TUI state to preserve.
func printValidateResult(alias string) {
	raw, _ := loadConfigRaw(alias)
	errs := validateConfig(raw)
	if len(errs) > 0 {
		fmt.Println(errorStyle.Render(fmt.Sprintf("%s '%s' has %d problem(s):", glyphCross, alias, len(errs))))
		for _, e := range errs {
			fmt.Println("   - " + e)
		}
		os.Exit(1)
	}
	windows, _ := asInterfaceList(raw["windows"])
	fmt.Println(successStyle.Render(fmt.Sprintf("%s '%s' looks valid (%d window(s)).", glyphCheck, alias, len(windows))))
}

// progressLineMsg carries one report line from a running job into the TUI.
type progressLineMsg string

// progressDoneMsg signals the job's goroutine finished, with its error (if
// any).
type progressDoneMsg struct{ err error }

// progressModel is a shared "run a job, stream its output" screen used by
// both the up-build and down-teardown flows. Once the job finishes (success
// or failure), the screen waits for a keypress and then calls onContinue —
// e.g. to attach to tmux, or just to signal the screen is finished.
type progressModel struct {
	title        string
	lines        []string
	spin         spinner.Model
	done         bool
	err          error
	lineCh       chan string
	doneCh       chan error
	onContinue   func(err error) tea.Cmd
	continueHint string
}

func newProgressModel(title string, job func(report reportFunc) error, continueHint string, onContinue func(err error) tea.Cmd) *progressModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = accentStyle

	lineCh := make(chan string, 64)
	doneCh := make(chan error, 1)

	go func() {
		err := job(func(line string) { lineCh <- line })
		doneCh <- err
		close(lineCh)
	}()

	if continueHint == "" {
		continueHint = "press any key to continue"
	}
	return &progressModel{title: title, spin: sp, lineCh: lineCh, doneCh: doneCh, onContinue: onContinue, continueHint: continueHint}
}

// waitForProgressLine always drains lineCh first, so every reported line is
// delivered before the final progressDoneMsg — lineCh is only closed by the
// job goroutine after doneCh already has its result buffered.
func waitForProgressLine(lineCh chan string, doneCh chan error) tea.Cmd {
	return func() tea.Msg {
		if line, ok := <-lineCh; ok {
			return progressLineMsg(line)
		}
		return progressDoneMsg{err: <-doneCh}
	}
}

func (m *progressModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, waitForProgressLine(m.lineCh, m.doneCh))
}

func (m *progressModel) Update(msg tea.Msg) (*progressModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.done {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case progressLineMsg:
		m.lines = append(m.lines, string(msg))
		return m, waitForProgressLine(m.lineCh, m.doneCh)
	case progressDoneMsg:
		m.done = true
		m.err = msg.err
		return m, nil
	case tea.KeyMsg:
		if m.done && m.onContinue != nil {
			return m, m.onContinue(m.err)
		}
		return m, nil
	}
	return m, nil
}

func (m *progressModel) View() string {
	var header string
	switch {
	case !m.done:
		header = m.spin.View() + " " + titleStyle.Render(m.title)
	case m.err != nil:
		header = errorStyle.Render(glyphCross + " " + m.title + " failed")
	default:
		header = successStyle.Render(glyphCheck + " " + m.title)
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n\n")
	for _, l := range m.lines {
		b.WriteString("  " + l + "\n")
	}
	if m.done && m.err != nil {
		b.WriteString("\n" + errorStyle.Render(m.err.Error()) + "\n")
	}
	if m.done {
		b.WriteString("\n" + subtleStyle.Render(m.continueHint) + "\n")
	}
	return "\n" + panelStyle.Render(strings.TrimRight(b.String(), "\n")) + "\n"
}

// confirmModel is a small y/n screen, used for the down confirmation.
type confirmModel struct {
	alias, title string
}

func newConfirmModel(alias, title string) *confirmModel {
	return &confirmModel{alias: alias, title: title}
}

func (m *confirmModel) Init() tea.Cmd { return nil }

func (m *confirmModel) Update(msg tea.Msg) (*confirmModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y", "enter":
		alias := m.alias
		return m, func() tea.Msg { return pushDownProgressMsg{alias: alias} }
	case "n", "N", "esc", "q":
		return m, func() tea.Msg { return screenFinishedMsg{} }
	}
	return m, nil
}

func (m *confirmModel) View() string {
	body := fmt.Sprintf(
		"%s\n\nTear down %s (%s)?\nThis runs teardown commands and kills the session.\n\n%s",
		warnStyle.Render(sym("warn")+"Confirm teardown"),
		m.title, m.alias,
		subtleStyle.Render("[y] yes    [n/esc] cancel"),
	)
	return "\n" + panelStyle.Render(body) + "\n"
}

// validateModel shows a one-shot pass/fail result for `validate`.
type validateModel struct {
	alias       string
	ok          bool
	windowCount int
	errs        []string
	loadErr     error
}

func newValidateModel(alias string) *validateModel {
	m := &validateModel{alias: alias}
	raw, err := loadConfigRawErr(alias)
	if err != nil {
		m.loadErr = err
		return m
	}
	m.errs = validateConfig(raw)
	windows, _ := asInterfaceList(raw["windows"])
	m.windowCount = len(windows)
	m.ok = len(m.errs) == 0
	return m
}

func (m *validateModel) Init() tea.Cmd { return nil }

func (m *validateModel) Update(msg tea.Msg) (*validateModel, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return m, func() tea.Msg { return screenFinishedMsg{} }
	}
	return m, nil
}

func (m *validateModel) View() string {
	var body string
	switch {
	case m.loadErr != nil:
		body = errorStyle.Render(glyphCross + " " + m.loadErr.Error())
	case m.ok:
		body = successStyle.Render(fmt.Sprintf("%s '%s' looks valid (%d window(s)).", glyphCheck, m.alias, m.windowCount))
	default:
		lines := []string{errorStyle.Render(fmt.Sprintf("%s '%s' has %d problem(s):", glyphCross, m.alias, len(m.errs)))}
		for _, e := range m.errs {
			lines = append(lines, "  - "+e)
		}
		body = strings.Join(lines, "\n")
	}
	return "\n" + panelStyle.Render(body+"\n\n"+subtleStyle.Render("press any key to continue")) + "\n"
}
