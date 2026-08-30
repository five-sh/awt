package command

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"awt/internal/git"
	"awt/internal/state"
	"awt/internal/tmux"
)

// Switch resolves repo/worktree and attaches, creating a tmux session and, if
// needed, the worktree itself (an existing branch, or the repo's default branch)
// on the way. Empty repoName opens a picker across every registered repo; an empty
// worktreeName opens a picker scoped to repoName.
func Switch(repoName, worktreeName string) error {
	if repoName == "" {
		return switchGlobal()
	}
	repo, err := resolveRepo(repoName)
	if err != nil {
		return err
	}
	entries, err := listRepo(repo)
	if err != nil {
		return err
	}

	if worktreeName == "" {
		target, err := pick(entries, false)
		if err != nil {
			return err
		}
		if target == nil {
			if len(entries) == 0 {
				return fmt.Errorf("no worktrees for %q yet — try 'awt new %s <name>'", repo.Name, repo.Name)
			}
			return nil
		}
		return attach(*target)
	}

	for i := range entries {
		if entries[i].Name == worktreeName {
			return attach(entries[i])
		}
	}

	wt, err := materialize(repo, worktreeName)
	if err != nil {
		return err
	}
	return attach(Entry{Worktree: wt, Alive: true})
}

func switchGlobal() error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	if len(st.Repos) == 0 {
		return fmt.Errorf("no repos registered yet — try 'awt new <repo> <name>'")
	}
	var all []Entry
	for _, r := range st.Repos {
		entries, err := listRepo(&r)
		if err != nil {
			continue
		}
		all = append(all, entries...)
	}
	if len(all) == 0 {
		return fmt.Errorf("no worktrees yet — try 'awt new <repo> <name>'")
	}
	target, err := pick(all, true)
	if err != nil {
		return err
	}
	if target == nil {
		return nil
	}
	return attach(*target)
}

// materialize creates a worktree for a name that has none yet. The default branch
// always resets to the latest origin, even over a stale local copy (e.g. one left
// by a bare clone); any other name just checks out its existing local branch.
func materialize(repo *state.Repo, name string) (state.Worktree, error) {
	branch, err := git.DefaultBranch(repo.Path)
	if err != nil {
		return state.Worktree{}, err
	}
	if name == branch {
		_, base, warn, err := git.FreshBase(repo.Path)
		if err != nil {
			return state.Worktree{}, err
		}
		if warn != "" {
			fmt.Fprintln(os.Stderr, "awt: "+warn)
		}
		return materializeDefault(repo, name, base)
	}
	if git.BranchExists(repo.Path, name) {
		return materializeExisting(repo, name)
	}
	return state.Worktree{}, fmt.Errorf("no branch or worktree named %q in %q", name, repo.Name)
}

func attach(e Entry) error {
	if !e.Alive {
		if err := createSessionLayout(e.Session, e.Path); err != nil {
			return err
		}
	}
	adopt(e.Worktree)
	return tmux.SwitchOrAttach(e.Session)
}

// adopt records a worktree in state (even one awt didn't create, like the main
// checkout) and bumps its LastAttached, so a discovered worktree becomes tracked.
func adopt(wt state.Worktree) {
	st, err := state.Load()
	if err != nil {
		return
	}
	wt.LastAttached = time.Now()
	st.Upsert(wt)
	_ = st.Save()
}

func pick(entries []Entry, showRepo bool) (*Entry, error) {
	if _, err := exec.LookPath("fzf"); err != nil {
		return pickPlain(entries, showRepo)
	}
	return pickFzf(entries, showRepo)
}

func pickKey(e Entry, showRepo bool) string {
	if showRepo {
		return e.Repo + "/" + e.Name
	}
	return e.Name
}

func pickFzf(entries []Entry, showRepo bool) (*Entry, error) {
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = fmt.Sprintf("%s\t%s\t%s", pickKey(e, showRepo), e.Branch, e.Parent)
	}
	cmd := exec.Command("fzf", "--with-nth=1,2,3", "--delimiter=\t")
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, nil // esc/cancel
	}
	chosen, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
	for i := range entries {
		if pickKey(entries[i], showRepo) == chosen {
			return &entries[i], nil
		}
	}
	return nil, nil
}

func pickPlain(entries []Entry, showRepo bool) (*Entry, error) {
	for i, e := range entries {
		fmt.Printf("%d) %s (%s)\n", i+1, pickKey(e, showRepo), e.Branch)
	}
	fmt.Print("select: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	var idx int
	if _, err := fmt.Sscanf(strings.TrimSpace(line), "%d", &idx); err != nil || idx < 1 || idx > len(entries) {
		return nil, fmt.Errorf("invalid selection")
	}
	return &entries[idx-1], nil
}
