package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var (
	configDir          string
	settingsPath       string
	legacySettingsPath string
	workspacesPath     string
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	configDir = filepath.Join(home, ".config", "tmux-workspaces")
	// Settings live in their own subdirectory, not alongside the
	// workspaces file, so a settings filename can never collide with a
	// workspace alias.
	settingsPath = filepath.Join(configDir, ".settings", "settings.yml")
	// Pre-YAML installs wrote settings.json / <alias>.json. Both are still
	// read transparently (valid JSON parses fine as YAML); every write goes
	// to the new .yml path, so profiles migrate the first time they're saved.
	legacySettingsPath = filepath.Join(configDir, ".settings", "settings.json")
	// All workspace profiles live together in one file; standalone
	// <alias>.yml/<alias>.json files predate this and are folded in by
	// `upgrade`.
	workspacesPath = filepath.Join(configDir, "workspaces.yml")
}

type Settings struct {
	BaseDir         string `yaml:"base_dir"`
	WindowBaseIndex int    `yaml:"window_base_index"`
	PaneBaseIndex   int    `yaml:"pane_base_index"`
	Editor          string `yaml:"editor"`
	ConfirmDown     bool   `yaml:"confirm_down"`
	UseEmoji        bool   `yaml:"use_emoji"`
	// QuitOnSwitch makes the dashboard exit once it has handed the terminal
	// over to a workspace, instead of coming back to the workspace list.
	// Off by default so the dashboard stays where it was.
	QuitOnSwitch bool `yaml:"quit_on_switch"`
}

var defaultSettings = Settings{
	BaseDir:         "~",
	WindowBaseIndex: 1,
	PaneBaseIndex:   1,
	Editor:          "nano",
	ConfirmDown:     true,
	UseEmoji:        true,
	QuitOnSwitch:    false,
}

// settings is the process-wide active settings, mutated by loadSettings and
// the --no-emoji flag before most other code ever runs.
var settings = defaultSettings

func loadSettings() Settings {
	result := defaultSettings
	path := firstExisting(settingsPath, legacySettingsPath)
	if path == "" {
		return result
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	if err := yaml.Unmarshal(data, &result); err != nil {
		errPrint("%sFailed to read %s (%v); using defaults.", sym("warn"), path, err)
		return defaultSettings
	}
	return result
}

// saveSettings writes s to settingsPath, creating its directory if needed.
// Used by the in-TUI settings form; editSettings (below) is the $EDITOR
// fallback for the non-TTY / --raw path.
func saveSettings(s Settings) error {
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, data, 0644)
}

func editSettings() {
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		fatal("%sFailed to create %s: %v", sym("error"), filepath.Dir(settingsPath), err)
	}
	path := firstExisting(settingsPath, legacySettingsPath)
	if path == "" {
		data, err := yaml.Marshal(defaultSettings)
		if err == nil {
			os.WriteFile(settingsPath, data, 0644)
		}
		fmt.Printf("%sCreated default settings file at %s\n", sym("sparkle"), settingsPath)
		path = settingsPath
	}
	openInEditor(path)
}
