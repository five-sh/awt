package naming

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const maxSessionLen = 60

var (
	nonSlugChars = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)
	dashRun      = regexp.MustCompile(`-+`)
	dotRun       = regexp.MustCompile(`\.{2,}`)
	edgeTrim     = regexp.MustCompile(`^[.-]+|[.-]+$`)
)

// Slugify turns free-form input into a name safe for both a directory and a git branch.
func Slugify(input string) (string, error) {
	s := nonSlugChars.ReplaceAllString(strings.TrimSpace(input), "-")
	s = dashRun.ReplaceAllString(s, "-")
	s = dotRun.ReplaceAllString(s, ".")
	s = edgeTrim.ReplaceAllString(s, "")
	if s == "" || s == "." || s == ".." {
		return "", fmt.Errorf("invalid name %q", input)
	}
	return s, nil
}

// SlugifyBranch is Slugify for a new branch name, preserving "/" as intentional
// namespacing (e.g. "codex/fix login!!" -> "codex/fix-login") by slugifying each
// segment on its own; a leading, trailing, or doubled "/" yields an empty segment
// and is rejected.
func SlugifyBranch(input string) (string, error) {
	parts := strings.Split(input, "/")
	for i, p := range parts {
		s, err := Slugify(p)
		if err != nil {
			return "", fmt.Errorf("invalid name %q", input)
		}
		parts[i] = s
	}
	return strings.Join(parts, "/"), nil
}

// WindowName is what a worktree's window is called, and so what the status bar
// shows: "<repo>:<worktree>", one entry per repo you have open. Any ":" inside
// either part is flattened so the separator stays the only one.
func WindowName(repoName, wtSlug string) string {
	clean := strings.NewReplacer(":", "-")
	return clean.Replace(repoName) + ":" + clean.Replace(wtSlug)
}

// SessionName builds the per-worktree session name v0.1 used. v0.2 keeps worktrees
// in windows instead, so this is only here to recognise sessions left over from
// v0.1 — see `awt migrate`.
func SessionName(repoSlug, wtSlug string) string {
	clean := strings.NewReplacer(":", "-", ".", "-")
	name := clean.Replace(repoSlug) + "--" + clean.Replace(wtSlug)
	if len(name) <= maxSessionLen {
		return name
	}
	return shortenWithHash(name)
}

func shortenWithHash(name string) string {
	sum := sha1.Sum([]byte(name))
	suffix := hex.EncodeToString(sum[:])[:6]
	max := maxSessionLen - len(suffix) - 1
	if max > len(name) {
		max = len(name)
	}
	if max < 0 {
		max = 0
	}
	return name[:max] + "-" + suffix
}
