package command

import (
	"fmt"

	"awt/internal/tmux"
)

// AgentAdd spawns another agent pane in a worktree's window. A worktree is one
// window now, so agents are panes in it: the first lands beside the editor and
// the rest stack under that one. The window is addressed by id, so this works on
// a parked worktree as much as the one on screen.
func AgentAdd(repoName, worktreeName, name string) error {
	repo, err := resolveRepo(repoName)
	if err != nil {
		return err
	}
	groups, err := groupOne(repo)
	if err != nil {
		return err
	}
	var target *Entry
	for _, e := range flatten(groups) {
		if e.Matches(worktreeName) {
			target = &e
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no worktree named %q in %q", worktreeName, repo.Name)
	}
	if !target.Alive {
		return fmt.Errorf("no window for %q yet — try 'awt switch %s %s' first",
			worktreeName, repo.Name, target.Name)
	}

	panes, err := tmux.ListPanes(target.Window)
	if err != nil {
		return err
	}
	if name == "" {
		name = nextAgentPane(panes)
	}
	// Focus the new pane only when you're looking at the window it appears in.
	detach := true
	if id, ok := tmux.CurrentWindow(); ok && id == target.Window {
		detach = false
	}
	split := agentSplit(target.Window, panes)
	split.CWD, split.Cmd, split.Detach = target.Path, claudeCmd, detach
	pane, err := tmux.SplitWindow(split)
	if err != nil {
		return err
	}
	return tmux.SetPaneTitle(pane, name)
}
