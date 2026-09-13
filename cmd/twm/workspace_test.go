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
			if got := buildPaneCommand(tc.pane); got != tc.want {
				t.Errorf("buildPaneCommand(%+v) = %q, want %q", tc.pane, got, tc.want)
			}
		})
	}
}
