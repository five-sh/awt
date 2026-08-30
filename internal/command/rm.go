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
	entries, err := listRepo(repo)
	if err != nil {
		return err
	}
	var target *Entry
	for i := range entries {
		if entries[i].Name == worktreeName {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no worktree named %q in %q", worktreeName, repo.Name)
	}
	if target.Dirty && !force {
		return fmt.Errorf("worktree %q has uncommitted changes — use --force to remove anyway", worktreeName)
	}

	if tmux.HasSession(target.Session) {
		if err := tmux.KillSession(target.Session); err != nil {
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
