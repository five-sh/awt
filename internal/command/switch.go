package command

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"awt/internal/git"
	"awt/internal/state"
)

// Switch resolves repo/worktree and brings it on screen, creating its tmux
// window and, if needed, the worktree itself (an existing branch, or the repo's
// default branch) on the way. Empty repoName opens a picker across every
// registered repo; an empty worktreeName opens a picker scoped to repoName.
func Switch(repoName, worktreeName string) error {
	if repoName == "" {
		return switchGlobal()
	}
	repo, err := resolveRepo(repoName)
	if err != nil {
		return err
	}
	if worktreeName != "" {
		return switchTo(repo, worktreeName, false)
	}
	groups, err := groupOne(repo)
	if err != nil {
		return err
	}
	ch, err := pick(groups)
	if err != nil {
		return err
	}
	switch {
	case ch.entry != nil:
		return focus(*ch.entry)
	case ch.typed != "":
		// Scoped picker: labels are bare branches, so the whole query is one —
		// slashes included, as in "codex/foo".
		return switchTo(repo, ch.typed, true)
	case len(flatten(groups)) == 0:
		return fmt.Errorf("no worktrees for %q yet — try 'awt new %s <name>'", repo.Name, repo.Name)
	}
	return nil
}

func switchGlobal() error {
	groups, err := groupAll()
	if err != nil {
		return err
	}
	ch, err := pick(groups)
	if err != nil {
		return err
	}
	if ch.entry != nil {
		return focus(*ch.entry)
	}
	if ch.typed != "" {
		repo, name, err := resolveTyped(groups, ch.typed)
		if err != nil {
			return err
		}
		return switchTo(repo, name, true)
	}
	// Nothing picked and nothing typed: a cancel, unless there was nothing to
	// pick from in the first place, which is worth saying out loud.
	switch {
	case len(groups) == 0:
		return fmt.Errorf("no repos registered yet — try 'awt new <repo> <name>'")
	case len(flatten(groups)) == 0:
		return fmt.Errorf("no worktrees yet — try 'awt new <repo> <name>'")
	}
	return nil
}

// switchTo attaches to repo's worktree named name, materializing it first if it
// doesn't exist yet. create widens that to a name nothing at all answers to —
// no worktree, no branch anywhere — which becomes a brand-new branch off the
// same base `awt new` would pick. It's how the picker turns a name you typed
// into a window on screen.
func switchTo(repo *state.Repo, name string, create bool) error {
	groups, err := groupOne(repo)
	if err != nil {
		return err
	}
	entries := flatten(groups)
	for i := range entries {
		if entries[i].Matches(name) {
			return focus(entries[i])
		}
	}
	wt, err := materialize(repo, name)
	if create && errors.Is(err, errNoSuchName) {
		wt, err = newWorktree(repo, name, "", false)
	}
	if err != nil {
		return err
	}
	return focus(Entry{Worktree: wt})
}

// resolveTyped turns a name typed into the global picker into a repo and a name
// within it. Rows there read "<repo>/<branch>", so the first segment names the
// repo and the rest is the branch — itself possibly containing "/", as in
// "codex/foo". A name with no "/" belongs to whichever repo pickerRepo settles
// on, and is ambiguous when that's none. An unregistered repo name gets
// registered (and cloned) on the spot, same as `awt <repo> <name>` does.
func resolveTyped(groups []Group, typed string) (*state.Repo, string, error) {
	repoName, name, ok := strings.Cut(typed, "/")
	if !ok {
		repoName, name = pickerRepo(groups), typed
		if repoName == "" {
			return nil, "", fmt.Errorf("type '<repo>/<branch>' to make a worktree for %q", typed)
		}
	}
	if repoName == "" || name == "" {
		return nil, "", fmt.Errorf("invalid name %q, want '<repo>/<branch>'", typed)
	}
	if repoName == untrackedRepo {
		return nil, "", fmt.Errorf("%s isn't a repo — its sessions have no worktree awt can track", untrackedRepo)
	}
	repo, err := resolveRepo(repoName)
	if err != nil {
		return nil, "", err
	}
	return repo, name, nil
}

// pickerRepo names the repo a bare typed name belongs to: the repo the picker was
// opened from, or else the only one it had to offer — whose rows are bare
// branches anyway, so a bare name is exactly what you'd type at it.
func pickerRepo(groups []Group) string {
	if current := currentGroup(groups); current != "" {
		return current
	}
	sole := ""
	for _, g := range groups {
		if g.Repo == untrackedRepo {
			continue
		}
		if sole != "" {
			return "" // more than one on offer: a bare name doesn't say which
		}
		sole = g.Repo
	}
	return sole
}

// errNoSuchName reports that nothing by that name exists to check out: no local
// branch, nothing on origin, and not the default branch either. The picker turns
// that into a new branch; an explicit `awt switch` reports it as the error it is.
var errNoSuchName = errors.New("no such branch or worktree")

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
	if ref, warn, err := git.RemoteBranch(repo.Path, name); err == nil {
		if warn != "" {
			fmt.Fprintln(os.Stderr, "awt: "+warn)
		}
		return materializeRemote(repo, name, ref)
	}
	return state.Worktree{}, fmt.Errorf("%w named %q in %q — try 'awt new %s %s'",
		errNoSuchName, name, repo.Name, repo.Name, name)
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

// choice is what the picker came back with: an existing entry to switch to, or a
// name that was typed rather than picked — one nothing matched, or one forced
// with createKey — meaning "make me this one". Both empty is a cancel.
type choice struct {
	entry *Entry
	typed string
}

const (
	// createKey forces the typed query through as a new worktree even when rows
	// still match it, for a name that's a substring of one you already have.
	createKey = "alt-enter"
	// noMatchExit is fzf's exit code for "nothing matched the query" — not a
	// failure here, but the cue that the query is a name to create.
	noMatchExit = 1
)

// pick lists every group's entries in one picker, in group order. More than one
// group means the repo goes in the label, since names repeat across repos.
func pick(groups []Group) (choice, error) {
	entries := flatten(groups)
	showRepo := len(groups) > 1
	if _, err := exec.LookPath("fzf"); err != nil {
		return pickPlain(entries, showRepo)
	}
	return pickFzf(entries, showRepo, seedQuery(groups))
}

// seedQuery pre-filters the picker to the repo it was opened from: most switches
// stay inside one repo, and ctrl-u in fzf widens it back out again.
func seedQuery(groups []Group) string {
	if len(groups) < 2 {
		return ""
	}
	if current := currentGroup(groups); current != "" {
		return current + "/"
	}
	return ""
}

func currentGroup(groups []Group) string {
	for _, g := range groups {
		if g.Current {
			return g.Repo
		}
	}
	return ""
}

// pickHeader spells out that the query is more than a filter, since a picker
// that creates things on enter isn't guessable from the rows.
func pickHeader(showRepo bool) string {
	name := "<branch>"
	if showRepo {
		name = "<repo>/<branch>"
	}
	nav := "enter: switch · j/k · g/G · ctrl-d/ctrl-u · q: quit"
	if showRepo {
		nav += " · D: clear filter"
	}
	// Neither the modes nor the fact that a name you type gets made is guessable
	// from the rows, so both get spelled out.
	return nav + "\ni: type a name · " + createKey + ": make " + name + " · esc: back to normal"
}

// pickLabel is the only thing the picker shows: repo/branch, or just the branch
// when everything on offer is from one repo. Entries with no branch of their own
// (a detached checkout, a window awt didn't create) fall back to their name.
func pickLabel(e Entry, showRepo bool) string {
	label := e.Branch
	if label == "" {
		label = e.Name
	}
	if showRepo {
		return e.Repo + "/" + label
	}
	return label
}

func pickFzf(entries []Entry, showRepo bool, query string) (choice, error) {
	lines := make([]string, len(entries))
	for i, e := range entries {
		// The index rides along as a second field so the label stays the only
		// thing fzf shows or matches on, and two identical labels can't collide.
		lines[i] = fmt.Sprintf("%s\t%d", pickLabel(e, showRepo), i)
	}
	// --tiebreak=index keeps the grouped order for equally good matches; without
	// it fzf re-sorts by score and the grouping dissolves on the first keystroke.
	// --print-query is what makes a name that matched nothing usable at all.
	// --prompt is set
	// explicitly because the vim mode reads the prompt to tell which mode it's
	// in, and a prompt from the user's own FZF_DEFAULT_OPTS would break that.
	args := append([]string{"--with-nth=1", "--delimiter=\t",
		"--tiebreak=index", "--layout=reverse", "--info=inline",
		"--print-query", "--expect=" + createKey,
		"--prompt=" + normalPrompt,
		"--header=" + pickHeader(showRepo), "--query=" + query,
	}, pickerBindings()...)
	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		// Exiting on "no match" is the interesting case, not a failure: with
		// --print-query the query still comes back, and it's the name to create.
		// Anything else (esc is 130) really is a cancel.
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != noMatchExit {
			return choice{}, nil
		}
	}
	// --print-query and --expect prepend two lines: the query, then the key that
	// ended it ("" for a plain enter). Selected rows, if any, follow.
	fields := strings.Split(string(out), "\n")
	typed, key, sel := strings.TrimSpace(fields[0]), "", ""
	if len(fields) > 1 {
		key = strings.TrimSpace(fields[1])
	}
	if len(fields) > 2 {
		sel = strings.TrimSpace(fields[2])
	}
	if sel == "" || key == createKey {
		return choice{typed: typed}, nil
	}
	_, idx, ok := strings.Cut(sel, "\t")
	if !ok {
		return choice{}, nil
	}
	i, err := strconv.Atoi(idx)
	if err != nil || i < 0 || i >= len(entries) {
		return choice{}, nil
	}
	return choice{entry: &entries[i]}, nil
}

// pickPlain is the fzf-less fallback: a number picks a row, anything else is
// taken as a name to create, mirroring what typing into fzf does.
func pickPlain(entries []Entry, showRepo bool) (choice, error) {
	for i, e := range entries {
		fmt.Printf("%d) %s\n", i+1, pickLabel(e, showRepo))
	}
	name := "<branch>"
	if showRepo {
		name = "<repo>/<branch>"
	}
	fmt.Printf("select (number, or %s to create): ", name)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return choice{}, nil
	}
	if idx, err := strconv.Atoi(line); err == nil {
		if idx < 1 || idx > len(entries) {
			return choice{}, fmt.Errorf("invalid selection")
		}
		return choice{entry: &entries[idx-1]}, nil
	}
	return choice{typed: line}, nil
}
