package command

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"awt/internal/state"
)

const (
	timeFormat = "Jan 2 15:04"
	lsColumns  = "NAME\tBRANCH\tFROM\tSESSION\tLAST ATTACHED"
)

type Entry struct {
	state.Worktree
	Alive bool
}

// Matches reports whether name identifies this entry, either by its (possibly
// slugified) name or by the raw branch it checks out.
func (e Entry) Matches(name string) bool {
	return e.Name == name || e.Branch == name
}

func Ls(repoName string) error {
	if repoName == "" {
		groups, err := groupAll()
		if err != nil {
			return err
		}
		return printLs(groups)
	}
	repo, err := resolveRepo(repoName)
	if err != nil {
		return err
	}
	groups, err := groupOne(repo)
	if err != nil {
		return err
	}
	return printLs(groups)
}

// printLs groups by repo on a terminal and falls back to one flat table when
// piped, so `awt ls | grep ...` keeps seeing the same columns it always did.
func printLs(groups []Group) error {
	if isTTY(os.Stdout) {
		return printGrouped(groups)
	}
	return printFlat(groups)
}

func printGrouped(groups []Group) error {
	for i, g := range groups {
		if i > 0 {
			fmt.Println()
		}
		if g.Err != nil {
			fmt.Printf("%s  unreadable: %v\n", g.Repo, g.Err)
			continue
		}
		fmt.Printf("%s  %s · %d live\n", g.Repo, plural(len(g.Entries), "worktree"), g.Live())
		if len(g.Entries) == 0 {
			continue
		}
		// A tabwriter per section: it sizes columns over a run of tabbed lines,
		// and the headings between groups would break that run anyway.
		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "  "+lsColumns)
		for _, e := range g.Entries {
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n",
				e.Name, e.Branch, e.Parent, yesNo(e.Alive), formatTime(e.LastAttached))
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func printFlat(groups []Group) error {
	showRepo := len(groups) > 1
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	if showRepo {
		fmt.Fprintln(w, "REPO\t"+lsColumns)
	} else {
		fmt.Fprintln(w, lsColumns)
	}
	for _, e := range flatten(groups) {
		if showRepo {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				e.Repo, e.Name, e.Branch, e.Parent, yesNo(e.Alive), formatTime(e.LastAttached))
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				e.Name, e.Branch, e.Parent, yesNo(e.Alive), formatTime(e.LastAttached))
		}
	}
	return w.Flush()
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
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
