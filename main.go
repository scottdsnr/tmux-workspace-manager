package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// version is overwritten at build time via -ldflags "-X main.version=...".
var version = "dev"

var validActions = []string{"up", "down", "list", "create", "edit", "validate", "config", "upgrade"}

type cliArgs struct {
	action  string
	project string
	raw     bool
	dryRun  bool
	yes     bool
	noEmoji bool
	help    bool
	showVer bool
}

func isValidAction(a string) bool {
	for _, v := range validActions {
		if v == a {
			return true
		}
	}
	return false
}

func parseArgs(args []string) (*cliArgs, error) {
	res := &cliArgs{}
	var positionals []string

	for _, a := range args {
		switch a {
		case "-h", "--help":
			res.help = true
		case "-v", "--version":
			res.showVer = true
		case "--raw":
			res.raw = true
		case "--dry-run":
			res.dryRun = true
		case "-y", "--yes":
			res.yes = true
		case "--no-emoji":
			res.noEmoji = true
		default:
			if strings.HasPrefix(a, "-") && a != "-" {
				return nil, fmt.Errorf("unrecognized arguments: %s", a)
			}
			positionals = append(positionals, a)
		}
	}

	if res.help || res.showVer {
		return res, nil
	}

	if len(positionals) == 0 {
		return nil, fmt.Errorf("the following arguments are required: action")
	}
	res.action = positionals[0]
	if !isValidAction(res.action) {
		return nil, fmt.Errorf("argument action: invalid choice: %q (choose from %s)", res.action, strings.Join(validActions, ", "))
	}
	if len(positionals) > 1 {
		res.project = positionals[1]
	}
	if len(positionals) > 2 {
		return nil, fmt.Errorf("unrecognized arguments: %s", strings.Join(positionals[2:], " "))
	}
	return res, nil
}

func progName() string {
	return filepath.Base(os.Args[0])
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, "usage: %s [-h] [--raw] [--dry-run] [-y] [--no-emoji]\n", progName())
	fmt.Fprintf(w, "%s{%s} [project]\n\n", strings.Repeat(" ", len(progName())+8), strings.Join(validActions, ","))
	fmt.Fprintln(w, "Centralized YAML-Driven TMUX Workspace Manager")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "positional arguments:")
	fmt.Fprintf(w, "  {%s}\n", strings.Join(validActions, ","))
	fmt.Fprintln(w, "                        Workspace lifecycle command")
	fmt.Fprintln(w, "  project               The short alias profile filename string")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "options:")
	fmt.Fprintln(w, "  -h, --help            show this help message and exit")
	fmt.Fprintln(w, "  -v, --version         show the installed version and exit")
	fmt.Fprintln(w, "  --raw                 Open raw YAML instead of the wizard menu (edit)")
	fmt.Fprintln(w, "  --dry-run             Print the tmux/teardown commands without executing them")
	fmt.Fprintln(w, "                        (up/down/upgrade)")
	fmt.Fprintln(w, "  -y, --yes             Skip the confirmation prompt (down)")
	fmt.Fprintln(w, "  --no-emoji            Disable emoji in output")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "examples:")
	name := progName()
	fmt.Fprintf(w, "  %s list\n", name)
	fmt.Fprintf(w, "  %s create myapp\n", name)
	fmt.Fprintf(w, "  %s up myapp\n", name)
	fmt.Fprintf(w, "  %s up myapp --dry-run\n", name)
	fmt.Fprintf(w, "  %s down myapp --yes\n", name)
	fmt.Fprintf(w, "  %s edit myapp\n", name)
	fmt.Fprintf(w, "  %s edit myapp --raw\n", name)
	fmt.Fprintf(w, "  %s validate myapp\n", name)
	fmt.Fprintf(w, "  %s config\n", name)
	fmt.Fprintf(w, "  %s upgrade\n", name)
	fmt.Fprintf(w, "  %s upgrade --dry-run\n", name)
}

func main() {
	args, err := parseArgs(os.Args[1:])
	if err != nil {
		printUsage(os.Stderr)
		fmt.Fprintf(os.Stderr, "\n%s: error: %v\n", progName(), err)
		os.Exit(2)
	}
	if args.help {
		printUsage(os.Stdout)
		return
	}
	if args.showVer {
		fmt.Printf("%s %s\n", progName(), version)
		return
	}

	if args.noEmoji {
		settings.UseEmoji = false // applied before loadSettings() so its own warnings honor the flag too
	}
	settings = loadSettings()
	if args.noEmoji {
		settings.UseEmoji = false
	}

	switch args.action {
	case "create":
		createWorkspaceWizard(args.project)
	case "config":
		editSettings()
	case "upgrade":
		upgradeConfigs(args.dryRun)
	case "list":
		listWorkspaces()
	case "up":
		if args.project == "" {
			listWorkspaces()
		} else {
			startWorkspace(args.project, args.dryRun)
		}
	case "down":
		if args.project == "" {
			listWorkspaces()
		} else {
			stopWorkspace(args.project, args.dryRun, args.yes)
		}
	case "edit":
		if args.project == "" {
			listWorkspaces()
		} else if args.raw {
			editWorkspaceRaw(args.project)
		} else {
			editWorkspaceInteractive(args.project)
		}
	case "validate":
		if args.project == "" {
			listWorkspaces()
		} else {
			validateWorkspaceCmd(args.project)
		}
	}
}
