# awt

CLI for working across multiple git worktrees without leaving the terminal. One
tmux session holds one window per repo you're working in; each window shows that
repo's current worktree, and switching worktrees swaps another one into its place
with everything inside it still running.

## Requirements

- `git`, `tmux` (3.0+)
- [`claude`](https://claude.com/claude-code) on `PATH` — runs in each agent pane
- `nvim`, or `$EDITOR` — runs in the `edit` pane
- `fzf` (optional) — fuzzy picker; falls back to a numbered prompt without it

## Install

```
make install   # build + copy to ~/.local/bin/awt
make build     # just build ./awt
make check     # fmt + vet + test + build
```

Coming from v0.1, which gave every worktree its own session:

```
awt migrate [--dry-run]
```

kills those sessions once. The worktrees themselves are untouched — each gets a
window the next time you switch to it. Sessions no worktree in state accounts
for are listed, never killed.

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
existing branch. No worktree → picker scoped to the repo. No repo → picker
across all registered repos.

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
awt park [<repo>]
```

Sends a repo's on-screen worktree back to the park session: the repo leaves the
status bar with its editor and agents still running. No repo name parks the one
you're in. Parking your last repo window destroys the front session and so
detaches you — which is what asking to park your last repo means.

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

`new`, `ls`, `switch`, `rm`, `agent`, `park`, `migrate` are reserved repo names.

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

## Picker from inside tmux

`awt` with no arguments opens the picker, so bind it to a popup (tmux 3.2+) to
switch worktrees without leaving whatever you're in:

```
bind-key g display-popup -E -w 80% -h 70% "awt"
```

A popup is the intended path: run `awt` inline in a pane and the swap still
works, but the pane you ran it from is the one that just got parked, with its
output.

Each row is just `<repo>/<branch>`. The list is grouped by repo, with what's on
screen first, then what's parked, and pre-filtered to the repo you opened it
from — `D` clears that to see everything.

The query isn't only a filter: a name no row answers to is one the picker will
make for you on `enter`, so `i`, `myrepo/feature-x`, enter gets the same
worktree and window `awt new myrepo feature-x` would. It resolves the same
way, too — an existing branch (local or on `origin`) gets checked out, anything
else becomes a new branch off the base `awt new` would pick, and a repo that
isn't registered yet is registered, cloning it if you give a URL. So a fresh
repo, a new branch and its window are all one prompt away.

`alt-enter` does that with the query even when rows still match it — the way to
get `feature` while `feature-x` exists. In a picker scoped to one repo the rows
are bare branches, and so is what you type (`codex/foo` is a branch, not a
repo); in the global one, the first path segment names the repo, unless only one
repo is on offer.

### Keys

The picker opens in typing mode — the prompt reads `>` — so you can filter
straight away. `esc` switches to normal mode (`normal>`), where vim keys move
around, and `i`, `a` or `/` switch back to typing.

| Key                 | Normal mode (`normal>`) | Typing (`>`)              |
| ------------------- | ----------------------- | ------------------------- |
| `j` / `k`           | down / up               | types the letter          |
| `g` / `G`           | first / last row        | types the letter          |
| `ctrl-d` / `ctrl-u` | half page down / up     | delete char / clear query |
| `ctrl-f` / `ctrl-b` | page down / up          | cursor right / left       |
| `ctrl-j` / `ctrl-k` | down / up               | down / up                 |
| `D`                 | clear the query         | types the letter          |
| `C`                 | clear it and type       | types the letter          |
| `i`, `a`, `/`       | start typing            | types the letter          |
| `esc`               | cancel                  | back to normal mode       |
| `q`                 | cancel                  | types the letter          |
| `enter`             | switch                  | switch                    |
| `alt-enter`         | make what you typed     | make what you typed       |

In normal mode, all other letters and digits do nothing. A key pressed by
mistake won't change the list or create a branch.

The picker starts filtered to your repo. Press `D` or `C` to clear that filter.
While typing, `ctrl-u` clears what you typed.

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

## Not yet there

- `awt repo ls`/`rm` — can't list/remove a registered repo, only its worktrees
- `awt agent ls`/`rm` — close a pane with tmux directly (`prefix+x`)
- `awt switch --pin` — give one worktree its own window instead of sharing its
  repo's, for the times two branches of one repo do need to be side by side
