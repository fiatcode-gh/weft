package views

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// perfFoldMS is the bound for a Tab or z once the page's row map is built, on
// a 10000-line page, before slack and race scaling.
const perfFoldMS = 30

// perfHeadingContent is perfContent with a "## " every 200 lines and a "### "
// every 50 lines.
func perfHeadingContent(n int) string {
	lines := strings.Split(perfContent(n), "\n")
	for i := range lines[:n] {
		switch {
		case i%200 == 0:
			lines[i] = fmt.Sprintf("## Section %d", i)
		case i%50 == 0:
			lines[i] = fmt.Sprintf("### Part %d", i)
		}
	}
	return strings.Join(lines, "\n")
}

func worstOf(n int, f func()) time.Duration {
	var worst time.Duration
	for range n {
		t0 := time.Now()
		f()
		worst = max(worst, time.Since(t0))
	}
	return worst
}

func TestFoldSpeed10000(t *testing.T) {
	if raceDetector {
		t.Skip("wall-clock bound; the race detector's slowdown makes it meaningless, the non-race CI step checks timing")
	}
	quietTerm(t)
	for _, tc := range []struct{ name, content string }{
		{"mixed", perfContent(10000)},
		{"headings", perfHeadingContent(10000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, idx := writeGraph(t, map[string]string{"pages/P.md": tc.content})
			body := strings.TrimSpace(tc.content) + "\n"
			fresh := func() *PageView {
				p := NewPageView(idx, "P", perfW, perfH)
				if tc.name == "mixed" {
					p.GotoTop()
					p.LineDown() // the first bullet with a child
				}
				return p
			}
			// The first Tab is bounded by the row map it has to build. Time the
			// two alternately, each from a collected heap, so concurrent test
			// binaries and the render garbage left by fresh() load both alike
			// (CI runs packages in parallel on two CPUs).
			rowMap := time.Duration(1<<63 - 1)
			first := rowMap
			for range 3 {
				p := fresh()
				runtime.GC()
				t0 := time.Now()
				hint := p.ToggleFold()
				first = min(first, time.Since(t0))
				if hint != "" {
					t.Fatalf("first Tab hint %q", hint)
				}
				runtime.GC()
				t0 = time.Now()
				render.SourceRows(body, perfW, "")
				rowMap = min(rowMap, time.Since(t0))
			}
			t.Logf("first Tab %v, row map alone %v", first, rowMap)
			if limit := rowMap*11/10 + time.Duration(50*raceFactor)*time.Millisecond; first > limit {
				t.Errorf("first Tab took %v, bound %v (row map alone %v)", first, limit, rowMap)
			}

			p := fresh()
			p.ToggleFold()
			limit := time.Duration(perfFoldMS*perfSlack*raceFactor) * time.Millisecond
			// Folding costs more than unfolding: time the slowest of each.
			toggle := worstOf(4, func() {
				if hint := p.ToggleFold(); hint != "" {
					t.Fatalf("Tab hint %q", hint)
				}
			})
			level := worstOf(6, func() { p.CycleFoldLevel() })
			t.Logf("later Tab %v, z %v", toggle, level)
			if toggle > limit {
				t.Errorf("later Tab took %v, bound %v", toggle, limit)
			}
			if level > limit {
				t.Errorf("z took %v, bound %v", level, limit)
			}

			for p.CycleFoldLevel() != "fold: top-level bullets" {
			}
			if len(p.marks) == 0 {
				t.Fatal("setup: no folds active")
			}
			p.Restore(2000, 2000, -1)
			_ = p.View()
			key := bestOf(perfBestOfRun, func() {
				p.LineDown()
				_ = p.View()
			})
			t.Logf("LineDown + View with folds %v", key)
			if limit := time.Duration(perfReadKeyMS*perfSlack*raceFactor) * time.Millisecond; key > limit {
				t.Errorf("LineDown + View with folds took %v, bound %v", key, limit)
			}
		})
	}
}

func benchHeadingPage(b *testing.B) *graph.Index {
	b.Helper()
	b.Setenv("NO_COLOR", "1")
	dir := b.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "P.md"), []byte(perfHeadingContent(10000)), 0o644); err != nil {
		b.Fatal(err)
	}
	idx, err := graph.BuildIndex(dir)
	if err != nil {
		b.Fatal(err)
	}
	return idx
}

func BenchmarkFoldFirst10000(b *testing.B) {
	idx := benchHeadingPage(b)
	for range b.N {
		b.StopTimer()
		p := NewPageView(idx, "P", perfW, perfH)
		b.StartTimer()
		p.ToggleFold()
	}
}

func BenchmarkFoldToggle10000(b *testing.B) {
	p := NewPageView(benchHeadingPage(b), "P", perfW, perfH)
	p.ToggleFold()
	b.ResetTimer()
	for range b.N {
		p.ToggleFold()
	}
}

func BenchmarkFoldLevel10000(b *testing.B) {
	p := NewPageView(benchHeadingPage(b), "P", perfW, perfH)
	p.CycleFoldLevel()
	b.ResetTimer()
	for range b.N {
		p.CycleFoldLevel()
	}
}
