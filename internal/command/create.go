package command

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"awt/internal/git"
	"awt/internal/naming"
	"awt/internal/state"
)

// finishCreate lays out the worktree directory and records it in state. add does
// the actual git worktree add (new branch or existing). The tmux window is not
// built here: focus() does that the first time the worktree comes on screen, so
// there's one path that creates windows and one that knows where to put them.
func finishCreate(repo *state.Repo, name, branch, parent string, add func(path string) error) (state.Worktree, error) {
	root, err := workspaceRoot()
	if err != nil {
		return state.Worktree{}, err
	}
	path := filepath.Join(root, repo.Name, name)
	if err := add(path); err != nil {
		return state.Worktree{}, err
	}
	wt := state.Worktree{
		Repo: repo.Name, Name: name, Branch: branch, Path: path,
		Parent: parent, CreatedAt: time.Now(),
	}
	st, err := state.Load()
	if err != nil {
		return state.Worktree{}, err
	}
	st.Upsert(wt)
	if err := st.Save(); err != nil {
		return state.Worktree{}, err
	}
	return wt, nil
}

// createWorktree creates a new branch named branch (which may contain "/" for
// namespacing, e.g. "codex/foo"), based off base. name is the flattened
// directory/state name, distinct from branch so the worktree stays flat on disk.
func createWorktree(repo *state.Repo, name, branch, base string) (state.Worktree, error) {
	return finishCreate(repo, name, branch, base, func(path string) error {
		return git.AddWorktree(repo.Path, path, branch, base)
	})
}

// materializeExisting checks out a branch that already exists into a new worktree.
// The branch itself is used as-is (it may contain "/", as with "feature/x"
// namespacing); the worktree's directory/state name is slugified so a branch like
// that doesn't nest the worktree under subdirectories the way its ref does. Before
// checking out, it syncs the branch against origin (fast-forwarding and setting up
// tracking) so a branch left stale since it was last materialized doesn't get
// silently reused as-is.
func materializeExisting(repo *state.Repo, branch string) (state.Worktree, error) {
	name, err := naming.Slugify(branch)
	if err != nil {
		return state.Worktree{}, err
	}
	if warn := git.SyncExisting(repo.Path, branch); warn != "" {
		fmt.Fprintln(os.Stderr, "awt: "+warn)
	}
	return finishCreate(repo, name, branch, "", func(path string) error {
		return git.AddWorktreeExisting(repo.Path, path, branch)
	})
}

// materializeRemote checks out a branch that exists only on origin (ref, e.g.
// "refs/remotes/origin/<branch>"), creating a local tracking branch for it. See
// materializeExisting for why the worktree name is slugified separately from branch.
func materializeRemote(repo *state.Repo, branch, ref string) (state.Worktree, error) {
	name, err := naming.Slugify(branch)
	if err != nil {
		return state.Worktree{}, err
	}
	return finishCreate(repo, name, branch, "", func(path string) error {
		return git.AddWorktreeTracking(repo.Path, path, branch, ref)
	})
}

// materializeDefault creates (or resets, if a stale local copy exists) the default
// branch's worktree at base, so it always reflects the latest origin state.
func materializeDefault(repo *state.Repo, branch, base string) (state.Worktree, error) {
	return finishCreate(repo, branch, branch, base, func(path string) error {
		return git.AddOrResetWorktree(repo.Path, path, branch, base)
	})
}
