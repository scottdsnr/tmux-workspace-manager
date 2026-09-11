package main

import "testing"

func TestIsValidAction(t *testing.T) {
	for _, a := range validActions {
		if !isValidAction(a) {
			t.Errorf("isValidAction(%q) = false, want true", a)
		}
	}
	if isValidAction("bogus") {
		t.Error("isValidAction(\"bogus\") = true, want false")
	}
}

func TestParseArgsBareInvocation(t *testing.T) {
	res, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.action != "" || res.project != "" {
		t.Fatalf("expected empty action/project, got %+v", res)
	}
}

func TestParseArgsHelpShortCircuits(t *testing.T) {
	// -h wins even alongside an otherwise-invalid action, since main() checks
	// args.help before dispatching on the action at all.
	res, err := parseArgs([]string{"bogus-action", "-h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.help {
		t.Fatal("expected help=true")
	}
}

func TestParseArgsVersion(t *testing.T) {
	res, err := parseArgs([]string{"-v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.showVer {
		t.Fatal("expected showVer=true")
	}
}

func TestParseArgsActionAndProject(t *testing.T) {
	res, err := parseArgs([]string{"up", "myapp", "--dry-run"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.action != "up" || res.project != "myapp" || !res.dryRun {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseArgsAllFlags(t *testing.T) {
	res, err := parseArgs([]string{"down", "myapp", "-y", "--no-emoji", "--raw"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.yes || !res.noEmoji || !res.raw {
		t.Fatalf("expected all flags set, got %+v", res)
	}
}

func TestParseArgsInvalidAction(t *testing.T) {
	if _, err := parseArgs([]string{"frobnicate"}); err == nil {
		t.Fatal("expected an error for an invalid action")
	}
}

func TestParseArgsUnrecognizedFlag(t *testing.T) {
	if _, err := parseArgs([]string{"up", "myapp", "--bogus"}); err == nil {
		t.Fatal("expected an error for an unrecognized flag")
	}
}

func TestParseArgsTooManyPositionals(t *testing.T) {
	if _, err := parseArgs([]string{"up", "myapp", "extra"}); err == nil {
		t.Fatal("expected an error for a third positional argument")
	}
}

func TestParseArgsBareDashIsPositional(t *testing.T) {
	// A lone "-" isn't a flag (common shell convention for "stdin"); it
	// should fall through as a positional and fail as an invalid *action*,
	// not get rejected as an unrecognized flag.
	_, err := parseArgs([]string{"-"})
	if err == nil {
		t.Fatal("expected an error")
	}
	const want = `argument action: invalid choice: "-"`
	if got := err.Error(); len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("got error %q, want it to start with %q", got, want)
	}
}
