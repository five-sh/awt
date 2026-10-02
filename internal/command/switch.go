package command

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"awt/internal/git"
	"awt/internal/state"
)

// Switch resolves repo/worktree and brings it on screen, creating its tmux
// window and, if needed, the worktree itself (an existing branch, or the repo's
// default branch) on the way. Empty repoName opens the tree across every
// registered repo; an empty worktreeName opens the tree for repoName alone.
func Switch(repoName, worktreeName string) error {
	if repoName == "" {
		return browse(nil)
	}
	repo, err := resolveRepo(repoName)
	if err != nil {
		return err
	}
	if worktreeName != "" {
		return switchTo(repo, worktreeName, nil)
	}
	return browse(repo)
}

// switchTo attaches to repo's worktree named name, materializing it first if it
// doesn't exist yet. create, if given, widens that to a name nothing at all
// answers to — no worktree, no branch anywhere — and makes the brand-new branch
// for it. It's how the tree turns a name you typed into a window on screen.
func switchTo(repo *state.Repo, name string, create func() (state.Worktree, error)) error {
	groups, err := groupOne(repo)
	if err != nil {
		return err
	}
	entries := flatten(groups)
	for i := range entries {
		if entries[i].Matches(name) {
			return focus(entries[i])
		}
	}
	wt, err := materialize(repo, name)
	if create != nil && errors.Is(err, errNoSuchName) {
		wt, err = create()
	}
	if err != nil {
		return err
	}
	return focus(Entry{Worktree: wt})
}

// resolveTyped turns a name typed into the global tree into a repo and a name
// within it. The first segment of "<repo>/<branch>" names the repo and the rest
// is the branch — itself possibly containing "/", as in
// "codex/foo". A name with no "/" belongs to whichever repo pickerRepo settles
// on, and is ambiguous when that's none. An unregistered repo name gets
// registered (and cloned) on the spot, same as `awt <repo> <name>` does.
func resolveTyped(groups []Group, typed string) (*state.Repo, string, error) {
	repoName, name, ok := strings.Cut(typed, "/")
	if !ok {
		repoName, name = pickerRepo(groups), typed
		if repoName == "" {
			return nil, "", fmt.Errorf("type '<repo>/<branch>' to make a worktree for %q", typed)
		}
	}
	if repoName == "" || name == "" {
		return nil, "", fmt.Errorf("invalid name %q, want '<repo>/<branch>'", typed)
	}
	if repoName == untrackedRepo {
		return nil, "", fmt.Errorf("%s isn't a repo — its sessions have no worktree awt can track", untrackedRepo)
	}
	repo, err := resolveRepo(repoName)
	if err != nil {
		return nil, "", err
	}
	return repo, name, nil
}

// pickerRepo names the repo a bare typed name belongs to: the repo the tree was
// opened from, or else the only one it had to offer.
func pickerRepo(groups []Group) string {
	if current := currentGroup(groups); current != "" {
		return current
	}
	sole := ""
	for _, g := range groups {
		if g.Repo == untrackedRepo {
			continue
		}
		if sole != "" {
			return "" // more than one on offer: a bare name doesn't say which
		}
		sole = g.Repo
	}
	return sole
}

// errNoSuchName reports that nothing by that name exists to check out: no local
// branch, nothing on origin, and not the default branch either. The tree turns
// that into a new branch; an explicit `awt switch` reports it as the error it is.
var errNoSuchName = errors.New("no such branch or worktree")

// materialize creates a worktree for a name that has none yet. The default branch
// always resets to the latest origin, even over a stale local copy (e.g. one left
// by a bare clone); any other name just checks out its existing local branch.
func materialize(repo *state.Repo, name string) (state.Worktree, error) {
	branch, err := git.DefaultBranch(repo.Path)
	if err != nil {
		return state.Worktree{}, err
	}
	if name == branch {
		_, base, warn, err := git.FreshBase(repo.Path)
		if err != nil {
			return state.Worktree{}, err
		}
		if warn != "" {
			fmt.Fprintln(os.Stderr, "awt: "+warn)
		}
		return materializeDefault(repo, name, base)
	}
	if git.BranchExists(repo.Path, name) {
		return materializeExisting(repo, name)
	}
	if ref, warn, err := git.RemoteBranch(repo.Path, name); err == nil {
		if warn != "" {
			fmt.Fprintln(os.Stderr, "awt: "+warn)
		}
		return materializeRemote(repo, name, ref)
	}
	return state.Worktree{}, fmt.Errorf("%w named %q in %q — try 'awt new %s %s'",
		errNoSuchName, name, repo.Name, repo.Name, name)
}

// adopt records a worktree in state (even one awt didn't create, like the main
// checkout) and bumps its LastAttached, so a discovered worktree becomes tracked.
func adopt(wt state.Worktree) {
	st, err := state.Load()
	if err != nil {
		return
	}
	wt.LastAttached = time.Now()
	st.Upsert(wt)
	_ = st.Save()
}

func currentGroup(groups []Group) string {
	for _, g := range groups {
		if g.Current {
			return g.Repo
		}
	}
	return ""
}
