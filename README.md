# tmux-workspace

A small YAML-driven tmux session manager. Define a project's windows, panes,
startup commands, and teardown steps once; then bring the whole workspace up
or down with a single command.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/scottdsnr/tmux-workspace-manager/master/install.sh | bash
```

This downloads a prebuilt release binary for your OS/architecture (Linux or
macOS, amd64 or arm64) to `~/.local/bin/tmux-workspace`, verifies its
checksum, and makes it executable. No Python, no Go toolchain, no other
runtime required on the target machine — just `tmux` itself. Inspect
[`install.sh`](./install.sh) before piping it to `bash` if you'd like to see
exactly what it does.

If `~/.local/bin` isn't already on your `PATH`, the installer will tell you
what to add to your shell profile.

**Requirements:** `tmux`. That's it — the installed binary is self-contained.

**Manual install:** download the archive for your platform from the
[latest release](https://github.com/scottdsnr/tmux-workspace-manager/releases/latest),
extract it, and put the `tmux-workspace` binary anywhere on your `PATH`.
Everything below assumes the command is called `tmux-workspace`; substitute
your own name/path if you installed it differently.

**Building from source:** with a Go toolchain installed, either
`go install github.com/scottdsnr/tmux-workspace-manager@latest`, or clone the
repo and run `go build -o tmux-workspace .`.

## Quick start

Run `tmux-workspace` with no arguments in a terminal to open the interactive
dashboard: browse profiles, bring one up or down, create/edit a profile, and
edit settings, all without leaving the TUI.

```sh
tmux-workspace                    # interactive dashboard
tmux-workspace create myproject   # interactive wizard, writes a profile
tmux-workspace up myproject       # builds the tmux session and attaches
tmux-workspace down myproject     # runs teardown, then kills the session
tmux-workspace list               # show all profiles and which are running
```

Every command above also works non-interactively — piped, redirected, or run
with `--dry-run` — for scripting and CI, falling back to plain text output
with no TUI involved.

## Commands

| Command                       | Description                                                        |
|--------------------------------|---------------------------------------------------------------------|
| *(no arguments)*                | Open the interactive dashboard (a TTY); plain usage otherwise.     |
| `list`                          | Dashboard on a TTY; plain list of profiles and status otherwise.   |
| `create [alias]`                | Interactive wizard (or piped-stdin wizard) that writes a new `<alias>.yml` profile. |
| `up <alias>`                    | Build (or attach to) the tmux session for a profile.                |
| `up <alias> --dry-run`          | Print the tmux/teardown commands without running them.              |
| `down <alias>`                  | Gracefully stop panes, run teardown, then kill the session.         |
| `down <alias> -y`               | Same, without the confirmation prompt.                              |
| `down <alias> --dry-run`        | Preview what `down` would do without touching the session.          |
| `edit <alias>`                  | Interactive wizard to modify an existing profile.                   |
| `edit <alias> --raw`            | Open the profile's YAML directly in `$EDITOR`.                      |
| `validate <alias>`              | Check a profile's YAML for structural problems.                     |
| `config`                        | Interactive settings form on a TTY; opens `$EDITOR` otherwise (or with `--raw`). |
| `upgrade`                       | Convert legacy `.json` profiles/settings to `.yml` in place.        |
| `upgrade --dry-run`             | Preview what `upgrade` would convert without changing anything.     |
| `update`                        | Check GitHub for a newer release and install it in place.           |
| `update --dry-run`              | Check for a newer release without installing it.                    |

Run `tmux-workspace --help` for the full flag list, including `--no-emoji`.

## Updating

```sh
tmux-workspace update
```

Checks the latest GitHub release, and if it's newer than the running binary,
downloads it, verifies its checksum, and replaces the current executable in
place — no need to re-run the installer or remember its URL. Pass `-y` to
skip the confirmation prompt (handy for scripting), or `--dry-run` to just
see whether an update is available. Only Linux and macOS on amd64/arm64 have
prebuilt binaries; a `go install`-built binary should instead be updated by
re-running `go install github.com/scottdsnr/tmux-workspace-manager@latest`.

## Profiles

Profiles live in `~/.config/tmux-workspaces/<alias>.yml`. Each one describes
a tmux session named after its alias:

```yaml
project_name_display: Tabs
project_path: ~/code/tabs
windows:
  - name: AI
    panes:
      - command: claude
    on_stop: /exit
  - name: Docker-Dev
    path: docker
    panes:
      - command: ./vendor/bin/sail up -d
      - command: ""
teardown:
  - ./vendor/bin/sail down
```

- **`project_path`** — the project's working directory. Relative paths are
  resolved against the `base_dir` setting (see below), not the directory
  `tmux-workspace` was run from.
- **`windows`** — created in order; each pane after the first splits the
  window horizontally.
  - **`name`** — the tmux window name.
  - **`panes`** — list of `{ command: "..." }`. An empty `command` just
    opens a plain shell.
  - **`path`** (optional) — working directory for this window only, relative
    to `project_path` (or absolute).
  - **`on_stop`** (optional) — a command sent to the window's first pane
    instead of Ctrl-C when running `down`. Use this for anything that needs a
    graceful exit (e.g. `/exit` for a Claude Code session, `:q` for an editor
    pane you don't want interrupted).
- **`teardown`** — commands run (and waited on) in `project_path` after
  panes are signaled to stop, before the session is killed. A single string
  is also accepted.

### Migrating from JSON

Older profiles written as `<alias>.json` (and `.settings/settings.json`) are
still read automatically — valid JSON is valid YAML. Run:

```sh
tmux-workspace upgrade            # convert every legacy .json config to .yml
tmux-workspace upgrade --dry-run  # preview what would be converted first
```

This converts every `<alias>.json` profile and the settings file to `.yml` in
place, renaming each original to `<alias>.json.bak` (never deleted, so
nothing is lost). It also cleans up a flat `settings.json` sitting directly
in `~/.config/tmux-workspaces/` from older installs, moving it into
`.settings/settings.yml` where the tool actually reads it — if you've had one
sitting there unread, this is the fix. Already-converted aliases are skipped,
so it's safe to run more than once.

`down` sends Ctrl-C to every pane without an `on_stop` command, waits briefly,
runs `teardown`, then kills the session. It never interrupts the pane you're
currently attached in.

## Global settings

`tmux-workspace config` opens `~/.config/tmux-workspaces/.settings/settings.yml`,
creating it with defaults on first use:

| Key                  | Default   | Purpose                                                          |
|-----------------------|-----------|-------------------------------------------------------------------|
| `base_dir`            | `"~"`     | Base directory relative paths (in `project_path`) resolve against.|
| `window_base_index`   | `1`       | First window number in sessions this tool creates.                |
| `pane_base_index`     | `1`       | First pane number in each window.                                  |
| `editor`              | `"nano"`  | Fallback editor for `edit --raw` / `config` if `$EDITOR` is unset. |
| `confirm_down`        | `true`    | Prompt for confirmation before `down` (unless `-y` is passed).     |
| `use_emoji`           | `true`    | Toggle emoji in output (also settable per-run with `--no-emoji`).  |

Example: if `base_dir` is `~/code`, a profile with `"project_path": "myapp"`
resolves to `~/code/myapp`. Absolute paths and `~`-paths in `project_path`
are left as-is.

## Uninstall

```sh
rm "$(command -v tmux-workspace)"
```

Your profiles and settings live in `~/.config/tmux-workspaces/` and are left
in place — remove that directory too if you want a clean slate.
