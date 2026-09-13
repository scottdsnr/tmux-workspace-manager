package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// loadYAMLNode reads a file and returns its top-level content node,
// preserving key order for a faithful round-trip re-encode.
func loadYAMLNode(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) > 0 {
		return doc.Content[0], nil
	}
	return &doc, nil
}

// upgradeConfigs converts legacy JSON profiles/settings under configDir to
// YAML in place.
func upgradeConfigs(dry bool) {
	if _, err := os.Stat(configDir); err != nil {
		fmt.Printf("%sNo config directory found at %s; nothing to upgrade.\n", sym("info"), configDir)
		return
	}

	converted := 0
	skipped := 0

	// Settings: the canonical legacy path (.settings/settings.json), plus a
	// flat configDir/settings.json some older installs mistakenly wrote to.
	legacyCandidates := []string{legacySettingsPath, filepath.Join(configDir, "settings.json")}
	for _, legacy := range legacyCandidates {
		if _, err := os.Stat(legacy); err != nil {
			continue
		}
		if _, err := os.Stat(settingsPath); err == nil {
			fmt.Printf("%s%s already exists; leaving %s untouched.\n", sym("warn"), settingsPath, legacy)
			skipped++
			break
		}
		node, err := loadYAMLNode(legacy)
		if err != nil {
			errPrint("%sFailed to parse %s: %v", sym("error"), legacy, err)
			break
		}
		if dry {
			fmt.Printf("   %s[dry-run] would convert %s -> %s\n", sym("dry"), legacy, settingsPath)
		} else {
			os.MkdirAll(filepath.Dir(settingsPath), 0755)
			data, _ := yaml.Marshal(node)
			os.WriteFile(settingsPath, data, 0644)
			backup := legacy + ".bak"
			os.Rename(legacy, backup)
			fmt.Printf("%sConverted settings: %s -> %s (backup: %s)\n", sym("check"), legacy, settingsPath, backup)
		}
		converted++
		break
	}

	// Workspace profiles: every top-level <alias>.json, skipping a flat
	// settings.json (handled above, never a real workspace alias).
	entries, _ := os.ReadDir(configDir)
	var filenames []string
	for _, e := range entries {
		filenames = append(filenames, e.Name())
	}
	sort.Strings(filenames)

	for _, filename := range filenames {
		if filename == "settings.json" || !strings.HasSuffix(filename, ".json") {
			continue
		}
		full := filepath.Join(configDir, filename)
		if info, err := os.Stat(full); err != nil || info.IsDir() {
			continue
		}
		alias := strings.TrimSuffix(filename, ".json")
		ymlPath, _ := configPaths(alias)
		if _, err := os.Stat(ymlPath); err == nil {
			fmt.Printf("%s%s already exists; leaving %s untouched.\n", sym("warn"), ymlPath, full)
			skipped++
			continue
		}
		node, err := loadYAMLNode(full)
		if err != nil {
			errPrint("%sFailed to parse %s: %v", sym("error"), full, err)
			continue
		}
		if dry {
			fmt.Printf("   %s[dry-run] would convert %s -> %s\n", sym("dry"), full, ymlPath)
		} else {
			data, _ := yaml.Marshal(node)
			os.MkdirAll(configDir, 0755)
			os.WriteFile(ymlPath, data, 0644)
			backup := full + ".bak"
			os.Rename(full, backup)
			fmt.Printf("%sConverted '%s': %s -> %s (backup: %s)\n", sym("check"), alias, full, ymlPath, backup)
		}
		converted++
	}

	if converted == 0 && skipped == 0 {
		fmt.Printf("%sNo legacy JSON configs found under %s; nothing to upgrade.\n", sym("info"), configDir)
	} else if dry {
		fmt.Printf("\n%sDry run: %d would be converted, %d would be skipped.\n", sym("info"), converted, skipped)
	} else {
		fmt.Printf("\n%sUpgrade complete: %d converted, %d skipped.\n", sym("sparkle"), converted, skipped)
	}
}
