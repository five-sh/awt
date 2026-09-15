# awt

CLI for working across multiple git worktrees without leaving the terminal.
Each worktree gets its own tmux session — an `edit` window plus one or more
agent windows — that you can detach from and reattach to at any time.

## Requirements

- `git`, `tmux`
- [`claude`](https://claude.com/claude-code) on `PATH` — runs in each agent window
- `nvim`, or `$EDITOR` — runs in the `edit` window
- `fzf` (optional) — fuzzy picker; falls back to a numbered prompt without it

## Install

```
make install   # build + copy to ~/.local/bin/awt
make build     # just build ./awt
```

## Usage

```
awt new <repo> <name> [--from <ref>] [--no-parent]
```

Creates a branch + worktree + session, and switches to it. Base ref: `--from`,
else the worktree you're currently in (same repo), else the repo's default
branch fetched fresh from `origin`. `--no-parent` skips the "current worktree" check.

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

Switches to a worktree, creating the worktree and/or session if needed. The
default branch is always rebuilt from `origin`; any other name checks out its
existing branch. No worktree → picker scoped to the repo. No repo → picker
across all registered repos.

```
awt ls [<repo>]
```

Worktrees grouped by repo — branch, base, session alive, last attached — with
repos you have live sessions in first, most recently attached first inside
each. Piped (`awt ls | grep ...`) it prints one flat table instead, with a
leading `REPO` column when more than one repo is listed.

```
awt rm <repo> <worktree> [--force]
```

Kills the session, removes the worktree, and deletes its branch. Refuses on
uncommitted changes unless `--force`; a branch with unmerged commits is left
in place with a warning unless `--force` is given, which deletes it too.

```
awt agent add <repo> <worktree> [name]
```

Adds an agent window (`agent-N` by default) to an already-running session.
Works from inside that session (focus jumps to it) or from anywhere else
(created in the background). Session must already be up — run `switch` first.

`new`, `ls`, `switch`, `rm`, `agent` are reserved repo names.

## Session layout

Every session starts with:

- `edit` — `$EDITOR .` (default `nvim`)
- `agent-1` — `claude --dangerously-skip-permissions`

`--dangerously-skip-permissions` is intentional for a smoother experience.

## Picker from inside tmux

`awt` with no arguments opens the picker, so bind it to a popup (tmux 3.2+) to
switch worktrees without leaving whatever you're in:

```
bind-key g display-popup -E -w 80% -h 70% "awt"
```

Each row is just `<repo>/<branch>`. The list is grouped by repo, running
sessions first, and pre-filtered to the repo you opened it from — `ctrl-u`
clears that to see everything.

The query isn't only a filter: a name no row answers to is one the picker will
make for you on `enter`, so typing `myrepo/feature-x` and hitting enter gets
the same worktree and session `awt new myrepo feature-x` would. It resolves the
same way, too — an existing branch (local or on `origin`) gets checked out,
anything else becomes a new branch off the base `awt new` would pick, and a
repo that isn't registered yet is registered, cloning it if you give a URL. So
a fresh repo, a new branch and its session are all one prompt away.

`alt-enter` does that with the query even when rows still match it — the way to
get `feature` while `feature-x` exists. In a picker scoped to one repo the
rows are bare branches, and so is what you type (`codex/foo` is a branch, not a
repo); in the global one, the first path segment names the repo, unless only
one repo is on offer.

## Config

| Env var              | Default            | Purpose                                |
| -------------------- | ------------------ | -------------------------------------- |
| `AWT_REPOS_ROOT`     | `~/awt/repos`      | bare clones, used as worktree sources  |
| `AWT_WORKSPACE_ROOT` | `~/awt/workspaces` | actual worktrees (`<repo>/<worktree>`) |
| `EDITOR`             | `nvim`             | command run in the `edit` window       |

State (`~/.awt/state.json`) is a cache reconciled against git/tmux on every
command, not a source of truth.

## Not yet there

- `awt repo ls`/`rm` — can't list/remove a registered repo, only its worktrees
- `awt agent ls`/`rm` — close a window with tmux directly (`prefix+&`)
