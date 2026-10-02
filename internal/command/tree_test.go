package command

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"awt/internal/state"
)

func treeEntry(repo, branch string, alive, active bool) Entry {
	return Entry{
		Worktree: state.Worktree{Repo: repo, Name: branch, Branch: branch},
		Alive:    alive,
		Active:   active,
	}
}

func testGroups() []Group {
	return []Group{
		{Repo: "myrepo", Current: true, Entries: []Entry{
			treeEntry("myrepo", "main", true, true),
			treeEntry("myrepo", "codex/long-name", true, false),
		}},
		{Repo: "other", Entries: []Entry{treeEntry("other", "main", false, false)}},
		{Repo: "empty"},
		{Repo: "broken", Err: errors.New("boom")},
	}
}

// names is what each line shows before its status, trailing padding dropped.
func names(lines []string) []string {
	var out []string
	for _, l := range lines {
		name, _, _ := strings.Cut(l, "\t")
		out = append(out, strings.TrimRight(name, " "))
	}
	return out
}

// Every repo starts folded to its own line.
func TestTreeViewStartsFolded(t *testing.T) {
	v := newTreeView(testGroups(), nil)
	got := names(v.lines(false))
	want := []string{"▸ myrepo", "▸ other", "▸ empty", "▸ broken"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("folded = %q, want %q", got, want)
	}
	statuses := map[string]string{}
	for _, l := range v.lines(false) {
		f := strings.Split(l, "\t")
		statuses[strings.TrimSpace(f[0])] = f[1]
	}
	for name, want := range map[string]string{
		"▸ myrepo": "2 worktrees · on screen",
		"▸ other":  "1 worktree",
		"▸ empty":  "no worktrees — a: add one",
		"▸ broken": "unreadable: boom",
	} {
		if statuses[name] != want {
			t.Errorf("%s status = %q, want %q", name, statuses[name], want)
		}
	}
}

// An open repo shows its worktrees under it, each line carrying its row index,
// and the status column doesn't move between folded and open.
func TestTreeViewOpen(t *testing.T) {
	v := newTreeView(testGroups(), map[string]bool{"myrepo": true})
	lines := v.lines(false)
	want := []string{"▾ myrepo", "  ├── main", "  └── codex/long-name", "▸ other", "▸ empty", "▸ broken"}
	if got := names(lines); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("open = %q, want %q", got, want)
	}
	if i, ok := lineIndex(lines[3], len(v.Rows)); !ok || i != 3 || v.Rows[i].Repo != "other" {
		t.Errorf("line 3 index = %d, want row 3, other's line", i)
	}
	width := -1
	for _, l := range append(lines, v.lines(true)...) {
		name, _, _ := strings.Cut(l, "\t")
		if n := len([]rune(name)); width != -1 && n != width {
			t.Errorf("%q is padded to %d, want %d like the rest", name, n, width)
		} else {
			width = n
		}
	}
	if all := v.lines(true); len(all) != len(v.Rows) {
		t.Errorf("lines(true) = %d lines, want all %d", len(all), len(v.Rows))
	}
}

func TestTreeViewAct(t *testing.T) {
	// Rows: 0 myrepo, 1 main, 2 codex/long-name, 3 other, 4 main, 5 empty, 6 broken.
	cases := []struct {
		name, verb, query string
		row               int
		open              []string
		wantActs          string // before any reload
		wantCursor        int    // row the cursor goes back to after the reload
		wantReload        bool
		wantOpen          []string
	}{
		{"l opens a repo", "open", "", 0, nil, "", 0, true, []string{"myrepo"}},
		{"l on an open repo steps in", "open", "", 0, []string{"myrepo"}, "down", -1, false, []string{"myrepo"}},
		{"l on a worktree switches", "open", "", 1, []string{"myrepo"}, "print(switch)+accept", -1, false, []string{"myrepo"}},
		{"h closes a repo", "close", "", 3, []string{"other"}, "", 3, true, nil},
		{"h on a closed repo does nothing", "close", "", 3, nil, "", -1, false, nil},
		{"h on a worktree folds its repo onto its line", "close", "", 4, []string{"myrepo", "other"}, "", 3, true, []string{"myrepo"}},
		{"enter toggles a repo open", "enter", "", 3, nil, "", 3, true, []string{"other"}},
		{"enter toggles a repo closed", "enter", "", 3, []string{"other"}, "", 3, true, nil},
		{"enter on a worktree switches", "enter", "", 4, []string{"other"}, "print(switch)+accept", -1, false, []string{"other"}},
		{"enter with a query switches", "enter", "ma", 0, nil, "print(switch)+accept", -1, false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			open := map[string]bool{}
			for _, r := range c.open {
				open[r] = true
			}
			v := newTreeView(testGroups(), open)
			acts, cursor, reload := v.act(c.verb, c.row, c.query)
			if got := strings.Join(acts, "+"); got != c.wantActs || reload != c.wantReload || cursor != c.wantCursor {
				t.Errorf("act = %q, cursor %d, reload %v; want %q, %d, %v",
					got, cursor, reload, c.wantActs, c.wantCursor, c.wantReload)
			}
			var got []string
			for _, r := range []string{"myrepo", "other", "empty", "broken"} {
				if v.Open[r] {
					got = append(got, r)
				}
			}
			if strings.Join(got, ",") != strings.Join(c.wantOpen, ",") {
				t.Errorf("open = %v, want %v", got, c.wantOpen)
			}
		})
	}
}

// A query shows everything, folds or not; clearing it folds the tree back,
// unfolding whichever repo the cursor ended up in.
func TestTreeViewQuery(t *testing.T) {
	v := newTreeView(testGroups(), nil)
	if _, _, reload := v.act("query", 0, "ma"); !reload || !v.All {
		t.Fatalf("a query should unfold everything")
	}
	if _, _, reload := v.act("query", 0, "mai"); reload {
		t.Errorf("a second keystroke reloaded again")
	}
	if acts, _, _ := v.act("open", 3, "mai"); len(acts) != 0 {
		t.Errorf("l while filtering = %q, want nothing", acts)
	}
	if _, cursor, reload := v.act("query", 4, ""); !reload || v.All || cursor != 4 {
		t.Fatalf("clearing the query should fold the tree back, cursor kept on row 4")
	}
	if !v.Open["other"] || v.Open["myrepo"] {
		t.Errorf("open = %v, want just other, where the cursor is", v.Open)
	}
}

func TestStartRow(t *testing.T) {
	v := newTreeView(testGroups(), map[string]bool{"other": true})
	// Lines: myrepo, other, main (row 4), empty, broken.
	for _, c := range []struct{ at, want int }{
		{-1, 1}, // fresh: the top line, the repo you're in
		{3, 2},  // back on other's line
		{4, 3},  // back on other's main
		{2, 1},  // inside folded myrepo: its line
		{99, 5}, // past the end, after a delete: the last line
	} {
		if got := startRow(v, c.at); got != c.want {
			t.Errorf("startRow(%d) = %d, want %d", c.at, got, c.want)
		}
	}
}

// Enter on a repo's own line, with a query in, goes to the worktree at the top
// of it.
func TestSwitchTarget(t *testing.T) {
	groups := []Group{
		{Repo: "a", Entries: []Entry{treeEntry("a", "main", true, true), treeEntry("a", "x", false, false)}},
		{Repo: "b"},
	}
	if e := switchTarget(groups, &row{repo: "a"}); e == nil || e.Branch != "main" {
		t.Errorf("switchTarget(a) = %v, want a's main", e)
	}
	if e := switchTarget(groups, &row{repo: "b"}); e != nil {
		t.Errorf("switchTarget(b) = %v, want nothing: b has no worktrees", e)
	}
	x := &groups[0].Entries[1]
	if e := switchTarget(groups, &row{repo: "a", entry: x}); e != x {
		t.Errorf("switchTarget on a worktree = %v, want that worktree", e)
	}
}

// Enter on a query is decided by re-running it through fzf, not by trusting
// where the cursor was when the keys arrived.
func TestSettleEnter(t *testing.T) {
	if _, err := exec.LookPath("fzf"); err != nil {
		t.Skip("no fzf")
	}
	groups := []Group{{Repo: "myrepo", Entries: []Entry{
		treeEntry("myrepo", "main", true, true),
		treeEntry("myrepo", "feat-a", true, false),
	}}}
	lines, rows := newTreeView(groups, nil).lines(true), treeRows(groups)
	sw := func(typed string, idx int) choice {
		return choice{action: actSwitch, typed: typed, row: &rows[idx], idx: idx}
	}

	// Nothing matches: a name to make, wherever the cursor was.
	if ch := settleEnter(sw("newone", 0), lines, rows); ch.action != actCreate || ch.typed != "newone" {
		t.Errorf("no match = %+v, want create newone", ch)
	}
	// The cursor lagging on a line that doesn't match moves to the first that does.
	if ch := settleEnter(sw("fa", 1), lines, rows); ch.action != actSwitch || ch.idx != 2 {
		t.Errorf("stale cursor = %+v, want switch on row 2 (feat-a)", ch)
	}
	// A cursor already on a match stays there.
	if ch := settleEnter(sw("a", 1), lines, rows); ch.idx != 1 {
		t.Errorf("cursor on a match = %+v, want it left on row 1", ch)
	}
	// The status column isn't searched: "parked" names no worktree.
	if ch := settleEnter(sw("parked", 0), lines, rows); ch.action != actCreate {
		t.Errorf("status query = %+v, want create", ch)
	}
}
