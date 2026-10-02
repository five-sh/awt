package command

import (
	"path/filepath"
	"sort"
	"sync"
	"time"

	"awt/internal/git"
	"awt/internal/naming"
	"awt/internal/state"
	"awt/internal/tmux"
)

const (
	untrackedRepo = "(untracked)"
	maxParallel   = 16
)

// Group is one repo's entries, sorted and ready to print or pick from.
type Group struct {
	Repo    string
	Entries []Entry
	Current bool  // the group the running command was invoked from
	Err     error // repo is registered but couldn't be listed
}

func (g Group) Live() int {
	n := 0
	for _, e := range g.Entries {
		if e.Alive {
			n++
		}
	}
	return n
}

func (g Group) lastAttached() time.Time {
	var latest time.Time
	for _, e := range g.Entries {
		if e.LastAttached.After(latest) {
			latest = e.LastAttached
		}
	}
	return latest
}

func flatten(groups []Group) []Entry {
	var out []Entry
	for _, g := range groups {
		out = append(out, g.Entries...)
	}
	return out
}

// snapshot is one invocation's view of the world: state and every window on the
// tmux server, each read once rather than once per repo.
type snapshot struct {
	st      *state.Store
	windows map[string]tmux.Window // by window id
	byName  map[string]tmux.Window // by window name, for entries state has lost
	front   string
}

func loadSnapshot() (*snapshot, error) {
	st, err := state.Load()
	if err != nil {
		return nil, err
	}
	live, err := tmux.ListWindows()
	if err != nil {
		return nil, err
	}
	s := &snapshot{
		st:      st,
		windows: make(map[string]tmux.Window, len(live)),
		byName:  make(map[string]tmux.Window, len(live)),
		front:   frontSession(),
	}
	park := parkSession()
	for _, w := range live {
		// Only awt's own two sessions are awt's business; a window in some other
		// session isn't a worktree window awt can move around.
		if w.Session != s.front && w.Session != park {
			continue
		}
		s.windows[w.ID] = w
		if _, seen := s.byName[w.Name]; !seen {
			s.byName[w.Name] = w
		}
	}
	return s, nil
}

// groupOne wraps a single repo's entries, so printers and the picker take
// []Group either way.
func groupOne(repo *state.Repo) ([]Group, error) {
	s, err := loadSnapshot()
	if err != nil {
		return nil, err
	}
	entries, err := s.repoEntries(repo)
	if err != nil {
		return nil, err
	}
	groups := []Group{{Repo: repo.Name, Entries: entries}}
	s.finish(groups)
	return groups, nil
}

// groupAll builds a group per registered repo plus, last, one for live windows
// no repo accounts for. A repo that can't be listed keeps its group with the
// error attached, rather than dropping out of the output entirely.
func groupAll() ([]Group, error) {
	s, err := loadSnapshot()
	if err != nil {
		return nil, err
	}
	// One `git worktree list` per repo, concurrently: they're independent and
	// sequentially they were the bulk of a listing's runtime.
	groups := make([]Group, len(s.st.Repos))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for i := range s.st.Repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			entries, err := s.repoEntries(&s.st.Repos[i])
			groups[i] = Group{Repo: s.st.Repos[i].Name, Entries: entries, Err: err}
		}(i)
	}
	wg.Wait()
	if untracked := s.untracked(groups); len(untracked) > 0 {
		groups = append(groups, Group{Repo: untrackedRepo, Entries: untracked})
	}
	s.finish(groups)
	return groups, nil
}

// repoEntries merges worktrees tracked in state with whatever git actually reports
// for repo, so worktrees awt didn't create (the main checkout, a manual `git
// worktree add`) still show up.
func (s *snapshot) repoEntries(repo *state.Repo) ([]Entry, error) {
	seen := make(map[string]bool)
	var entries []Entry
	for _, w := range s.st.ForRepo(repo.Name) {
		entries = append(entries, s.entry(w))
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
		w := state.Worktree{Repo: repo.Name, Name: name, Branch: lw.Branch, Path: lw.Path}
		// No window recorded, but there may well be one: state can be lost while
		// the window it described is still running, and the window carries the
		// same "<repo>:<worktree>" name awt gave it.
		if win, ok := s.byName[naming.WindowName(repo.Name, name)]; ok {
			w.Window = win.ID
		}
		entries = append(entries, s.entry(w))
	}
	return entries, nil
}

// entry marks a worktree live if its window still exists, and active if that
// window is the one its repo currently shows in the front session.
func (s *snapshot) entry(w state.Worktree) Entry {
	win, alive := tmux.Window{}, false
	if w.Window != "" {
		win, alive = s.windows[w.Window]
	}
	// Window activity is a fallback, not a preference: it moves whenever an agent
	// prints something, so it would reorder the picker by output rather than by
	// attention. It's only worth having for a window awt has never switched to.
	if alive && w.LastAttached.IsZero() {
		w.LastAttached = win.Activity
	}
	return Entry{Worktree: w, Alive: alive, Active: alive && win.Session == s.front}
}

// untracked reports live windows no group already covers: a worktree removed
// behind awt's back, or a window awt never created. awt's own bookkeeping
// windows aren't worktrees and stay out of it.
func (s *snapshot) untracked(groups []Group) []Entry {
	covered := make(map[string]bool)
	for _, g := range groups {
		for _, e := range g.Entries {
			if e.Window != "" {
				covered[e.Window] = true
			}
		}
	}
	var out []Entry
	for id, win := range s.windows {
		if covered[id] || win.Name == keeperWindow || win.Name == scratchWindow {
			continue
		}
		out = append(out, Entry{
			Worktree: state.Worktree{
				Repo: untrackedRepo, Name: win.Name, Window: id, LastAttached: win.Activity,
			},
			Alive:  true,
			Active: win.Session == s.front,
		})
	}
	return out
}

// finish puts everything in display order.
func (s *snapshot) finish(groups []Group) {
	current := s.currentRepo()
	for i := range groups {
		groups[i].Current = current != "" && groups[i].Repo == current
	}
	sortGroups(groups)
}

// currentRepo is the repo owning the window this command was run from, if any.
func (s *snapshot) currentRepo() string {
	id, ok := tmux.CurrentWindow()
	if !ok {
		return ""
	}
	for _, w := range s.st.Worktrees {
		if w.Window == id {
			return w.Repo
		}
	}
	return ""
}

// sortGroups orders groups by the current repo, then liveness, then recency; and
// each group's entries the same way, so the worktrees you'd actually switch to
// sit at the top of both the listing and the picker.
func sortGroups(groups []Group) {
	for _, g := range groups {
		sort.SliceStable(g.Entries, func(i, j int) bool { return entryLess(g.Entries[i], g.Entries[j]) })
	}
	sort.SliceStable(groups, func(i, j int) bool { return groupLess(groups[i], groups[j]) })
}

func entryLess(a, b Entry) bool {
	if a.Active != b.Active {
		return a.Active
	}
	if a.Alive != b.Alive {
		return a.Alive
	}
	if !a.LastAttached.Equal(b.LastAttached) {
		return a.LastAttached.After(b.LastAttached)
	}
	return a.Name < b.Name
}

func groupLess(a, b Group) bool {
	if a.Current != b.Current {
		return a.Current
	}
	if (a.Repo == untrackedRepo) != (b.Repo == untrackedRepo) {
		return b.Repo == untrackedRepo
	}
	if (a.Live() > 0) != (b.Live() > 0) {
		return a.Live() > 0
	}
	if !a.lastAttached().Equal(b.lastAttached()) {
		return a.lastAttached().After(b.lastAttached())
	}
	return a.Repo < b.Repo
}
