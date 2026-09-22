package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	stateDirName  = ".awt"
	stateFileName = "state.json"
	dirPerm       = 0o755
	filePerm      = 0o644
)

type Worktree struct {
	Repo   string `json:"repo"`
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
	// Window is the tmux window id ("@17") the worktree's editor and agents live
	// in. It replaces v0.1's per-worktree session, and like that field it's a
	// cache: a window id tmux no longer knows just means the window needs
	// rebuilding.
	Window string `json:"window,omitempty"`
	// Session is v0.1's per-worktree tmux session, kept only so `awt migrate`
	// can find and kill those sessions. Nothing writes it any more.
	Session      string    `json:"session,omitempty"`
	Parent       string    `json:"parent,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	LastAttached time.Time `json:"lastAttached"`
}

// Repo is a registered repo: a name the CLI accepts as an argument, mapped to
// where it actually lives on disk (bare hub clone or a pre-existing checkout).
type Repo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	RemoteURL string `json:"remoteUrl,omitempty"`
	Bare      bool   `json:"bare"`
}

type Store struct {
	Worktrees []Worktree `json:"worktrees"`
	Repos     []Repo     `json:"repos"`
}

func filePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, stateDirName, stateFileName), nil
}

func Load() (*Store, error) {
	path, err := filePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Store{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *Store) Save() error {
	path, err := filePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, filePerm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Find looks up a worktree by its (possibly slugified) name or by the raw branch
// it checks out, so a caller can identify it either way.
func (s *Store) Find(repo, name string) (*Worktree, bool) {
	for i := range s.Worktrees {
		if s.Worktrees[i].Repo == repo && (s.Worktrees[i].Name == name || s.Worktrees[i].Branch == name) {
			return &s.Worktrees[i], true
		}
	}
	return nil, false
}

func (s *Store) Upsert(w Worktree) {
	for i := range s.Worktrees {
		if s.Worktrees[i].Repo == w.Repo && s.Worktrees[i].Name == w.Name {
			s.Worktrees[i] = w
			return
		}
	}
	s.Worktrees = append(s.Worktrees, w)
}

func (s *Store) Remove(repo, name string) {
	out := s.Worktrees[:0]
	for _, w := range s.Worktrees {
		if w.Repo != repo || w.Name != name {
			out = append(out, w)
		}
	}
	s.Worktrees = out
}

func (s *Store) ForRepo(repo string) []Worktree {
	var out []Worktree
	for _, w := range s.Worktrees {
		if w.Repo == repo {
			out = append(out, w)
		}
	}
	return out
}

func (s *Store) FindRepo(name string) (*Repo, bool) {
	for i := range s.Repos {
		if s.Repos[i].Name == name {
			return &s.Repos[i], true
		}
	}
	return nil, false
}

func (s *Store) UpsertRepo(r Repo) {
	for i := range s.Repos {
		if s.Repos[i].Name == r.Name {
			s.Repos[i] = r
			return
		}
	}
	s.Repos = append(s.Repos, r)
}
