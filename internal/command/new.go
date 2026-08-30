package command

import (
	"fmt"
	"os"

	"awt/internal/git"
	"awt/internal/naming"
	"awt/internal/state"
)

type NewOptions struct {
	Repo     string
	Name     string
	From     string
	NoParent bool
}

func New(opts NewOptions) error {
	repo, err := resolveRepo(opts.Repo)
	if err != nil {
		return err
	}
	slug, err := naming.Slugify(opts.Name)
	if err != nil {
		return err
	}
	base, err := resolveBase(repo, opts.From, opts.NoParent)
	if err != nil {
		return err
	}
	wt, err := createWorktree(repo, slug, base)
	if err != nil {
		return err
	}
	return attach(Entry{Worktree: wt, Alive: true})
}

// resolveBase picks the base ref for a new branch: an explicit --from, else the
// worktree cwd is already in (if it belongs to this repo), else the repo's default
// branch freshly fetched from origin.
func resolveBase(repo *state.Repo, from string, noParent bool) (string, error) {
	if from != "" {
		return resolveNamedBase(repo, from)
	}
	if !noParent {
		if cwd, err := os.Getwd(); err == nil {
			if root, rerr := git.RepoRoot(cwd); rerr == nil && root == repo.Path {
				if top, terr := git.Toplevel(cwd); terr == nil && top != repo.Path {
					return git.CurrentBranch(top)
				}
			}
		}
	}
	_, base, warn, err := git.FreshBase(repo.Path)
	if warn != "" {
		fmt.Fprintln(os.Stderr, "awt: "+warn)
	}
	return base, err
}

func resolveNamedBase(repo *state.Repo, from string) (string, error) {
	st, err := state.Load()
	if err != nil {
		return "", err
	}
	if wt, ok := st.Find(repo.Name, from); ok {
		return wt.Branch, nil
	}
	if !git.BranchExists(repo.Path, from) {
		return "", fmt.Errorf("unknown branch or worktree %q", from)
	}
	return from, nil
}
