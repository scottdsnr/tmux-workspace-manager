package main

import "testing"

func TestBuildPaneCommand(t *testing.T) {
	cases := []struct {
		name string
		pane Pane
		want string
	}{
		{"command only", Pane{Command: "claude"}, "claude"},
		{"empty pane", Pane{}, ""},
		{
			"single env var, no command",
			Pane{Env: map[string]string{"FOO": "bar"}},
			"export FOO=bar",
		},
		{
			"env vars sorted regardless of map order",
			Pane{Command: "go run .", Env: map[string]string{"ZEBRA": "1", "APPLE": "2"}},
			"export APPLE=2 ZEBRA=1 && go run .",
		},
		{
			"env value needing quoting",
			Pane{Command: "echo hi", Env: map[string]string{"MSG": "has space"}},
			`export MSG='has space' && echo hi`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildPaneCommand(tc.pane.Env, tc.pane.Command); got != tc.want {
				t.Errorf("buildPaneCommand(%+v) = %q, want %q", tc.pane, got, tc.want)
			}
		})
	}
}

func TestResolvePaneEnv(t *testing.T) {
	cases := []struct {
		name                       string
		config, window, pane, want map[string]string
	}{
		{"all empty", nil, nil, nil, nil},
		{
			"workspace env reaches a pane that sets none",
			map[string]string{"A": "1"}, nil, nil,
			map[string]string{"A": "1"},
		},
		{
			"narrower scopes override wider ones key by key",
			map[string]string{"A": "1", "B": "1"},
			map[string]string{"B": "2", "C": "2"},
			map[string]string{"C": "3"},
			map[string]string{"A": "1", "B": "2", "C": "3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolvePaneEnv(tc.config, tc.window, tc.pane)
			if len(got) != len(tc.want) {
				t.Fatalf("resolvePaneEnv() = %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("resolvePaneEnv()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}
