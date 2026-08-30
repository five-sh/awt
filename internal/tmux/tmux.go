package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("tmux %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

func HasSession(name string) bool {
	_, err := run("has-session", "-t", name)
	return err == nil
}

// NewSession creates a detached session; winName/cmd may be empty to fall
// back to tmux's own defaults.
func NewSession(name, cwd, winName, cmd string) error {
	args := []string{"new-session", "-d", "-s", name, "-c", cwd}
	if winName != "" {
		args = append(args, "-n", winName)
	}
	if cmd != "" {
		args = append(args, cmd)
	}
	_, err := run(args...)
	return err
}

// NewWindow creates a window in an existing session. detach keeps the
// session's current window focused instead of switching to the new one.
func NewWindow(session, name, cwd, cmd string, detach bool) error {
	args := []string{"new-window", "-t", session, "-n", name, "-c", cwd}
	if detach {
		args = append(args, "-d")
	}
	args = append(args, cmd)
	_, err := run(args...)
	return err
}

func KillSession(name string) error {
	_, err := run("kill-session", "-t", name)
	return err
}

func ListSessions() ([]string, error) {
	out, err := run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		if strings.Contains(err.Error(), "no server running") {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			names = append(names, l)
		}
	}
	return names, nil
}

func ListWindows(session string) ([]string, error) {
	out, err := run("list-windows", "-t", session, "-F", "#{window_name}")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			names = append(names, l)
		}
	}
	return names, nil
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

// SwitchOrAttach switches the current client if already inside tmux, else attaches.
func SwitchOrAttach(name string) error {
	sub := "attach-session"
	if os.Getenv("TMUX") != "" {
		sub = "switch-client"
	}
	cmd := exec.Command("tmux", sub, "-t", name)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
