package command

import (
	"fmt"
	"os"

	"github.com/five-sh/awt/internal/git"
	"github.com/five-sh/awt/internal/state"
	"github.com/five-sh/awt/internal/tmux"
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

	return removeEntry(repo, *target, force, force, func(branch string, err error) bool {
		fmt.Fprintf(os.Stderr, "awt: could not delete branch %q: %v\n", branch, err)
		return false
	})
}

// removeEntry removes a worktree, its branch and its window, which takes the
// editor and agents with it. If it was the window its repo had on screen, the
// repo simply leaves the status bar; whatever siblings it had parked are
// untouched. force removes a dirty worktree; forceBranch deletes the branch
// even with unmerged commits. Short of that, a branch git refuses to delete is
// handed to unmerged, which decides whether to delete it anyway.
//
// The window goes last: it may well be the one awt is running in — `awt rm`
// typed into it, or the tree in a popup over it — and killing it first would
// stop awt with the worktree half removed.
func removeEntry(repo *state.Repo, target Entry, force, forceBranch bool, unmerged func(branch string, err error) bool) error {
	if err := git.RemoveWorktree(repo.Path, target.Path, force); err != nil {
		return err
	}
	if target.Branch != "" {
		if err := git.DeleteBranch(repo.Path, target.Branch, forceBranch); err != nil && unmerged(target.Branch, err) {
			if err := git.DeleteBranch(repo.Path, target.Branch, true); err != nil {
				fmt.Fprintf(os.Stderr, "awt: could not delete branch %q: %v\n", target.Branch, err)
			}
		}
	}

	st, err := state.Load()
	if err != nil {
		return err
	}
	st.Remove(target.Repo, target.Name)
	if err := st.Save(); err != nil {
		return err
	}
	if target.Alive {
		return tmux.KillWindow(target.Window)
	}
	return nil
}
