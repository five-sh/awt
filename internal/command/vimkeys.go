package command

import (
	"fmt"
	"strings"
)

// Vim keys in the picker.
//
// fzf has no modes of its own and allows one binding per key, so normal mode is
// emulated. The picker opens in normal mode, which is simply the keys as bound
// on the command line; `i`, `a` and `/` unbind the lot — an unbound key in fzf
// types its character — and `esc` rebinds them. The prompt doubles as the mode flag:
// fzf hands `$FZF_PROMPT` to a `transform` binding, which is what lets one key
// mean two things, so `ctrl-u` can still clear the query while typing and scroll
// half a page in normal mode — the two things ctrl-u does in vim.
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
	{"j", "down"},
	{"k", "up"},
	{"g", "first"},
	{"G", "last"},
	{"q", "abort"},
	{"i", leaveNormal},
	{"a", leaveNormal},
	{"/", leaveNormal},
	// D and C are how you get rid of the query the picker seeded — the repo
	// filter — without leaving normal mode first. As in vim, C also drops you
	// into typing, which is usually why you cleared it.
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
const ignoredInNormal = "bcdefhlmnoprstuvwxyzABEFHIJKLMNOPQRSTUVWXYZ0123456789"

// enterNormal and leaveNormal are placeholders the bindings fill in: the key
// sets can only be spelled out once the whole list is known.
const (
	enterNormal = "\x00enter-normal"
	leaveNormal = "\x00leave-normal"
)

// pickerBindings is the --bind flags that give the picker its vim mode.
func pickerBindings() []string {
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
	resolve := strings.NewReplacer(enterNormal, enter, leaveNormal, leave).Replace

	// The picker opens in normal mode: the keys are live as bound below, and the
	// prompt is set here as well as with --prompt, since the prompt is what every
	// mode test reads — it has to say "normal" from the first keypress.
	binds := []string{"--bind=start:" + fmt.Sprintf("change-prompt(%s)", normalPrompt)}
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
