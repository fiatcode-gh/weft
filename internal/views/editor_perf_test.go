package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// perfContent is n lines mixing bullets with wiki links and bold text, long
// wrapping lines, plain paragraphs and one fenced block.
func perfContent(n int) string {
	var b strings.Builder
	for i := range n {
		switch {
		case i == n/2:
			b.WriteString("```go\n")
		case i == n/2+3:
			b.WriteString("```\n")
		case i > n/2 && i < n/2+3:
			fmt.Fprintf(&b, "func f%d() { return }\n", i)
		case i%5 == 0:
			fmt.Fprintf(&b, "- item %d with [[Link %d]] and **bold** words\n", i, i)
		case i%5 == 1:
			fmt.Fprintf(&b, "- item %d %s\n", i, strings.Repeat("a long wrapping sentence ", 8))
		case i%5 == 2:
			fmt.Fprintf(&b, "  - nested %d with `code` and *emphasis*\n", i)
		case i%5 == 3:
			fmt.Fprintf(&b, "A plain paragraph line %d, with a [markdown link](http://example.com).\n", i)
		default:
			b.WriteString("\n")
		}
	}
	return b.String()
}

const (
	perfW, perfH  = 100, 40
	perfWindowH   = perfH - 2 // text rows: the rule row and the status line take two
	perfOpenMS    = 56
	perfKeyMS     = 26
	perfPageMS    = 4
	perfSlack     = 4 // machine slack over the v1 numbers
	perfBestOfRun = 5
)

func bestOf(n int, f func()) time.Duration {
	best := time.Duration(1<<63 - 1)
	for range n {
		t0 := time.Now()
		f()
		best = min(best, time.Since(t0))
	}
	return best
}

// perfModes are the two looks the bounds hold for.
var perfModes = []struct {
	name   string
	source bool
}{{"live", false}, {"source", true}}

func perfEditor(content string, anchor int, source bool) *EditorView {
	return NewEditorView(nil, "P", "/tmp/p.md", content, false, perfW, perfH, Anchor{anchor, 0, 10}, nil, source)
}

// The wall-clock bounds are the v1 numbers (56/26/4 ms) times a machine slack
// and the race factor; the real proof of independence from the buffer length
// is TestEditorWorkIsWindowBounded.
func TestEditorSpeed10000(t *testing.T) {
	quietTerm(t)
	content := perfContent(10000)
	bound := func(ms int) time.Duration { return time.Duration(ms*perfSlack*raceFactor) * time.Millisecond }

	for _, mode := range perfModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Run("open", func(t *testing.T) {
				got := bestOf(perfBestOfRun, func() {
					e := perfEditor(content, 9000, mode.source)
					_ = e.View()
				})
				if got > bound(perfOpenMS) {
					t.Errorf("open took %v, bound %v", got, bound(perfOpenMS))
				}
			})
			e := perfEditor(content, 9000, mode.source)
			_ = e.View()
			t.Run("keystroke", func(t *testing.T) {
				got := bestOf(perfBestOfRun, func() {
					e.Update(key("x"))
					_ = e.View()
				})
				if got > bound(perfKeyMS) {
					t.Errorf("keystroke took %v, bound %v", got, bound(perfKeyMS))
				}
			})
			t.Run("reveal", func(t *testing.T) {
				got := bestOf(perfBestOfRun, func() {
					e.Update(tea.KeyPressMsg{Code: tea.KeyDown})
					_ = e.View()
				})
				if got > bound(perfKeyMS) {
					t.Errorf("reveal took %v, bound %v", got, bound(perfKeyMS))
				}
			})
			t.Run("page", func(t *testing.T) {
				got := bestOf(perfBestOfRun, func() {
					e.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
					_ = e.View()
				})
				if got > bound(perfPageMS) {
					t.Errorf("page took %v, bound %v", got, bound(perfPageMS))
				}
			})
		})
	}
}

// Per-operation work is bounded by the window, not the buffer: the same
// bounds hold at 2500 and 20000 lines.
func TestEditorWorkIsWindowBounded(t *testing.T) {
	quietTerm(t)
	for _, mode := range perfModes {
		for _, n := range []int{2500, 20000} {
			t.Run(fmt.Sprintf("%s/%d", mode.name, n), func(t *testing.T) {
				content := perfContent(n)
				anchor := n * 9 / 10
				e := perfEditor(content, anchor, mode.source)
				_ = e.View()
				h := e.textHeight()
				if h != perfWindowH {
					t.Fatalf("text height %d, want %d", h, perfWindowH)
				}
				scanSlack := 2
				if !mode.source {
					scanSlack = 8
				}
				rendered := func() int { return e.preview.Stats().Lines }
				if got := e.stats.wrapped; got > 3*h {
					t.Errorf("open wrapped %d lines, bound %d", got, 3*h)
				}
				if got := e.scanner.Scanned(); got > anchor+h+scanSlack {
					t.Errorf("open scanned %d lines, bound anchor+H+%d = %d", got, scanSlack, anchor+h+scanSlack)
				}
				if got := rendered(); !mode.source && got > 3*h+8 {
					t.Errorf("open rendered %d preview lines, bound %d", got, 3*h+8)
				}

				for range 5 {
					w0, p0, s0, r0 := e.stats.wrapped, e.stats.painted, e.scanner.Scanned(), rendered()
					e.Update(key("x"))
					_ = e.View()
					if work := e.stats.wrapped - w0 + e.stats.painted - p0; work > 3*h {
						t.Errorf("keystroke wrapped+painted %d, bound %d", work, 3*h)
					}
					if grown := e.scanner.Scanned() - s0; grown > h+scanSlack {
						t.Errorf("keystroke scanned %d lines, bound %d", grown, h+scanSlack)
					}
					if got := rendered() - r0; got > h {
						t.Errorf("keystroke rendered %d preview lines, bound %d", got, h)
					}
				}
				for range 5 {
					r0 := rendered()
					e.Update(tea.KeyPressMsg{Code: tea.KeyDown})
					_ = e.View()
					if got := rendered() - r0; got > 8 {
						t.Errorf("reveal rendered %d preview lines, bound 8", got)
					}
				}
				for _, code := range []rune{tea.KeyPgDown, tea.KeyPgDown, tea.KeyPgUp} {
					w0, p0, r0 := e.stats.wrapped, e.stats.painted, rendered()
					e.Update(tea.KeyPressMsg{Code: code})
					_ = e.View()
					if work := e.stats.wrapped - w0 + e.stats.painted - p0; work > 4*h {
						t.Errorf("page wrapped+painted %d, bound %d", work, 4*h)
					}
					if got := rendered() - r0; got > 2*h {
						t.Errorf("page rendered %d preview lines, bound %d", got, 2*h)
					}
				}
			})
		}
	}
}

func benchEditor(b *testing.B, source bool) (*EditorView, string) {
	b.Setenv("NO_COLOR", "1")
	content := perfContent(10000)
	e := perfEditor(content, 9000, source)
	_ = e.View()
	return e, content
}

func benchOpen(b *testing.B, source bool) {
	_, content := benchEditor(b, source)
	b.ResetTimer()
	for range b.N {
		e := perfEditor(content, 9000, source)
		_ = e.View()
	}
}

func benchKeystroke(b *testing.B, source bool) {
	e, _ := benchEditor(b, source)
	b.ResetTimer()
	for range b.N {
		e.Update(key("x"))
		_ = e.View()
	}
}

// benchPageDown pages down through fresh rows: the window reaches the end only
// after hundreds of pages, far more than the preview caches, then starts over.
func benchPageDown(b *testing.B, source bool) {
	e, _ := benchEditor(b, source)
	b.ResetTimer()
	for range b.N {
		if e.buf.Cursor().Line > 9900 {
			e.Update(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl})
		} else {
			e.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
		}
		_ = e.View()
	}
}

// benchReveal alternates Down and Up so the cursor stays in the window.
func benchReveal(b *testing.B, source bool) {
	e, _ := benchEditor(b, source)
	b.ResetTimer()
	for i := range b.N {
		code := tea.KeyDown
		if i%2 == 1 {
			code = tea.KeyUp
		}
		e.Update(tea.KeyPressMsg{Code: code})
		_ = e.View()
	}
}

func BenchmarkEditorOpen10000(b *testing.B)      { benchOpen(b, true) }
func BenchmarkEditorKeystroke10000(b *testing.B) { benchKeystroke(b, true) }
func BenchmarkEditorPageDown10000(b *testing.B)  { benchPageDown(b, true) }
func BenchmarkEditorReveal10000(b *testing.B)    { benchReveal(b, true) }

func BenchmarkEditorLiveOpen10000(b *testing.B)      { benchOpen(b, false) }
func BenchmarkEditorLiveKeystroke10000(b *testing.B) { benchKeystroke(b, false) }
func BenchmarkEditorLivePageDown10000(b *testing.B)  { benchPageDown(b, false) }
func BenchmarkEditorLiveReveal10000(b *testing.B)    { benchReveal(b, false) }
