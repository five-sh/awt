package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Worktree struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
	Bare     bool
	Locked   bool
	Prunable bool
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// RepoRoot returns the repo's root: the main worktree for a normal clone, or the
// bare repo's own directory when it's a bare hub with no working tree of its own.
func RepoRoot(cwd string) (string, error) {
	out, err := run(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	gitDir := strings.TrimSpace(out)
	if filepath.Base(gitDir) != ".git" {
		return gitDir, nil // bare repo: common-dir IS the repo root, not a .git subdir
	}
	return filepath.Dir(gitDir), nil
}

// Toplevel returns the root of whichever worktree cwd is currently inside.
func Toplevel(cwd string) (string, error) {
	out, err := run(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func CurrentBranch(path string) (string, error) {
	out, err := run(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func BranchExists(repoRoot, name string) bool {
	_, err := run(repoRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

func IsBare(repoRoot string) bool {
	out, err := run(repoRoot, "rev-parse", "--is-bare-repository")
	return err == nil && strings.TrimSpace(out) == "true"
}

var defaultBranchCandidates = []string{"main", "master"}

func DefaultBranch(repoRoot string) (string, error) {
	if out, err := run(repoRoot, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimPrefix(strings.TrimSpace(out), "refs/remotes/origin/"), nil
	}
	for _, name := range defaultBranchCandidates {
		if BranchExists(repoRoot, name) {
			return name, nil
		}
	}
	return "", fmt.Errorf("could not determine default branch")
}

func HasRemote(repoRoot, name string) bool {
	_, err := run(repoRoot, "remote", "get-url", name)
	return err == nil
}

// FreshBase resolves the repo's default branch and a base ref reflecting the latest
// origin state when possible. It fetches before returning; on a repo with no
// remote, or when the fetch fails, it falls back to whatever's available locally.
func FreshBase(repoRoot string) (branch, base, warn string, err error) {
	branch, err = DefaultBranch(repoRoot)
	if err != nil {
		return "", "", "", err
	}
	if !HasRemote(repoRoot, "origin") {
		return branch, branch, "", nil
	}
	if _, ferr := run(repoRoot, "fetch", "origin", branch); ferr != nil {
		return branch, "origin/" + branch, fmt.Sprintf("could not fetch origin/%s, using cached ref (%v)", branch, ferr), nil
	}
	return branch, "origin/" + branch, "", nil
}

// Clone sets up a bare hub clone at dest: no working tree of its own, with a proper
// origin fetch refspec and origin/HEAD, so every branch (including the default one)
// becomes a worktree only when first switched to.
func Clone(url, dest string) error {
	if _, err := run("", "clone", "--bare", url, dest); err != nil {
		return err
	}
	if _, err := run(dest, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return err
	}
	if _, err := run(dest, "fetch", "origin"); err != nil {
		return err
	}
	_, err := run(dest, "remote", "set-head", "origin", "--auto")
	return err
}

func AddWorktree(repoRoot, path, branch, base string) error {
	_, err := run(repoRoot, "worktree", "add", "-b", branch, path, base)
	return err
}

// AddWorktreeExisting checks out a branch that already exists locally into a new worktree.
func AddWorktreeExisting(repoRoot, path, branch string) error {
	_, err := run(repoRoot, "worktree", "add", path, branch)
	return err
}

// AddOrResetWorktree creates branch at base, resetting it to base if it already
// exists. Used for the default branch, which must always reflect the latest
// origin even when a stale local copy already exists (e.g. from a bare clone).
func AddOrResetWorktree(repoRoot, path, branch, base string) error {
	_, err := run(repoRoot, "worktree", "add", "-B", branch, path, base)
	return err
}

func RemoveWorktree(repoRoot, path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	_, err := run(repoRoot, args...)
	return err
}

// DeleteBranch deletes a local branch. force uses -D (deletes even if unmerged);
// otherwise -d, which refuses to delete a branch not fully merged.
func DeleteBranch(repoRoot, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := run(repoRoot, "branch", flag, branch)
	return err
}

func IsDirty(path string) (bool, error) {
	out, err := run(path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func ListWorktrees(repoRoot string) ([]Worktree, error) {
	out, err := run(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			list = append(list, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			cur = &Worktree{Path: val}
		case "HEAD":
			if cur != nil {
				cur.Head = val
			}
		case "branch":
			if cur != nil {
				cur.Branch = strings.TrimPrefix(val, "refs/heads/")
			}
		case "detached":
			if cur != nil {
				cur.Detached = true
			}
		case "bare":
			if cur != nil {
				cur.Bare = true
			}
		case "locked":
			if cur != nil {
				cur.Locked = true
			}
		case "prunable":
			if cur != nil {
				cur.Prunable = true
			}
		}
	}
	flush()
	return list, nil
}
