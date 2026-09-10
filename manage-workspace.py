#!/usr/bin/env python3
import os
import shlex
import shutil
import subprocess
import sys
import argparse
import time

try:
    import yaml
except ImportError:
    print("error: PyYAML is required (pip install pyyaml, or your system's python-yaml package).", file=sys.stderr)
    sys.exit(1)

CONFIG_DIR = os.path.expanduser("~/.config/tmux-workspaces")
# Settings live in their own subdirectory, not alongside the flat
# <alias>.yml workspace profiles, so a settings filename can never
# collide with a workspace alias.
SETTINGS_PATH = os.path.join(CONFIG_DIR, ".settings", "settings.yml")
# Pre-YAML installs wrote settings.json / <alias>.json. Both are still read
# transparently (valid JSON parses fine as YAML); every write goes to the
# new .yml path, so profiles migrate the first time they're saved.
LEGACY_SETTINGS_PATH = os.path.join(CONFIG_DIR, ".settings", "settings.json")

DEFAULT_SETTINGS = {
    "base_dir": "~",
    "window_base_index": 1,
    "pane_base_index": 1,
    "editor": "nano",
    "confirm_down": True,
    "use_emoji": True,
}

EMOJI = {
    "info": "ℹ️  ",
    "list": "📋 ",
    "active": "🟢 ",
    "inactive": "⚪ ",
    "error": "❌ ",
    "edit": "📝 ",
    "warn": "⚠️  ",
    "rocket": "🚀 ",
    "package": "📦 ",
    "trash": "🗑️  ",
    "plus": "➕ ",
    "broom": "🧹 ",
    "check": "✅ ",
    "wave": "🔄 ",
    "stop": "🛑 ",
    "sparkle": "✨ ",
    "tool": "🛠️  ",
    "label": "🏷️  ",
    "folder": "📂 ",
    "arrow": "↳ ",
    "dry": "🧪 ",
}

SETTINGS = {}
DRY_RUN = False


def err(msg):
    print(msg, file=sys.stderr)


def sym(key):
    if not SETTINGS.get("use_emoji", True):
        return ""
    return EMOJI.get(key, "")


def load_settings():
    settings = dict(DEFAULT_SETTINGS)
    path = SETTINGS_PATH if os.path.exists(SETTINGS_PATH) else LEGACY_SETTINGS_PATH
    if os.path.exists(path):
        try:
            with open(path) as f:
                settings.update(yaml.safe_load(f) or {})
        except (yaml.YAMLError, OSError) as exc:
            err(f"{sym('warn')}Failed to read {path} ({exc}); using defaults.")
    return settings


def resolve_path(path):
    return os.path.abspath(os.path.expanduser(path))


def resolve_project_path(raw_path, base_dir):
    expanded = os.path.expanduser(raw_path)
    if os.path.isabs(expanded):
        return resolve_path(expanded)
    return resolve_path(os.path.join(os.path.expanduser(base_dir), expanded))


def resolve_window_dir(project_dir, win):
    sub_path = win.get("path")
    if not sub_path:
        return project_dir
    expanded = os.path.expanduser(sub_path)
    if os.path.isabs(expanded):
        return resolve_path(expanded)
    return resolve_path(os.path.join(project_dir, expanded))


def open_in_editor(path):
    editor_cmd = os.environ.get("EDITOR") or SETTINGS.get("editor", "nano")
    try:
        parts = shlex.split(editor_cmd)
    except ValueError:
        parts = [editor_cmd]

    print(f"{sym('edit')}Opening {path} with {editor_cmd}...")
    try:
        if not parts:
            raise FileNotFoundError
        result = subprocess.run(parts + [path])
    except (FileNotFoundError, PermissionError):
        err(f"{sym('warn')}Editor '{editor_cmd}' not found. Falling back to nano.")
        if not shutil.which("nano"):
            err(f"{sym('error')}nano is not available either. Set $EDITOR or 'editor' in {SETTINGS_PATH}.")
            sys.exit(1)
        result = subprocess.run(["nano", path])

    if result.returncode != 0:
        err(f"{sym('warn')}Editor exited with code {result.returncode}.")


# --- tmux helpers -----------------------------------------------------------
# All mutating calls go through argv lists (never shell=True) so paths,
# aliases, and pane commands can't be misparsed or break out of quoting.

def tmux_query(args):
    return subprocess.run(args, capture_output=True, text=True)


def tmux_run(args, capture=False):
    if DRY_RUN:
        print(f"   {sym('dry')}[dry-run] tmux {' '.join(shlex.quote(a) for a in args[1:])}")
        return "" if capture else None
    if capture:
        res = subprocess.run(args, capture_output=True, text=True)
        return res.stdout.strip()
    subprocess.run(args)
    return None


def has_session(name):
    return tmux_query(["tmux", "has-session", "-t", name]).returncode == 0


def list_pane_ids(session_name, win_name):
    res = tmux_query(["tmux", "list-panes", "-t", f"{session_name}:{win_name}", "-F", "#{pane_id}"])
    return [line.strip() for line in res.stdout.splitlines() if line.strip()]


def tmux_new_session(session, win_name, cwd):
    return tmux_run(
        ["tmux", "new-session", "-d", "-s", session, "-n", win_name, "-c", cwd, "-P", "-F", "#{pane_id}"],
        capture=True,
    )


def tmux_new_window(session, prev_win_name, win_name, cwd):
    return tmux_run(
        ["tmux", "new-window", "-a", "-t", f"{session}:{prev_win_name}", "-n", win_name, "-c", cwd,
         "-P", "-F", "#{pane_id}"],
        capture=True,
    )


def tmux_split_window(pane_id, cwd):
    return tmux_run(
        ["tmux", "split-window", "-h", "-t", pane_id, "-c", cwd, "-P", "-F", "#{pane_id}"],
        capture=True,
    )


def tmux_send_keys(pane_id, command):
    tmux_run(["tmux", "send-keys", "-t", pane_id, command, "C-m"])


def tmux_send_ctrl_c(pane_id):
    tmux_run(["tmux", "send-keys", "-t", pane_id, "C-c"])


def tmux_set_option(session, name, value, window_option=False):
    args = ["tmux", "set-option"]
    if window_option:
        args.append("-w")
    args += ["-t", session, name, str(value)]
    tmux_run(args)


def tmux_renumber_windows(session):
    tmux_run(["tmux", "move-window", "-r", "-t", session])


def tmux_kill_session(session):
    tmux_run(["tmux", "kill-session", "-t", session])


def tmux_attach(session):
    if DRY_RUN:
        print(f"   {sym('dry')}[dry-run] would attach to session '{session}'")
        return
    subprocess.run(["tmux", "attach-session", "-t", session])


# --- config -----------------------------------------------------------------

def config_paths(alias):
    """Return (yml_path, legacy_json_path) for an alias."""
    return (
        os.path.join(CONFIG_DIR, f"{alias}.yml"),
        os.path.join(CONFIG_DIR, f"{alias}.json"),
    )


def find_config_path(alias):
    yml_path, json_path = config_paths(alias)
    if os.path.exists(yml_path):
        return yml_path
    if os.path.exists(json_path):
        return json_path
    return None


def load_config(alias):
    config_path = find_config_path(alias)
    if not config_path:
        err(f"{sym('error')}Workspace alias '{alias}' does not exist.")
        sys.exit(1)
    try:
        with open(config_path) as f:
            return yaml.safe_load(f) or {}
    except yaml.YAMLError as exc:
        err(f"{sym('error')}Failed to parse {config_path}: {exc}")
        sys.exit(1)


def save_config(alias, config):
    yml_path, _ = config_paths(alias)
    os.makedirs(CONFIG_DIR, exist_ok=True)
    with open(yml_path, "w") as f:
        yaml.safe_dump(config, f, sort_keys=False, default_flow_style=False)
    return yml_path


def validate_config(config):
    errors = []
    if not isinstance(config.get("project_path"), str) or not config.get("project_path"):
        errors.append("project_path must be a non-empty string")

    windows = config.get("windows")
    if not isinstance(windows, list) or not windows:
        errors.append("windows must be a non-empty list")
    else:
        for i, win in enumerate(windows):
            if not isinstance(win, dict) or not win.get("name"):
                errors.append(f"window #{i + 1} is missing a 'name'")
                continue
            panes = win.get("panes", [])
            if not isinstance(panes, list) or not panes:
                errors.append(f"window '{win.get('name')}' must have at least one pane")
            elif not all(isinstance(p, dict) for p in panes):
                errors.append(f"window '{win.get('name')}' has a pane that is not an object")

    teardown = config.get("teardown", [])
    if isinstance(teardown, str):
        teardown = [teardown]
    if not isinstance(teardown, list) or not all(isinstance(c, str) for c in teardown):
        errors.append("teardown must be a string or a list of strings")

    return errors


def validate_workspace_cmd(alias):
    config = load_config(alias)
    errors = validate_config(config)
    if errors:
        err(f"{sym('error')}'{alias}' has {len(errors)} problem(s):")
        for e in errors:
            err(f"   - {e}")
        sys.exit(1)
    print(f"{sym('check')}'{alias}' looks valid ({len(config.get('windows', []))} window(s)).")


def list_workspaces():
    if not os.path.exists(CONFIG_DIR) or not os.listdir(CONFIG_DIR):
        print(f"{sym('info')}No workspaces found. Create .yml profiles inside: {CONFIG_DIR}")
        return

    aliases = {}
    for filename in sorted(os.listdir(CONFIG_DIR)):
        if filename.endswith(".yml"):
            aliases[filename[:-4]] = filename
        elif filename.endswith(".json"):
            aliases.setdefault(filename[:-5], filename)  # .yml takes priority if both exist

    entries = []
    for alias, filename in sorted(aliases.items()):
        try:
            with open(os.path.join(CONFIG_DIR, filename)) as f:
                cfg = yaml.safe_load(f) or {}
            display_name = cfg.get("project_name_display", "Unnamed Project")
            entries.append((alias, display_name, has_session(alias), None))
        except (yaml.YAMLError, OSError) as exc:
            entries.append((alias, None, None, str(exc)))

    if not entries:
        print(f"{sym('info')}No workspaces found. Create .yml profiles inside: {CONFIG_DIR}")
        return

    alias_w = max([6] + [len(a) for a, *_ in entries])
    name_w = max([12] + [len(n) for _, n, _, e in entries if n])

    print(f"{sym('list')}Available Project Workspaces:")
    print("-" * (alias_w + name_w + 20))
    for alias, name, active, error in entries:
        if error:
            print(f"   {sym('error')}{alias.ljust(alias_w)} | Error parsing json config: {error}")
            continue
        status = f"{sym('active')}ACTIVE" if active else f"{sym('inactive')}inactive"
        print(f"   {alias.ljust(alias_w)} | {name.ljust(name_w)} | {status}")
    print("-" * (alias_w + name_w + 20))


def edit_workspace_raw(alias):
    config_path = find_config_path(alias)
    if not config_path:
        err(f"{sym('error')}Workspace alias '{alias}' does not exist.")
        sys.exit(1)
    open_in_editor(config_path)


def edit_settings():
    os.makedirs(os.path.dirname(SETTINGS_PATH), exist_ok=True)
    if not os.path.exists(SETTINGS_PATH) and not os.path.exists(LEGACY_SETTINGS_PATH):
        with open(SETTINGS_PATH, "w") as f:
            yaml.safe_dump(DEFAULT_SETTINGS, f, sort_keys=False, default_flow_style=False)
        print(f"{sym('sparkle')}Created default settings file at {SETTINGS_PATH}")
    path = SETTINGS_PATH if os.path.exists(SETTINGS_PATH) else LEGACY_SETTINGS_PATH
    open_in_editor(path)


def edit_workspace_interactive(alias):
    config = load_config(alias)

    print(f"\n{sym('wave')}Interactive Editor for Profile: '{alias}' {sym('wave')}")
    print("-" * 50)

    current_title = config.get("project_name_display", alias.upper())
    new_title = input(f"{sym('label')}Project Title [{current_title}]: ").strip()
    if new_title:
        config["project_name_display"] = new_title

    current_path = config.get("project_path", "")
    while True:
        new_path = input(f"{sym('folder')}Project Path, relative to {SETTINGS['base_dir']} [{current_path}]: ").strip()
        if not new_path:
            break
        resolved = resolve_project_path(new_path, SETTINGS["base_dir"])
        if not os.path.exists(resolved):
            if input(f"{sym('warn')}'{resolved}' does not exist. Use it anyway? (y/N): ").strip().lower() == 'y':
                config["project_path"] = new_path
                break
            continue
        config["project_path"] = new_path
        break

    print(f"\n{sym('package')}Reviewing Window and Pane Configurations...")
    updated_windows = []

    for idx, win in enumerate(config.get("windows", [])):
        print(f"\n--- Window #{idx + 1} ---")
        win_name = win.get("name", f"win-{idx + 1}")
        new_win_name = input(f"  Name [{win_name}] (Type 'DELETE' to remove window): ").strip()

        if new_win_name.upper() == "DELETE":
            print(f"  {sym('trash')}Removing window '{win_name}'")
            continue

        final_win_name = new_win_name if new_win_name else win_name

        current_win_path = win.get("path", "")
        new_win_path = input(f"  Working dir override [{current_win_path or '(project path)'}]: ").strip()
        final_win_path = new_win_path if new_win_path else current_win_path

        current_on_stop = win.get("on_stop", "")
        new_on_stop = input(f"  Graceful exit command on 'down' [{current_on_stop or '(Ctrl-C)'}]: ").strip()
        final_on_stop = new_on_stop if new_on_stop else current_on_stop

        updated_panes = []
        for p_idx, pane in enumerate(win.get("panes", [])):
            current_cmd = pane.get("command", "")
            new_cmd = input(f"    {sym('arrow')}Pane {p_idx + 1} Command [{current_cmd or '(shell)'}]: ").strip()
            final_cmd = new_cmd if new_cmd else current_cmd
            updated_panes.append({"command": final_cmd})

        while True:
            add_more = input(f"    {sym('plus')}Add an additional pane to window '{final_win_name}'? (y/N): ").strip().lower()
            if add_more != 'y':
                break
            extra_cmd = input(f"      {sym('arrow')}Pane {len(updated_panes) + 1} Command (blank for shell): ").strip()
            updated_panes.append({"command": extra_cmd})

        win_entry = {"name": final_win_name, "panes": updated_panes}
        if final_win_path:
            win_entry["path"] = final_win_path
        if final_on_stop:
            win_entry["on_stop"] = final_on_stop
        updated_windows.append(win_entry)

    while True:
        add_win = input(f"\n{sym('plus')}Add an entirely new window to this profile? (y/N): ").strip().lower()
        if add_win != 'y':
            break
        new_name = input(f"  Window #{len(updated_windows) + 1} Name: ").strip()
        if not new_name:
            continue

        new_win_path = input("  Working dir override, relative to project path (blank = project path): ").strip()

        new_panes = []
        pane_counter = 1
        while True:
            new_cmd = input(f"    {sym('arrow')}Pane {pane_counter} Command (blank for shell): ").strip()
            new_panes.append({"command": new_cmd})
            if input(f"    {sym('plus')}Add another side-by-side pane split? (y/N): ").strip().lower() != 'y':
                break
            pane_counter += 1

        new_on_stop = input("  Graceful exit command on 'down' (blank = Ctrl-C): ").strip()

        win_entry = {"name": new_name, "panes": new_panes}
        if new_win_path:
            win_entry["path"] = new_win_path
        if new_on_stop:
            win_entry["on_stop"] = new_on_stop
        updated_windows.append(win_entry)

    config["windows"] = updated_windows

    existing_teardown = config.get("teardown", [])
    if isinstance(existing_teardown, str):
        existing_teardown = [existing_teardown]
    print(f"\n{sym('broom')}Teardown commands (blocking, run before the session is killed):")
    if existing_teardown:
        for t_idx, cmd in enumerate(existing_teardown):
            print(f"   {t_idx + 1}. {cmd}")
    else:
        print("   (none set)")

    updated_teardown = existing_teardown
    if input("   Replace teardown commands? (y/N): ").strip().lower() == 'y':
        updated_teardown = []
        while True:
            cmd = input(f"   {sym('arrow')}Teardown command #{len(updated_teardown) + 1} (blank to finish): ").strip()
            if not cmd:
                break
            updated_teardown.append(cmd)

    config["teardown"] = updated_teardown

    errors = validate_config(config)
    if errors:
        err(f"{sym('error')}Not saving — config is invalid:")
        for e in errors:
            err(f"   - {e}")
        sys.exit(1)

    saved_path = save_config(alias, config)
    print(f"\n{sym('check')}Profile '{alias}' updated successfully! ({saved_path})")


def _validate_alias(alias):
    """Return an error string if alias is unusable as a workspace filename, else None."""
    if not alias or " " in alias:
        return "cannot be empty or contain spaces"
    if alias == "settings":
        return "'settings' is reserved"
    candidate = os.path.abspath(os.path.join(CONFIG_DIR, f"{alias}.yml"))
    if os.path.dirname(candidate) != os.path.abspath(CONFIG_DIR):
        return "cannot contain path separators or '..'"
    return None


def create_workspace_wizard(preset_alias=None):
    print(f"{sym('sparkle')}Interactive TMUX Workspace Creator {sym('sparkle')}")
    print("-" * 40)

    alias = (preset_alias or "").strip().lower()
    while True:
        alias_error = _validate_alias(alias)
        if alias_error is None:
            break
        if alias:
            print(f"{sym('error')}Invalid alias ({alias_error}).")
        alias = input(f"{sym('label')}Enter short workspace alias (e.g., am): ").strip().lower()

    config_path = find_config_path(alias)
    if config_path:
        overwrite = input(f"{sym('warn')}Alias '{alias}' already exists. Overwrite? (y/N): ").strip().lower()
        if overwrite != 'y':
            print(f"{sym('error')}Operation cancelled.")
            sys.exit(0)

    display_name = input(f"{sym('label')}Enter friendly project title: ").strip() or alias.upper()

    while True:
        raw_path = input(f"{sym('folder')}Project directory (relative to {SETTINGS['base_dir']}, or absolute/~) [.]: ").strip() or "."
        resolved = resolve_project_path(raw_path, SETTINGS["base_dir"])
        if not os.path.exists(resolved):
            if input(f"{sym('warn')}'{resolved}' does not exist. Create it now? (y/N): ").strip().lower() == 'y':
                os.makedirs(resolved, exist_ok=True)
                break
            continue
        break

    windows = []
    print(f"\n{sym('tool')}Let's configure your windows and panes...")

    while True:
        win_name = input(f"\n{sym('package')}Window #{len(windows) + 1} Name (or hit Enter to finish): ").strip()
        if not win_name:
            if not windows:
                print(f"{sym('error')}You must create at least one window layout.")
                continue
            break

        win_path = input(f"   {sym('folder')}Working dir override, relative to project path (blank = project path): ").strip()

        panes = []
        pane_count = 1
        while True:
            cmd = input(f"   {sym('arrow')}Pane {pane_count} command (blank for shell): ").strip()
            panes.append({"command": cmd})
            if input(f"   {sym('plus')}Add an additional side-by-side pane split? (y/N): ").strip().lower() != 'y':
                break
            pane_count += 1

        on_stop = input(f"   {sym('stop')}Graceful exit command on 'down' instead of Ctrl-C (blank = none, e.g. /exit): ").strip()

        win_entry = {"name": win_name, "panes": panes}
        if win_path:
            win_entry["path"] = win_path
        if on_stop:
            win_entry["on_stop"] = on_stop
        windows.append(win_entry)

    teardown = []
    print(f"\n{sym('broom')}Teardown commands (run and waited on before the session is killed).")
    while True:
        cmd = input(f"   {sym('arrow')}Teardown command #{len(teardown) + 1} (blank to finish): ").strip()
        if not cmd:
            break
        teardown.append(cmd)

    workspace_data = {
        "project_name_display": display_name,
        "project_path": raw_path,
        "windows": windows,
        "teardown": teardown,
    }

    saved_path = save_config(alias, workspace_data)
    print(f"\n{sym('check')}Configuration saved to: {saved_path}")


def start_workspace(alias, dry_run=False):
    global DRY_RUN
    DRY_RUN = dry_run

    config = load_config(alias)
    errors = validate_config(config)
    if errors:
        err(f"{sym('error')}Cannot start '{alias}' — invalid config:")
        for e in errors:
            err(f"   - {e}")
        sys.exit(1)

    project_dir = resolve_project_path(config["project_path"], SETTINGS["base_dir"])
    session_name = alias

    if has_session(session_name):
        print(f"{sym('wave')}Workspace '{session_name}' is already running. Attaching...")
        tmux_attach(session_name)
        return

    display_title = config.get("project_name_display", session_name)
    print(f"{sym('rocket')}Building '{display_title}' workspace...")

    windows_list = config["windows"]
    prev_win_name = None

    for win_idx, win in enumerate(windows_list):
        win_name = win["name"]
        win_dir = resolve_window_dir(project_dir, win)
        print(f"   {sym('package')}Window {win_idx + 1}/{len(windows_list)}: {win_name}")

        if win_idx == 0:
            current_pane_id = tmux_new_session(session_name, win_name, win_dir)
            tmux_set_option(session_name, "base-index", SETTINGS["window_base_index"])
            tmux_renumber_windows(session_name)
        else:
            current_pane_id = tmux_new_window(session_name, prev_win_name, win_name, win_dir)

        # pane-base-index is a per-window option: "-w -t session" only ever
        # touches the session's *current* window, so it has to be set again,
        # explicitly per window, right after each one is created.
        tmux_set_option(f"{session_name}:{win_name}", "pane-base-index", SETTINGS["pane_base_index"], window_option=True)

        panes = win.get("panes", [])
        first_cmd = panes[0].get("command", "") if panes else ""
        if first_cmd:
            tmux_send_keys(current_pane_id, first_cmd)

        for pane in panes[1:]:
            current_pane_id = tmux_split_window(current_pane_id, win_dir)
            cmd = pane.get("command", "")
            if cmd:
                tmux_send_keys(current_pane_id, cmd)

        prev_win_name = win_name

    tmux_attach(session_name)


def run_teardown(config, project_dir, dry_run=False):
    commands = config.get("teardown", [])
    if isinstance(commands, str):
        commands = [commands]
    if not commands:
        return

    print(f"{sym('broom')}Running teardown commands...")
    for cmd in commands:
        if not cmd:
            continue
        if dry_run:
            print(f"   {sym('dry')}[dry-run] would run: {cmd}")
            continue
        print(f" -> {cmd}")
        result = subprocess.run(cmd, shell=True, cwd=project_dir)
        if result.returncode != 0:
            print(f"    {sym('warn')}Exited with code {result.returncode}")
    print(" -> Teardown complete.")


def stop_workspace(alias, dry_run=False, assume_yes=False):
    global DRY_RUN
    DRY_RUN = dry_run

    config = load_config(alias)
    errors = validate_config(config)
    if errors:
        err(f"{sym('error')}Cannot stop '{alias}' — invalid config:")
        for e in errors:
            err(f"   - {e}")
        sys.exit(1)

    project_dir = resolve_project_path(config["project_path"], SETTINGS["base_dir"])
    session_name = alias

    if not has_session(session_name):
        print(f"{sym('info')}No active session found for alias '{session_name}'")
        return

    if not dry_run and not assume_yes and SETTINGS.get("confirm_down", True):
        display_title = config.get("project_name_display", session_name)
        answer = input(
            f"{sym('warn')}Tear down '{display_title}' ({session_name})? "
            f"This runs teardown commands and kills the session. (y/N): "
        ).strip().lower()
        if answer != 'y':
            print("Cancelled.")
            return

    print(f"{sym('stop')}Safely bringing down workspace: {session_name}...")

    self_pane = os.environ.get("TMUX_PANE")

    for win in config.get("windows", []):
        win_name = win["name"]
        exit_cmd = win.get("on_stop")
        pane_ids = list_pane_ids(session_name, win_name)

        if exit_cmd and pane_ids:
            tmux_send_keys(pane_ids[0], exit_cmd)
            continue

        for pane_id in pane_ids:
            if pane_id == self_pane:
                continue  # never interrupt the pane we are running in
            tmux_send_ctrl_c(pane_id)

    if not dry_run:
        time.sleep(0.5)  # give Ctrl-C / exit commands a moment to land before teardown runs

    run_teardown(config, project_dir, dry_run=dry_run)

    print(f"   {sym('broom')}Cleaning environment allocations...")
    tmux_kill_session(session_name)
    print(f"{sym('check')}Completed clean exit!")


def upgrade_configs(dry_run=False):
    """Convert legacy JSON profiles/settings under CONFIG_DIR to YAML in place."""
    if not os.path.isdir(CONFIG_DIR):
        print(f"{sym('info')}No config directory found at {CONFIG_DIR}; nothing to upgrade.")
        return

    converted = 0
    skipped = 0

    # Settings: the canonical legacy path (.settings/settings.json), plus a
    # flat CONFIG_DIR/settings.json some older installs mistakenly wrote to.
    for legacy_settings in (LEGACY_SETTINGS_PATH, os.path.join(CONFIG_DIR, "settings.json")):
        if not os.path.exists(legacy_settings):
            continue
        if os.path.exists(SETTINGS_PATH):
            print(f"{sym('warn')}{SETTINGS_PATH} already exists; leaving {legacy_settings} untouched.")
            skipped += 1
            break
        try:
            with open(legacy_settings) as f:
                data = yaml.safe_load(f) or {}
        except yaml.YAMLError as exc:
            err(f"{sym('error')}Failed to parse {legacy_settings}: {exc}")
            break
        if dry_run:
            print(f"   {sym('dry')}[dry-run] would convert {legacy_settings} -> {SETTINGS_PATH}")
        else:
            os.makedirs(os.path.dirname(SETTINGS_PATH), exist_ok=True)
            with open(SETTINGS_PATH, "w") as f:
                yaml.safe_dump(data, f, sort_keys=False, default_flow_style=False)
            backup = legacy_settings + ".bak"
            os.rename(legacy_settings, backup)
            print(f"{sym('check')}Converted settings: {legacy_settings} -> {SETTINGS_PATH} (backup: {backup})")
        converted += 1
        break

    # Workspace profiles: every top-level <alias>.json, skipping a flat
    # settings.json (handled above, never a real workspace alias).
    for filename in sorted(os.listdir(CONFIG_DIR)):
        if filename == "settings.json" or not filename.endswith(".json"):
            continue
        if not os.path.isfile(os.path.join(CONFIG_DIR, filename)):
            continue
        alias = filename[:-5]
        legacy_path = os.path.join(CONFIG_DIR, filename)
        yml_path, _ = config_paths(alias)
        if os.path.exists(yml_path):
            print(f"{sym('warn')}{yml_path} already exists; leaving {legacy_path} untouched.")
            skipped += 1
            continue
        try:
            with open(legacy_path) as f:
                data = yaml.safe_load(f) or {}
        except yaml.YAMLError as exc:
            err(f"{sym('error')}Failed to parse {legacy_path}: {exc}")
            continue
        if dry_run:
            print(f"   {sym('dry')}[dry-run] would convert {legacy_path} -> {yml_path}")
        else:
            save_config(alias, data)
            backup = legacy_path + ".bak"
            os.rename(legacy_path, backup)
            print(f"{sym('check')}Converted '{alias}': {legacy_path} -> {yml_path} (backup: {backup})")
        converted += 1

    if converted == 0 and skipped == 0:
        print(f"{sym('info')}No legacy JSON configs found under {CONFIG_DIR}; nothing to upgrade.")
    elif dry_run:
        print(f"\n{sym('info')}Dry run: {converted} would be converted, {skipped} would be skipped.")
    else:
        print(f"\n{sym('sparkle')}Upgrade complete: {converted} converted, {skipped} skipped.")


EPILOG = """\
examples:
  manage-workspace.py list
  manage-workspace.py create myapp
  manage-workspace.py up myapp
  manage-workspace.py up myapp --dry-run
  manage-workspace.py down myapp --yes
  manage-workspace.py edit myapp
  manage-workspace.py edit myapp --raw
  manage-workspace.py validate myapp
  manage-workspace.py config
  manage-workspace.py upgrade
  manage-workspace.py upgrade --dry-run
"""

if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Centralized YAML-Driven TMUX Workspace Manager",
        epilog=EPILOG,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "action",
        choices=["up", "down", "list", "create", "edit", "validate", "config", "upgrade"],
        help="Workspace lifecycle command",
    )
    parser.add_argument("project", nargs="?", default=None, help="The short alias profile filename string")
    parser.add_argument("--raw", action="store_true", help="Open raw YAML instead of the wizard menu (edit)")
    parser.add_argument("--dry-run", action="store_true", help="Print the tmux/teardown commands without executing them (up/down/upgrade)")
    parser.add_argument("-y", "--yes", action="store_true", help="Skip the confirmation prompt (down)")
    parser.add_argument("--no-emoji", action="store_true", help="Disable emoji in output")
    args = parser.parse_args()

    if args.no_emoji:
        SETTINGS["use_emoji"] = False  # applied before load_settings() so its own warnings honor the flag too
    SETTINGS = load_settings()
    if args.no_emoji:
        SETTINGS["use_emoji"] = False

    if args.action == "create":
        create_workspace_wizard(args.project)
    elif args.action == "config":
        edit_settings()
    elif args.action == "upgrade":
        upgrade_configs(dry_run=args.dry_run)
    elif args.action == "list" or (args.action in ["up", "down", "edit", "validate"] and not args.project):
        list_workspaces()
    elif args.action == "edit":
        if args.raw:
            edit_workspace_raw(args.project)
        else:
            edit_workspace_interactive(args.project)
    elif args.action == "up":
        start_workspace(args.project, dry_run=args.dry_run)
    elif args.action == "down":
        stop_workspace(args.project, dry_run=args.dry_run, assume_yes=args.yes)
    elif args.action == "validate":
        validate_workspace_cmd(args.project)
