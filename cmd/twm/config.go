package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

type Pane struct {
	Command string            `yaml:"command"`
	Path    string            `yaml:"path,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
}

type Window struct {
	Name   string `yaml:"name"`
	Path   string `yaml:"path,omitempty"`
	OnStop string `yaml:"on_stop,omitempty"`
	Layout string `yaml:"layout,omitempty"`
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
// and for edits that must preserve unrecognized top-level keys. It aborts
// the process on error, so it's only for plain CLI codepaths — a TUI
// screen must use loadConfigRawErr instead.
func loadConfigRaw(alias string) (map[string]interface{}, string) {
	raw, err := loadConfigRawErr(alias)
	if err != nil {
		fatal("%s%s", sym("error"), err)
	}
	path, _ := configPaths(alias)
	if p := findConfigPath(alias); p != "" {
		path = p
	}
	return raw, path
}

// loadConfigRawErr is the non-fatal counterpart to loadConfigRaw, for
// callers (TUI screens) that need to report failure themselves rather than
// exiting the whole process.
func loadConfigRawErr(alias string) (map[string]interface{}, error) {
	path := findConfigPath(alias)
	if path == "" {
		return nil, fmt.Errorf("workspace alias '%s' does not exist", alias)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	return raw, nil
}

// loadConfigTyped loads a workspace profile into the strongly-typed Config,
// for use once validateConfig has already confirmed the shape is sound. It
// aborts the process on error; TUI screens must use loadConfigTypedErr.
func loadConfigTyped(alias string) Config {
	cfg, err := loadConfigTypedErr(alias)
	if err != nil {
		fatal("%s%s", sym("error"), err)
	}
	return cfg
}

// loadConfigTypedErr is the non-fatal counterpart to loadConfigTyped.
func loadConfigTypedErr(alias string) (Config, error) {
	path := findConfigPath(alias)
	if path == "" {
		return Config{}, fmt.Errorf("workspace alias '%s' does not exist", alias)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return cfg, nil
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
		seenNames := make(map[string]bool, len(windows))
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
			// tmux targets windows by "session:name" (see tmuxNewWindow,
			// listPaneIDs) — a duplicate name makes that target ambiguous.
			if seenNames[name] {
				errs = append(errs, fmt.Sprintf("window name '%s' is used more than once", name))
			}
			seenNames[name] = true

			if layoutRaw, hasLayout := w["layout"]; hasLayout {
				if _, ok := layoutRaw.(string); !ok {
					errs = append(errs, fmt.Sprintf("window '%s' layout must be a string", name))
				}
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
					pm, ok := p.(map[string]interface{})
					if !ok {
						errs = append(errs, fmt.Sprintf("window '%s' has a pane that is not an object", name))
						break
					}
					if pathRaw, hasPath := pm["path"]; hasPath {
						if _, ok := pathRaw.(string); !ok {
							errs = append(errs, fmt.Sprintf("window '%s' has a pane with a non-string 'path'", name))
						}
					}
					if envRaw, hasEnv := pm["env"]; hasEnv {
						if !isStringMap(envRaw) {
							errs = append(errs, fmt.Sprintf("window '%s' has a pane whose 'env' is not a map of strings", name))
						}
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
		seenNames := make(map[string]bool, len(windows))
		for i, w := range windows {
			if w.Name == "" {
				errs = append(errs, fmt.Sprintf("window #%d is missing a 'name'", i+1))
				continue
			}
			if seenNames[w.Name] {
				errs = append(errs, fmt.Sprintf("window name '%s' is used more than once", w.Name))
			}
			seenNames[w.Name] = true
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

// isStringMap reports whether v decoded (via yaml.v3) as a mapping whose
// values are all strings — the shape a pane's "env" field must have.
func isStringMap(v interface{}) bool {
	m, ok := v.(map[string]interface{})
	if !ok {
		return false
	}
	for _, val := range m {
		if _, ok := val.(string); !ok {
			return false
		}
	}
	return true
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
