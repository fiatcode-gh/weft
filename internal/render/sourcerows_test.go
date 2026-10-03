package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
)

// sourceRowOf returns the first styled row whose visible text contains substr.
func sourceRowOf(t *testing.T, res Result, substr string) int {
	t.Helper()
	for i, row := range strings.Split(res.Styled, "\n") {
		if strings.Contains(ansi.Strip(row), substr) {
			return i
		}
	}
	t.Fatalf("no row contains %q in:\n%s", substr, ansi.Strip(res.Styled))
	return -1
}

// mustSourceRows returns the real render of body plus its row map, failing when
// the map is nil.
func mustSourceRows(t *testing.T, body string, width int, emphasis string) (Result, []int) {
	t.Helper()
	res := mustRenderEmphasis(t, body, width, emphasis)
	rows := SourceRows(body, width, emphasis)
	if rows == nil {
		t.Fatal("SourceRows is nil")
	}
	return res, rows
}

func TestRenderCallsGlamourOnce(t *testing.T) {
	orig := glamourRender
	t.Cleanup(func() { glamourRender = orig })
	calls := 0
	glamourRender = func(r *glamour.TermRenderer, in string) (string, error) {
		calls++
		return orig(r, in)
	}
	if _, err := Render("- a\n- b\n", 80); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("Render called glamourRender %d times, want 1", calls)
	}
	calls = 0
	if _, err := RenderWithEmphasis("- a\n- b\n", 80, "x"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("RenderWithEmphasis called glamourRender %d times, want 1", calls)
	}
}

func TestSourceRowsCoverEveryStyledRow(t *testing.T) {
	body := "# Heading\n\nline one\nline two\n\n- see [[Alpha]] here\n  - nested\n- TODO task\n```go\nx := 1\n```\n> quote\n\n---\n\n1. first\n2. second\n"
	for _, emphasis := range []string{"", "first"} {
		t.Run("emphasis="+emphasis, func(t *testing.T) {
			res, rows := mustSourceRows(t, body, 40, emphasis)
			if want := strings.Count(res.Styled, "\n") + 1; len(rows) != want {
				t.Fatalf("len(rows) = %d, want %d", len(rows), want)
			}
			lines := strings.Count(body, "\n") + 1
			prev := 0
			for i, v := range rows {
				if v < 0 || v >= lines {
					t.Errorf("row %d maps to %d, outside [0,%d)", i, v, lines)
				}
				if v < prev {
					t.Errorf("row %d maps to %d, below previous %d", i, v, prev)
				}
				prev = v
			}
			if rowTagRe.MatchString(res.Styled) {
				t.Error("row tag leaked into Styled")
			}
		})
	}
}

func TestSourceRowsSkipStrippedBlocks(t *testing.T) {
	tests := []struct {
		name, body, find string
		want             int
	}{
		{"logbook", "- a normal bullet\n  :LOGBOOK:\n  CLOCK: x\n  :END:\n- another bullet\n", "another bullet", 4},
		{"query", "- before\n{{query (todo now)\n}}\n- after\n", "after", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, rows := mustSourceRows(t, tt.body, 80, "")
			if got := rows[sourceRowOf(t, res, tt.find)]; got != tt.want {
				t.Errorf("%q maps to %d, want %d", tt.find, got, tt.want)
			}
		})
	}
}

func TestSourceRowsTrackFencedCode(t *testing.T) {
	res, rows := mustSourceRows(t, "- intro\n```go\nx := 1\ny := 2\n```\n- after\n", 80, "")
	for find, want := range map[string]int{"x := 1": 2, "y := 2": 3, "after": 5} {
		if got := rows[sourceRowOf(t, res, find)]; got != want {
			t.Errorf("%q maps to %d, want %d", find, got, want)
		}
	}
}

func TestSourceRowsTrackWrappedBullet(t *testing.T) {
	res, rows := mustSourceRows(t, "- This bullet has enough text that Glamour will wrap it across two lines for sure.\n- next\n", 40, "")
	first, next := sourceRowOf(t, res, "This bullet"), sourceRowOf(t, res, "next")
	if next-first < 2 {
		t.Fatalf("bullet did not wrap: rows %d..%d", first, next)
	}
	for i := first; i < next; i++ {
		if rows[i] != 0 {
			t.Errorf("row %d maps to %d, want 0", i, rows[i])
		}
	}
	if got := rows[next]; got != 1 {
		t.Errorf("next maps to %d, want 1", got)
	}
}

func TestSourceRowsTrackLinkOnlyBullets(t *testing.T) {
	res, rows := mustSourceRows(t, "- intro\n- [[Alpha]]\n- [text](https://example.com)\n- TODO [[Beta]]\n- outro\n", 80, "")
	for find, want := range map[string]int{"Alpha": 1, "text": 2, "Beta": 3, "outro": 4} {
		if got := rows[sourceRowOf(t, res, find)]; got != want {
			t.Errorf("%q maps to %d, want %d", find, got, want)
		}
	}
}

func TestSourceRowsRealignAfterLayoutDivergence(t *testing.T) {
	res, rows := mustSourceRows(t, "- before\n\n[foo]: http://example.com/a/very/long/path/that/wraps/around\n\n- text [foo]\n- after one\n- after two\n", 40, "")
	if want := strings.Count(res.Styled, "\n") + 1; len(rows) != want {
		t.Fatalf("len(rows) = %d, want %d", len(rows), want)
	}
	for find, want := range map[string]int{
		"before":          0,
		"http://example.": 4,
		"wraps/around":    4,
		"after one":       5,
		"after two":       6,
	} {
		if got := rows[sourceRowOf(t, res, find)]; got != want {
			t.Errorf("%q maps to %d, want %d", find, got, want)
		}
	}
}

func TestSourceRowsOnGlamourFallback(t *testing.T) {
	orig := glamourRender
	t.Cleanup(func() { glamourRender = orig })
	glamourRender = func(*glamour.TermRenderer, string) (string, error) {
		return "", errors.New("boom")
	}
	body := "- a\n  :LOGBOOK:\n  x\n  :END:\n- b\n"
	res := mustRender(t, body, 80)
	rows := SourceRows(body, 80, "")
	want := []int{0, 4, 5}
	if len(rows) != len(want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("rows = %v, want %v", rows, want)
		}
	}
	if got := strings.Count(res.Styled, "\n") + 1; len(rows) != got {
		t.Errorf("len(rows) = %d, Styled has %d rows", len(rows), got)
	}
}

func TestSourceRowsNilWhenTaggedRenderFails(t *testing.T) {
	orig := glamourRender
	t.Cleanup(func() { glamourRender = orig })
	glamourRender = func(r *glamour.TermRenderer, in string) (string, error) {
		if strings.Contains(in, "\x1b") {
			return "", errors.New("boom")
		}
		return orig(r, in)
	}
	if rows := SourceRows("- a\n- b\n", 80, ""); rows != nil {
		t.Errorf("SourceRows = %v, want nil", rows)
	}
	res := mustRender(t, "- a\n- b\n", 80)
	if res.FallbackErr != nil {
		t.Errorf("FallbackErr = %v, want nil", res.FallbackErr)
	}
	if res.Styled == "" {
		t.Error("Styled is empty")
	}
}

func TestTagSourceLinesKeepsGlamourLayout(t *testing.T) {
	long := strings.Repeat("word ", 30)
	bodies := map[string]string{
		"multiline emphasis":  "*\"quoted\nacross\"*\n",
		"multiline link":      "[link\ntext](#)\n",
		"setext":              "Title\n=====\n",
		"table":               "| a | b |\n|---|---|\n| 1 | 2 |\n",
		"hard breaks":         "one  \ntwo\\\nthree\n",
		"atx closing":         "# T #\n",
		"empty bullet":        "-\n",
		"bare hash":           "#\n",
		"indented code":       "para\n\n    code line\n    more\n",
		"tilde fence":         "~~~\n```\ninner\n~~~\n",
		"long code line":      "```go\nx := \"" + long + "\"\n```\n",
		"html block":          "<div>\nhello\n</div>\n",
		"image":               "![alt text](img.png)\n",
		"autolink":            "<http://x.com>\n",
		"inline html":         "<b>bold</b> text\n",
		"cjk bullet":          "- " + strings.Repeat("日本語のテキスト", 12) + "\n",
		"ordered bare marker": "1.\n2) \n",
	}
	for name, body := range bodies {
		for _, width := range []int{40, 80} {
			t.Run(name, func(t *testing.T) {
				r, err := rendererFor(width)
				if err != nil {
					t.Fatal(err)
				}
				src := make([]int, strings.Count(body, "\n")+1)
				for i := range src {
					src[i] = i
				}
				want, err := glamourRender(r, body)
				if err != nil {
					t.Fatal(err)
				}
				got, err := glamourRender(r, tagSourceLines(body, src))
				if err != nil {
					t.Fatal(err)
				}
				wantRows, gotRows := strings.Split(want, "\n"), strings.Split(got, "\n")
				if len(wantRows) != len(gotRows) {
					t.Fatalf("width %d: %d rows, want %d", width, len(gotRows), len(wantRows))
				}
				for i := range wantRows {
					if g, w := ansi.Strip(gotRows[i]), ansi.Strip(wantRows[i]); g != w {
						t.Fatalf("width %d row %d:\n got %q\nwant %q", width, i, g, w)
					}
				}
			})
		}
	}
}
