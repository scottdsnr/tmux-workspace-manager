package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

type Pane struct {
	Command string `yaml:"command"`
}

type Window struct {
	Name   string `yaml:"name"`
	Path   string `yaml:"path,omitempty"`
	OnStop string `yaml:"on_stop,omitempty"`
	Panes  []Pane `yaml:"panes"`
}

// Teardown accepts either a bare string or a list of strings in YAML, like
// the Python tool's teardown field.
type Teardown []string

func (t *Teardown) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		if s == "" {
			*t = Teardown{}
		} else {
			*t = Teardown{s}
		}
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*t = Teardown(list)
	default:
		return fmt.Errorf("teardown must be a string or a list of strings")
	}
	return nil
}

type Config struct {
	ProjectNameDisplay string   `yaml:"project_name_display,omitempty"`
	ProjectPath        string   `yaml:"project_path"`
	Windows            []Window `yaml:"windows"`
	Teardown           Teardown `yaml:"teardown,omitempty"`
}

// configPaths returns (yml_path, legacy_json_path) for an alias.
func configPaths(alias string) (string, string) {
	return filepath.Join(configDir, alias+".yml"), filepath.Join(configDir, alias+".json")
}

func findConfigPath(alias string) string {
	ymlPath, jsonPath := configPaths(alias)
	return firstExisting(ymlPath, jsonPath)
}

// loadConfigRaw loads a workspace profile as a generic map, for validation
// and for edits that must preserve unrecognized top-level keys.
func loadConfigRaw(alias string) (map[string]interface{}, string) {
	path := findConfigPath(alias)
	if path == "" {
		fatal("%sWorkspace alias '%s' does not exist.", sym("error"), alias)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fatal("%sFailed to read %s: %v", sym("error"), path, err)
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		fatal("%sFailed to parse %s: %v", sym("error"), path, err)
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	return raw, path
}

// loadConfigTyped loads a workspace profile into the strongly-typed Config,
// for use once validateConfig has already confirmed the shape is sound.
func loadConfigTyped(alias string) Config {
	path := findConfigPath(alias)
	if path == "" {
		fatal("%sWorkspace alias '%s' does not exist.", sym("error"), alias)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fatal("%sFailed to read %s: %v", sym("error"), path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fatal("%sFailed to parse %s: %v", sym("error"), path, err)
	}
	return cfg
}

var topLevelOrder = []string{"project_name_display", "project_path", "windows", "teardown"}

// saveConfigRaw writes a workspace profile as YAML, with known top-level
// keys in a stable, readable order followed by any unrecognized keys
// sorted alphabetically.
func saveConfigRaw(alias string, cfg map[string]interface{}) (string, error) {
	ymlPath, _ := configPaths(alias)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}
	node, err := encodeOrdered(cfg, topLevelOrder)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(node)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(ymlPath, data, 0644); err != nil {
		return "", err
	}
	return ymlPath, nil
}

func encodeOrdered(m map[string]interface{}, order []string) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	seen := make(map[string]bool, len(m))

	addKey := func(k string) error {
		v, ok := m[k]
		if !ok {
			return nil
		}
		seen[k] = true
		keyNode := &yaml.Node{}
		if err := keyNode.Encode(k); err != nil {
			return err
		}
		valNode := &yaml.Node{}
		if err := valNode.Encode(v); err != nil {
			return err
		}
		node.Content = append(node.Content, keyNode, valNode)
		return nil
	}

	for _, k := range order {
		if err := addKey(k); err != nil {
			return nil, err
		}
	}
	var rest []string
	for k := range m {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		if err := addKey(k); err != nil {
			return nil, err
		}
	}
	return node, nil
}

// validateConfig mirrors the Python tool's structural checks exactly,
// operating on the generic map so malformed YAML produces the same
// human-readable diagnostics rather than an opaque decode error.
func validateConfig(cfg map[string]interface{}) []string {
	var errs []string

	pp, ok := cfg["project_path"].(string)
	if !ok || pp == "" {
		errs = append(errs, "project_path must be a non-empty string")
	}

	windowsRaw, hasWindows := cfg["windows"]
	windows, isList := asInterfaceList(windowsRaw)
	if !hasWindows || !isList || len(windows) == 0 {
		errs = append(errs, "windows must be a non-empty list")
	} else {
		for i, wRaw := range windows {
			w, isMap := wRaw.(map[string]interface{})
			name, hasName := "", false
			if isMap {
				if n, ok := w["name"].(string); ok && n != "" {
					name = n
					hasName = true
				}
			}
			if !isMap || !hasName {
				errs = append(errs, fmt.Sprintf("window #%d is missing a 'name'", i+1))
				continue
			}

			panesRaw := w["panes"]
			panes, panesIsList := asInterfaceList(panesRaw)
			if panesRaw == nil {
				panes, panesIsList = []interface{}{}, true
			}
			if !panesIsList || len(panes) == 0 {
				errs = append(errs, fmt.Sprintf("window '%s' must have at least one pane", name))
			} else {
				for _, p := range panes {
					if _, ok := p.(map[string]interface{}); !ok {
						errs = append(errs, fmt.Sprintf("window '%s' has a pane that is not an object", name))
						break
					}
				}
			}
		}
	}

	teardownRaw, hasTeardown := cfg["teardown"]
	teardownValid := true
	if hasTeardown {
		switch t := teardownRaw.(type) {
		case string:
		case []interface{}:
			for _, item := range t {
				if _, ok := item.(string); !ok {
					teardownValid = false
					break
				}
			}
		case nil:
		default:
			teardownValid = false
		}
	}
	if !teardownValid {
		errs = append(errs, "teardown must be a string or a list of strings")
	}

	return errs
}

// validateTypedConfig applies the same structural rules as validateConfig,
// but against data the wizards just built in-process (already typed, so
// most of validateConfig's shape checks are true by construction) rather
// than against a value freshly decoded from YAML.
func validateTypedConfig(projectPath string, windows []Window) []string {
	var errs []string

	if projectPath == "" {
		errs = append(errs, "project_path must be a non-empty string")
	}

	if len(windows) == 0 {
		errs = append(errs, "windows must be a non-empty list")
	} else {
		for i, w := range windows {
			if w.Name == "" {
				errs = append(errs, fmt.Sprintf("window #%d is missing a 'name'", i+1))
				continue
			}
			if len(w.Panes) == 0 {
				errs = append(errs, fmt.Sprintf("window '%s' must have at least one pane", w.Name))
			}
		}
	}

	return errs
}

func asInterfaceList(v interface{}) ([]interface{}, bool) {
	list, ok := v.([]interface{})
	return list, ok
}

// validateAlias returns an error string if alias is unusable as a workspace
// filename, else "".
func validateAlias(alias string) string {
	if alias == "" || containsSpace(alias) {
		return "cannot be empty or contain spaces"
	}
	if alias == "settings" {
		return "'settings' is reserved"
	}
	candidate, _ := filepath.Abs(filepath.Join(configDir, alias+".yml"))
	absConfigDir, _ := filepath.Abs(configDir)
	if filepath.Dir(candidate) != absConfigDir {
		return "cannot contain path separators or '..'"
	}
	return ""
}

func containsSpace(s string) bool {
	for _, r := range s {
		if r == ' ' {
			return true
		}
	}
	return false
}
