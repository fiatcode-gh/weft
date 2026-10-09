package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// perfReadKeyMS is the bound for one read-view key plus a frame on a
// 10000-line page, before slack and race scaling.
const perfReadKeyMS = 5

func perfReadApp(t *testing.T) *App {
	t.Helper()
	quietTerm(t)
	a := newApp(t, map[string]string{
		"pages/P.md": perfContent(10000),
		"pages/Q.md": "- q\n",
	}, perfW, perfH)
	a.navigate("P")
	return a
}

func TestReadKeysNeverBuildRowMap(t *testing.T) {
	a := perfReadApp(t)
	orig := sourceRowsFor
	t.Cleanup(func() { sourceRowsFor = orig })
	sourceRowsFor = func(string, int, string) ([]int, []bool) {
		t.Fatal("a read-view key built the row map")
		return nil, nil
	}
	for _, k := range []string{"j", "k", "ctrl+d", "ctrl+u", "G", "g", "n", "N", "n"} {
		switch k {
		case "ctrl+d":
			a.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
		case "ctrl+u":
			a.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		default:
			a.Update(key(k))
		}
		_ = a.View()
	}
	a.navigate("Q")
	a.Update(key("["))
	a.Update(key("]"))
	_ = a.View()
}

func TestReadViewSpeed10000(t *testing.T) {
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{"pages/P.md": perfContent(10000)})
	body := strings.TrimSpace(perfContent(10000)) + "\n"
	renderTime := bestOf(2, func() { _, _ = render.RenderWithEmphasis(body, perfW, "") })
	open := bestOf(2, func() { _ = NewPageView(idx, "P", perfW, perfH).View() })
	t.Logf("open %v, render alone %v", open, renderTime)
	if limit := renderTime*11/10 + time.Duration(20*raceFactor)*time.Millisecond; open > limit {
		t.Errorf("open took %v, bound %v (render alone %v)", open, limit, renderTime)
	}

	p := NewPageView(idx, "P", perfW, perfH)
	p.Restore(9000, 9000, -1)
	_ = p.View()
	got := bestOf(perfBestOfRun, func() {
		p.LineDown()
		_ = p.View()
	})
	t.Logf("LineDown + View %v", got)
	if limit := time.Duration(perfReadKeyMS*perfSlack*raceFactor) * time.Millisecond; got > limit {
		t.Errorf("LineDown + View took %v, bound %v", got, limit)
	}
}

// benchPage writes a one-page graph holding perfContent(10000) and indexes it.
func benchPage(b *testing.B) *graph.Index {
	b.Helper()
	b.Setenv("NO_COLOR", "1")
	dir := b.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "P.md"), []byte(perfContent(10000)), 0o644); err != nil {
		b.Fatal(err)
	}
	idx, err := graph.BuildIndex(dir)
	if err != nil {
		b.Fatal(err)
	}
	return idx
}

func BenchmarkReadKey10000(b *testing.B) {
	p := NewPageView(benchPage(b), "P", perfW, perfH)
	p.Restore(9000, 9000, -1)
	_ = p.View()
	b.ResetTimer()
	for range b.N {
		p.LineDown()
		_ = p.View()
	}
}

func BenchmarkReadOpen10000(b *testing.B) {
	idx := benchPage(b)
	b.ResetTimer()
	for range b.N {
		_ = NewPageView(idx, "P", perfW, perfH).View()
	}
}
