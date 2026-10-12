package layout

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func write(t *testing.T, content string) string {
	t.Helper()
	g := t.TempDir()
	if err := os.MkdirAll(filepath.Join(g, ".weft"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g, ".weft", "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return g
}

func mustLoad(t *testing.T, content string) Layout {
	t.Helper()
	l, err := Load(write(t, content))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return l
}

func TestDefault(t *testing.T) {
	l := Default()
	if l.PagesDir() != "pages" || l.JournalsDir() != "journals" || l.Namespace() != TripleLowbar {
		t.Fatalf("defaults: %q %q %q", l.PagesDir(), l.JournalsDir(), l.Namespace())
	}
	d := day(2026, 10, 12)
	if got := l.JournalName(d); got != "2026_10_12" {
		t.Errorf("JournalName = %q", got)
	}
	if got := l.JournalRel(d); got != "journals/2026_10_12.md" {
		t.Errorf("JournalRel = %q", got)
	}
	if got, err := l.NewPageRel("A/B"); err != nil || got != "pages/A___B.md" {
		t.Errorf("NewPageRel = %q, %v", got, err)
	}
	if got := l.Roots(); !reflect.DeepEqual(got, []string{"pages", "journals"}) {
		t.Errorf("Roots = %v", got)
	}
}

func TestLoadMissingFileIsDefault(t *testing.T) {
	for name, g := range map[string]string{"missing": t.TempDir(), "empty": write(t, "")} {
		t.Run(name, func(t *testing.T) {
			l, err := Load(g)
			if err != nil {
				t.Fatal(err)
			}
			d := Default()
			if l.PagesDir() != d.PagesDir() || l.JournalsDir() != d.JournalsDir() ||
				l.Namespace() != d.Namespace() || l.JournalRel(day(2026, 1, 2)) != d.JournalRel(day(2026, 1, 2)) {
				t.Errorf("not default: %+v", l)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct{ name, content, problem string }{
		{"syntax", "pages_dir = ", "toml: "},
		{"wrong type", "pages_dir = 3", "toml: line 1"},
		{"duplicate key", "pages_dir = \"a\"\npages_dir = \"b\"", "toml: line 2"},
		{"unknown key", "bogus = 1", `unknown key "bogus"`},
		{"unknown table", "[x]\nk = 1", `unknown key "x"`},
		{"abs pages", `pages_dir = "/abs"`, `pages_dir: "/abs" is outside the graph`},
		{"parent pages", `pages_dir = "../x"`, `pages_dir: "../x" is outside the graph`},
		{"parent journals", `journals_dir = "a/../../x"`, `journals_dir: "a/../../x" is outside the graph`},
		{"dotdot", `journals_dir = ".."`, `journals_dir: ".." is outside the graph`},
		{"namespace", `namespace_files = "x"`, `namespace_files: "x" must be "triple-lowbar" or "folders"`},
		{"empty pattern", `journal_file_pattern = ""`, `journal_file_pattern: "" must not be empty`},
		{"empty segment", `journal_file_pattern = "YYYY//MM-DD"`, `journal_file_pattern: "YYYY//MM-DD" has an empty, "." or ".." path segment`},
		{"dotdot segment", `journal_file_pattern = "../YYYY-MM-DD"`, `journal_file_pattern: "../YYYY-MM-DD" has an empty, "." or ".." path segment`},
		{"dot segment", `journal_file_pattern = "./YYYY-MM-DD"`, `has an empty, "." or ".." path segment`},
		{"hidden segment", `journal_file_pattern = ".hidden/YYYY-MM-DD"`, `journal_file_pattern: ".hidden/YYYY-MM-DD" has a path segment starting with "."`},
		{"last segment lacks DD", `journal_file_pattern = "YYYY/MM"`, `journal_file_pattern: "YYYY/MM" the last path segment must contain YYYY, MM and DD`},
		{"no DD", `journal_file_pattern = "YYYY-MM"`, `the last path segment must contain YYYY, MM and DD`},
		{"first failure wins", "pages_dir = \"/abs\"\nnamespace_files = \"x\"", `pages_dir: "/abs"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := write(t, c.content)
			_, err := Load(g)
			if err == nil {
				t.Fatal("want error")
			}
			cfg := filepath.Join(g, ".weft", "config.toml")
			if !strings.HasPrefix(err.Error(), cfg+": ") || !strings.Contains(err.Error(), c.problem) {
				t.Errorf("error %q lacks %q / %q", err, cfg, c.problem)
			}
		})
	}
	t.Run("unreadable", func(t *testing.T) {
		g := t.TempDir()
		cfg := filepath.Join(g, ".weft", "config.toml")
		if err := os.MkdirAll(cfg, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Load(g)
		if err == nil || !strings.HasPrefix(err.Error(), cfg+": ") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestLoadObsidianConfig(t *testing.T) {
	l := mustLoad(t, "pages_dir = \".\"\njournals_dir = \"Daily/\"\njournal_file_pattern = \"YYYY-MM-DD\"\nnamespace_files = \"folders\"\n")
	if l.PagesDir() != "" || l.JournalsDir() != "Daily" {
		t.Fatalf("dirs %q %q", l.PagesDir(), l.JournalsDir())
	}
	if got := l.Roots(); !reflect.DeepEqual(got, []string{""}) {
		t.Errorf("Roots = %q", got)
	}
	if got, err := l.NewPageRel("a/b"); err != nil || got != "a/b.md" {
		t.Errorf("NewPageRel = %q, %v", got, err)
	}
	if got := l.JournalRel(day(2026, 5, 24)); got != "Daily/2026-05-24.md" {
		t.Errorf("JournalRel = %q", got)
	}
}

func TestRoots(t *testing.T) {
	cases := []struct {
		name, cfg string
		want      []string
	}{
		{"disjoint", "pages_dir = \"p\"\njournals_dir = \"j\"", []string{"p", "j"}},
		{"journals inside pages", "pages_dir = \"p\"\njournals_dir = \"p/j\"", []string{"p"}},
		{"pages inside journals", "pages_dir = \"j/p\"\njournals_dir = \"j\"", []string{"j"}},
		{"pages inside root journals", "journals_dir = \"\"", []string{""}},
		{"equal", "pages_dir = \"x\"\njournals_dir = \"x\"", []string{"x"}},
		{"both root", "pages_dir = \".\"\njournals_dir = \".\"", []string{""}},
		{"prefix is not containment", "pages_dir = \"p\"\njournals_dir = \"pp\"", []string{"p", "pp"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mustLoad(t, c.cfg).Roots(); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Roots = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSkipDir(t *testing.T) {
	d := Default()
	if !d.SkipDir(".obsidian") || !d.SkipDir("pages/.git") {
		t.Error("dot folders must be skipped")
	}
	if d.SkipDir("pages") || d.SkipDir("pages/sub") {
		t.Error("plain folders must not be skipped")
	}
	l := mustLoad(t, `journals_dir = "pages/.daily"`)
	if l.SkipDir("pages/.daily") || l.SkipDir("pages") {
		t.Error("journals dir and its ancestors must not be skipped")
	}
	if !l.SkipDir("pages/.other") {
		t.Error("sibling dot folder must be skipped")
	}
	h := mustLoad(t, "pages_dir = \".hid/pages\"")
	if h.SkipDir(".hid") || h.SkipDir(".hid/pages") {
		t.Error("pages dir ancestors must not be skipped")
	}
}

func TestNewPageRelFolders(t *testing.T) {
	l := mustLoad(t, `namespace_files = "folders"`)
	for _, name := range []string{"", "../x", "a//b", "a/./b", ".git/x", "/abs"} {
		_, err := l.NewPageRel(name)
		if err == nil || !strings.Contains(err.Error(), "cannot place page "+`"`+name+`"`) {
			t.Errorf("NewPageRel(%q) err = %v", name, err)
		}
	}
	if got, err := l.NewPageRel(".x"); err != nil || got != "pages/.x.md" {
		t.Errorf("NewPageRel(.x) = %q, %v", got, err)
	}
	if got, err := l.NewPageRel("a/b"); err != nil || got != "pages/a/b.md" {
		t.Errorf("NewPageRel(a/b) = %q, %v", got, err)
	}
}

func TestNewPageRelTripleLowbar(t *testing.T) {
	l := Default()
	if got, err := l.NewPageRel("../x"); err != nil || got != "pages/..___x.md" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := l.NewPageRel(""); err == nil {
		t.Error("empty name must fail")
	}
}
