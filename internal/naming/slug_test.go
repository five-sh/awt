package naming

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"feature x":   "feature-x",
		"Fix/Login!!": "Fix-Login",
		"  spaced  ":  "spaced",
		"café-launch": "café-launch",
		"../../etc":   "etc",
		"a..b":        "a.b",
		"---":         "",
		".":           "",
		"..":          "",
	}
	for in, want := range cases {
		got, err := Slugify(in)
		if want == "" {
			if err == nil {
				t.Errorf("Slugify(%q) = %q, want error", in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Slugify(%q) unexpected error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSessionName(t *testing.T) {
	got := SessionName("my.repo", "fix:bug")
	want := "my-repo--fix-bug"
	if got != want {
		t.Errorf("SessionName = %q, want %q", got, want)
	}
}

func TestDisambiguate(t *testing.T) {
	taken := map[string]bool{"repo--wt": true}
	got := Disambiguate("repo--wt", func(n string) bool { return taken[n] })
	if got == "repo--wt" {
		t.Errorf("Disambiguate did not rename a taken session")
	}
	got2 := Disambiguate("repo--free", func(n string) bool { return taken[n] })
	if got2 != "repo--free" {
		t.Errorf("Disambiguate renamed a free session: %q", got2)
	}
}
