package views

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v3/internal/buffer"
	"github.com/fiatcode-gh/weft/v3/internal/render"
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
			fmt.Fprintf(&b, "A plain paragraph line %d, with a [markdown link](http://example.com), #topic%d and issue #%d.\n", i, i, i)
		default:
			b.WriteString("\n")
		}
	}
	return b.String()
}

// perfTaskContent is n lines cycling a prioritised task with a link and its
// two stamp lines.
func perfTaskContent(n int) string {
	var b strings.Builder
	for i := range n {
		switch i % 3 {
		case 0:
			fmt.Fprintf(&b, "- TODO [#A] task %d with [[Link %d]]\n", i, i)
		case 1:
			b.WriteString("  SCHEDULED: <2026-05-25 Mon 09:30 .+1w>\n")
		default:
			b.WriteString("  DEADLINE: <2026-05-27 Wed>\n")
		}
	}
	return b.String()
}

const (
	perfW, perfH  = 100, 40
	perfWindowH   = perfH - 2 // text rows: the rule row and the status line take two
	perfSlack     = 4         // machine slack over the v1 numbers
	perfBestOfRun = 5
	perfPageMoves = 20 // fresh page moves whose mean is bounded
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

// perfReference is the editor-independent part of opening at line 9000: build the buffer
// and scan its source lines up to the anchor.
func perfReference(content string) { render.NewScanner().Info(buffer.New(content), 9000) }

// perfRefRounds is how many times the reference and each operation are timed
// alternately; the minimum of each is kept.
const perfRefRounds = 3

// perfKeyBatch is how many consecutive Update+View calls make one keystroke or
// reveal sample; the mean per call is compared. One sub-millisecond call is
// too noisy to bound.
const perfKeyBatch = 20

// perfPermille bounds each operation as thousandths of perfReference timed in
// the same run, each from a collected heap, so a slow or loaded machine
// stretches the reference and the operation alike. Keys are
// "<corpus>/<mode>/<op>"; each is twice the largest ratio seen over repeated
// runs (10 without -race, 5 with). The race detector slows the reference far
// more than the batched operations, so it has its own table. The real proof of
// independence from the buffer length is TestEditorWorkIsWindowBounded.
var perfPermille = map[bool]map[string]int{
	false: {
		"mixed/live/open":        4983,
		"mixed/live/keystroke":   38,
		"mixed/live/reveal":      15,
		"mixed/live/page":        441,
		"mixed/source/open":      2067,
		"mixed/source/keystroke": 45,
		"mixed/source/reveal":    33,
		"mixed/source/page":      71,
		"tasks/live/open":        4191,
		"tasks/live/keystroke":   50,
		"tasks/live/reveal":      9,
		"tasks/live/page":        448,
		"tasks/source/open":      2065,
		"tasks/source/keystroke": 23,
		"tasks/source/reveal":    15,
		"tasks/source/page":      73,
	},
	true: {
		"mixed/live/open":        3345,
		"mixed/live/keystroke":   9,
		"mixed/live/reveal":      3,
		"mixed/live/page":        134,
		"mixed/source/open":      2092,
		"mixed/source/keystroke": 8,
		"mixed/source/reveal":    6,
		"mixed/source/page":      19,
		"tasks/live/open":        3017,
		"tasks/live/keystroke":   15,
		"tasks/live/reveal":      1,
		"tasks/live/page":        138,
		"tasks/source/open":      2032,
		"tasks/source/keystroke": 5,
		"tasks/source/reveal":    3,
		"tasks/source/page":      19,
	},
}

func TestEditorSpeed10000(t *testing.T) {
	quietTerm(t)
	for _, corpus := range []struct {
		name    string
		content string
	}{{"mixed", perfContent(10000)}, {"tasks", perfTaskContent(10000)}} {
		t.Run(corpus.name, func(t *testing.T) {
			testEditorSpeed(t, corpus.name, corpus.content)
		})
	}
}

// perfTimed collects garbage, then times f once.
func perfTimed(f func()) time.Duration {
	runtime.GC()
	t0 := time.Now()
	f()
	return time.Since(t0)
}

func testEditorSpeed(t *testing.T, corpus, content string) {
	for _, mode := range perfModes {
		t.Run(mode.name, func(t *testing.T) {
			const unset = time.Duration(1<<63 - 1)
			ref, open, keyT, reveal, page := unset, unset, unset, unset, unset
			e := perfEditor(content, 9000, mode.source)
			_ = e.View()
			for range perfRefRounds {
				ref = min(ref, perfTimed(func() { perfReference(content) }))
				open = min(open, perfTimed(func() {
					o := perfEditor(content, 9000, mode.source)
					_ = o.View()
				}))
				keyT = min(keyT, perfTimed(func() {
					for range perfKeyBatch {
						e.Update(key("x"))
						_ = e.View()
					}
				})/perfKeyBatch)
				reveal = min(reveal, perfTimed(func() {
					for range perfKeyBatch {
						e.Update(tea.KeyPressMsg{Code: tea.KeyDown})
						_ = e.View()
					}
				})/perfKeyBatch)
				// Fresh page moves from the top: every move shows rows no
				// earlier move rendered, so a cached window cannot hide a slow
				// one. The mean is bounded, not the best of a few, so a
				// slowdown of the typical move fails.
				p := perfEditor(content, 0, mode.source)
				_ = p.View()
				page = min(page, perfTimed(func() {
					for range perfPageMoves {
						p.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
						_ = p.View()
					}
				})/perfPageMoves)
			}
			for _, op := range []struct {
				name string
				got  time.Duration
			}{{"open", open}, {"keystroke", keyT}, {"reveal", reveal}, {"page", page}} {
				t.Run(op.name, func(t *testing.T) {
					permille, ok := perfPermille[raceDetector][corpus+"/"+mode.name+"/"+op.name]
					if !ok {
						t.Fatalf("no per-mille bound for %s/%s/%s", corpus, mode.name, op.name)
					}
					limit := ref * time.Duration(permille) / 1000
					t.Logf("%s %v, ref %v, ratio %.4f", op.name, op.got, ref, float64(op.got)/float64(ref))
					if op.got > limit {
						t.Errorf("%s took %v, bound %v (reference %v)", op.name, op.got, limit, ref)
					}
				})
			}
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
