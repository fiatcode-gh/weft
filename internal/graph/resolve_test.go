package graph

import "testing"

func TestPageNameFromFilename(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Alpha.md", "Alpha"},
		{"proj___nested.md", "proj/nested"},
		{"a___b___c.md", "a/b/c"},
		{"2026_05_24.md", "2026-05-24"},
	}
	for _, c := range cases {
		got := PageNameFromFilename(c.in)
		if got != c.want {
			t.Errorf("PageNameFromFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsJournalFilename(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"2026_05_24.md", true},
		{"2026_5_24.md", false},
		{"Alpha.md", false},
		{"2026_05_24.txt", false},
	}
	for _, c := range cases {
		got := IsJournalFilename(c.in)
		if got != c.want {
			t.Errorf("IsJournalFilename(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsJournalPageName(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"2026-05-24", true},
		{"2026-01-01", true},
		{"2026-5-24", false},
		{"Alpha", false},
		{"", false},
		{"2026-05-24-extra", false},
		{"foo-2026-05-24", false},
	}
	for _, c := range cases {
		got := IsJournalPageName(c.in)
		if got != c.want {
			t.Errorf("IsJournalPageName(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
