package command

import (
	"fmt"
	"os"
	"strings"

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
	wt, err := newWorktree(repo, opts.Name, opts.From, opts.NoParent)
	if err != nil {
		return err
	}
	return focus(Entry{Worktree: wt})
}

// newWorktree makes a brand-new branch and its worktree: the whole of `awt new`
// bar the attach, shared with the picker, where typing a name nothing answers to
// lands here too.
func newWorktree(repo *state.Repo, input, from string, noParent bool) (state.Worktree, error) {
	branch, err := naming.SlugifyBranch(input)
	if err != nil {
		return state.Worktree{}, err
	}
	name, err := naming.Slugify(branch)
	if err != nil {
		return state.Worktree{}, err
	}
	base, err := resolveBase(repo, from, noParent)
	if err != nil {
		return state.Worktree{}, err
	}
	return createWorktree(repo, name, branch, base)
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

// resolveNamedBase resolves an explicit --from into a base ref: a tracked worktree
// (by name or branch), else a local branch, else a branch on origin, named either
// bare ("main") or the way ls reports a parent ("origin/main") so that round-trips.
//
// A local branch is used exactly as it stands on disk, deliberately unfetched.
// Forking off a sibling worktree has to see that worktree's unpushed commits, and
// syncing the ref would move a branch out from under a live checkout, so staleness
// is the caller's to manage here — unlike the no---from default, which fetches.
func resolveNamedBase(repo *state.Repo, from string) (string, error) {
	st, err := state.Load()
	if err != nil {
		return "", err
	}
	// A stale state entry can outlive its branch (a worktree removed outside awt),
	// so fall through rather than handing git a ref that no longer resolves.
	if wt, ok := st.Find(repo.Name, from); ok && git.BranchExists(repo.Path, wt.Branch) {
		return wt.Branch, nil
	}
	if git.BranchExists(repo.Path, from) {
		return from, nil
	}
	remote := strings.TrimPrefix(from, "origin/")
	_, warn, rerr := git.RemoteBranch(repo.Path, remote)
	if rerr != nil {
		return "", fmt.Errorf("unknown branch or worktree %q", from)
	}
	if warn != "" {
		fmt.Fprintln(os.Stderr, "awt: "+warn)
	}
	// Short form, matching the ref FreshBase reports, since it lands in Parent.
	return "origin/" + remote, nil
}
