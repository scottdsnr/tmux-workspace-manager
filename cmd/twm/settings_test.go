package main

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempSettings points settingsPath/legacySettingsPath at a scratch dir
// for the duration of a test, so nothing ever reads or writes the real
// config directory.
func withTempSettings(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origSettings, origLegacy := settingsPath, legacySettingsPath
	settingsPath = filepath.Join(dir, ".settings", "settings.yml")
	legacySettingsPath = filepath.Join(dir, ".settings", "settings.json")
	t.Cleanup(func() {
		settingsPath, legacySettingsPath = origSettings, origLegacy
	})
	return dir
}

func TestQuitOnSwitchDefaultsOff(t *testing.T) {
	if defaultSettings.QuitOnSwitch {
		t.Fatal("quit_on_switch should default to off")
	}
}

func TestLoadSettingsQuitOnSwitchAbsent(t *testing.T) {
	withTempSettings(t)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	// A settings file written before this option existed.
	if err := os.WriteFile(settingsPath, []byte("base_dir: \"~\"\nconfirm_down: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(); got.QuitOnSwitch {
		t.Fatal("quit_on_switch should stay off when the key is absent")
	}
}

func TestSaveLoadSettingsRoundTripsQuitOnSwitch(t *testing.T) {
	withTempSettings(t)
	want := defaultSettings
	want.QuitOnSwitch = true
	if err := saveSettings(want); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(); !got.QuitOnSwitch {
		t.Fatal("quit_on_switch did not survive a save/load round trip")
	}
}

func TestAttachFinishedMsgFollowsQuitOnSwitch(t *testing.T) {
	orig := settings
	t.Cleanup(func() { settings = orig })

	settings.QuitOnSwitch = false
	if msg := attachFinishedMsg("demo"); msg.quitAfterExec {
		t.Fatal("expected the dashboard to stay open with quit_on_switch off")
	}

	settings.QuitOnSwitch = true
	msg := attachFinishedMsg("demo")
	if !msg.quitAfterExec {
		t.Fatal("expected twm to quit after the jump with quit_on_switch on")
	}
	if msg.exec == nil {
		t.Fatal("expected an attach/switch command regardless of the setting")
	}
}
