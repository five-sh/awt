package command

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"awt/internal/state"
	"awt/internal/tmux"
)

const (
	editWindow    = "edit"
	defaultEditor = "nvim"
	agentPrefix   = "agent-"
	firstAgent    = agentPrefix + "1"
	claudeCmd     = "claude --dangerously-skip-permissions"
)

func editorCmd() string {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = defaultEditor
	}
	return editor + " ."
}

// createSessionLayout starts a tmux session with the default worktree layout:
// an "edit" window and a first agent window.
func createSessionLayout(session, path string) error {
	if err := tmux.NewSession(session, path, editWindow, editorCmd()); err != nil {
		return err
	}
	return tmux.NewWindow(session, firstAgent, path, claudeCmd, true) // detached: keep focus on "edit"
}

// AgentAdd spawns another agent window into an already-running worktree
// session, called from inside that session or from anywhere else.
func AgentAdd(repoName, worktreeName, name string) error {
	repo, err := resolveRepo(repoName)
	if err != nil {
		return err
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	wt, ok := st.Find(repo.Name, worktreeName)
	if !ok {
		return fmt.Errorf("no worktree named %q in %q", worktreeName, repo.Name)
	}
	if !tmux.HasSession(wt.Session) {
		return fmt.Errorf("session for %q isn't running — try 'awt switch %s %s' first", worktreeName, repo.Name, worktreeName)
	}

	if name == "" {
		windows, err := tmux.ListWindows(wt.Session)
		if err != nil {
			return err
		}
		name = nextAgentWindow(windows)
	}

	detach := true
	if cur, ok := tmux.CurrentSession(); ok && cur == wt.Session {
		detach = false
	}
	return tmux.NewWindow(wt.Session, name, wt.Path, claudeCmd, detach)
}

// nextAgentWindow returns "agent-N" one past the highest N found in existing.
func nextAgentWindow(existing []string) string {
	max := 0
	for _, w := range existing {
		n, ok := strings.CutPrefix(w, agentPrefix)
		if !ok {
			continue
		}
		if i, err := strconv.Atoi(n); err == nil && i > max {
			max = i
		}
	}
	return fmt.Sprintf("%s%d", agentPrefix, max+1)
}
