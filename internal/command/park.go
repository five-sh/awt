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
	if repoName != "" {
		repo, err := resolveRepo(repoName)
		if err != nil {
			return err
		}
		name, err := parkRepo(repo.Name)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "awt: parked %s\n", name)
		return nil
	}

	park, err := ensurePark()
	if err != nil {
		return err
	}
	windows, err := tmux.ListWindows()
	if err != nil {
		return err
	}
	win, err := currentFrontWindow(windows, frontSession())
	if err != nil {
		return err
	}
	if err := parkWindow(windows, park, win); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "awt: parked %s\n", win.Name)
	return nil
}

// parkRepo parks whichever of repo's worktrees is on screen. It goes by repo,
// not worktree: a repo has one slot in the front session, so there's only ever
// one window of it to park. It returns that window's name.
func parkRepo(repo string) (string, error) {
	park, err := ensurePark()
	if err != nil {
		return "", err
	}
	windows, err := tmux.ListWindows()
	if err != nil {
		return "", err
	}
	win, ok := repoFrontWindow(windows, frontSession(), repo)
	if !ok {
		return "", fmt.Errorf("%q has nothing on screen to park", repo)
	}
	return win.Name, parkWindow(windows, park, win)
}

// parkWindow moves win to the end of the park session. windows has to be listed
// after ensurePark, so the next free index accounts for the keeper window.
//
// The front session is renumbered after, so the repos left on screen close up
// rather than keep a hole where this one was: renumber-windows only acts when a
// window closes, and a window moving to another session isn't closing.
func parkWindow(windows []tmux.Window, park string, win tmux.Window) error {
	if err := tmux.MoveWindow(win.ID, park, nextIndex(windows, park, tmux.BaseIndex())); err != nil {
		return err
	}
	if front := win.Session; tmux.HasSession(front) { // gone if win was its last window
		return tmux.RenumberWindows(front)
	}
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
