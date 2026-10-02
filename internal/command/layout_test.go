package command

import (
	"testing"

	"github.com/five-sh/awt/internal/tmux"
)

// win is shorthand for the fields the layout decisions actually look at.
func win(id, session, name string, idx int) tmux.Window {
	return tmux.Window{ID: id, Session: session, Name: name, Index: idx}
}

func TestFindFrontSpot(t *testing.T) {
	const front, park = "awt", "awt-park"
	windows := []tmux.Window{
		win("@1", front, "pair-be:codex-auth", 1),
		win("@2", front, "hackerrank:master", 2),
		win("@3", park, "pair-be:main", 0),
		win("@4", park, "vmprovider:master", 1),
	}

	cases := []struct {
		name      string
		target    string
		evictable map[string]bool
		want      frontSpot
	}{
		{
			name:      "already on screen: stays where it is",
			target:    "@2",
			evictable: map[string]bool{"@1": true},
			want:      frontSpot{Index: 2, Occupant: "@2"},
		},
		{
			name:      "sibling of the same repo is on screen: swap with it",
			target:    "@3",
			evictable: map[string]bool{"@1": true},
			want:      frontSpot{Index: 1, Occupant: "@1"},
		},
		{
			name:      "repo isn't on screen at all: next index along",
			target:    "@4",
			evictable: nil,
			want:      frontSpot{Index: 3},
		},
		{
			name:      "lowest evictable index wins",
			target:    "@4",
			evictable: map[string]bool{"@2": true, "@1": true},
			want:      frontSpot{Index: 1, Occupant: "@1"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := findFrontSpot(windows, front, c.target, c.evictable, 0); got != c.want {
				t.Errorf("findFrontSpot = %+v, want %+v", got, c.want)
			}
		})
	}
}

// A brand-new front session has only its scratch window, and the first repo
// window has to land in that window's place so it doesn't sit at index 1 with a
// dead scratch window beside it.
func TestFindFrontSpotScratchOnly(t *testing.T) {
	windows := []tmux.Window{win("@9", "awt", scratchWindow, 0)}
	got := findFrontSpot(windows, "awt", "@1", map[string]bool{"@9": true}, 0)
	want := frontSpot{Index: 0, Occupant: "@9"}
	if got != want {
		t.Errorf("findFrontSpot = %+v, want %+v", got, want)
	}
}

func TestNextIndex(t *testing.T) {
	windows := []tmux.Window{
		win("@1", "awt", "a", 1),
		win("@2", "awt", "b", 4),
		win("@3", "awt-park", "c", 9),
	}
	if got := nextIndex(windows, "awt", 0); got != 5 {
		t.Errorf("nextIndex(awt) = %d, want 5", got)
	}
	if got := nextIndex(windows, "awt-park", 0); got != 10 {
		t.Errorf("nextIndex(awt-park) = %d, want 10", got)
	}
	// An empty session starts at whatever base-index tmux is configured with.
	if got := nextIndex(windows, "other", 1); got != 1 {
		t.Errorf("nextIndex(other) = %d, want 1", got)
	}
	if got := nextIndex(nil, "awt", 0); got != 0 {
		t.Errorf("nextIndex(nil) = %d, want 0", got)
	}
}

func TestBelongsToRepo(t *testing.T) {
	ids := map[string]bool{"@7": true}
	cases := []struct {
		w    tmux.Window
		repo string
		want bool
	}{
		{win("@7", "awt", "renamed-by-hand", 1), "pair-be", true},  // known id
		{win("@8", "awt", "pair-be:main", 2), "pair-be", true},     // name prefix
		{win("@8", "awt", "pair-fe:main", 2), "pair-be", false},    // different repo
		{win("@8", "awt", "pair-be-other:x", 2), "pair-be", false}, // prefix must include the ":"
		{win("@8", "awt", keeperWindow, 2), "pair-be", false},
	}
	for _, c := range cases {
		if got := belongsToRepo(c.w, c.repo, ids); got != c.want {
			t.Errorf("belongsToRepo(%q, %q) = %v, want %v", c.w.Name, c.repo, got, c.want)
		}
	}
}

func TestNextAgentPane(t *testing.T) {
	// pane_title defaults to the hostname, so anything awt didn't title itself
	// has to be ignored rather than counted.
	panes := []tmux.Pane{
		{ID: "%1", Title: editPane},
		{ID: "%2", Title: "agent-1"},
		{ID: "%3", Title: "MyLaptop"},
		{ID: "%4", Title: "agent-3"},
	}
	if got := nextAgentPane(panes); got != "agent-4" {
		t.Errorf("nextAgentPane = %q, want agent-4", got)
	}
	if got := nextAgentPane([]tmux.Pane{{ID: "%1", Title: editPane}}); got != firstAgent {
		t.Errorf("nextAgentPane(editor only) = %q, want %q", got, firstAgent)
	}
}

func TestAgentSplit(t *testing.T) {
	// No agent left in the window: a fresh column, to the left of the editor.
	got := agentSplit("@1", []tmux.Pane{{ID: "%1", Title: editPane}})
	want := tmux.Split{Target: "@1", Horizontal: true, Before: true, Size: agentColumn}
	if got != want {
		t.Errorf("first agent = %+v, want %+v", got, want)
	}
	// Later agents stack under the last one, leaving the editor's width alone.
	panes := []tmux.Pane{
		{ID: "%1", Title: editPane},
		{ID: "%2", Title: "agent-1"},
		{ID: "%3", Title: "agent-2"},
	}
	got = agentSplit("@1", panes)
	if want = (tmux.Split{Target: "%3"}); got != want {
		t.Errorf("later agent = %+v, want %+v", got, want)
	}
}

func TestAgentNumber(t *testing.T) {
	if n, ok := agentNumber("agent-12"); !ok || n != 12 {
		t.Errorf("agentNumber(agent-12) = (%d, %v), want (12, true)", n, ok)
	}
	for _, bad := range []string{"edit", "agent-", "agent-x", "MyLaptop", ""} {
		if _, ok := agentNumber(bad); ok {
			t.Errorf("agentNumber(%q) = ok, want not ok", bad)
		}
	}
}
