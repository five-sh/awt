package naming

import (
	"fmt"
	"regexp"
	"strings"
)

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
