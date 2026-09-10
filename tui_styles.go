package main

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// interactive reports whether stdout (and, when it matters, stdin) is a
// real terminal. Explicit flags like --dry-run and --raw always win over
// this check — see main.go's dispatch.
func interactive() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd()))
}

var (
	colorPrimary = lipgloss.AdaptiveColor{Light: "#5B21B6", Dark: "#A78BFA"}
	colorAccent  = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	colorWarn    = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	colorError   = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	colorBorder  = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#374151"}

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, 1)

	subtleStyle = lipgloss.NewStyle().Foreground(colorMuted)

	successStyle = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	accentStyle  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	activeDotStyle   = lipgloss.NewStyle().Foreground(colorSuccess)
	inactiveDotStyle = lipgloss.NewStyle().Foreground(colorMuted)

	helpBarStyle = lipgloss.NewStyle().Foreground(colorMuted).Padding(1, 1, 0, 1)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2)

	focusedFieldLabel = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	fieldLabel        = lipgloss.NewStyle().Foreground(colorMuted)
)

const (
	glyphActive   = "●"
	glyphInactive = "○"
	glyphCheck    = "✓"
	glyphCross    = "✗"
)
