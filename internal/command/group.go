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

// snapshot is one invocation's view of the world: state and the live tmux
// sessions, each read once rather than once per repo.
type snapshot struct {
	st       *state.Store
	sessions map[string]tmux.Session
}

func loadSnapshot() (*snapshot, error) {
	st, err := state.Load()
	if err != nil {
		return nil, err
	}
	live, err := tmux.ListSessions()
	if err != nil {
		return nil, err
	}
	sessions := make(map[string]tmux.Session, len(live))
	for _, s := range live {
		sessions[s.Name] = s
	}
	return &snapshot{st: st, sessions: sessions}, nil
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

// groupAll builds a group per registered repo plus, last, one for live sessions
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
		entries = append(entries, s.entry(state.Worktree{
			Repo: repo.Name, Name: name, Branch: lw.Branch, Path: lw.Path,
			Session: naming.SessionName(repo.Name, name),
		}))
	}
	return entries, nil
}

// entry marks a worktree live if its session exists, preferring tmux's own
// last-attached time: state only records the attaches awt itself performed.
func (s *snapshot) entry(w state.Worktree) Entry {
	sess, alive := s.sessions[w.Session]
	if alive && sess.LastAttached.After(w.LastAttached) {
		w.LastAttached = sess.LastAttached
	}
	return Entry{Worktree: w, Alive: alive}
}

// untracked reports live sessions no group already covers: one whose worktree
// was removed behind awt's back, or one awt never created.
func (s *snapshot) untracked(groups []Group) []Entry {
	covered := make(map[string]bool)
	for _, g := range groups {
		for _, e := range g.Entries {
			covered[e.Session] = true
		}
	}
	var out []Entry
	for name, sess := range s.sessions {
		if covered[name] {
			continue
		}
		out = append(out, Entry{
			Worktree: state.Worktree{
				Repo: untrackedRepo, Name: name, Session: name, LastAttached: sess.LastAttached,
			},
			Alive: true,
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

// currentRepo is the repo owning the session this command was run from, if any.
func (s *snapshot) currentRepo() string {
	name, ok := tmux.CurrentSession()
	if !ok {
		return ""
	}
	for _, w := range s.st.Worktrees {
		if w.Session == name {
			return w.Repo
		}
	}
	return ""
}

// sortGroups orders groups by the current repo, then liveness, then recency; and
// each group's entries the same way, so the sessions you'd actually switch to
// sit at the top of both the listing and the picker.
func sortGroups(groups []Group) {
	for _, g := range groups {
		sort.SliceStable(g.Entries, func(i, j int) bool { return entryLess(g.Entries[i], g.Entries[j]) })
	}
	sort.SliceStable(groups, func(i, j int) bool { return groupLess(groups[i], groups[j]) })
}

func entryLess(a, b Entry) bool {
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
