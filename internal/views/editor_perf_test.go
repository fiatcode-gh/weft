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
	perfWindowH   = perfH - 1 // text rows: the status line takes one
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

// The wall-clock bounds are the v1 numbers (56/26/4 ms) times a machine slack
// and the race factor; the real proof of independence from the buffer length
// is TestEditorWorkIsWindowBounded.
func TestEditorSpeed10000(t *testing.T) {
	quietTerm(t)
	content := perfContent(10000)
	bound := func(ms int) time.Duration { return time.Duration(ms*perfSlack*raceFactor) * time.Millisecond }

	t.Run("open", func(t *testing.T) {
		got := bestOf(perfBestOfRun, func() {
			e := NewEditorView(nil, "P", "/tmp/p.md", content, false, perfW, perfH, Anchor{9000, 0, 10}, nil)
			_ = e.View()
		})
		if got > bound(perfOpenMS) {
			t.Errorf("open took %v, bound %v", got, bound(perfOpenMS))
		}
	})
	e := NewEditorView(nil, "P", "/tmp/p.md", content, false, perfW, perfH, Anchor{9000, 0, 10}, nil)
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
	t.Run("page", func(t *testing.T) {
		got := bestOf(perfBestOfRun, func() {
			e.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
			_ = e.View()
		})
		if got > bound(perfPageMS) {
			t.Errorf("page took %v, bound %v", got, bound(perfPageMS))
		}
	})
}

// Per-operation work is bounded by the window, not the buffer: the same
// bounds hold at 2500 and 20000 lines.
func TestEditorWorkIsWindowBounded(t *testing.T) {
	quietTerm(t)
	for _, n := range []int{2500, 20000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			content := perfContent(n)
			anchor := n * 9 / 10
			e := NewEditorView(nil, "P", "/tmp/p.md", content, false, perfW, perfH, Anchor{anchor, 0, 10}, nil)
			_ = e.View()
			h := e.textHeight()
			if h != perfWindowH {
				t.Fatalf("text height %d, want %d", h, perfWindowH)
			}
			if got := e.stats.wrapped; got > 3*h {
				t.Errorf("open wrapped %d lines, bound %d", got, 3*h)
			}
			if got := e.scanner.Scanned(); got > anchor+h+2 {
				t.Errorf("open scanned %d lines, bound anchor+H+2 = %d", got, anchor+h+2)
			}

			for range 5 {
				w0, p0, s0 := e.stats.wrapped, e.stats.painted, e.scanner.Scanned()
				e.Update(key("x"))
				_ = e.View()
				if work := e.stats.wrapped - w0 + e.stats.painted - p0; work > 3*h {
					t.Errorf("keystroke wrapped+painted %d, bound %d", work, 3*h)
				}
				if grown := e.scanner.Scanned() - s0; grown > h+2 {
					t.Errorf("keystroke scanned %d lines, bound %d", grown, h+2)
				}
			}
			for _, code := range []rune{tea.KeyPgDown, tea.KeyPgDown, tea.KeyPgUp} {
				w0, p0 := e.stats.wrapped, e.stats.painted
				e.Update(tea.KeyPressMsg{Code: code})
				_ = e.View()
				if work := e.stats.wrapped - w0 + e.stats.painted - p0; work > 4*h {
					t.Errorf("page wrapped+painted %d, bound %d", work, 4*h)
				}
			}
		})
	}
}

func benchEditor(b *testing.B) (*EditorView, string) {
	b.Setenv("NO_COLOR", "1")
	content := perfContent(10000)
	e := NewEditorView(nil, "P", "/tmp/p.md", content, false, perfW, perfH, Anchor{9000, 0, 10}, nil)
	_ = e.View()
	return e, content
}

func BenchmarkEditorOpen10000(b *testing.B) {
	_, content := benchEditor(b)
	b.ResetTimer()
	for range b.N {
		e := NewEditorView(nil, "P", "/tmp/p.md", content, false, perfW, perfH, Anchor{9000, 0, 10}, nil)
		_ = e.View()
	}
}

func BenchmarkEditorKeystroke10000(b *testing.B) {
	e, _ := benchEditor(b)
	b.ResetTimer()
	for range b.N {
		e.Update(key("x"))
		_ = e.View()
	}
}

func BenchmarkEditorPageDown10000(b *testing.B) {
	e, _ := benchEditor(b)
	b.ResetTimer()
	for i := range b.N {
		code := tea.KeyPgDown
		if i%2 == 1 {
			code = tea.KeyPgUp
		}
		e.Update(tea.KeyPressMsg{Code: code})
		_ = e.View()
	}
}
