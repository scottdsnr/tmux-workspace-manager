package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

// upgradeConfigs converts legacy JSON settings to YAML, and folds any
// standalone <alias>.yml/<alias>.json workspace profile under configDir
// into the consolidated workspaces file — both in place, with the
// originals backed up as .bak.
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

	// Workspace profiles: fold every standalone <alias>.yml/<alias>.json
	// left over from before workspaces were consolidated into one file.
	legacy := legacyWorkspaceFiles()
	var aliases []string
	for alias := range legacy {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	doc, err := loadWorkspacesDoc()
	if err != nil {
		errPrint("%sFailed to read %s: %v", sym("error"), workspacesPath, err)
		doc = workspacesDoc{}
	}

	var toBackup []string
	for _, alias := range aliases {
		filename := legacy[alias]
		full := filepath.Join(configDir, filename)
		if info, err := os.Stat(full); err != nil || info.IsDir() {
			continue
		}
		if _, exists := doc[alias]; exists {
			fmt.Printf("%s'%s' already exists in %s; leaving %s untouched.\n", sym("warn"), alias, workspacesPath, full)
			skipped++
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			errPrint("%sFailed to read %s: %v", sym("error"), full, err)
			continue
		}
		var raw map[string]interface{}
		if err := yaml.Unmarshal(data, &raw); err != nil {
			errPrint("%sFailed to parse %s: %v", sym("error"), full, err)
			continue
		}
		if dry {
			fmt.Printf("   %s[dry-run] would merge '%s' from %s into %s\n", sym("dry"), alias, full, workspacesPath)
		} else {
			doc[alias] = raw
			toBackup = append(toBackup, full)
			fmt.Printf("%sMerged '%s' from %s into %s\n", sym("check"), alias, full, workspacesPath)
		}
		converted++
	}

	if !dry && len(toBackup) > 0 {
		if err := saveWorkspacesDoc(doc); err != nil {
			fatal("%sFailed to write %s: %v", sym("error"), workspacesPath, err)
		}
		for _, full := range toBackup {
			backup := full + ".bak"
			os.Rename(full, backup)
			fmt.Printf("   %sbacked up %s -> %s\n", sym("info"), full, backup)
		}
	}

	if converted == 0 && skipped == 0 {
		fmt.Printf("%sNo legacy workspace profiles found under %s; nothing to upgrade.\n", sym("info"), configDir)
	} else if dry {
		fmt.Printf("\n%sDry run: %d would be converted, %d would be skipped.\n", sym("info"), converted, skipped)
	} else {
		fmt.Printf("\n%sUpgrade complete: %d converted, %d skipped.\n", sym("sparkle"), converted, skipped)
	}
}
