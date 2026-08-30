package command

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"awt/internal/git"
	"awt/internal/state"
)

func resolveRepo(name string) (*state.Repo, error) {
	st, err := state.Load()
	if err != nil {
		return nil, err
	}
	if r, ok := st.FindRepo(name); ok {
		return r, nil
	}
	return registerRepo(st, name)
}

// registerRepo handles an unknown repo name: offer to register the repo cwd is
// already in under that name, otherwise ask for a URL and clone it as a bare hub.
func registerRepo(st *state.Store, name string) (*state.Repo, error) {
	fmt.Fprintf(os.Stderr, "awt: no repo named %q is registered.\n", name)

	if cwd, err := os.Getwd(); err == nil {
		if root, err := git.RepoRoot(cwd); err == nil {
			if confirm(fmt.Sprintf("Register the repo at %s as %q?", root, name)) {
				return saveRepo(st, state.Repo{Name: name, Path: root, Bare: git.IsBare(root)})
			}
		}
	}

	url := prompt("Git URL to clone for " + name + ": ")
	if url == "" {
		return nil, fmt.Errorf("no URL given, aborting")
	}
	root, err := reposRoot()
	if err != nil {
		return nil, err
	}
	dest := filepath.Join(root, name)
	fmt.Fprintf(os.Stderr, "awt: cloning %s into %s ...\n", url, dest)
	if err := git.Clone(url, dest); err != nil {
		return nil, err
	}
	return saveRepo(st, state.Repo{Name: name, Path: dest, RemoteURL: url, Bare: true})
}

func saveRepo(st *state.Store, r state.Repo) (*state.Repo, error) {
	st.UpsertRepo(r)
	if err := st.Save(); err != nil {
		return nil, err
	}
	return &r, nil
}

func confirm(msg string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", msg)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func prompt(msg string) string {
	fmt.Fprint(os.Stderr, msg)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}
