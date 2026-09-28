package command

import (
	"fmt"
	"io"
)

// usages is one line per subcommand, in the order main dispatches them.
var usages = []string{
	"awt new <repo> <name> [--from <ref>] [--no-parent]",
	"awt ls [<repo>]",
	"awt switch <repo> [<worktree>]",
	"awt <repo> [<worktree>]",
	"awt rm <repo> <worktree> [--force]",
	"awt agent add <repo> <worktree> [name]",
	"awt park [<repo>]",
	"awt migrate [--dry-run]",
	"awt help",
}

// Help prints a short usage summary: every subcommand with its usage line.
func Help(w io.Writer) error {
	_, err := fmt.Fprintf(w, "usage:\n")
	if err != nil {
		return err
	}
	for _, u := range usages {
		if _, err := fmt.Fprintf(w, "  %s\n", u); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "\nawt with no arguments opens the worktree picker.\n")
	return err
}
