package command

import (
	"path/filepath"
	"time"

	"awt/internal/git"
	"awt/internal/naming"
	"awt/internal/state"
	"awt/internal/tmux"
)

// finishCreate lays out the worktree directory, spins up its tmux session, and
// records it in state. add does the actual git worktree add (new branch or existing).
func finishCreate(repo *state.Repo, name, branch, parent string, add func(path string) error) (state.Worktree, error) {
	root, err := workspaceRoot()
	if err != nil {
		return state.Worktree{}, err
	}
	path := filepath.Join(root, repo.Name, name)
	if err := add(path); err != nil {
		return state.Worktree{}, err
	}
	session := naming.Disambiguate(naming.SessionName(repo.Name, name), tmux.HasSession)
	if err := createSessionLayout(session, path); err != nil {
		return state.Worktree{}, err
	}
	wt := state.Worktree{
		Repo: repo.Name, Name: name, Branch: branch, Path: path,
		Session: session, Parent: parent, CreatedAt: time.Now(),
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

// createWorktree creates a new branch named name, based off base.
func createWorktree(repo *state.Repo, name, base string) (state.Worktree, error) {
	return finishCreate(repo, name, name, base, func(path string) error {
		return git.AddWorktree(repo.Path, path, name, base)
	})
}

// materializeExisting checks out a branch that already exists into a new worktree.
func materializeExisting(repo *state.Repo, branch string) (state.Worktree, error) {
	return finishCreate(repo, branch, branch, "", func(path string) error {
		return git.AddWorktreeExisting(repo.Path, path, branch)
	})
}

// materializeDefault creates (or resets, if a stale local copy exists) the default
// branch's worktree at base, so it always reflects the latest origin state.
func materializeDefault(repo *state.Repo, branch, base string) (state.Worktree, error) {
	return finishCreate(repo, branch, branch, base, func(path string) error {
		return git.AddOrResetWorktree(repo.Path, path, branch, base)
	})
}
