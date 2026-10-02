package command

import (
	"strings"
	"testing"
)

// bindings maps each key to the action pickerBindings gave it.
func bindings(t *testing.T) map[string]string {
	t.Helper()
	out := make(map[string]string)
	for _, b := range pickerBindings() {
		spec, ok := strings.CutPrefix(b, "--bind=")
		if !ok {
			t.Fatalf("binding %q doesn't start with --bind=", b)
		}
		key, action, ok := strings.Cut(spec, ":")
		if !ok {
			t.Fatalf("binding %q has no action", b)
		}
		if _, dup := out[key]; dup {
			t.Errorf("key %q bound twice; fzf keeps only one binding per key", key)
		}
		out[key] = action
	}
	return out
}

// The picker opens in insert mode: start does what i does, unbinding the
// normal-mode keys and setting the prompt the modal bindings read.
func TestPickerStartsInInsertMode(t *testing.T) {
	b := bindings(t)
	start, ok := b["start"]
	if !ok {
		t.Fatal("no start binding: nothing would set the prompt the mode tests read")
	}
	if start != b["i"] {
		t.Errorf("start = %q, want it to leave normal mode like i does (%q)", start, b["i"])
	}
	if !strings.HasPrefix(start, "unbind(") || !strings.Contains(start, "change-prompt("+insertPrompt+")") {
		t.Errorf("start = %q, want the normal-mode keys unbound from the off", start)
	}
	// Leaving normal mode has to unbind every key that would otherwise swallow a
	// character of what you type.
	leave := b["i"]
	for _, k := range normalKeys {
		if !strings.Contains(leave, "("+k.key+",") && !strings.Contains(leave, ","+k.key+",") &&
			!strings.Contains(leave, ","+k.key+")") {
			t.Errorf("key %q acts in normal mode but i doesn't unbind it", k.key)
		}
	}
	for _, r := range ignoredInNormal {
		if !strings.Contains(leave, string(r)) {
			t.Errorf("key %q is ignored in normal mode but i doesn't unbind it", string(r))
		}
	}
}

func TestPickerBindingsNavigation(t *testing.T) {
	b := bindings(t)
	for key, want := range map[string]string{
		"j": "down", "k": "up", "g": "first", "G": "last", "q": "abort",
		"D": "clear-query",
	} {
		if b[key] != want {
			t.Errorf("%q = %q, want %q", key, b[key], want)
		}
	}
	// C clears the query and drops straight into typing, as it does in vim.
	if c := b["C"]; !strings.HasPrefix(c, "clear-query+") || !strings.Contains(c, "unbind(") {
		t.Errorf("C = %q, want it to clear the query and leave normal mode", c)
	}
	// i, a and / go back to typing: they rebind nothing and restore the prompt.
	for _, key := range []string{"i", "a", "/"} {
		if !strings.HasPrefix(b[key], "unbind(") ||
			!strings.Contains(b[key], "change-prompt("+insertPrompt+")") {
			t.Errorf("%q = %q, want it to leave normal mode", key, b[key])
		}
	}
}

// esc, and the ctrl keys that vim gives a job in both modes, have to be
// transforms: fzf's unbind leaves a key dead rather than restoring its default,
// so unbinding ctrl-u in insert mode would lose "clear the query".
func TestPickerBindingsModalKeys(t *testing.T) {
	b := bindings(t)
	for _, m := range modalKeys {
		got := b[m.key]
		if !strings.HasPrefix(got, "transform:") {
			t.Errorf("%q = %q, want a transform so it can act per mode", m.key, got)
		}
		if !strings.Contains(got, m.normal) {
			t.Errorf("%q = %q, want the normal-mode action %q", m.key, got, m.normal)
		}
		if !strings.Contains(got, normalPrompt) {
			t.Errorf("%q = %q, want it to test the prompt for the mode", m.key, got)
		}
	}
	// esc enters normal mode from insert, and aborts when already there.
	esc := b["esc"]
	if !strings.Contains(esc, "rebind(") || !strings.Contains(esc, "change-prompt("+normalPrompt+")") {
		t.Errorf("esc = %q, want it to enter normal mode", esc)
	}
	if !strings.Contains(esc, "abort") {
		t.Errorf("esc = %q, want a second press to abort", esc)
	}
	// ctrl-u keeps clearing the query while typing, which the header promises.
	if !strings.Contains(b["ctrl-u"], "unix-line-discard") {
		t.Errorf("ctrl-u = %q, want it to still clear the query in insert mode", b["ctrl-u"])
	}
}

// A key with a meaning must not also be in the ignore list, or it would be
// bound twice and fzf would keep only one of them.
func TestPickerBindingsNoIgnoredKeyHasAMeaning(t *testing.T) {
	meaning := make(map[string]bool)
	for _, k := range normalKeys {
		meaning[k.key] = true
	}
	for _, r := range ignoredInNormal {
		if meaning[string(r)] {
			t.Errorf("key %q is both ignored and bound to an action", string(r))
		}
	}
}
