package command

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Folding, the way nvim-tree folds directories: every repo starts collapsed to
// its own line, l opens it, h closes it. fzf can't hide lines on its own, so a
// fold is a reload: the bindings call back into awt (`awt __tree`), which flips
// the fold in a view file and hands fzf the lines that are now visible. The
// view file outlives each fzf run, so folds stay as they were when the tree
// comes back after an a or a d.

// treeHelper is the hidden subcommand fzf's bindings run.
const treeHelper = "__tree"

// viewRow is one line of the tree, folded or not.
type viewRow struct {
	Repo   string `json:"repo"`
	Head   bool   `json:"head"` // the repo's own line
	Name   string `json:"name"` // the repo, or "├── branch" under it
	Status string `json:"status"`
}

// treeView is the whole tree and which of it is showing.
type treeView struct {
	Rows []viewRow       `json:"rows"`
	Open map[string]bool `json:"open"` // repos unfolded
	// All shows everything regardless of folds: a query is in, and a match
	// mustn't hide inside a folded repo.
	All bool `json:"all"`
}

func newTreeView(groups []Group, open map[string]bool) *treeView {
	v := &treeView{Open: open}
	if v.Open == nil {
		v.Open = map[string]bool{}
	}
	for _, g := range groups {
		v.Rows = append(v.Rows, viewRow{Repo: g.Repo, Head: true, Name: g.Repo, Status: repoStatus(g)})
		for i, e := range g.Entries {
			branch := "├── "
			if i == len(g.Entries)-1 {
				branch = "└── "
			}
			v.Rows = append(v.Rows, viewRow{Repo: g.Repo, Name: branch + treeLabel(e), Status: treeStatus(e)})
		}
	}
	return v
}

// repoStatus sums a repo up on its own line, since folded that's all of it
// that shows.
func repoStatus(g Group) string {
	switch {
	case g.Err != nil:
		return "unreadable: " + g.Err.Error()
	case len(g.Entries) == 0:
		return "no worktrees — a: add one"
	}
	s := plural(len(g.Entries), "worktree")
	for _, e := range g.Entries {
		if e.Active {
			return s + " · on screen"
		}
	}
	return s
}

func (v *treeView) shown(i int) bool {
	return v.All || v.Rows[i].Head || v.Open[v.Rows[i].Repo]
}

// visible is the indexes of the rows on screen, in order.
func (v *treeView) visible() []int {
	var out []int
	for i := range v.Rows {
		if v.shown(i) {
			out = append(out, i)
		}
	}
	return out
}

// line is row i as fzf gets it: name, status, and the row's index, which
// stays put however the tree is folded, so it's what the cursor tracks.
func (v *treeView) line(i int, width int) string {
	r := v.Rows[i]
	name := "  " + r.Name
	if r.Head {
		name = "▸ " + r.Name
		if v.All || v.Open[r.Repo] {
			name = "▾ " + r.Name
		}
	}
	// The status column only lines up if the names are padded to one width:
	// fzf expands the tab between them to the next tab stop, not to a column.
	pad := strings.Repeat(" ", max(0, width-utf8.RuneCountInString(name)))
	return fmt.Sprintf("%s%s\t%s\t%d", name, pad, r.Status, i)
}

// width is what every name is padded to — over every row, folded or not, so
// the status column stays put when a repo opens.
func (v *treeView) width() int {
	w := 0
	for _, r := range v.Rows {
		w = max(w, 2+utf8.RuneCountInString(r.Name))
	}
	return w
}

// lines is what fzf shows: the visible rows, or all of them.
func (v *treeView) lines(all bool) []string {
	w := v.width()
	var out []string
	for i := range v.Rows {
		if all || v.shown(i) {
			out = append(out, v.line(i, w))
		}
	}
	return out
}

// headOf is the index of the repo line row i hangs off.
func (v *treeView) headOf(i int) int {
	for i > 0 && !v.Rows[i].Head {
		i--
	}
	return i
}

func loadTreeView(path string) (*treeView, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v treeView
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	if v.Open == nil {
		v.Open = map[string]bool{}
	}
	return &v, nil
}

func (v *treeView) save(path string) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// TreeHelper is `awt __tree <view> <verb> [row]`, run by the tree's bindings.
// render prints the visible lines, for fzf's reload. The rest print fzf
// actions back to a transform binding:
//
//	open   l, o: unfold a repo; on a worktree, switch to it
//	close  h: fold a repo; on a worktree, fold its repo and land on it
//	enter  unfold or fold a repo; switch to a worktree; settle a query
//	query  the query changed: show everything while there is one
func TreeHelper(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: awt %s <view> <verb> [row]", treeHelper)
	}
	path, verb := args[0], args[1]
	v, err := loadTreeView(path)
	if err != nil {
		return err
	}
	row := -1
	if len(args) > 2 {
		if i, err := strconv.Atoi(strings.TrimSpace(args[2])); err == nil && i >= 0 && i < len(v.Rows) {
			row = i
		}
	}
	if verb == "render" {
		fmt.Println(strings.Join(v.lines(false), "\n"))
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	acts, cursor, reload := v.act(verb, row, os.Getenv("FZF_QUERY"))
	if reload {
		if err := v.save(path); err != nil {
			return err
		}
		acts = append(acts, fmt.Sprintf("reload-sync(%s %s %s render)", shellQuote(exe), treeHelper, shellQuote(path)))
		// The cursor stays on its line number across a reload, not its row, so
		// it's put back on the row by hand. Not with --track: that holds keys
		// typed during the reload and drops them.
		if cursor >= 0 {
			acts = append(acts, fmt.Sprintf("pos(%d)", v.position(cursor)))
		}
	}
	fmt.Println(strings.Join(acts, "+"))
	return nil
}

// act works out what a key does to the tree: fzf actions to run, and whether
// the folds changed, which calls for a reload after them with the cursor put
// on row cursor (-1: wherever the reload leaves it).
func (v *treeView) act(verb string, row int, query string) (acts []string, cursor int, reload bool) {
	switchTo := []string{"print(" + string(actSwitch) + ")", "accept"}
	if verb == "query" {
		all := query != ""
		if all == v.All {
			return nil, -1, false
		}
		v.All = all
		if all {
			return nil, -1, true // the result binding moves to the best match
		}
		// Back to the folded tree with the cursor on a match inside a folded
		// repo: unfold it rather than lose the cursor, the way nvim-tree
		// reveals the file you found.
		if row >= 0 && !v.Rows[row].Head {
			v.Open[v.Rows[row].Repo] = true
		}
		return nil, row, true
	}
	if verb == "enter" && query != "" {
		return switchTo, -1, false // settleEnter makes sense of it
	}
	if row < 0 || v.All {
		return nil, -1, false
	}
	r := v.Rows[row]
	switch verb {
	case "open", "enter":
		if !r.Head {
			return switchTo, -1, false
		}
		if v.Open[r.Repo] {
			if verb == "enter" {
				delete(v.Open, r.Repo)
				return nil, row, true
			}
			return []string{"down"}, -1, false // already open: l steps into it
		}
		v.Open[r.Repo] = true
		return nil, row, true
	case "close":
		if r.Head && !v.Open[r.Repo] {
			return nil, -1, false
		}
		// On a worktree, its row folds away: the cursor goes up to the repo.
		delete(v.Open, r.Repo)
		return nil, v.headOf(row), true
	}
	return nil, -1, false
}

// position is the 1-based line row i is on right now.
func (v *treeView) position(i int) int {
	n := 0
	for j := 0; j <= i; j++ {
		if v.shown(j) {
			n++
		}
	}
	return n
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
