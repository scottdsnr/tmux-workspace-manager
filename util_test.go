package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandUser(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := map[string]string{
		"~":         home,
		"~/foo/bar": filepath.Join(home, "foo/bar"),
		"/abs/path": "/abs/path",
		"relative":  "relative",
		"":          "",
	}
	for in, want := range cases {
		if got := expandUser(in); got != want {
			t.Errorf("expandUser(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveProjectPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := []struct {
		name, rawPath, baseDir, want string
	}{
		{"absolute path ignores base", "/opt/app", "~/code", "/opt/app"},
		{"relative joins base", "myapp", "~/code", filepath.Join(home, "code", "myapp")},
		{"tilde path ignores base", "~/elsewhere", "~/code", filepath.Join(home, "elsewhere")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveProjectPath(tc.rawPath, tc.baseDir); got != tc.want {
				t.Errorf("resolveProjectPath(%q, %q) = %q, want %q", tc.rawPath, tc.baseDir, got, tc.want)
			}
		})
	}
}

func TestResolveWindowDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := filepath.Join(home, "code", "myapp")

	cases := []struct {
		name string
		win  Window
		want string
	}{
		{"no path uses project dir", Window{}, projectDir},
		{"relative path joins project dir", Window{Path: "backend"}, filepath.Join(projectDir, "backend")},
		{"absolute path used as-is", Window{Path: "/srv/other"}, "/srv/other"},
		{"tilde path expands", Window{Path: "~/scratch"}, filepath.Join(home, "scratch")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveWindowDir(projectDir, tc.win); got != tc.want {
				t.Errorf("resolveWindowDir(%q, %+v) = %q, want %q", projectDir, tc.win, got, tc.want)
			}
		})
	}
}

func TestResolvePaneDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	winDir := filepath.Join(home, "code", "myapp", "backend")

	cases := []struct {
		name string
		pane Pane
		want string
	}{
		{"no path uses window dir", Pane{}, winDir},
		{"relative path joins window dir", Pane{Path: "logs"}, filepath.Join(winDir, "logs")},
		{"absolute path used as-is", Pane{Path: "/srv/other"}, "/srv/other"},
		{"tilde path expands", Pane{Path: "~/scratch"}, filepath.Join(home, "scratch")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolvePaneDir(winDir, tc.pane); got != tc.want {
				t.Errorf("resolvePaneDir(%q, %+v) = %q, want %q", winDir, tc.pane, got, tc.want)
			}
		})
	}
}

func TestShlexSplit(t *testing.T) {
	cases := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"vim", []string{"vim"}, false},
		{"code --wait", []string{"code", "--wait"}, false},
		{"  spaced   out  ", []string{"spaced", "out"}, false},
		{`arg 'single quoted'`, []string{"arg", "single quoted"}, false},
		{`arg "double quoted"`, []string{"arg", "double quoted"}, false},
		{`say "she said \"hi\""`, []string{"say", `she said "hi"`}, false},
		{`escaped\ space`, []string{"escaped space"}, false},
		{"", nil, false},
		{`unbalanced 'quote`, nil, true},
		{`unbalanced "quote`, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := shlexSplit(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got none (result: %v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":            "''",
		"plain":       "plain",
		"a-b_c.d/e:f": "a-b_c.d/e:f",
		"has space":   `'has space'`,
		"it's":        `'it'"'"'s'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"tmux", "send-keys", "-t", "%1", "echo hi"})
	want := `tmux send-keys -t %1 'echo hi'`
	if got != want {
		t.Errorf("shellJoin(...) = %q, want %q", got, want)
	}
}

func TestFirstExisting(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(existing, nil, 0644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.txt")

	if got := firstExisting(missing, existing); got != existing {
		t.Errorf("firstExisting = %q, want %q", got, existing)
	}
	if got := firstExisting(missing); got != "" {
		t.Errorf("firstExisting = %q, want empty", got)
	}
}
