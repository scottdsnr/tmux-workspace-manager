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
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	configDir = filepath.Join(home, ".config", "tmux-workspaces")
	// Settings live in their own subdirectory, not alongside the flat
	// <alias>.yml workspace profiles, so a settings filename can never
	// collide with a workspace alias.
	settingsPath = filepath.Join(configDir, ".settings", "settings.yml")
	// Pre-YAML installs wrote settings.json / <alias>.json. Both are still
	// read transparently (valid JSON parses fine as YAML); every write goes
	// to the new .yml path, so profiles migrate the first time they're saved.
	legacySettingsPath = filepath.Join(configDir, ".settings", "settings.json")
}

type Settings struct {
	BaseDir         string `yaml:"base_dir"`
	WindowBaseIndex int    `yaml:"window_base_index"`
	PaneBaseIndex   int    `yaml:"pane_base_index"`
	Editor          string `yaml:"editor"`
	ConfirmDown     bool   `yaml:"confirm_down"`
	UseEmoji        bool   `yaml:"use_emoji"`
}

var defaultSettings = Settings{
	BaseDir:         "~",
	WindowBaseIndex: 1,
	PaneBaseIndex:   1,
	Editor:          "nano",
	ConfirmDown:     true,
	UseEmoji:        true,
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
