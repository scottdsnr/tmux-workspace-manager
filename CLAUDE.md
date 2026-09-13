# Testing rules

- Never run or test this tool against the user's real config directory
  (`~/.config/tmux-workspaces`, including `workspaces.yml`, per-alias
  profiles, or `.settings/settings.yml`). That is live user data, not a
  fixture.
- All manual/manual-CLI testing (building the binary and invoking commands
  like `create`, `upgrade`, `up`, `down`, `edit`, etc.) must point at a
  temporary directory instead — e.g. override `$HOME` for the test invocation
  (`HOME=$(mktemp -d) ./twm ...`) so `configDir`/`workspacesPath` resolve
  under it, or otherwise redirect config resolution to a scratch directory.
- `go test ./...` is fine as-is (it doesn't touch the real config dir), but
  do not add tests or ad-hoc scripts that read from or write to
  `~/.config/tmux-workspaces` directly.
