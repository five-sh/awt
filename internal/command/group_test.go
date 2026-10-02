package command

import (
	"testing"
	"time"

	"github.com/five-sh/awt/internal/state"
	"github.com/five-sh/awt/internal/tmux"
)

func entryOf(name string, alive, active bool, lastAttached time.Time) Entry {
	return Entry{
		Worktree: state.Worktree{Name: name, LastAttached: lastAttached},
		Alive:    alive,
		Active:   active,
	}
}

// The picker's top row should be the thing you'd most likely switch to: what's
// on screen, then what's parked and running, then by recency.
func TestEntryLess(t *testing.T) {
	now := time.Now()
	older := now.Add(-time.Hour)

	cases := []struct {
		name string
		a, b Entry
		want bool
	}{
		{"active beats parked", entryOf("a", true, true, older), entryOf("b", true, false, now), true},
		{"parked beats no window", entryOf("a", true, false, older), entryOf("b", false, false, now), true},
		{"recent beats stale", entryOf("a", true, false, now), entryOf("b", true, false, older), true},
		{"name breaks a tie", entryOf("a", true, false, now), entryOf("b", true, false, now), true},
		{"stale loses", entryOf("b", true, false, older), entryOf("a", true, false, now), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := entryLess(c.a, c.b); got != c.want {
				t.Errorf("entryLess = %v, want %v", got, c.want)
			}
		})
	}
}

func TestWhere(t *testing.T) {
	cases := []struct {
		e    Entry
		want string
	}{
		{entryOf("a", true, true, time.Time{}), "active"},
		{entryOf("a", true, false, time.Time{}), "parked"},
		{entryOf("a", false, false, time.Time{}), "-"},
	}
	for _, c := range cases {
		if got := where(c.e); got != c.want {
			t.Errorf("where(%+v) = %q, want %q", c.e, got, c.want)
		}
	}
}

// A worktree awt has never switched to has no LastAttached of its own, so the
// window's activity stands in. One it has switched to keeps its own record:
// window activity moves whenever an agent prints something, which would order
// the picker by output rather than by attention.
func TestEntryActivityFallback(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	attached := now.Add(-time.Hour)
	s := &snapshot{
		front: "awt",
		windows: map[string]tmux.Window{
			"@1": {ID: "@1", Session: "awt", Activity: now},
			"@2": {ID: "@2", Session: "awt-park", Activity: now},
		},
	}

	e := s.entry(state.Worktree{Name: "never-switched", Window: "@1"})
	if !e.Active || !e.Alive {
		t.Errorf("window in the front session should be alive and active: %+v", e)
	}
	if !e.LastAttached.Equal(now) {
		t.Errorf("LastAttached = %v, want the window's activity %v", e.LastAttached, now)
	}

	e = s.entry(state.Worktree{Name: "parked", Window: "@2", LastAttached: attached})
	if e.Active || !e.Alive {
		t.Errorf("parked window should be alive but not active: %+v", e)
	}
	if !e.LastAttached.Equal(attached) {
		t.Errorf("LastAttached = %v, want awt's own record %v", e.LastAttached, attached)
	}

	if e := s.entry(state.Worktree{Name: "gone", Window: "@9"}); e.Alive || e.Active {
		t.Errorf("a window tmux has never heard of should be dead: %+v", e)
	}
	if e := s.entry(state.Worktree{Name: "unbuilt"}); e.Alive || e.Active {
		t.Errorf("a worktree with no window should be dead: %+v", e)
	}
}
