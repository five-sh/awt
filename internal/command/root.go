package command

import (
	"os"
	"path/filepath"
)

const (
	workspaceRootEnv    = "AWT_WORKSPACE_ROOT"
	reposRootEnv        = "AWT_REPOS_ROOT"
	defaultWorkspaceDir = "workspaces"
	defaultReposDir     = "repos"
)

func workspaceRoot() (string, error) {
	return rootDir(workspaceRootEnv, defaultWorkspaceDir)
}

func reposRoot() (string, error) {
	return rootDir(reposRootEnv, defaultReposDir)
}

func rootDir(envVar, subdir string) (string, error) {
	if v := os.Getenv(envVar); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "awt", subdir), nil
}
