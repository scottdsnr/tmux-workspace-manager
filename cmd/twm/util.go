package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func errPrint(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func fatal(format string, args ...interface{}) {
	errPrint(format, args...)
	os.Exit(1)
}

// firstExisting returns the first path that exists on disk, or "" if none do.
func firstExisting(paths ...string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// expandUser mimics Python's os.path.expanduser for the common "~" and
// "~/..." forms used throughout this tool's config values.
func expandUser(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func resolvePath(path string) string {
	abs, err := filepath.Abs(expandUser(path))
	if err != nil {
		return expandUser(path)
	}
	return abs
}

func resolveProjectPath(rawPath, baseDir string) string {
	expanded := expandUser(rawPath)
	if filepath.IsAbs(expanded) {
		return resolvePath(expanded)
	}
	return resolvePath(filepath.Join(expandUser(baseDir), expanded))
}

// resolveRelativeDir resolves path against base: an empty path yields base
// as-is, an absolute or "~"-path is used on its own (after expansion),
// otherwise it's joined onto base.
func resolveRelativeDir(base, path string) string {
	if path == "" {
		return base
	}
	expanded := expandUser(path)
	if filepath.IsAbs(expanded) {
		return resolvePath(expanded)
	}
	return resolvePath(filepath.Join(base, expanded))
}

func resolveWindowDir(projectDir string, win Window) string {
	return resolveRelativeDir(projectDir, win.Path)
}

// resolvePaneDir resolves a pane's optional path override against its
// window's directory, the same way resolveWindowDir resolves a window's
// path against the project directory.
func resolvePaneDir(windowDir string, pane Pane) string {
	return resolveRelativeDir(windowDir, pane.Path)
}

// resolvePaneEnv merges the env maps that apply to a pane, from widest to
// narrowest scope: workspace-level env, then the window's, then the pane's
// own. A narrower scope overrides the same key from a wider one; keys it
// doesn't mention are inherited. Returns nil when nothing is set anywhere.
func resolvePaneEnv(configEnv, windowEnv, paneEnv map[string]string) map[string]string {
	if len(configEnv) == 0 && len(windowEnv) == 0 && len(paneEnv) == 0 {
		return nil
	}
	merged := make(map[string]string, len(configEnv)+len(windowEnv)+len(paneEnv))
	for _, src := range []map[string]string{configEnv, windowEnv, paneEnv} {
		for k, v := range src {
			merged[k] = v
		}
	}
	return merged
}

// parseEnvAssignments turns a single line of "KEY=value KEY2='two words'"
// assignments — the shape both wizards ask for env in — into a map. It
// returns an error string (empty when fine) describing the first malformed
// token, so callers can show it inline rather than silently dropping input.
func parseEnvAssignments(line string) (map[string]string, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, ""
	}
	tokens, err := shlexSplit(line)
	if err != nil {
		return nil, err.Error()
	}
	env := map[string]string{}
	for _, tok := range tokens {
		key, val, found := strings.Cut(tok, "=")
		if !found || key == "" {
			return nil, fmt.Sprintf("'%s' is not a KEY=value assignment", tok)
		}
		env[key] = val
	}
	if len(env) == 0 {
		return nil, ""
	}
	return env, ""
}

// formatEnvAssignments renders an env map back into the single-line form
// parseEnvAssignments accepts, sorted so editing a profile round-trips
// deterministically.
func formatEnvAssignments(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+shellQuote(env[k]))
	}
	return strings.Join(parts, " ")
}

// stringMapFromRaw converts an "env" value freshly decoded from YAML into a
// typed map, skipping non-string values (validateConfig reports those).
func stringMapFromRaw(v interface{}) map[string]string {
	m, ok := v.(map[string]interface{})
	if !ok || len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// shlexSplit is a small approximation of Python's shlex.split, sufficient
// for typical $EDITOR values like `code --wait` or `vim`.
func shlexSplit(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inSingle, inDouble, hasCur := false, false, false

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(c)
			}
		case inDouble:
			if c == '"' {
				inDouble = false
			} else if c == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
				i++
				cur.WriteByte(s[i])
			} else {
				cur.WriteByte(c)
			}
		case c == '\'':
			inSingle = true
			hasCur = true
		case c == '"':
			inDouble = true
			hasCur = true
		case c == ' ' || c == '\t':
			if hasCur || cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
				hasCur = false
			}
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			hasCur = true
		default:
			cur.WriteByte(c)
			hasCur = true
		}
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unbalanced quotes in: %s", s)
	}
	if hasCur || cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args, nil
}

// shellQuote/shellJoin mimic shlex.quote for dry-run display purposes only.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("@%_-+=:,./", r)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}
