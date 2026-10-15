package graph

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fiatcode-gh/weft/v3/internal/layout"
)

func TestLinkSpans(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []linkSpan
	}{
		{"wiki", "[[A]]", []linkSpan{{"A", 1, 0, 5, 2, 3, linkWiki}}},
		{"padded alias", "[[ A |x]]", []linkSpan{{"A", 1, 0, 9, 2, 5, linkWiki}}},
		{"tag", "#tag", []linkSpan{{"tag", 1, 0, 4, 1, 4, linkTag}}},
		{"bracket tag", "#[[B C]]", []linkSpan{{"B C", 1, 0, 8, 3, 6, linkBracketTag}}},
		{"bracket tag alias", "x #[[B|al]]", []linkSpan{{"B", 1, 2, 11, 5, 6, linkBracketTag}}},
		{"inline code", "`[[code]]`", nil},
		{"unpaired backtick", "` [[A]]", nil},
		{"tag not at tag start", "[[A]]#t", []linkSpan{{"A", 1, 0, 5, 2, 3, linkWiki}}},
		{"markdown fragment", "[x](#frag)", nil},
		{"blank bracket tag", "#[[ ]]", nil},
		{"mixed order", "#t [[A]] #u", []linkSpan{
			{"t", 1, 0, 2, 1, 2, linkTag},
			{"A", 1, 3, 8, 5, 6, linkWiki},
			{"u", 1, 9, 11, 10, 11, linkTag},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendLineLinks(nil, tt.line, 1)
			if !slices.Equal(got, tt.want) {
				t.Errorf("appendLineLinks(%q) = %+v, want %+v", tt.line, got, tt.want)
			}
		})
	}
}

func rewriteTarget(t *testing.T, idx *Index, name string) *PageMeta {
	t.Helper()
	p, ok := idx.Resolve(name)
	if !ok {
		t.Fatalf("page %q not found", name)
	}
	return p
}

func TestRewriteLinksForms(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{
		"Old.md":   "# Old\n\n## Plan\n\nself [[Old]]\n",
		"Other.md": "# Other\n",
	})
	old := rewriteTarget(t, idx, "Old")
	tests := []struct {
		name, key, in, want string
		n                   int
	}{
		{"exact", "New", "[[Old]]", "[[New]]", 1},
		{"case", "New", "[[old]]", "[[New]]", 1},
		{"alias kept", "New", "[[OLD|alias]]", "[[New|alias]]", 1},
		{"padding dropped", "New", "[[ Old ]]", "[[New]]", 1},
		{"fragment kept", "New", "[[Old#Plan]]", "[[New#Plan]]", 1},
		{"fragment and alias", "New", "[[Old#Plan|al]]", "[[New#Plan|al]]", 1},
		{"path link", "New", "[[pages/Old]]", "[[New]]", 1},
		{"tag", "New", "#Old", "#New", 1},
		{"tag case", "New", "#old", "#New", 1},
		{"bracket tag stays bracket", "New", "#[[Old]]", "#[[New]]", 1},
		{"bracket tag trailing text", "New", "#[[Old]]x", "#[[New]]x", 1},
		{"bracket tag alias", "New", "#[[Old|al]]", "#[[New|al]]", 1},
		{"bracket tag fragment", "New", "#[[Old#Plan]]", "#[[New#Plan]]", 1},
		{"tag to spaced key", "New Page", "#Old", "#[[New Page]]", 1},
		{"bracket tag to spaced key", "New Page", "#[[Old]]", "#[[New Page]]", 1},
		{"inline code", "New", "`[[Old]]`", "`[[Old]]`", 0},
		{"fenced", "New", "```\n[[Old]]\n```", "```\n[[Old]]\n```", 0},
		{"bare mention", "New", "Old", "Old", 0},
		{"unresolved fragment", "New", "[[Old#Missing]]", "[[Old#Missing]]", 0},
		{"other page", "New", "[[Other]]", "[[Other]]", 0},
		{"several, CRLF", "New", "a [[Old]] b #Old [[Other]] [[old|x]]\r\nz", "a [[New]] b #New [[Other]] [[New|x]]\r\nz", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n := idx.RewriteLinks(tt.in, old, tt.key)
			if got != tt.want || n != tt.n {
				t.Errorf("RewriteLinks(%q, %q) = %q, %d; want %q, %d", tt.in, tt.key, got, n, tt.want, tt.n)
			}
		})
	}
}

func TestRewriteLinksSelfLink(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{"Old.md": "# Old\n\n## Plan\n\nself [[Old]]\n"})
	old := rewriteTarget(t, idx, "Old")
	got, n := idx.RewriteLinks("# Old\n\n## Plan\n\nself [[Old]]\n", old, "New")
	if want := "# Old\n\n## Plan\n\nself [[New]]\n"; got != want || n != 1 {
		t.Errorf("got %q, %d; want %q, 1", got, n, want)
	}
}

func TestRewriteLinksPathLinkOnlyTargetsPage(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".weft/config.toml", "pages_dir = \"\"\nnamespace_files = \"folders\"\n")
	write("a/Note.md", "a\n")
	write("b/Note.md", "b\n")
	write("Home.md", "[[a/Note]] [[B/NOTE]] [[Note]]\n")
	lay, err := layout.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(dir, lay)
	if err != nil {
		t.Fatal(err)
	}
	loser := rewriteTarget(t, idx, "b/Note")
	got, n := idx.RewriteLinks("[[a/Note]] [[B/NOTE]] [[Note]]", loser, "Memo")
	if want := "[[a/Note]] [[Memo]] [[Note]]"; got != want || n != 1 {
		t.Errorf("got %q, %d; want %q, 1", got, n, want)
	}
}

func TestRewriteLinksNoMatchReturnsBodyUnchanged(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{"Old.md": "x\n", "Other.md": "y\n"})
	old := rewriteTarget(t, idx, "Old")
	in := "no links\r\n[[Other]]\n"
	if got, n := idx.RewriteLinks(in, old, "New"); got != in || n != 0 {
		t.Errorf("got %q, %d; want unchanged, 0", got, n)
	}
}

func TestLinksToPageMatchIndex(t *testing.T) {
	vault, err := filepath.Abs("../../testdata/obsidian-vault")
	if err != nil {
		t.Fatal(err)
	}
	vaultLay, err := layout.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		dir  string
		lay  layout.Layout
	}{
		{"fixture-graph", fixturePath(t), layout.Default()},
		{"obsidian-vault", vault, vaultLay},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx, err := BuildIndex(tc.dir, tc.lay)
			if err != nil {
				t.Fatal(err)
			}
			type file struct {
				key  string
				body string
			}
			var files []file
			extracted := 0
			for _, p := range idx.Pages {
				b, err := os.ReadFile(p.Path)
				if err != nil {
					t.Fatal(err)
				}
				extracted += len(ExtractLinks(string(b)))
				files = append(files, file{key: p.Key, body: string(b)})
			}
			total := 0
			for pi := range idx.Pages {
				page := &idx.Pages[pi]
				var got []Ref
				for _, f := range files {
					lines, links := idx.linksToPage(f.body, page)
					for _, l := range links {
						got = append(got, Ref{FromPage: f.key, LineNumber: l.line, Context: lineContext(lines, l.line)})
					}
				}
				want := idx.linksTo[page.Path]
				if len(got) != len(want) || (len(want) > 0 && !slices.Equal(got, want)) {
					t.Errorf("%s: linksToPage refs = %+v, index linksTo = %+v", page.Key, got, want)
				}
				total += len(got)
			}
			for _, refs := range idx.unresolved {
				total += len(refs)
			}
			if total != extracted {
				t.Errorf("resolved+unresolved = %d, ExtractLinks total = %d", total, extracted)
			}
			if extracted == 0 {
				t.Error("fixture has no links; parity proves nothing")
			}
		})
	}
}
