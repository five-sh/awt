package command

import (
	"fmt"
	"os"

	"github.com/five-sh/awt/internal/tmux"
)

// Park sends a repo's on-screen worktree back to the park session: the repo
// leaves the status bar with its editor and agents still running, ready to come
// back with the next switch. An empty repoName parks the repo you're in.
//
// Parking the only window the front session has left destroys that session, and
// so detaches the client — which is "clear my screen", and exactly what asking
// to park your last repo means. The next switch builds the session again.
func Park(repoName string) error {
	front := frontSession()
	park, err := ensurePark()
	if err != nil {
		return err
	}
	windows, err := tmux.ListWindows()
	if err != nil {
		return err
	}

	var win tmux.Window
	if repoName == "" {
		win, err = currentFrontWindow(windows, front)
		if err != nil {
			return err
		}
	} else {
		repo, err := resolveRepo(repoName)
		if err != nil {
			return err
		}
		var ok bool
		if win, ok = repoFrontWindow(windows, front, repo.Name); !ok {
			return fmt.Errorf("%q has nothing on screen to park", repo.Name)
		}
	}

	if err := tmux.MoveWindow(win.ID, park, nextIndex(windows, park, tmux.BaseIndex())); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "awt: parked %s\n", win.Name)
	return nil
}

// currentFrontWindow is the window awt was called from, provided that's a window
// in the front session — the only kind parking means anything for.
func currentFrontWindow(windows []tmux.Window, front string) (tmux.Window, error) {
	id, ok := tmux.CurrentWindow()
	if !ok {
		return tmux.Window{}, fmt.Errorf("not inside tmux — try 'awt park <repo>'")
	}
	for _, w := range windows {
		if w.ID == id && w.Session == front {
			return w, nil
		}
	}
	return tmux.Window{}, fmt.Errorf("this isn't a worktree window — try 'awt park <repo>'")
}
