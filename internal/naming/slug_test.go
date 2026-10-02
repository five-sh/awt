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

func TestSlugifyBranch(t *testing.T) {
	cases := map[string]string{
		"codex/fix login!!": "codex/fix-login",
		"Fix/Login!!":       "Fix/Login",
		"feature x":         "feature-x",
	}
	for in, want := range cases {
		got, err := SlugifyBranch(in)
		if err != nil {
			t.Errorf("SlugifyBranch(%q) unexpected error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("SlugifyBranch(%q) = %q, want %q", in, got, want)
		}
	}

	badCases := []string{"/codex", "codex/", "codex//foo", ""}
	for _, in := range badCases {
		if _, err := SlugifyBranch(in); err == nil {
			t.Errorf("SlugifyBranch(%q) = nil error, want error", in)
		}
	}
}

func TestWindowName(t *testing.T) {
	cases := map[[2]string]string{
		{"pair-be", "codex-auth"}: "pair-be:codex-auth",
		{"my:repo", "fix:bug"}:    "my-repo:fix-bug",
	}
	for in, want := range cases {
		if got := WindowName(in[0], in[1]); got != want {
			t.Errorf("WindowName(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
