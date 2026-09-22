package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// socket, when set, scopes every command to a private tmux server. Only tests
// set it, so they can't disturb the server the user is actually working in.
var socket string

func args(rest ...string) []string {
	if socket == "" {
		return rest
	}
	return append([]string{"-L", socket}, rest...)
}

func run(rest ...string) (string, error) {
	a := args(rest...)
	cmd := exec.Command("tmux", a...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("tmux %s: %s", strings.Join(a, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// Exact targets, everywhere. tmux resolves a bare "-t awt" by prefix too, so
// with a park session called "awt-park" sitting next to the front session "awt",
// every unprefixed target is a coin flip between the two: "=" makes tmux take
// the name literally.
func sessionTarget(name string) string { return "=" + name }

// sessionWindowTarget is the same thing where tmux wants a window or pane (as
// set-option and new-window do): the trailing ":" means "that session's current
// window", which is enough to name the session unambiguously.
func sessionWindowTarget(name string) string { return "=" + name + ":" }

func target(session string, idx int) string {
	return "=" + session + ":" + strconv.Itoa(idx)
}

func HasSession(name string) bool {
	_, err := run("has-session", "-t", sessionTarget(name))
	return err == nil
}

// NewSession creates a detached session; winName/cmd may be empty to fall
// back to tmux's own defaults.
func NewSession(name, cwd, winName, cmd string) error {
	a := []string{"new-session", "-d", "-s", name, "-c", cwd}
	if winName != "" {
		a = append(a, "-n", winName)
	}
	if cmd != "" {
		a = append(a, cmd)
	}
	_, err := run(a...)
	return err
}

func KillSession(name string) error {
	_, err := run("kill-session", "-t", sessionTarget(name))
	return err
}

// SetOption sets a session option on the named session.
func SetOption(session, name, value string) error {
	_, err := run("set-option", "-t", sessionWindowTarget(session), name, value)
	return err
}

// SetWindowOption sets a window option (target is a window id).
func SetWindowOption(target, name, value string) error {
	_, err := run("set-option", "-w", "-t", target, name, value)
	return err
}

// BaseIndex is the index tmux gives a session's first window, which awt has to
// respect when it picks an index of its own. Defaults to 0 as tmux does.
func BaseIndex() int {
	out, err := run("show-options", "-gv", "base-index")
	if err != nil {
		return 0
	}
	i, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0
	}
	return i
}

// Session is a live tmux session. Only `awt migrate` still needs these: v0.2
// keeps its state in windows, not sessions.
type Session struct {
	Name         string
	Attached     bool
	LastAttached time.Time
}

const sessionFormat = "#{session_name}\t#{session_attached}\t#{session_last_attached}"

func ListSessions() ([]Session, error) {
	out, err := run("list-sessions", "-F", sessionFormat)
	if err != nil {
		if strings.Contains(err.Error(), "no server running") {
			return nil, nil
		}
		return nil, err
	}
	var sessions []Session
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(l, "\t")
		if len(f) < 3 || f[0] == "" {
			continue
		}
		s := Session{Name: f[0], Attached: f[1] == "1"}
		if secs, err := strconv.ParseInt(f[2], 10, 64); err == nil && secs > 0 {
			s.LastAttached = time.Unix(secs, 0)
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// Window is one tmux window. ID ("@17") is stable for the window's whole life
// and survives every move between sessions, which is why awt tracks that rather
// than a session name or an index: indices shift under `renumber-windows`.
type Window struct {
	ID       string
	Session  string
	Name     string
	Index    int
	Panes    int
	Activity time.Time
}

const windowFormat = "#{window_id}\t#{session_name}\t#{window_index}\t#{window_name}\t#{window_panes}\t#{window_activity}"

// ListWindows reports every window on the server, in one call: a command needs
// the whole picture (which windows are live, and which session each sits in) and
// this way it costs one exec no matter how many repos are registered.
func ListWindows() ([]Window, error) {
	out, err := run("list-windows", "-a", "-F", windowFormat)
	if err != nil {
		if strings.Contains(err.Error(), "no server running") {
			return nil, nil
		}
		return nil, err
	}
	var windows []Window
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(l, "\t")
		if len(f) < 6 || f[0] == "" {
			continue
		}
		w := Window{ID: f[0], Session: f[1], Name: f[3]}
		w.Index, _ = strconv.Atoi(f[2])
		w.Panes, _ = strconv.Atoi(f[4])
		if secs, err := strconv.ParseInt(f[5], 10, 64); err == nil && secs > 0 {
			w.Activity = time.Unix(secs, 0)
		}
		windows = append(windows, w)
	}
	return windows, nil
}

// NewWindow creates a detached window at the end of session, returning its
// window id and the pane id of the command it started.
func NewWindow(session, name, cwd, cmd string) (windowID, paneID string, err error) {
	a := []string{"new-window", "-d", "-t", sessionWindowTarget(session), "-n", name, "-c", cwd,
		"-P", "-F", "#{window_id} #{pane_id}"}
	if cmd != "" {
		a = append(a, cmd)
	}
	out, err := run(a...)
	if err != nil {
		return "", "", err
	}
	f := strings.Fields(strings.TrimSpace(out))
	if len(f) < 2 {
		return "", "", fmt.Errorf("tmux new-window: unexpected output %q", out)
	}
	return f[0], f[1], nil
}

// Split describes a new pane. Target is the window or pane it splits.
type Split struct {
	Target     string
	CWD        string
	Cmd        string
	Horizontal bool   // side by side rather than stacked
	Before     bool   // the new pane goes left of (or above) the target, not right/below
	Detach     bool   // leave the focus where it is instead of moving to the new pane
	Size       string // a tmux -l value like "40%"; empty splits evenly
}

// SplitWindow makes the pane and returns its id.
func SplitWindow(s Split) (string, error) {
	a := []string{"split-window", "-t", s.Target, "-c", s.CWD, "-P", "-F", "#{pane_id}"}
	if s.Horizontal {
		a = append(a, "-h")
	} else {
		a = append(a, "-v")
	}
	if s.Before {
		a = append(a, "-b")
	}
	if s.Detach {
		a = append(a, "-d")
	}
	if s.Size != "" {
		a = append(a, "-l", s.Size)
	}
	if s.Cmd != "" {
		a = append(a, s.Cmd)
	}
	out, err := run(a...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Pane is one pane of a window. Title is what awt writes with SetPaneTitle, so
// it can tell the editor pane from the agent panes later.
type Pane struct {
	ID    string
	Title string
	Index int
}

const paneFormat = "#{pane_id}\t#{pane_title}\t#{pane_index}"

func ListPanes(target string) ([]Pane, error) {
	out, err := run("list-panes", "-t", target, "-F", paneFormat)
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(l, "\t")
		if len(f) < 3 || f[0] == "" {
			continue
		}
		p := Pane{ID: f[0], Title: f[1]}
		p.Index, _ = strconv.Atoi(f[2])
		panes = append(panes, p)
	}
	return panes, nil
}

func SetPaneTitle(pane, title string) error {
	_, err := run("select-pane", "-t", pane, "-T", title)
	return err
}

// MoveWindow moves a window into session at idx, which must be free.
func MoveWindow(id, session string, idx int) error {
	_, err := run("move-window", "-s", id, "-t", target(session, idx))
	return err
}

// SwapWindow exchanges the window at session:idx with the window id, wherever
// that one currently lives. This is how a worktree comes on screen: the session's
// current-window pointer is by index, so a client watching that index simply
// starts rendering the window swapped in — panes and processes untouched.
func SwapWindow(id, session string, idx int) error {
	_, err := run("swap-window", "-s", id, "-t", target(session, idx), "-d")
	return err
}

func SelectWindow(session string, idx int) error {
	_, err := run("select-window", "-t", target(session, idx))
	return err
}

func KillWindow(id string) error {
	_, err := run("kill-window", "-t", id)
	return err
}

func RenameWindow(id, name string) error {
	_, err := run("rename-window", "-t", id, name)
	return err
}

// CurrentSession reports the calling process's attached session; ok is false
// when not running inside a tmux client.
func CurrentSession() (name string, ok bool) {
	if os.Getenv("TMUX") == "" {
		return "", false
	}
	out, err := run("display-message", "-p", "#S")
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(out), true
}

// CurrentWindow reports the window id awt was invoked from, which is how it
// works out which worktree (and so which repo) you're currently in.
func CurrentWindow() (id string, ok bool) {
	if os.Getenv("TMUX") == "" {
		return "", false
	}
	out, err := run("display-message", "-p", "#{window_id}")
	if err != nil {
		return "", false
	}
	id = strings.TrimSpace(out)
	return id, id != ""
}

// SwitchOrAttach switches the current client if already inside tmux, else attaches.
func SwitchOrAttach(name string) error {
	sub := "attach-session"
	if os.Getenv("TMUX") != "" {
		sub = "switch-client"
	}
	cmd := exec.Command("tmux", args(sub, "-t", sessionTarget(name))...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
