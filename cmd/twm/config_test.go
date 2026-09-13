package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func validRawConfig() map[string]interface{} {
	return map[string]interface{}{
		"project_path": "myapp",
		"windows": []interface{}{
			map[string]interface{}{
				"name": "AI",
				"panes": []interface{}{
					map[string]interface{}{"command": "claude"},
				},
			},
		},
	}
}

func TestValidateConfigValid(t *testing.T) {
	if errs := validateConfig(validRawConfig()); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateConfigMissingProjectPath(t *testing.T) {
	cfg := validRawConfig()
	delete(cfg, "project_path")
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "project_path must be a non-empty string" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigEmptyProjectPath(t *testing.T) {
	cfg := validRawConfig()
	cfg["project_path"] = ""
	errs := validateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %v", errs)
	}
}

func TestValidateConfigMissingWindows(t *testing.T) {
	cfg := validRawConfig()
	delete(cfg, "windows")
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "windows must be a non-empty list" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigEmptyWindows(t *testing.T) {
	cfg := validRawConfig()
	cfg["windows"] = []interface{}{}
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "windows must be a non-empty list" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigWindowsNotList(t *testing.T) {
	cfg := validRawConfig()
	cfg["windows"] = "not a list"
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "windows must be a non-empty list" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigWindowMissingName(t *testing.T) {
	cfg := validRawConfig()
	cfg["windows"] = []interface{}{
		map[string]interface{}{"panes": []interface{}{map[string]interface{}{"command": "x"}}},
	}
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "window #1 is missing a 'name'" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigWindowNotAMap(t *testing.T) {
	cfg := validRawConfig()
	cfg["windows"] = []interface{}{"not a map"}
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "window #1 is missing a 'name'" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigWindowNoPanes(t *testing.T) {
	cfg := validRawConfig()
	cfg["windows"] = []interface{}{
		map[string]interface{}{"name": "AI", "panes": []interface{}{}},
	}
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "window 'AI' must have at least one pane" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigWindowMissingPanesKey(t *testing.T) {
	cfg := validRawConfig()
	cfg["windows"] = []interface{}{
		map[string]interface{}{"name": "AI"},
	}
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "window 'AI' must have at least one pane" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigPaneNotAnObject(t *testing.T) {
	cfg := validRawConfig()
	cfg["windows"] = []interface{}{
		map[string]interface{}{"name": "AI", "panes": []interface{}{"echo hi"}},
	}
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "window 'AI' has a pane that is not an object" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigDuplicateWindowNames(t *testing.T) {
	cfg := validRawConfig()
	cfg["windows"] = []interface{}{
		map[string]interface{}{"name": "AI", "panes": []interface{}{map[string]interface{}{"command": "a"}}},
		map[string]interface{}{"name": "AI", "panes": []interface{}{map[string]interface{}{"command": "b"}}},
	}
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "window name 'AI' is used more than once" {
		t.Fatalf("expected a single duplicate-name error, got %v", errs)
	}
}

func TestValidateConfigWindowLayout(t *testing.T) {
	cfg := validRawConfig()
	windows, _ := cfg["windows"].([]interface{})
	win, _ := windows[0].(map[string]interface{})
	win["layout"] = 5
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "window 'AI' layout must be a string" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigPanePath(t *testing.T) {
	cfg := validRawConfig()
	cfg["windows"] = []interface{}{
		map[string]interface{}{
			"name":  "AI",
			"panes": []interface{}{map[string]interface{}{"command": "x", "path": 5}},
		},
	}
	errs := validateConfig(cfg)
	if len(errs) != 1 || errs[0] != "window 'AI' has a pane with a non-string 'path'" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateConfigPaneEnv(t *testing.T) {
	cases := []struct {
		name    string
		env     interface{}
		wantErr bool
	}{
		{"valid map of strings", map[string]interface{}{"FOO": "bar"}, false},
		{"non-string value", map[string]interface{}{"FOO": 5}, true},
		{"not a map", "FOO=bar", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validRawConfig()
			cfg["windows"] = []interface{}{
				map[string]interface{}{
					"name":  "AI",
					"panes": []interface{}{map[string]interface{}{"command": "x", "env": tc.env}},
				},
			}
			errs := validateConfig(cfg)
			hasEnvErr := false
			for _, e := range errs {
				if e == "window 'AI' has a pane whose 'env' is not a map of strings" {
					hasEnvErr = true
				}
			}
			if hasEnvErr != tc.wantErr {
				t.Fatalf("env=%v: wantErr=%v, errs=%v", tc.env, tc.wantErr, errs)
			}
		})
	}
}

func TestValidateConfigWorkspaceAndWindowEnv(t *testing.T) {
	cases := []struct {
		name    string
		env     interface{}
		wantErr bool
	}{
		{"valid map of strings", map[string]interface{}{"FOO": "bar"}, false},
		{"non-string value", map[string]interface{}{"FOO": 5}, true},
		{"not a map", "FOO=bar", true},
	}
	for _, tc := range cases {
		t.Run("workspace/"+tc.name, func(t *testing.T) {
			cfg := validRawConfig()
			cfg["env"] = tc.env
			errs := validateConfig(cfg)
			if contains(errs, "env must be a map of strings") != tc.wantErr {
				t.Fatalf("env=%v: wantErr=%v, errs=%v", tc.env, tc.wantErr, errs)
			}
		})
		t.Run("window/"+tc.name, func(t *testing.T) {
			cfg := validRawConfig()
			cfg["windows"] = []interface{}{
				map[string]interface{}{
					"name":  "AI",
					"env":   tc.env,
					"panes": []interface{}{map[string]interface{}{"command": "x"}},
				},
			}
			errs := validateConfig(cfg)
			if contains(errs, "window 'AI' env must be a map of strings") != tc.wantErr {
				t.Fatalf("env=%v: wantErr=%v, errs=%v", tc.env, tc.wantErr, errs)
			}
		})
	}
}

func contains(errs []string, want string) bool {
	for _, e := range errs {
		if e == want {
			return true
		}
	}
	return false
}

func TestValidateConfigTeardownVariants(t *testing.T) {
	cases := []struct {
		name    string
		val     interface{}
		wantErr bool
	}{
		{"string", "./stop.sh", false},
		{"list of strings", []interface{}{"a", "b"}, false},
		{"nil", nil, false},
		{"absent", nil, false}, // handled separately below by omitting the key
		{"number", 5, true},
		{"list with non-string", []interface{}{"a", 5}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validRawConfig()
			if tc.name == "absent" {
				delete(cfg, "teardown")
			} else {
				cfg["teardown"] = tc.val
			}
			errs := validateConfig(cfg)
			hasTeardownErr := false
			for _, e := range errs {
				if e == "teardown must be a string or a list of strings" {
					hasTeardownErr = true
				}
			}
			if hasTeardownErr != tc.wantErr {
				t.Fatalf("teardown=%v: wantErr=%v, errs=%v", tc.val, tc.wantErr, errs)
			}
		})
	}
}

func TestValidateTypedConfigValid(t *testing.T) {
	errs := validateTypedConfig("myapp", []Window{
		{Name: "AI", Panes: []Pane{{Command: "claude"}}},
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateTypedConfigEmptyProjectPath(t *testing.T) {
	errs := validateTypedConfig("", []Window{{Name: "AI", Panes: []Pane{{Command: "x"}}}})
	if len(errs) != 1 || errs[0] != "project_path must be a non-empty string" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateTypedConfigNoWindows(t *testing.T) {
	errs := validateTypedConfig("myapp", nil)
	if len(errs) != 1 || errs[0] != "windows must be a non-empty list" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateTypedConfigMissingName(t *testing.T) {
	errs := validateTypedConfig("myapp", []Window{{Panes: []Pane{{Command: "x"}}}})
	if len(errs) != 1 || errs[0] != "window #1 is missing a 'name'" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateTypedConfigNoPanes(t *testing.T) {
	errs := validateTypedConfig("myapp", []Window{{Name: "AI"}})
	if len(errs) != 1 || errs[0] != "window 'AI' must have at least one pane" {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateTypedConfigDuplicateNames(t *testing.T) {
	errs := validateTypedConfig("myapp", []Window{
		{Name: "AI", Panes: []Pane{{Command: "a"}}},
		{Name: "AI", Panes: []Pane{{Command: "b"}}},
	})
	if len(errs) != 1 || errs[0] != "window name 'AI' is used more than once" {
		t.Fatalf("expected a single duplicate-name error, got %v", errs)
	}
}

func TestConfigUnmarshalPaneAndLayout(t *testing.T) {
	doc := `
project_path: myapp
windows:
  - name: AI
    layout: main-vertical
    panes:
      - command: claude
        path: sub
        env:
          FOO: bar
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(cfg.Windows))
	}
	win := cfg.Windows[0]
	if win.Layout != "main-vertical" {
		t.Errorf("Layout = %q, want %q", win.Layout, "main-vertical")
	}
	if len(win.Panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(win.Panes))
	}
	pane := win.Panes[0]
	if pane.Path != "sub" {
		t.Errorf("Pane.Path = %q, want %q", pane.Path, "sub")
	}
	if pane.Env["FOO"] != "bar" {
		t.Errorf("Pane.Env[FOO] = %q, want %q", pane.Env["FOO"], "bar")
	}
}

func TestTeardownUnmarshalYAML(t *testing.T) {
	cases := []struct {
		name    string
		yamlDoc string
		want    Teardown
		wantErr bool
	}{
		{"empty scalar", `teardown: ""`, Teardown{}, false},
		{"single scalar", `teardown: "./stop.sh"`, Teardown{"./stop.sh"}, false},
		{"sequence", "teardown:\n  - a\n  - b", Teardown{"a", "b"}, false},
		{"empty sequence", "teardown: []", Teardown{}, false},
		{"mapping is invalid", "teardown:\n  key: value", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var doc struct {
				Teardown Teardown `yaml:"teardown"`
			}
			err := yaml.Unmarshal([]byte(tc.yamlDoc), &doc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(doc.Teardown) != len(tc.want) {
				t.Fatalf("got %#v, want %#v", doc.Teardown, tc.want)
			}
			for i := range tc.want {
				if doc.Teardown[i] != tc.want[i] {
					t.Fatalf("got %#v, want %#v", doc.Teardown, tc.want)
				}
			}
		})
	}
}

func TestValidateAlias(t *testing.T) {
	cases := []struct {
		alias   string
		wantErr bool
	}{
		{"myapp", false},
		{"my-app_2", false},
		{"", true},
		{"has space", true},
		{"settings", true},
		{"../escape", true},
		{"sub/dir", true},
	}
	for _, tc := range cases {
		t.Run(tc.alias, func(t *testing.T) {
			got := validateAlias(tc.alias)
			if (got != "") != tc.wantErr {
				t.Fatalf("validateAlias(%q) = %q, wantErr=%v", tc.alias, got, tc.wantErr)
			}
		})
	}
}
