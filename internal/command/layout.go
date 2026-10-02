package command

import (
	"os"
	"strconv"
	"strings"

	"github.com/five-sh/awt/internal/naming"
	"github.com/five-sh/awt/internal/state"
	"github.com/five-sh/awt/internal/tmux"
)

const (
	sessionEnv     = "AWT_SESSION"
	defaultSession = "awt"
	parkSuffix     = "-park"

	// keeperWindow stops the park session from being destroyed: tmux drops a
	// session the moment its last window leaves, and parking is exactly that.
	keeperWindow = "awt-keeper"
	// scratchWindow is the window tmux insists on giving a brand-new session.
	// The first repo window swaps into its place and it's killed right after.
	scratchWindow = "awt-scratch"

	editPane    = "edit"
	agentPrefix = "agent-"
	firstAgent  = agentPrefix + "1"
	// agentColumn is how much of the window the agents' column takes, on the
	// left; the editor keeps the rest.
	agentColumn = "40%"

	defaultEditor = "nvim"
	claudeCmd     = "claude --dangerously-skip-permissions"
)

// frontSession is the one session you attach to: one window per repo you've
// opened, each showing that repo's current worktree.
func frontSession() string {
	if v := os.Getenv(sessionEnv); v != "" {
		return v
	}
	return defaultSession
}

// parkSession holds every worktree window that isn't on screen. It's never
// attached — it exists so those windows, and the processes in them, stay alive
// while only one per repo is visible.
func parkSession() string {
	return frontSession() + parkSuffix
}

func editorCmd() string {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = defaultEditor
	}
	return editor + " ."
}

// ensurePark creates the park session if it isn't up yet.
func ensurePark() (string, error) {
	park := parkSession()
	if tmux.HasSession(park) {
		return park, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/"
	}
	if err := tmux.NewSession(park, home, keeperWindow, ""); err != nil {
		return "", err
	}
	// tmux sizes a window from the clients of whatever session holds it, and it
	// gets this right by itself: a parked window keeps the size it had, and a
	// window coming back on screen takes the client's current size even if the
	// terminal was resized while it was away. So awt never resizes a window
	// explicitly — `resize-window` would switch the window to manual sizing and
	// strand it at 80x24 when it runs with no client attached. This option is
	// only insurance for the odd case of someone attaching to the park session
	// itself; it changes nothing about the paths awt drives.
	_ = tmux.SetOption(park, "window-size", "manual")
	// Belt and braces on the keeper: if its shell ever exits, the window (and so
	// the session, and so everything parked in it) stays.
	if windows, err := tmux.ListWindows(); err == nil {
		for _, w := range windows {
			if w.Session == park && w.Name == keeperWindow {
				_ = tmux.SetWindowOption(w.ID, "remain-on-exit", "on")
			}
		}
	}
	return park, nil
}

// createWorktreeWindow builds a worktree's window: an agent down the left,
// the editor on the right. It's created in the park session and moved on screen
// by the caller, so a window never appears in the status bar for the instant
// before it's in the right place.
func createWorktreeWindow(park string, wt state.Worktree) (string, error) {
	id, pane, err := tmux.NewWindow(park, naming.WindowName(wt.Repo, wt.Name), wt.Path, editorCmd())
	if err != nil {
		return "", err
	}
	_ = tmux.SetPaneTitle(pane, editPane)
	// Before, so the agent lands to the left of the editor rather than the right.
	// Detached, so the editor is the pane you land in.
	agent, err := tmux.SplitWindow(tmux.Split{
		Target: id, CWD: wt.Path, Cmd: claudeCmd,
		Horizontal: true, Before: true, Detach: true, Size: agentColumn,
	})
	if err != nil {
		_ = tmux.KillWindow(id) // half a layout is worse than none: start over next time
		return "", err
	}
	_ = tmux.SetPaneTitle(agent, firstAgent)
	return id, nil
}

// frontSpot is where a worktree's window has to go to be its repo's window in
// the front session.
type frontSpot struct {
	Index    int    // the index in the front session this repo occupies
	Occupant string // the window id sitting there now, "" when the index is free
}

// findFrontSpot decides that index without touching tmux, so the decision itself
// is testable. Three cases: the target is already on screen and nothing moves; a
// window it's allowed to take the place of is there (a sibling worktree of the
// same repo, or a new session's scratch window) and gets swapped out to park; or
// the repo isn't on screen at all and takes a fresh index at the end.
func findFrontSpot(windows []tmux.Window, front, target string, evictable map[string]bool, baseIndex int) frontSpot {
	spot := frontSpot{Index: -1}
	for _, w := range windows {
		if w.Session != front {
			continue
		}
		if w.ID == target {
			return frontSpot{Index: w.Index, Occupant: target}
		}
		if evictable[w.ID] && (spot.Index < 0 || w.Index < spot.Index) {
			spot = frontSpot{Index: w.Index, Occupant: w.ID}
		}
	}
	if spot.Index >= 0 {
		return spot
	}
	return frontSpot{Index: nextIndex(windows, front, baseIndex)}
}

// nextIndex is the first index past the end of a session's windows, or the
// session's base index when it has none.
func nextIndex(windows []tmux.Window, session string, baseIndex int) int {
	last := -1
	for _, w := range windows {
		if w.Session == session && w.Index > last {
			last = w.Index
		}
	}
	if last < 0 {
		return baseIndex
	}
	return last + 1
}

// belongsToRepo reports whether a window is one of repo's worktree windows: one
// state knows the id of, or — for anything state has lost track of — one still
// carrying the "<repo>:" prefix awt named it with.
func belongsToRepo(w tmux.Window, repo string, ids map[string]bool) bool {
	return ids[w.ID] || strings.HasPrefix(w.Name, naming.WindowName(repo, ""))
}

// repoWindowIDs is the set of window ids state records for a repo's worktrees.
func repoWindowIDs(repo string) map[string]bool {
	ids := make(map[string]bool)
	st, err := state.Load()
	if err != nil {
		return ids
	}
	for _, w := range st.ForRepo(repo) {
		if w.Window != "" {
			ids[w.Window] = true
		}
	}
	return ids
}

// repoFrontWindow finds the window a repo currently has on screen, if any.
func repoFrontWindow(windows []tmux.Window, front, repo string) (tmux.Window, bool) {
	ids := repoWindowIDs(repo)
	for _, w := range windows {
		if w.Session == front && belongsToRepo(w, repo, ids) {
			return w, true
		}
	}
	return tmux.Window{}, false
}

// evictableFront names the front-session windows the target may take the place
// of: the other worktrees of its own repo, since only one of them is on screen
// at a time, and the scratch window a brand-new front session comes with.
func evictableFront(windows []tmux.Window, front string, wt state.Worktree) (evictable map[string]bool, scratch string) {
	evictable = make(map[string]bool)
	ids := repoWindowIDs(wt.Repo)
	for _, w := range windows {
		if w.Session != front || w.ID == wt.Window {
			continue
		}
		if w.Name == scratchWindow {
			scratch, evictable[w.ID] = w.ID, true
			continue
		}
		if belongsToRepo(w, wt.Repo, ids) {
			evictable[w.ID] = true
		}
	}
	return evictable, scratch
}

// focus brings a worktree on screen: its window becomes its repo's window in the
// front session, at the index that repo already holds, and the client is pointed
// at it. v0.2's replacement for attaching to a per-worktree session.
func focus(e Entry) error {
	park, err := ensurePark()
	if err != nil {
		return err
	}
	wt := e.Worktree
	if wt.Window == "" || !e.Alive {
		id, err := createWorktreeWindow(park, wt)
		if err != nil {
			return err
		}
		wt.Window = id
	}

	front := frontSession()
	if !tmux.HasSession(front) {
		if err := tmux.NewSession(front, wt.Path, scratchWindow, ""); err != nil {
			return err
		}
	}
	windows, err := tmux.ListWindows()
	if err != nil {
		return err
	}
	evictable, scratch := evictableFront(windows, front, wt)
	spot := findFrontSpot(windows, front, wt.Window, evictable, tmux.BaseIndex())
	switch {
	case spot.Occupant == wt.Window:
		// Already this repo's window on screen; nothing to move.
	case spot.Occupant != "":
		if err := tmux.SwapWindow(wt.Window, front, spot.Index); err != nil {
			return err
		}
	default:
		if err := tmux.MoveWindow(wt.Window, front, spot.Index); err != nil {
			return err
		}
	}
	if scratch != "" && scratch != wt.Window {
		_ = tmux.KillWindow(scratch) // swapped out to park a moment ago
	}
	// Keep the name in step with the worktree — but only for a window awt named
	// in the first place. Bringing up a window awt merely found (the untracked
	// group) shouldn't relabel somebody else's window.
	if wt.Repo != untrackedRepo {
		if want := naming.WindowName(wt.Repo, wt.Name); want != nameOf(windows, wt.Window) {
			_ = tmux.RenameWindow(wt.Window, want)
		}
	}
	if err := tmux.SelectWindow(front, spot.Index); err != nil {
		return err
	}
	adopt(wt)
	if cur, ok := tmux.CurrentSession(); ok && cur == front {
		return nil // already in the front session: select-window was the whole job
	}
	return tmux.SwitchOrAttach(front)
}

func nameOf(windows []tmux.Window, id string) string {
	for _, w := range windows {
		if w.ID == id {
			return w.Name
		}
	}
	return ""
}

// agentNumber reads the N out of an "agent-N" pane title.
func agentNumber(title string) (int, bool) {
	n, ok := strings.CutPrefix(title, agentPrefix)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(n)
	if err != nil {
		return 0, false
	}
	return i, true
}

// nextAgentPane returns "agent-N" one past the highest N among a window's panes.
func nextAgentPane(panes []tmux.Pane) string {
	high := 0
	for _, p := range panes {
		if n, ok := agentNumber(p.Title); ok && n > high {
			high = n
		}
	}
	return agentPrefix + strconv.Itoa(high+1)
}

// agentSplit says where a new agent pane goes: under the last agent, so they
// stack down the left column, or — for a window whose agents have all been
// closed — a fresh column to the left of the editor.
func agentSplit(window string, panes []tmux.Pane) tmux.Split {
	last := ""
	for _, p := range panes {
		if _, ok := agentNumber(p.Title); ok {
			last = p.ID
		}
	}
	if last == "" {
		return tmux.Split{Target: window, Horizontal: true, Before: true, Size: agentColumn}
	}
	return tmux.Split{Target: last}
}
