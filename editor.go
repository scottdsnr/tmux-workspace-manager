package main

import (
	"fmt"
	"os"
	"os/exec"
)

func openInEditor(path string) {
	editorCmd := os.Getenv("EDITOR")
	if editorCmd == "" {
		editorCmd = settings.Editor
	}

	parts, err := shlexSplit(editorCmd)
	if err != nil || len(parts) == 0 {
		parts = []string{editorCmd}
	}

	fmt.Printf("%sOpening %s with %s...\n", sym("edit"), path, editorCmd)

	runErr := runInteractive(append(append([]string{}, parts...), path))
	if runErr != nil {
		if _, isExitErr := runErr.(*exec.ExitError); !isExitErr {
			// The editor itself never started (not found / not permitted) —
			// mirrors Python catching FileNotFoundError/PermissionError only.
			errPrint("%sEditor '%s' not found. Falling back to nano.", sym("warn"), editorCmd)
			if _, lookErr := exec.LookPath("nano"); lookErr != nil {
				fatal("%snano is not available either. Set $EDITOR or 'editor' in %s.", sym("error"), settingsPath)
			}
			runErr = runInteractive([]string{"nano", path})
		}
	}

	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			errPrint("%sEditor exited with code %d.", sym("warn"), ee.ExitCode())
		}
	}
}

func runInteractive(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
