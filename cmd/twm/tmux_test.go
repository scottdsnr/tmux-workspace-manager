package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestTmuxTargetsAreExactMatches(t *testing.T) {
	if got, want := sessionTarget("am"), "=am"; got != want {
		t.Errorf("sessionTarget(\"am\") = %q, want %q", got, want)
	}
	if got, want := sessionPaneTarget("am"), "=am:"; got != want {
		t.Errorf("sessionPaneTarget(\"am\") = %q, want %q", got, want)
	}
	if got, want := windowTarget("am", "Git"), "=am:=Git"; got != want {
		t.Errorf("windowTarget(\"am\", \"Git\") = %q, want %q", got, want)
	}
}

// tmuxTestServer points tmux at a throwaway socket for the duration of the
// test, so these never see (or touch) a tmux server the user is actually
// working in. Skips when tmux isn't installed.
func tmuxTestServer(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	// Short path on purpose: the socket lives under $TMUX_TMPDIR, and unix
	// socket paths have a tight length limit that t.TempDir()'s test-derived
	// names can blow past.
	dir, err := os.MkdirTemp("", "twm")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "") // not "inside" the throwaway server
	dryRun = false

	t.Cleanup(func() {
		exec.Command("tmux", "kill-server").Run()
		os.RemoveAll(dir)
	})
}

// A workspace alias that is a prefix of another one ("am" vs. "am-app") used
// to resolve to its neighbour, because tmux falls back to prefix matching:
// `up am` reported am-app as already running and attached to it, and
// `down am` would have killed it.
func TestHasSessionIgnoresPrefixNeighbour(t *testing.T) {
	tmuxTestServer(t)
	if res := tmuxQuery([]string{"tmux", "new-session", "-d", "-s", "am-app", "-n", "AI"}); res.code != 0 {
		t.Fatalf("could not create test session: exit %d", res.code)
	}

	if hasSession("am") {
		t.Error("hasSession(\"am\") = true while only 'am-app' is running")
	}
	if !hasSession("am-app") {
		t.Error("hasSession(\"am-app\") = false while it is running")
	}
	if sessionAttached("am") {
		t.Error("sessionAttached(\"am\") = true while only 'am-app' is running")
	}
	if got := windowCount("am"); got != 0 {
		t.Errorf("windowCount(\"am\") = %d, want 0", got)
	}
	if got := windowCount("am-app"); got != 1 {
		t.Errorf("windowCount(\"am-app\") = %d, want 1", got)
	}
}

func TestKillSessionLeavesPrefixNeighbourAlone(t *testing.T) {
	tmuxTestServer(t)
	if res := tmuxQuery([]string{"tmux", "new-session", "-d", "-s", "am-app", "-n", "AI"}); res.code != 0 {
		t.Fatalf("could not create test session: exit %d", res.code)
	}

	tmuxKillSession("am")
	if !hasSession("am-app") {
		t.Fatal("killing 'am' took 'am-app' down with it")
	}
}

// Window names match by prefix too, so a window target has to be exact on
// both halves — otherwise `down` sends its on_stop keys into the wrong
// window.
func TestListPaneIDsIgnoresPrefixWindowMatch(t *testing.T) {
	tmuxTestServer(t)
	if res := tmuxQuery([]string{"tmux", "new-session", "-d", "-s", "am-app", "-n", "Laravel"}); res.code != 0 {
		t.Fatalf("could not create test session: exit %d", res.code)
	}

	if ids := listPaneIDs("am-app", "Lara"); len(ids) != 0 {
		t.Errorf("listPaneIDs(\"am-app\", \"Lara\") = %v, want none (only 'Laravel' exists)", ids)
	}
	if ids := listPaneIDs("am-app", "Laravel"); len(ids) != 1 {
		t.Errorf("listPaneIDs(\"am-app\", \"Laravel\") = %v, want 1 pane", ids)
	}
}
