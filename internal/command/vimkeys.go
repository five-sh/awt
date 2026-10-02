package command

import (
	"fmt"
	"strings"
)

// Vim keys in the tree.
//
// fzf has no modes of its own and allows one binding per key, so normal mode is
// emulated. Normal mode is simply the keys as bound on the command line, and the
// tree opens in it, as choose-tree does: the rows are the point, and a and d act
// on them straight away. `i` and `/` unbind the lot — an unbound key in fzf
// types its character — and `esc` rebinds them. The prompt doubles as the mode
// flag: fzf hands `$FZF_PROMPT` to a `transform` binding, which is what lets one
// key mean two things, so `ctrl-u` can still clear the query while typing and
// scroll half a page in normal mode — the two things ctrl-u does in vim.
//
// Note that fzf's `unbind` leaves a key dead rather than restoring its default,
// so a key with an insert-mode job of its own is driven by `transform` and never
// unbound.
const (
	insertPrompt = "> "
	normalPrompt = "normal> "
)

// normalKeys are the keys that act in normal mode and type in insert mode.
// Anything not listed here keeps one meaning in both modes (enter, alt-enter).
var normalKeys = []struct{ key, action string }{
	// down-match, not down: while filtering, the tree keeps the lines that
	// don't match on screen, and j/k hop between the ones that do.
	{"j", "down-match"},
	{"k", "up-match"},
	{"g", "first"},
	{"G", "last"},
	{"q", "abort"},
	{"i", leaveNormal},
	{"/", leaveNormal},
	// nvim-tree's folding: l or o opens a repo, h closes it.
	{"l", openFold},
	{"o", openFold},
	{"h", closeFold},
	// a and d hand the row under the cursor back to awt, which does the asking.
	{"a", "print(" + string(actAdd) + ")+accept"},
	{"d", "print(" + string(actDelete) + ")+accept"},
	// D and C get rid of the query without going back to typing first. As in
	// vim, C also drops you into typing, which is usually why you cleared it.
	{"D", "clear-query"},
	{"C", "clear-query+" + leaveNormal},
}

// modalKeys keep their insert-mode meaning — the one they have in vim's insert
// mode too — and gain a normal-mode one.
var modalKeys = []struct{ key, insert, normal string }{
	{"esc", enterNormal, "abort"},
	{"ctrl-u", "unix-line-discard", "half-page-up"},
	{"ctrl-d", "delete-char/eof", "half-page-down"},
	{"ctrl-f", "forward-char", "page-down"},
	{"ctrl-b", "backward-char", "page-up"},
}

// ignoredInNormal is everything else that would otherwise type a character:
// in normal mode a stray letter must do nothing, not quietly re-filter the list.
const ignoredInNormal = "bcefmnprstuvwxyzABEFHIJKLMNOPQRSTUVWXYZ0123456789"

// enterNormal and leaveNormal are placeholders the bindings fill in: the key
// sets can only be spelled out once the whole list is known. openFold and
// closeFold are the actions that fold the tree, which only the tree can name.
const (
	enterNormal = "\x00enter-normal"
	leaveNormal = "\x00leave-normal"
	openFold    = "\x00open-fold"
	closeFold   = "\x00close-fold"
)

// pickerBindings is the --bind flags that give the tree its vim mode, with open
// and close as the actions l/o and h run.
func pickerBindings(open, close string) []string {
	keys := make([]string, 0, len(normalKeys)+len(ignoredInNormal))
	for _, k := range normalKeys {
		keys = append(keys, k.key)
	}
	for _, r := range ignoredInNormal {
		keys = append(keys, string(r))
	}
	set := strings.Join(keys, ",")

	enter := fmt.Sprintf("rebind(%s)+change-prompt(%s)", set, normalPrompt)
	leave := fmt.Sprintf("unbind(%s)+change-prompt(%s)", set, insertPrompt)
	resolve := strings.NewReplacer(enterNormal, enter, leaveNormal, leave,
		openFold, open, closeFold, close).Replace

	var binds []string
	for _, k := range normalKeys {
		binds = append(binds, "--bind="+k.key+":"+resolve(k.action))
	}
	for _, r := range ignoredInNormal {
		binds = append(binds, "--bind="+string(r)+":ignore")
	}
	for _, m := range modalKeys {
		binds = append(binds, "--bind="+m.key+":"+modalAction(resolve(m.insert), m.normal))
	}
	return binds
}

// modalAction is one key doing two jobs. fzf runs a transform binding through
// $SHELL and reads actions back from its output, so the mode test is a POSIX
// case statement over the prompt. That costs a shell start per press of one of
// these keys — measured at 8ms with zsh, 15ms with sh, so not worth pinning the
// shell to avoid.
func modalAction(insert, normal string) string {
	return fmt.Sprintf(`transform:case "$FZF_PROMPT" in %q) echo %q;; *) echo %q;; esac`,
		normalPrompt, normal, insert)
}
