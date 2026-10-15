package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v3/internal/buffer"
)

// searchBody has beta at 0:6, 2:0 and 3:8.
const searchBody = "alpha beta\ngamma\nbeta delta\nepsilon beta\n"

var (
	ctrlFKey = tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl}
	ctrlAKey = tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
)

// searchEditor opens a vim editor on content with the cursor at p.
func searchEditor(content string, p buffer.Pos) *EditorView {
	e := vimEditor(nil, content, 0)
	e.buf.MoveTo(p, false)
	return e
}

// searchTo types a search command and presses Enter.
func searchTo(e *EditorView, cmd string) {
	vimType(e, cmd)
	e.Update(enterKey)
}

func wantCursor(t *testing.T, e *EditorView, want buffer.Pos) {
	t.Helper()
	if got := e.buf.Cursor(); got != want {
		t.Fatalf("cursor %v, want %v", got, want)
	}
}

func TestVimSearchParse(t *testing.T) {
	for _, tc := range []struct {
		tokens []string
		want   vimCmd
		st     vimParse
	}{
		{[]string{"/"}, vimCmd{key: "/"}, vimDone},
		{[]string{"?"}, vimCmd{key: "?"}, vimDone},
		{[]string{"n"}, vimCmd{key: "n"}, vimDone},
		{[]string{"N"}, vimCmd{key: "N"}, vimDone},
		{[]string{"3", "n"}, vimCmd{count: 3, key: "n"}, vimDone},
		{[]string{"2", "N"}, vimCmd{count: 2, key: "N"}, vimDone},
		{[]string{"2", "/"}, vimCmd{}, vimBad},
		{[]string{"2", "?"}, vimCmd{}, vimBad},
		{[]string{"d", "/"}, vimCmd{}, vimBad},
		{[]string{"d", "n"}, vimCmd{}, vimBad},
	} {
		got, st := parseVim(tc.tokens, false)
		if got != tc.want || st != tc.st {
			t.Errorf("parseVim(%q) = %+v, %v; want %+v, %v", tc.tokens, got, st, tc.want, tc.st)
		}
	}
	for _, tok := range []string{"/", "?", "n", "N"} {
		if _, st := parseVim([]string{tok}, true); st != vimBad {
			t.Errorf("visual %q parses %v, want vimBad", tok, st)
		}
	}
}

func TestVimSearchForward(t *testing.T) {
	e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 6})
	vimType(e, "/beta")
	e.Update(enterKey)
	wantCursor(t, e, buffer.Pos{Line: 2, Col: 0})
	if e.find != nil {
		t.Fatal("the bar stays open after Enter")
	}
	if e.lastQuery != "beta" || e.vim.searchDir != 1 {
		t.Fatalf("lastQuery %q searchDir %d, want beta and 1", e.lastQuery, e.vim.searchDir)
	}
}

func TestVimSearchBackward(t *testing.T) {
	e := searchEditor(searchBody, buffer.Pos{Line: 2, Col: 0})
	vimType(e, "?beta")
	e.Update(enterKey)
	wantCursor(t, e, buffer.Pos{Line: 0, Col: 6})
	if e.find != nil {
		t.Fatal("the bar stays open after Enter")
	}
	if e.lastQuery != "beta" || e.vim.searchDir != -1 {
		t.Fatalf("lastQuery %q searchDir %d, want beta and -1", e.lastQuery, e.vim.searchDir)
	}
}

func TestVimSearchNext(t *testing.T) {
	t.Run("n and N", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 6})
		searchTo(e, "/beta")
		vimType(e, "n")
		wantCursor(t, e, buffer.Pos{Line: 3, Col: 8})
		vimType(e, "N")
		wantCursor(t, e, buffer.Pos{Line: 2, Col: 0})
	})
	t.Run("count wraps", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 6})
		searchTo(e, "/beta")
		vimType(e, "3n")
		wantCursor(t, e, buffer.Pos{Line: 2, Col: 0})
		if e.notice != "search wrapped" {
			t.Fatalf("notice %q, want search wrapped", e.notice)
		}
	})
	t.Run("no wrap no notice", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 6})
		searchTo(e, "/beta")
		vimType(e, "n")
		if e.notice != "" {
			t.Fatalf("notice %q, want none", e.notice)
		}
	})
	t.Run("n follows ? backwards", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 2, Col: 0})
		searchTo(e, "?beta")
		wantCursor(t, e, buffer.Pos{Line: 0, Col: 6})
		vimType(e, "n")
		wantCursor(t, e, buffer.Pos{Line: 3, Col: 8})
		if e.notice != "search wrapped" {
			t.Fatalf("notice %q, want search wrapped", e.notice)
		}
		vimType(e, "N")
		wantCursor(t, e, buffer.Pos{Line: 0, Col: 6})
	})
}

func TestVimSearchEdges(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 1, Col: 2})
		searchTo(e, "/zzz")
		if e.find != nil {
			t.Fatal("the bar stays open")
		}
		wantCursor(t, e, buffer.Pos{Line: 1, Col: 2})
		if e.notice != "pattern not found: zzz" {
			t.Fatalf("notice %q", e.notice)
		}
	})
	t.Run("incremental and esc", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 0})
		vimType(e, "/gam")
		wantCursor(t, e, buffer.Pos{Line: 1, Col: 0})
		if e.find == nil {
			t.Fatal("the bar is closed while typing")
		}
		e.Update(escKey)
		if e.find != nil {
			t.Fatal("esc leaves the bar open")
		}
		wantCursor(t, e, buffer.Pos{Line: 0, Col: 0})
	})
	t.Run("incremental backward", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 3, Col: 0})
		vimType(e, "?gam")
		wantCursor(t, e, buffer.Pos{Line: 1, Col: 0})
	})
	t.Run("empty field reuses the last query", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 6})
		searchTo(e, "/beta")
		searchTo(e, "/")
		wantCursor(t, e, buffer.Pos{Line: 3, Col: 8})
		if e.find != nil {
			t.Fatal("the bar stays open")
		}
	})
	t.Run("empty field without a query", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 1, Col: 1})
		searchTo(e, "/")
		wantCursor(t, e, buffer.Pos{Line: 1, Col: 1})
		if e.find != nil || e.notice != "no previous search" {
			t.Fatalf("find %v notice %q", e.find, e.notice)
		}
	})
	t.Run("tab does not show replace", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 0})
		vimType(e, "/be")
		for _, k := range []tea.KeyPressMsg{tabKey, shiftTabKey, ctrlAKey} {
			e.Update(k)
		}
		if e.find == nil || e.find.replaceShown || e.find.field != findField {
			t.Fatalf("bar after tab, shift+tab, ctrl+a: %+v", e.find)
		}
		if got := e.find.hints(); got != "↵ jump · esc cancel" {
			t.Fatalf("hints %q", got)
		}
		if e.Content() != searchBody {
			t.Fatal("ctrl+a replaced text")
		}
	})
	t.Run("n without a query", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 0})
		vimType(e, "n")
		if e.notice != "no previous search" {
			t.Fatalf("notice %q", e.notice)
		}
		wantCursor(t, e, buffer.Pos{Line: 0, Col: 0})
	})
	t.Run("ctrl+f keeps the modeless bar", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 0})
		e.Update(ctrlFKey)
		if e.find == nil || e.find.dir != 0 {
			t.Fatalf("ctrl+f bar %+v, want dir 0", e.find)
		}
		if got := e.find.hints(); got != "↵ next · ↑ prev · tab replace · esc close" {
			t.Fatalf("hints %q", got)
		}
		vimType(e, "beta")
		wantCursor(t, e, buffer.Pos{Line: 0, Col: 6})
		e.Update(enterKey)
		wantCursor(t, e, buffer.Pos{Line: 2, Col: 0})
		if e.find == nil {
			t.Fatal("Enter closes the modeless bar")
		}
		e.Update(escKey)
		if e.find != nil {
			t.Fatal("esc leaves the bar open")
		}
	})
	t.Run("cursor stays on a character", func(t *testing.T) {
		e := searchEditor("x\nend beta\n", buffer.Pos{Line: 0, Col: 0})
		searchTo(e, "/beta")
		if cur := e.buf.Cursor(); cur.Col > e.buf.LineLast(cur).Col {
			t.Fatalf("cursor %v past the last character", cur)
		}
	})
	t.Run("live match is strictly after the cursor", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 6})
		vimType(e, "/beta")
		wantCursor(t, e, buffer.Pos{Line: 2, Col: 0})
		if e.find == nil {
			t.Fatal("the bar closed while typing")
		}
	})
	t.Run("esc keeps the previous search", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 6})
		searchTo(e, "/beta")
		vimType(e, "/gam")
		e.Update(escKey)
		if e.lastQuery != "beta" {
			t.Fatalf("lastQuery %q after an esc-cancelled search, want beta", e.lastQuery)
		}
		vimType(e, "n")
		wantCursor(t, e, buffer.Pos{Line: 3, Col: 8})
	})
	t.Run("no match keeps the previous search", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 6})
		searchTo(e, "/beta")
		searchTo(e, "/zzz")
		if e.lastQuery != "beta" {
			t.Fatalf("lastQuery %q after a failed search, want beta", e.lastQuery)
		}
		vimType(e, "n")
		wantCursor(t, e, buffer.Pos{Line: 3, Col: 8}) // the failed search left the cursor on the match
	})
	t.Run("modeless esc still remembers the query", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 0})
		e.Update(ctrlFKey)
		vimType(e, "gam")
		e.Update(escKey)
		if e.lastQuery != "gam" {
			t.Fatalf("lastQuery %q, want gam", e.lastQuery)
		}
	})
	t.Run("search keys are not a recorded change", func(t *testing.T) {
		e := searchEditor(searchBody, buffer.Pos{Line: 0, Col: 0})
		vimType(e, "x")
		searchTo(e, "/beta")
		vimType(e, "n")
		if e.vim.last == nil || e.vim.last.cmd.key != "x" {
			t.Fatalf("last change %+v, want x", e.vim.last)
		}
	})
}
