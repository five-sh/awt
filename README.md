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

Table of worktrees: branch, base, dirty, session alive, last attached.

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
- No global tmux keybinding to open the picker from inside a session
