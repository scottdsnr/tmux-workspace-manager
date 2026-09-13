package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

// workspacesDoc is the consolidated on-disk shape: every workspace alias
// mapped to its profile (the same generic shape a single <alias>.yml used
// to hold).
type workspacesDoc map[string]map[string]interface{}

// legacyWorkspaceFiles returns alias -> filename for every pre-consolidation
// standalone profile still sitting in configDir (skipping workspacesPath
// itself, the flat legacy settings.json, and anything that isn't a
// top-level .yml/.json file). .yml takes priority over .json per alias,
// matching the old findConfigPath lookup order.
func legacyWorkspaceFiles() map[string]string {
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil
	}
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == filepath.Base(workspacesPath) || name == "settings.json" {
			continue
		}
		switch {
		case strings.HasSuffix(name, ".yml"):
			files[strings.TrimSuffix(name, ".yml")] = name
		case strings.HasSuffix(name, ".json"):
			alias := strings.TrimSuffix(name, ".json")
			if _, exists := files[alias]; !exists {
				files[alias] = name
			}
		}
	}
	return files
}

// loadWorkspacesDoc reads the consolidated workspaces file. A missing file
// is not an error — it just means no workspace has been saved yet (or
// existing per-alias profiles haven't been migrated with `upgrade`).
func loadWorkspacesDoc() (workspacesDoc, error) {
	doc := workspacesDoc{}
	data, err := os.ReadFile(workspacesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", workspacesPath, err)
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", workspacesPath, err)
	}
	if doc == nil {
		doc = workspacesDoc{}
	}
	return doc, nil
}

// saveWorkspacesDoc writes doc back to workspacesPath, aliases sorted for a
// deterministic, diff-friendly file.
func saveWorkspacesDoc(doc workspacesDoc) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	aliases := make([]string, 0, len(doc))
	for alias := range doc {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, alias := range aliases {
		keyNode := &yaml.Node{}
		if err := keyNode.Encode(alias); err != nil {
			return err
		}
		valNode, err := encodeOrdered(doc[alias], topLevelOrder)
		if err != nil {
			return err
		}
		root.Content = append(root.Content, keyNode, valNode)
	}
	data, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(workspacesPath, data, 0644)
}

// findConfigPath reports whether alias exists in the consolidated
// workspaces file, returning workspacesPath if so (the historical callers
// of this function only ever checked for "" vs. a real path).
func findConfigPath(alias string) string {
	doc, err := loadWorkspacesDoc()
	if err != nil {
		return ""
	}
	if _, ok := doc[alias]; ok {
		return workspacesPath
	}
	return ""
}

// notMigratedErr builds a helpful error when alias isn't in the
// consolidated file but a pre-consolidation standalone profile for it still
// is, pointing the user at `upgrade` instead of just saying "not found".
func notMigratedErr(alias string) error {
	if _, ok := legacyWorkspaceFiles()[alias]; ok {
		return fmt.Errorf("workspace alias '%s' hasn't been migrated to %s yet — run '%s upgrade'", alias, workspacesPath, progName())
	}
	return fmt.Errorf("workspace alias '%s' does not exist", alias)
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
	return raw, workspacesPath
}

// loadConfigRawErr is the non-fatal counterpart to loadConfigRaw, for
// callers (TUI screens) that need to report failure themselves rather than
// exiting the whole process.
func loadConfigRawErr(alias string) (map[string]interface{}, error) {
	doc, err := loadWorkspacesDoc()
	if err != nil {
		return nil, err
	}
	raw, ok := doc[alias]
	if !ok {
		return nil, notMigratedErr(alias)
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

// loadConfigTypedErr is the non-fatal counterpart to loadConfigTyped. It
// re-marshals the raw profile map back to YAML and decodes that into
// Config, reusing the same decode path (custom Teardown unmarshaling
// included) a standalone file would have gone through.
func loadConfigTypedErr(alias string) (Config, error) {
	raw, err := loadConfigRawErr(alias)
	if err != nil {
		return Config{}, err
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse workspace '%s': %w", alias, err)
	}
	return cfg, nil
}

var topLevelOrder = []string{"project_name_display", "project_path", "windows", "teardown"}

// saveConfigRaw writes a workspace profile into the consolidated workspaces
// file, with known top-level keys in a stable, readable order followed by
// any unrecognized keys sorted alphabetically.
func saveConfigRaw(alias string, cfg map[string]interface{}) (string, error) {
	doc, err := loadWorkspacesDoc()
	if err != nil {
		return "", err
	}
	doc[alias] = cfg
	if err := saveWorkspacesDoc(doc); err != nil {
		return "", err
	}
	return workspacesPath, nil
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
// key, else "". Path separators and ".." are still rejected even though
// aliases no longer name a file directly — tmux targets it as a literal
// "alias:window" string, where a slash would be ambiguous.
func validateAlias(alias string) string {
	if alias == "" || containsSpace(alias) {
		return "cannot be empty or contain spaces"
	}
	if alias == "settings" {
		return "'settings' is reserved"
	}
	if strings.ContainsAny(alias, "/\\") || strings.Contains(alias, "..") {
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
