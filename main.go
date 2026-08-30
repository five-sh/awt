package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"awt/internal/command"
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
	default:
		runBareSwitch(os.Args[1:])
	}
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

func check(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "awt: "+msg)
	os.Exit(1)
}
