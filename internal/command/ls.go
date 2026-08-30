package command

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"awt/internal/git"
	"awt/internal/naming"
	"awt/internal/state"
	"awt/internal/tmux"
)

const timeFormat = "Jan 2 15:04"

type Entry struct {
	state.Worktree
	Dirty bool
	Alive bool
}

// listRepo merges worktrees tracked in state with whatever git actually reports for
// repo, so worktrees awt didn't create (the main checkout, a manual `git worktree
// add`) still show up.
func listRepo(repo *state.Repo) ([]Entry, error) {
	st, err := state.Load()
	if err != nil {
		return nil, err
	}
	sessions, err := tmux.ListSessions()
	if err != nil {
		return nil, err
	}
	alive := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		alive[s] = true
	}

	seen := make(map[string]bool)
	var entries []Entry
	for _, w := range st.ForRepo(repo.Name) {
		dirty, _ := git.IsDirty(w.Path)
		entries = append(entries, Entry{Worktree: w, Dirty: dirty, Alive: alive[w.Session]})
		seen[w.Path] = true
	}

	live, err := git.ListWorktrees(repo.Path)
	if err != nil {
		return nil, err
	}
	for _, lw := range live {
		if lw.Bare || seen[lw.Path] {
			continue
		}
		name := lw.Branch
		if name == "" {
			name = filepath.Base(lw.Path)
		}
		session := naming.SessionName(repo.Name, name)
		dirty, _ := git.IsDirty(lw.Path)
		entries = append(entries, Entry{
			Worktree: state.Worktree{Repo: repo.Name, Name: name, Branch: lw.Branch, Path: lw.Path, Session: session},
			Dirty:    dirty,
			Alive:    alive[session],
		})
	}
	return entries, nil
}

func Ls(repoName string) error {
	if repoName == "" {
		return lsAll()
	}
	repo, err := resolveRepo(repoName)
	if err != nil {
		return err
	}
	entries, err := listRepo(repo)
	if err != nil {
		return err
	}
	return printLs(entries, false)
}

func lsAll() error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	var all []Entry
	for _, r := range st.Repos {
		entries, err := listRepo(&r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "awt: skipping %q: %v\n", r.Name, err)
			continue
		}
		all = append(all, entries...)
	}
	return printLs(all, true)
}

func printLs(entries []Entry, showRepo bool) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	if showRepo {
		fmt.Fprintln(w, "REPO\tNAME\tBRANCH\tFROM\tDIRTY\tSESSION\tLAST ATTACHED")
	} else {
		fmt.Fprintln(w, "NAME\tBRANCH\tFROM\tDIRTY\tSESSION\tLAST ATTACHED")
	}
	for _, e := range entries {
		if showRepo {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				e.Repo, e.Name, e.Branch, e.Parent, yesNo(e.Dirty), yesNo(e.Alive), formatTime(e.LastAttached))
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				e.Name, e.Branch, e.Parent, yesNo(e.Dirty), yesNo(e.Alive), formatTime(e.LastAttached))
		}
	}
	return w.Flush()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(timeFormat)
}
