package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/five-sh/awt/internal/command"
)

func main() {
	if len(os.Args) < 2 {
		check(command.Switch("", ""))
		return
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "new":
		runNew(args)
	case "ls":
		runLs(args)
	case "switch":
		runSwitch(args)
	case "rm":
		runRm(args)
	case "agent":
		runAgent(args)
	case "park":
		runPark(args)
	case "migrate":
		runMigrate(args)
	case "__tree": // the tree's own bindings call back in; not for typing
		check(command.TreeHelper(args))
	case "help", "-h", "--help":
		fmt.Print(usage)
	case "version", "--version":
		fmt.Println("awt " + buildVersion())
	default:
		runBareSwitch(os.Args[1:])
	}
}

const usage = `awt: work on many git worktrees from one tmux session

Usage:
  awt                                       browse every repo's worktrees as a tree
  awt <repo> [worktree]                     same as 'awt switch'
  awt switch <repo> [worktree]              switch to a worktree, creating it if needed
  awt new <repo> <name> [--from ref] [--no-parent]
                                            create a branch and worktree, then switch to it
  awt ls [repo]                             list worktrees
  awt park [repo]                           move a repo's window off screen
  awt rm <repo> <worktree> [--force]        remove a worktree and its branch
  awt agent add <repo> <worktree> [name]    add a Claude agent pane to a worktree
  awt migrate [--dry-run]                   kill v0.1 per-worktree sessions
  awt version                               print the version
  awt help                                  show this help

Environment:
  AWT_SESSION         main tmux session name (default: awt)
  AWT_REPOS_ROOT      bare clones (default: ~/awt/repos)
  AWT_WORKSPACE_ROOT  worktrees (default: ~/awt/workspaces)
  EDITOR              editor in the edit pane (default: nvim)
`

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

// buildVersion is version if the build set it, else the module version that
// `go install ...@v1.2.3` records, else "dev" for a plain checkout build.
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func runBareSwitch(args []string) {
	name := ""
	if len(args) > 1 {
		name = args[1]
	}
	check(command.Switch(args[0], name))
}

// splitArgs separates flags (with their values) from positional args, in any order.
// valueFlags names flags that consume a following token (e.g. "from" for --from x).
func splitArgs(args []string, valueFlags map[string]bool) (flags, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return flags, positional
}

func runNew(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	from := fs.String("from", "", "base branch or worktree name")
	noParent := fs.Bool("no-parent", false, "don't fork from the current worktree")
	flags, positional := splitArgs(args, map[string]bool{"from": true})
	fs.Parse(flags)
	if len(positional) < 2 {
		fatal("usage: awt new <repo> <name> [--from ref] [--no-parent]")
	}
	opts := command.NewOptions{Repo: positional[0], Name: positional[1], From: *from, NoParent: *noParent}
	check(command.New(opts))
}

func runLs(args []string) {
	fs := flag.NewFlagSet("ls", flag.ExitOnError)
	fs.Parse(args)
	repo := ""
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	check(command.Ls(repo))
}

func runSwitch(args []string) {
	fs := flag.NewFlagSet("switch", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal("usage: awt switch <repo> [worktree]")
	}
	name := ""
	if fs.NArg() > 1 {
		name = fs.Arg(1)
	}
	check(command.Switch(fs.Arg(0), name))
}

func runRm(args []string) {
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	force := fs.Bool("force", false, "remove even with uncommitted changes")
	flags, positional := splitArgs(args, map[string]bool{})
	fs.Parse(flags)
	if len(positional) < 2 {
		fatal("usage: awt rm <repo> <worktree> [--force]")
	}
	check(command.Rm(positional[0], positional[1], *force))
}

func runAgent(args []string) {
	if len(args) < 1 || args[0] != "add" {
		fatal("usage: awt agent add <repo> <worktree> [name]")
	}
	a := args[1:]
	if len(a) < 2 {
		fatal("usage: awt agent add <repo> <worktree> [name]")
	}
	name := ""
	if len(a) > 2 {
		name = a[2]
	}
	check(command.AgentAdd(a[0], a[1], name))
}

func runPark(args []string) {
	fs := flag.NewFlagSet("park", flag.ExitOnError)
	fs.Parse(args)
	repo := ""
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	check(command.Park(repo))
}

func runMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "list what would be killed, kill nothing")
	flags, positional := splitArgs(args, map[string]bool{})
	fs.Parse(flags)
	if len(positional) > 0 {
		fatal("usage: awt migrate [--dry-run]")
	}
	check(command.Migrate(*dryRun))
}

func check(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "awt: "+msg)
	os.Exit(1)
}
