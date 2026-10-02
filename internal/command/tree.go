package command

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"awt/internal/git"
	"awt/internal/state"
	"awt/internal/tmux"
)

// The tree: every repo with its worktrees hanging off it, folded the way
// nvim-tree folds directories (see treeview.go). Enter switches, `a` adds a
// worktree, `d` deletes one. Adding and deleting happen outside fzf — it exits, awt asks
// what it needs to on the terminal, and the tree comes back with the result —
// so there's one place that knows how to make or remove a worktree, not a
// second copy in shell inside a --bind.

// row is one line of the tree: a repo, or one of its worktrees.
type row struct {
	repo  string
	entry *Entry // nil for the repo's own line
}

// action is what the tree came back asking for. Each name is what the binding
// prints, so the action is read straight off fzf's output.
type action string

const (
	actNone   action = ""
	actSwitch action = "switch"
	actCreate action = "create" // the query is a name to make, not a filter
	actAdd    action = "add"
	actDelete action = "delete"
)

// choice is what the tree came back with: the action, the row the cursor was on
// (and its index, to put the cursor back on), and the query. Nothing at all is a
// cancel.
type choice struct {
	action action
	row    *row
	idx    int
	typed  string
}

const (
	// createKey forces the typed query through as a new worktree even when rows
	// still match it, for a name that's a substring of one you already have.
	createKey = "alt-enter"
	// fzfMinVersion is the first fzf with --raw, which keeps the tree whole
	// while it filters.
	fzfMinVersion = "0.66"
)

// browse is the tree's loop. Switching ends it; adding and deleting come back
// to it, with what happened in the header, so you can tidy up several worktrees
// in one go. scope limits it to one repo; nil shows them all.
func browse(scope *state.Repo) error {
	dir, err := os.MkdirTemp("", "awt-tree-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	view := filepath.Join(dir, "view.json")

	note, at, open := "", -1, map[string]bool{}
	for {
		groups, err := loadTree(scope)
		if err != nil {
			return err
		}
		if len(groups) == 0 {
			return fmt.Errorf("no repos registered yet — try 'awt new <repo> <name>'")
		}
		ch, err := pick(groups, view, open, note, at)
		if err != nil {
			return err
		}
		note, at = "", ch.idx
		// Keep the folds as l and h left them, for the tree's next round.
		if v, err := loadTreeView(view); err == nil {
			open = v.Open
		}
		switch ch.action {
		case actNone:
			return nil
		case actSwitch:
			if e := switchTarget(groups, ch.row); e != nil {
				return focus(*e)
			}
			note = fmt.Sprintf("no worktrees in %s yet — a: add one", ch.row.repo)
		case actCreate:
			if ch.typed == "" {
				return nil
			}
			if err := createTyped(scope, groups, ch.typed); err != nil {
				note = err.Error()
				continue
			}
			return nil
		case actAdd:
			done, msg := addFromTree(ch.row)
			if done {
				return nil
			}
			note = msg
		case actDelete:
			note = deleteFromTree(ch.row)
		}
	}
}

func loadTree(scope *state.Repo) ([]Group, error) {
	if scope == nil {
		return groupAll()
	}
	return groupOne(scope)
}

// switchTarget is the worktree enter goes to: the row's own, or for a repo's
// line, the one at the top of it — on screen, else parked, else the most recent.
func switchTarget(groups []Group, r *row) *Entry {
	if r == nil {
		return nil
	}
	if r.entry != nil {
		return r.entry
	}
	for _, g := range groups {
		if g.Repo == r.repo && len(g.Entries) > 0 {
			return &g.Entries[0]
		}
	}
	return nil
}

// createTyped makes the worktree a typed name nothing answered to. In a tree
// scoped to one repo the whole query is a branch — slashes included, as in
// "codex/foo"; in the global one, resolveTyped works out the repo.
func createTyped(scope *state.Repo, groups []Group, typed string) error {
	repo, name := scope, typed
	if repo == nil {
		var err error
		if repo, name, err = resolveTyped(groups, typed); err != nil {
			return err
		}
	}
	return switchTo(repo, name, func() (state.Worktree, error) {
		return newWorktree(repo, name, "", false)
	})
}

// addFromTree asks for a name and makes a worktree for it in the row's repo,
// then switches to it. On a worktree's line the new branch forks off that
// worktree, unpushed commits and all, as `awt new --from` would; on the repo's
// line it forks off the default branch freshly fetched. A name that's already a
// worktree or a branch is switched to or checked out rather than made anew.
// done reports that it switched; otherwise note says why not, for the header.
func addFromTree(r *row) (done bool, note string) {
	if r == nil {
		return false, ""
	}
	if r.repo == untrackedRepo {
		return false, untrackedRepo + " isn't a repo — move onto one to add a worktree"
	}
	repo, err := resolveRepo(r.repo)
	if err != nil {
		return false, err.Error()
	}
	from, base := "", "the default branch"
	if r.entry != nil && r.entry.Branch != "" {
		from, base = r.entry.Branch, r.entry.Branch
	}
	clearScreen()
	name := prompt(fmt.Sprintf("new worktree in %s, off %s (empty to go back): ", repo.Name, base))
	if name == "" {
		return false, ""
	}
	err = switchTo(repo, name, func() (state.Worktree, error) {
		return newWorktree(repo, name, from, true)
	})
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}

// deleteFromTree removes the row's worktree with its branch and window, after
// asking. Uncommitted changes and an unmerged branch each get a question of
// their own rather than being thrown away by the first yes. A window no repo
// accounts for is just killed. The note says what happened, for the header.
func deleteFromTree(r *row) string {
	if r == nil {
		return ""
	}
	if r.entry == nil {
		return "d deletes a worktree — move onto one of " + r.repo + "'s"
	}
	e := *r.entry
	clearScreen()
	if e.Repo == untrackedRepo {
		if !e.Alive || !confirm(fmt.Sprintf("kill window %q?", e.Name)) {
			return ""
		}
		if err := tmux.KillWindow(e.Window); err != nil {
			return err.Error()
		}
		return "killed window " + e.Name
	}
	repo, err := resolveRepo(e.Repo)
	if err != nil {
		return err.Error()
	}
	label := e.Repo + "/" + treeLabel(e)
	what := "worktree and branch"
	if e.Alive {
		what = "window, worktree and branch"
	}
	if !confirm(fmt.Sprintf("delete %s — its %s?", label, what)) {
		return "kept " + label
	}
	dirty, _ := git.IsDirty(e.Path)
	if dirty && !confirm(label+" has uncommitted changes — delete them too?") {
		return "kept " + label + ": it has uncommitted changes"
	}
	kept := ""
	err = removeEntry(repo, e, dirty, false, func(branch string, err error) bool {
		if confirm(fmt.Sprintf("branch %q isn't merged — delete it anyway?", branch)) {
			return true
		}
		kept = fmt.Sprintf(" (kept its branch %q)", branch)
		return false
	})
	if err != nil {
		return err.Error()
	}
	return "deleted " + label + kept
}

// treeRows says what each of the tree's lines is, in treeView's order.
func treeRows(groups []Group) []row {
	var rows []row
	for _, g := range groups {
		rows = append(rows, row{repo: g.Repo})
		for i := range g.Entries {
			rows = append(rows, row{repo: g.Repo, entry: &g.Entries[i]})
		}
	}
	return rows
}

// treeLabel is a worktree's name in the tree: its branch, or for one with no
// branch of its own (a detached checkout, a window awt didn't create) its name.
func treeLabel(e Entry) string {
	if e.Branch != "" {
		return e.Branch
	}
	return e.Name
}

func treeStatus(e Entry) string {
	switch {
	case e.Active:
		return "on screen"
	case e.Alive:
		return "parked"
	}
	return ""
}

// startRow is the 1-based line the cursor starts on. Coming back from an a or
// d, that's the row it was on — or, if that row is gone or folded away, the
// nearest one above it that's showing. Otherwise it's the top line: the repo
// the tree was opened from, which sorts first.
func startRow(v *treeView, at int) int {
	if at < 0 || len(v.Rows) == 0 {
		return 1
	}
	at = min(at, len(v.Rows)-1)
	for at > 0 && !v.shown(at) {
		at--
	}
	return v.position(at)
}

// treeHeader spells out the keys, since neither the modes nor a/d are
// guessable from the rows. note, if any, is what the last a or d did.
func treeHeader(note string) string {
	h := "l/enter: open · h: close · a: add worktree · d: delete · /: filter · q: quit\n" +
		"filtering: esc back to the tree · enter on no match or " + createKey + ": make that name"
	if note != "" {
		h = note + "\n" + h
	}
	return h
}

// pick shows the tree once, folded as open says, writing its view to view for
// the bindings to fold and unfold. at is the row to put the cursor on, -1 for
// the default.
func pick(groups []Group, view string, open map[string]bool, note string, at int) (choice, error) {
	v, rows := newTreeView(groups, open), treeRows(groups)
	if _, err := exec.LookPath("fzf"); err != nil {
		return pickPlain(v.lines(true), rows)
	}
	if err := v.save(view); err != nil {
		return choice{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return choice{}, err
	}
	helper := func(verb string) string {
		return fmt.Sprintf("transform:%s %s %s %s {3}", shellQuote(exe), treeHelper, shellQuote(view), verb)
	}
	// --raw keeps every line on screen while filtering, matches highlighted and
	// the rest dimmed, so a match never loses the repo it sits under. --prompt
	// is set explicitly because the vim mode reads it to tell which mode it's
	// in, and one from FZF_DEFAULT_OPTS would break that.
	args := append(matchArgs(), "--raw", "--layout=reverse", "--info=inline",
		"--print-query", "--prompt="+normalPrompt, "--header="+treeHeader(note),
		// load, not start: start fires before the lines are in, too early to
		// move to one. Once only, or every fold's reload would move it back.
		fmt.Sprintf("--bind=load:pos(%d)+unbind(load)", startRow(v, at)),
		// A query unfolds everything, so nothing it matches hides in a fold.
		"--bind=change:"+helper("query"),
		// Typing moves the cursor to the best match; raw mode would leave it put.
		// result, not change: change fires before the search has run.
		`--bind=result:transform:[ -n "$FZF_QUERY" ] && echo best`,
		// Every way out prints the action it was, after the query.
		"--bind=enter:"+helper("enter"),
		"--bind="+createKey+":print("+string(actCreate)+")+accept",
	)
	args = append(args, pickerBindings(helper("open"), helper("close"))...)
	return runFzf(args, v.lines(false), v.lines(true), rows)
}

// runFzf runs the tree and reads back what it was asked to do. all is every
// line, folded or not, for settleEnter.
func runFzf(args, lines, all []string, rows []row) (choice, error) {
	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 2 {
			return choice{}, fmt.Errorf("fzf failed — the tree needs fzf %s or newer", fzfMinVersion)
		}
		return choice{}, nil // esc, q, ctrl-c: a cancel
	}
	// --print-query puts the query first, then the action the binding printed,
	// then the line the cursor was on.
	fields := strings.Split(string(out), "\n")
	ch := choice{typed: strings.TrimSpace(fields[0]), idx: -1}
	if len(fields) > 1 {
		ch.action = action(strings.TrimSpace(fields[1]))
	}
	if len(fields) > 2 {
		if i, ok := lineIndex(fields[2], len(rows)); ok {
			ch.row, ch.idx = &rows[i], i
		}
	}
	if ch.action == actSwitch && ch.typed != "" {
		ch = settleEnter(ch, all, rows)
	}
	return ch, nil
}

// matchArgs are the flags that decide what a query matches, shared by the tree
// and settleEnter's re-run of it. --nth=1 matches on the name alone, not
// "parked"; --no-sort --tiebreak=index keep the tree's order.
func matchArgs() []string {
	return []string{"--with-nth=1,2", "--nth=1", "--delimiter=\t", "--no-sort", "--tiebreak=index"}
}

// settleEnter works out what enter on a query meant. In raw mode fzf accepts
// the line under the cursor even when nothing matches, and anything fzf could
// check from a binding — $FZF_MATCH_COUNT, the cursor following the best match
// — can still describe the previous query when enter comes hard on the heels
// of typing. So the query is run again through `fzf --filter`, the same
// matcher, settled: no match makes it a name to create, and a cursor left on a
// line that doesn't match moves to the first one that does.
func settleEnter(ch choice, lines []string, rows []row) choice {
	cmd := exec.Command("fzf", append(matchArgs(), "--filter="+ch.typed)...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	out, _ := cmd.Output() // exit 1 is "no match", and empty output says as much
	var matched []int
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i, ok := lineIndex(l, len(rows)); ok {
			matched = append(matched, i)
		}
	}
	if len(matched) == 0 {
		return choice{action: actCreate, typed: ch.typed, idx: -1}
	}
	for _, i := range matched {
		if ch.idx == i {
			return ch
		}
	}
	i := matched[0]
	return choice{action: actSwitch, row: &rows[i], idx: i, typed: ch.typed}
}

// lineIndex reads the row index off the end of one of the tree's lines.
func lineIndex(line string, n int) (int, bool) {
	i, err := strconv.Atoi(strings.TrimSpace(line[strings.LastIndex(line, "\t")+1:]))
	return i, err == nil && i >= 0 && i < n
}

// pickPlain is the fzf-less fallback: the tree numbered, a number to switch,
// "a N" or "d N" to add or delete at line N, and anything else a name to make.
func pickPlain(labels []string, rows []row) (choice, error) {
	for i, l := range labels {
		l = l[:strings.LastIndex(l, "\t")] // the row index, there for fzf
		fmt.Printf("%3d  %s\n", i+1, strings.Replace(l, "\t", "  ", 1))
	}
	fmt.Print("select (N to switch, a N to add, d N to delete, or a name to make): ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return choice{idx: -1}, nil
	}
	act, arg := actSwitch, line
	if verb, rest, ok := strings.Cut(line, " "); ok && (verb == "a" || verb == "d") {
		act, arg = actAdd, strings.TrimSpace(rest)
		if verb == "d" {
			act = actDelete
		}
	}
	idx, err := strconv.Atoi(arg)
	if err != nil {
		if act != actSwitch {
			return choice{}, fmt.Errorf("invalid line %q", arg)
		}
		return choice{action: actCreate, typed: line, idx: -1}, nil
	}
	if idx < 1 || idx > len(rows) {
		return choice{}, fmt.Errorf("invalid selection")
	}
	return choice{action: act, row: &rows[idx-1], idx: idx - 1}, nil
}

// clearScreen wipes what the last round of questions left behind, so each one
// asked after the tree closes starts on a clean screen.
func clearScreen() {
	if isTTY(os.Stderr) {
		fmt.Fprint(os.Stderr, "\033[H\033[2J")
	}
}
