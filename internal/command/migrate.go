package command

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/five-sh/awt/internal/naming"
	"github.com/five-sh/awt/internal/state"
	"github.com/five-sh/awt/internal/tmux"
)

// Migrate is the one-shot v0.1 → v0.2 switchover. v0.1 gave every worktree its
// own tmux session; v0.2 gives it a window in one shared session. Rather than
// rebuild those sessions in place, migrate kills them: the worktrees themselves
// stay in state and each gets a fresh window the next time it's switched to.
//
// Sessions state doesn't account for are reported, never killed — awt has no
// business guessing that someone else's session is one of its own.
func Migrate(dryRun bool) error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	sessions, err := tmux.ListSessions()
	if err != nil {
		return err
	}
	live := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		live[s.Name] = true
	}
	front, park := frontSession(), parkSession()

	type legacy struct {
		session string
		label   string
		idx     int // index into st.Worktrees, so the field can be cleared after
	}
	var found []legacy
	claimed := make(map[string]bool)
	for i, w := range st.Worktrees {
		name := w.Session
		if name == "" {
			// State can have lost the field (v0.2 stopped writing it); the name
			// v0.1 would have given this worktree is the next best guess.
			name = naming.SessionName(w.Repo, w.Name)
		}
		if name == front || name == park || !live[name] || claimed[name] {
			continue
		}
		claimed[name] = true
		found = append(found, legacy{session: name, label: w.Repo + "/" + w.Name, idx: i})
	}

	var strays []string
	for _, s := range sessions {
		// "--" is how v0.1 joined repo and worktree, so a session with one is a
		// plausible leftover; anything else is somebody else's session entirely.
		if s.Name != front && s.Name != park && !claimed[s.Name] && strings.Contains(s.Name, "--") {
			strays = append(strays, s.Name)
		}
	}
	sort.Strings(strays)

	switch {
	case len(found) == 0 && len(strays) == 0:
		fmt.Println("nothing to migrate — no v0.1 sessions are running")
		return nil
	case len(found) == 0:
		fmt.Println("no v0.1 session awt has a worktree for is still running")
	default:
		for _, l := range found {
			verb := "kill"
			if !dryRun {
				verb = "killed"
			}
			fmt.Printf("%s %s  (%s)\n", verb, l.session, l.label)
		}
		if dryRun {
			fmt.Printf("%s — rerun without --dry-run to kill them\n", plural(len(found), "session"))
		}
	}
	if len(strays) > 0 {
		fmt.Fprintf(os.Stderr, "\nawt: left alone, no worktree in state claims them:\n")
		for _, s := range strays {
			fmt.Fprintf(os.Stderr, "  %s   (tmux kill-session -t %s)\n", s, s)
		}
	}
	if dryRun || len(found) == 0 {
		return nil
	}

	for _, l := range found {
		if err := tmux.KillSession(l.session); err != nil {
			fmt.Fprintf(os.Stderr, "awt: could not kill %s: %v\n", l.session, err)
			continue
		}
		st.Worktrees[l.idx].Session = ""
	}
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf("\nworktrees are untouched — switch to one to build its window\n")
	return nil
}
