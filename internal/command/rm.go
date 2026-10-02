package command

import (
	"fmt"
	"os"

	"awt/internal/git"
	"awt/internal/state"
	"awt/internal/tmux"
)

func Rm(repoName, worktreeName string, force bool) error {
	repo, err := resolveRepo(repoName)
	if err != nil {
		return err
	}
	groups, err := groupOne(repo)
	if err != nil {
		return err
	}
	entries := flatten(groups)
	var target *Entry
	for i := range entries {
		if entries[i].Matches(worktreeName) {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no worktree named %q in %q", worktreeName, repo.Name)
	}
	// Checked here rather than carried on every entry: this is the only place a
	// worktree's dirtiness is acted on, and `git status` across a whole repo is
	// slow enough to dominate a listing.
	dirty, _ := git.IsDirty(target.Path)
	if dirty && !force {
		return fmt.Errorf("worktree %q has uncommitted changes — use --force to remove anyway", worktreeName)
	}

	// Killing the window takes the editor and agents with it. If it was the one
	// its repo had on screen, the repo simply leaves the status bar; whatever
	// siblings it had parked are untouched.
	if target.Alive {
		if err := tmux.KillWindow(target.Window); err != nil {
			return err
		}
	}
	if err := git.RemoveWorktree(repo.Path, target.Path, force); err != nil {
		return err
	}
	if err := git.DeleteBranch(repo.Path, target.Branch, force); err != nil {
		fmt.Fprintf(os.Stderr, "awt: could not delete branch %q: %v\n", target.Branch, err)
	}

	st, err := state.Load()
	if err != nil {
		return err
	}
	st.Remove(target.Repo, target.Name)
	return st.Save()
}
