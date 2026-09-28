package command

import (
	"strings"
	"testing"
)

// Every subcommand main dispatches shows up in the help, with its usage line.
func TestHelpListsEverySubcommand(t *testing.T) {
	var b strings.Builder
	if err := Help(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"awt new <repo> <name>",
		"awt ls",
		"awt switch <repo>",
		"awt rm <repo> <worktree>",
		"awt agent add <repo> <worktree>",
		"awt park",
		"awt migrate",
		"awt help",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help is missing %q:\n%s", want, out)
		}
	}
}
