# awt

CLI for working across multiple git worktrees without leaving the terminal. One
tmux session holds one window per repo you're working in; each window shows that
repo's current worktree, and switching worktrees swaps another one into its place
with everything inside it still running.

## Requirements

- `git`, `tmux` (3.0+)
- [`claude`](https://claude.com/claude-code) on `PATH` — runs in each agent pane
- `nvim`, or `$EDITOR` — runs in the `edit` pane
- `fzf` 0.66+ (optional) — the worktree tree; falls back to a numbered prompt without it

## Install

A prebuilt binary for macOS or Linux (amd64/arm64) from the
[releases page](https://github.com/five-sh/awt/releases), put on your `PATH`, or
with Go 1.26+:

```
go install github.com/five-sh/awt@latest
```

or from a checkout:

```
make install   # build + copy to ~/.local/bin/awt
make build     # just build ./awt
make check     # fmt + vet + test + build
```

`awt version` prints the version you have.

## Quick start

There is no separate setup step. The first time you name a repo awt doesn't
know, it registers it on the spot:

```
cd ~/code/myrepo
awt myrepo main
```

Run from inside a checkout, awt offers to register that checkout as `myrepo`.
Anywhere else, it asks for a git URL and clones it as a bare repo into
`$AWT_REPOS_ROOT/myrepo`. Either way, you land in the `awt` tmux session with
the repo's `main` worktree on screen (use whatever your default branch is): a `claude` agent on the left and your
editor on the right.

From there:

```
awt new myrepo feature-a        # new branch + worktree, switched to
awt agent add myrepo feature-a  # a second agent pane in it
awt                             # the tree: switch, add or delete worktrees
awt rm myrepo feature-a         # done with it
```

Bind `awt` to a tmux popup (see [The tree from inside tmux](#the-tree-from-inside-tmux))
and you rarely need to type the rest.

## Usage

```
awt new <repo> <name> [--from <ref>] [--no-parent]
```

Creates a branch + worktree, and switches to it. Base ref: `--from`, else the
worktree you're currently in (same repo), else the repo's default branch fetched
fresh from `origin`. `--no-parent` skips the "current worktree" check.

`--from` takes an existing worktree (by its name or its branch), a local branch,
or a branch that so far only exists on `origin` — which gets fetched first. So

```
awt new myrepo feature-b --from feature-a
```

forks `feature-b` off the `feature-a` worktree as it stands right now, including
commits it hasn't pushed. A local base ref is used as-is and never fetched, so
forking off a sibling worktree can't move a branch out from under it; pass
`--from origin/main` (or omit `--from`) when you want a freshly fetched base.

```
awt switch <repo> [<worktree>]
awt <repo> [<worktree>]
awt
```

Switches to a worktree, creating the worktree and/or its window if needed. The
default branch is always rebuilt from `origin`; any other name checks out its
existing branch. No worktree → the tree for just that repo. No repo → the tree
of every registered repo.

```
awt ls [<repo>]
```

Worktrees grouped by repo — branch, base, where its window is, last attached —
with repos you have windows in first, most recently attached first inside each.
`WHERE` is `active` (on screen as its repo's window), `parked` (window alive,
another worktree has the repo's slot) or `-` (no window yet). Piped (`awt ls |
grep ...`) it prints one flat table instead, with a leading `REPO` column when
more than one repo is listed.

```
awt rm <repo> <worktree> [--force]
```

Kills the worktree's window, removes the worktree, and deletes its branch.
Refuses on uncommitted changes unless `--force`; a branch with unmerged commits
is left in place with a warning unless `--force` is given, which deletes it too.
If the worktree was the one its repo had on screen, the repo just leaves the
status bar; whatever it had parked is untouched.

```
awt agent add <repo> <worktree> [name]
```

Adds an agent pane (`agent-N` by default) to a worktree's window: agents stack
down the left column, and the editor keeps the right. Works on a parked worktree
as much as the one on screen, and focus only follows when you're already looking
at that window.

`new`, `ls`, `switch`, `rm`, `agent`, `version`, `help` are reserved repo names.

## Windows and panes

Two tmux sessions, however many repos and worktrees you have:

- **`awt`** — the one you attach to. One window per repo you've opened, named
  `<repo>:<worktree>`, in the order you opened them.
- **`awt-park`** — never attached. Every worktree window that isn't on screen,
  so its editor and agents keep running while a sibling holds the repo's slot.
  An `awt-keeper` window sits in it because tmux destroys a session the moment
  its last window leaves.

A worktree is exactly one window, which is what lets it move as a unit —
agents down the left, the editor on the right, and the editor is the pane you
land in:

```
┌─────────────┬──────────────────┐
│ claude      │ nvim .           │
│ agent-1     │ edit             │
├─────────────┤                  │
│ claude      │                  │
│ agent-2     │                  │
└─────────────┴──────────────────┘
window: myrepo:feature-a
```

Switching worktrees inside a repo is a `swap-window` between the two sessions at
that repo's index. A tmux session points at its current window *by index*, so the
client keeps watching the same slot and simply starts rendering what was swapped
in — no process restarts, no state lost, and the status bar never grows past one
entry per repo. A parked window keeps the size it had on screen, so nothing
reflows on the way out or back.

The flip side: one worktree per repo is on screen at a time, so two branches of
the same repo can't sit side by side, and two clients attached to `awt` share
its window indices.

Windows are tracked by tmux window id (`@17`), never by index, so a
`renumber-windows` config can shuffle them freely. State (`~/.awt/state.json`)
is a cache reconciled against git and tmux on every command, not a source of
truth: a window id tmux no longer knows just means that worktree gets a fresh
window next time.

## The tree from inside tmux

`awt` with no arguments opens the tree, so bind it to a popup (tmux 3.2+) to
switch, add and delete worktrees without leaving whatever you're in:

```
bind-key g display-popup -E -w 80% -h 70% "awt"
```

A popup is the intended path: run `awt` inline in a pane and the swap still
works, but the pane you ran it from is the one that just got parked, with its
output.

Every repo is a line, folded the way nvim-tree folds a directory, with its
worktrees hanging off it once you open it:

```
▾ myrepo           3 worktrees · on screen
  ├── feature-a    on screen
  ├── feature-b    parked
  └── main
▸ other            1 worktree
```

Every repo starts folded. The repo you're in comes first, with the cursor on
it, then repos with windows, then by recency.

- `j`/`k` move, `l` (or `o`, or `enter`) opens a repo, and `h` closes it. `h` on
  a worktree closes its repo and puts the cursor on it. `l` on an open repo
  steps into it, and `enter` on one closes it again.
- `enter` (or `l`) on a worktree switches to it.
- `a` asks for a name and makes a worktree for it in the cursor's repo, then
  switches to it. On a worktree's line the new branch forks off that worktree,
  including commits it hasn't pushed, as `awt new --from` would. On the repo's
  line it forks off the default branch, freshly fetched. A name that's already
  a branch is checked out instead.
- `d` deletes the worktree under the cursor: its window (with the editor and
  agents in it), the worktree and its branch, after you say `y`. Uncommitted
  changes and an unmerged branch each get their own question. The tree comes
  back after a `d`, with what happened at the top, so you can clear out several
  in a row, folded as you left it. On a window no repo accounts for, `d` just
  kills the window.

Filtering opens every repo, so nothing you're looking for hides in a fold.
Matches are highlighted, the rest are dimmed, and the cursor jumps to the best
match. Clear the query and the tree folds back up, leaving open the repo the
cursor ended up in. A name nothing matches is one
`enter` will make for you, so `/`, `myrepo/feature-x`, enter gets the same
worktree and window `awt new myrepo feature-x` would. It resolves the same way,
too — an existing branch (local or on `origin`) gets checked out, anything else
becomes a new branch off the base `awt new` would pick, and a repo that isn't
registered yet is registered, cloning it if you give a URL. A bare name with no
`repo/` goes to the repo you opened the tree from.

`alt-enter` does that with the query even when rows still match it — the way to
get `feature` while `feature-x` exists. In a tree scoped to one repo
(`awt myrepo`), what you type is always a branch (`codex/foo` is a branch, not a
repo).

### Keys

The tree opens in normal mode — the prompt reads `normal>` — so `a` and `d` act
straight away. `i` or `/` switches to typing (`>`), and `esc` switches back.

| Key                 | Normal mode (`normal>`)           | Typing (`>`)              |
| ------------------- | --------------------------------- | ------------------------- |
| `j` / `k`           | down / up (next / previous match) | types the letter          |
| `l` / `o`           | open a repo; switch to a worktree | types the letter          |
| `h`                 | close a repo                      | types the letter          |
| `g` / `G`           | first / last row                  | types the letter          |
| `ctrl-d` / `ctrl-u` | half page down / up               | delete char / clear query |
| `ctrl-f` / `ctrl-b` | page down / up                    | cursor right / left       |
| `ctrl-j` / `ctrl-k` | down / up                         | down / up                 |
| `a`                 | add a worktree                    | types the letter          |
| `d`                 | delete the worktree               | types the letter          |
| `D`                 | clear the query                   | types the letter          |
| `C`                 | clear it and type                 | types the letter          |
| `i`, `/`            | start typing                      | types the letter          |
| `esc`               | cancel                            | back to normal mode       |
| `q`                 | cancel                            | types the letter          |
| `enter`             | open/close a repo; switch         | switch, or make the name  |
| `alt-enter`         | make what you typed               | make what you typed       |

In normal mode, all other letters and digits do nothing. A key pressed by
mistake won't change the list or create a branch.

Without fzf, the tree is printed numbered and unfolded: a number switches,
`a N` and `d N` add and delete at line `N`, and anything else is a name to make.

## Config

| Env var              | Default            | Purpose                                |
| -------------------- | ------------------ | -------------------------------------- |
| `AWT_SESSION`        | `awt`              | front session; park is `<name>-park`   |
| `AWT_REPOS_ROOT`     | `~/awt/repos`      | bare clones, used as worktree sources  |
| `AWT_WORKSPACE_ROOT` | `~/awt/workspaces` | actual worktrees (`<repo>/<worktree>`) |
| `EDITOR`             | `nvim`             | command run in the `edit` pane         |

`claude --dangerously-skip-permissions` is what agent panes run;
`--dangerously-skip-permissions` is intentional for a smoother experience.

## Tests

```
make test          # unit tests, no tmux involved
make test-tmux     # + integration tests against a private tmux server
```

The integration tests are behind a `tmux` build tag. They run on their own
socket with an empty config, so neither your server nor your `.tmux.conf` is
touched.
