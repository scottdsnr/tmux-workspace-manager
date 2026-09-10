package main

import (
	"fmt"
	"os"
	"path/filepath"
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

func resolveWindowDir(projectDir string, win Window) string {
	if win.Path == "" {
		return projectDir
	}
	expanded := expandUser(win.Path)
	if filepath.IsAbs(expanded) {
		return resolvePath(expanded)
	}
	return resolvePath(filepath.Join(projectDir, expanded))
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
