# tmux-workspace

A small YAML-driven tmux session manager. Define a project's windows, panes,
startup commands, and teardown steps once; then bring the whole workspace up
or down with a single command.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/scottdsnr/tmux-workspace-manager/master/install.sh | bash
```

This downloads a prebuilt release binary for your OS/architecture (Linux or
macOS, amd64 or arm64) to `~/.local/bin/twm`, verifies its
checksum, and makes it executable. No Python, no Go toolchain, no other
runtime required on the target machine — just `tmux` itself. Inspect
[`install.sh`](./install.sh) before piping it to `bash` if you'd like to see
exactly what it does.

If `~/.local/bin` isn't already on your `PATH`, the installer will tell you
what to add to your shell profile.

**Requirements:** `tmux`. That's it — the installed binary is self-contained.

**Manual install:** download the archive for your platform from the
[latest release](https://github.com/scottdsnr/tmux-workspace-manager/releases/latest),
extract it, and put the `twm` binary anywhere on your `PATH`.
Everything below assumes the command is called `twm`; substitute
your own name/path if you installed it differently.

**Building from source:** with a Go toolchain installed, either
`go install github.com/scottdsnr/tmux-workspace-manager/cmd/twm@latest`,
or clone the repo and run `go build -o twm ./cmd/twm`.

## Quick start

Run `twm` with no arguments in a terminal to open the interactive
dashboard: browse profiles, bring one up or down, create/edit a profile, and
edit settings, all without leaving the TUI.

```sh
twm                    # interactive dashboard
twm create myproject   # interactive wizard, writes a profile
twm up myproject       # builds the tmux session and attaches
twm down myproject     # runs teardown, then kills the session
twm list               # show all profiles and which are running
```

Every command above also works non-interactively — piped, redirected, or run
with `--dry-run` — for scripting and CI, falling back to plain text output
with no TUI involved.

## Commands

| Command                       | Description                                                        |
|--------------------------------|---------------------------------------------------------------------|
| *(no arguments)*                | Open the interactive dashboard (a TTY); plain usage otherwise.     |
| `list`                          | Dashboard on a TTY; plain list of profiles and status otherwise.   |
| `create [alias]`                | Interactive wizard (or piped-stdin wizard) that adds a new profile to `workspaces.yml`. |
| `up <alias>`                    | Build (or attach to) the tmux session for a profile.                |
| `up <alias> --dry-run`          | Print the tmux/teardown commands without running them.              |
| `down <alias>`                  | Gracefully stop panes, run teardown, then kill the session.         |
| `down <alias> -y`               | Same, without the confirmation prompt.                              |
| `down <alias> --dry-run`        | Preview what `down` would do without touching the session.          |
| `edit <alias>`                  | Interactive wizard to modify an existing profile.                   |
| `edit <alias> --raw`            | Open `workspaces.yml` directly in `$EDITOR` (all profiles, not just this one). |
| `validate <alias>`              | Check a profile's YAML for structural problems.                     |
| `config`                        | Interactive settings form on a TTY; opens `$EDITOR` otherwise (or with `--raw`). |
| `upgrade`                       | Fold legacy per-alias `.yml`/`.json` profiles into `workspaces.yml`, and convert legacy `.json` settings to `.yml`. |
| `upgrade --dry-run`             | Preview what `upgrade` would convert without changing anything.     |
| `update`                        | Check GitHub for a newer release and install it in place.           |
| `update --dry-run`              | Check for a newer release without installing it.                    |

Run `twm --help` for the full flag list, including `--no-emoji`.

## Updating

```sh
twm update
```

Checks the latest GitHub release, and if it's newer than the running binary,
downloads it, verifies its checksum, and replaces the current executable in
place — no need to re-run the installer or remember its URL. Pass `-y` to
skip the confirmation prompt (handy for scripting), or `--dry-run` to just
see whether an update is available. Only Linux and macOS on amd64/arm64 have
prebuilt binaries; a `go install`-built binary should instead be updated by
re-running `go install github.com/scottdsnr/tmux-workspace-manager/cmd/twm@latest`.

## Profiles

Every profile lives together in `~/.config/tmux-workspaces/workspaces.yml`,
keyed by alias. Each entry describes a tmux session named after its alias:

```yaml
tabs:
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
    - name: App
      layout: main-vertical
      panes:
        - command: npm run dev
          path: frontend
          env:
            NODE_ENV: development
            PORT: "3000"
        - command: tail -f storage/logs/laravel.log
          path: backend
  teardown:
    - ./vendor/bin/sail down
```

- **`project_path`** — the project's working directory. Relative paths are
  resolved against the `base_dir` setting (see below), not the directory
  `twm` was run from.
- **`windows`** — created in order; each pane after the first splits the
  window horizontally, then `layout` (if set) rearranges all of them.
  - **`name`** — the tmux window name.
  - **`path`** (optional) — working directory for this window only, relative
    to `project_path` (or absolute).
  - **`layout`** (optional) — applied via `tmux select-layout` once every
    pane in the window exists. Accepts tmux's built-in presets
    (`even-horizontal`, `even-vertical`, `main-horizontal`, `main-vertical`,
    `tiled`) or a literal tmux layout string (e.g. one copied from
    `tmux list-windows -F '#{window_layout}'`).
  - **`on_stop`** (optional) — a command sent to the window's first pane
    instead of Ctrl-C when running `down`. Use this for anything that needs a
    graceful exit (e.g. `/exit` for a Claude Code session, `:q` for an editor
    pane you don't want interrupted).
  - **`panes`** — list of pane definitions.
    - **`command`** — command to run in the pane. An empty `command` just
      opens a plain shell.
    - **`path`** (optional) — working directory for this pane only, relative
      to the window's `path` (or absolute). Overrides the window's directory
      for just this one pane.
    - **`env`** (optional) — map of environment variables exported in the
      pane before `command` runs (e.g. `NODE_ENV: development`).
- **`teardown`** — commands run (and waited on) in `project_path` after
  panes are signaled to stop, before the session is killed. A single string
  is also accepted.

### Migrating older installs

Versions before profiles were consolidated wrote one `<alias>.yml` (or
`<alias>.json`) file per workspace. `twm` still finds those, but only well
enough to tell you to migrate — commands report which alias needs it. Run:

```sh
twm upgrade            # fold standalone profiles into workspaces.yml
twm upgrade --dry-run  # preview what would be converted first
```

This merges every standalone `<alias>.yml`/`<alias>.json` profile into
`workspaces.yml`, renaming each original to `<alias>.yml.bak` (never
deleted, so nothing is lost) — an alias already present in `workspaces.yml`
is left untouched rather than overwritten. The same run also converts a
legacy `.json` settings file to `.yml`, including a flat `settings.json`
sitting directly in `~/.config/tmux-workspaces/` from older installs, moving
it into `.settings/settings.yml` where the tool actually reads it. Already
migrated aliases are skipped, so it's safe to run more than once.

`down` sends Ctrl-C to every pane without an `on_stop` command, waits briefly,
runs `teardown`, then kills the session. It never interrupts the pane you're
currently attached in.

## Global settings

`twm config` opens `~/.config/tmux-workspaces/.settings/settings.yml`,
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
rm "$(command -v twm)"
```

Your profiles and settings live in `~/.config/tmux-workspaces/` and are left
in place — remove that directory too if you want a clean slate.
