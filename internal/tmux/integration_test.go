//go:build tmux

// Integration tests for the tmux behaviours awt's layout depends on. They need a
// real tmux, so they're behind a build tag: `go test -tags tmux ./internal/tmux`.
// Everything runs on a private socket started with an empty config, so neither
// the user's server nor their .tmux.conf is involved.
package tmux

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"
)

// startServer brings up a private tmux server holding a front and a park
// session, and points the package at it for the rest of the test.
func startServer(t *testing.T) (front, park string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	sock := fmt.Sprintf("awt-test-%d", time.Now().UnixNano())
	front, park = "awt", "awt-park"
	// -f /dev/null only takes effect on the command that starts the server, so
	// this one goes through exec directly rather than the package's own run().
	for _, name := range []string{front, park} {
		cmd := exec.Command("tmux", "-L", sock, "-f", "/dev/null",
			"new-session", "-d", "-s", name, "-x", "200", "-y", "50", "-n", "keeper")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("starting %s: %v: %s", name, err, out)
		}
	}
	socket = sock
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", sock, "kill-server").Run()
		socket = ""
	})
	if err := SetOption(park, "window-size", "manual"); err != nil {
		t.Fatalf("window-size manual: %v", err)
	}
	return front, park
}

func windowByID(t *testing.T, id string) (Window, bool) {
	t.Helper()
	windows, err := ListWindows()
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}
	for _, w := range windows {
		if w.ID == id {
			return w, true
		}
	}
	return Window{}, false
}

// worktreeWindow builds the two-pane layout awt gives a worktree.
func worktreeWindow(t *testing.T, park, name string) string {
	t.Helper()
	id, pane, err := NewWindow(park, name, "/tmp", "")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	if err := SetPaneTitle(pane, "edit"); err != nil {
		t.Fatalf("SetPaneTitle: %v", err)
	}
	agent, err := SplitWindow(Split{
		Target: id, CWD: "/tmp", Horizontal: true, Before: true, Detach: true, Size: "40%",
	})
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}
	if err := SetPaneTitle(agent, "agent-1"); err != nil {
		t.Fatalf("SetPaneTitle: %v", err)
	}
	return id
}

// paneLayout reports each pane of a window left to right, so a test can assert
// which side the agent column ended up on.
func paneLayout(t *testing.T, window string) string {
	t.Helper()
	out, err := run("list-panes", "-t", window, "-F", "#{pane_left} #{pane_title}")
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	var cells []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 {
			cells = append(cells, f[0]+":"+f[1])
		}
	}
	sort.Strings(cells)
	return strings.Join(cells, " ")
}

// The agent column sits to the left of the editor: the agent pane starts at
// column 0 and the editor starts further right.
func TestAgentColumnIsOnTheLeft(t *testing.T) {
	_, park := startServer(t)
	id := worktreeWindow(t, park, "repo:sides")
	got := paneLayout(t, id)
	if !strings.HasPrefix(got, "0:agent-1") {
		t.Errorf("panes left to right = %q, want the agent pane at column 0", got)
	}
	if !strings.Contains(got, ":edit") || strings.HasPrefix(got, "0:edit") {
		t.Errorf("panes left to right = %q, want the editor to the right of it", got)
	}
}

// The whole design rests on this: a worktree comes on screen by swapping into
// its repo's index, and comes back with its id, panes and processes intact.
func TestSwapPreservesWindows(t *testing.T) {
	front, park := startServer(t)
	a := worktreeWindow(t, park, "repo:wt-a")
	b := worktreeWindow(t, park, "repo:wt-b")

	if err := MoveWindow(a, front, 5); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	got, ok := windowByID(t, a)
	if !ok || got.Session != front || got.Index != 5 || got.Panes != 2 {
		t.Fatalf("after move, wt-a = %+v, want front:5 with 2 panes", got)
	}

	if err := SwapWindow(b, front, 5); err != nil {
		t.Fatalf("SwapWindow: %v", err)
	}
	if got, ok := windowByID(t, b); !ok || got.Session != front || got.Index != 5 || got.Panes != 2 {
		t.Errorf("after swap, wt-b = %+v, want front:5 with 2 panes", got)
	}
	if got, ok := windowByID(t, a); !ok || got.Session != park || got.Panes != 2 {
		t.Errorf("after swap, wt-a = %+v, want parked with 2 panes", got)
	}
	// Pane titles are how awt tells the editor from the agents; they have to
	// survive the round trip too. tmux indexes panes by position, so the agent
	// column on the left comes first.
	panes, err := ListPanes(a)
	if err != nil {
		t.Fatalf("ListPanes: %v", err)
	}
	var titles []string
	for _, p := range panes {
		titles = append(titles, p.Title)
	}
	if strings.Join(titles, ",") != "agent-1,edit" {
		t.Errorf("pane titles = %v, want [agent-1 edit]", titles)
	}
}

// tmux destroys a session the moment its last window leaves, and parking is
// exactly that — hence the keeper window awt puts in the park session.
func TestKeeperKeepsParkAlive(t *testing.T) {
	front, park := startServer(t)
	id := worktreeWindow(t, park, "repo:only")
	if err := MoveWindow(id, front, 1); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	if !HasSession(park) {
		t.Fatal("park session died with its last worktree window: keeper isn't holding it")
	}

	// And without a keeper it really does die, which is what the keeper is for.
	if err := NewSession("bare", "/tmp", "solo", ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	windows, err := ListWindows()
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}
	solo := ""
	for _, w := range windows {
		if w.Session == "bare" {
			solo = w.ID
		}
	}
	if solo == "" {
		t.Fatal("no window in the bare session")
	}
	if err := MoveWindow(solo, front, 2); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	if HasSession("bare") {
		t.Error("a session with no keeper survived losing its last window")
	}
}

// A window killed outside awt has to read as simply not live, so the next switch
// rebuilds it rather than failing.
func TestKillWindowReconciles(t *testing.T) {
	_, park := startServer(t)
	id := worktreeWindow(t, park, "repo:doomed")
	if _, ok := windowByID(t, id); !ok {
		t.Fatalf("window %s missing right after creation", id)
	}
	if err := KillWindow(id); err != nil {
		t.Fatalf("KillWindow: %v", err)
	}
	if _, ok := windowByID(t, id); ok {
		t.Errorf("window %s still listed after being killed", id)
	}
}

func TestListWindowsFields(t *testing.T) {
	_, park := startServer(t)
	id := worktreeWindow(t, park, "repo:fields")
	w, ok := windowByID(t, id)
	if !ok {
		t.Fatal("window not listed")
	}
	if w.Session != park || w.Name != "repo:fields" || w.Panes != 2 {
		t.Errorf("window = %+v, want park/repo:fields with 2 panes", w)
	}
	if w.Activity.IsZero() {
		t.Error("window activity not parsed")
	}
	if err := RenameWindow(id, "repo:renamed"); err != nil {
		t.Fatalf("RenameWindow: %v", err)
	}
	if w, _ := windowByID(t, id); w.Name != "repo:renamed" {
		t.Errorf("name after rename = %q, want repo:renamed", w.Name)
	}
}

// Moving onto an occupied index fails, which is why bringing a worktree on
// screen over a sibling has to be a swap rather than a move.
func TestMoveOntoOccupiedIndexFails(t *testing.T) {
	front, park := startServer(t)
	a := worktreeWindow(t, park, "repo:a")
	b := worktreeWindow(t, park, "repo:b")
	if err := MoveWindow(a, front, 3); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	if err := MoveWindow(b, front, 3); err == nil {
		t.Error("moving onto an occupied index succeeded, expected an error")
	}
	if err := SwapWindow(b, front, 3); err != nil {
		t.Errorf("SwapWindow onto an occupied index: %v", err)
	}
}

// The trap this whole package guards against: tmux resolves "-t awt" by prefix
// as well as exactly, so with a park session named "awt-park" beside a front
// session named "awt", a bare target hits whichever it feels like. Every target
// here is exact, so an absent front session reads as absent even while the park
// session sits next to it.
func TestSessionTargetsAreExact(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	sock := fmt.Sprintf("awt-test-%d", time.Now().UnixNano())
	cmd := exec.Command("tmux", "-L", sock, "-f", "/dev/null",
		"new-session", "-d", "-s", "awt-park", "-n", "keeper")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("starting park: %v: %s", err, out)
	}
	socket = sock
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", sock, "kill-server").Run()
		socket = ""
	})

	if HasSession("awt") {
		t.Error(`HasSession("awt") matched "awt-park" — targets aren't exact`)
	}
	if !HasSession("awt-park") {
		t.Error(`HasSession("awt-park") missed the session that is there`)
	}
	// And a window created in the park session has to land there, not in a
	// session whose name merely starts the same way.
	if err := NewSession("awt", "/tmp", "scratch", ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	id, _, err := NewWindow("awt-park", "repo:wt", "/tmp", "")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	if w, ok := windowByID(t, id); !ok || w.Session != "awt-park" {
		t.Errorf("new window landed in %q, want awt-park", w.Session)
	}
	// Same for a session option: set it on park, and front must not have it.
	if err := SetOption("awt-park", "window-size", "manual"); err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	out, err := run("show-options", "-t", sessionWindowTarget("awt"), "window-size")
	if err != nil {
		t.Fatalf("show-options: %v", err)
	}
	if strings.Contains(out, "manual") {
		t.Errorf("window-size leaked onto the front session: %q", out)
	}
}
