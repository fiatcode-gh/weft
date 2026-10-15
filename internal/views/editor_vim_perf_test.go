package views

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Vim keys do window-bounded work like the modeless keys
// (TestEditorWorkIsWindowBounded): the same bounds hold at 2500 and 20000
// lines, in both looks.
func TestVimWorkIsWindowBounded(t *testing.T) {
	quietTerm(t)
	for _, mode := range perfModes {
		for _, n := range []int{2500, 20000} {
			t.Run(fmt.Sprintf("%s/%d", mode.name, n), func(t *testing.T) {
				anchor := n * 9 / 10
				e := perfEditor(perfContent(n), anchor, mode.source)
				e.enableVim()
				_ = e.View()
				h := e.textHeight()
				scanSlack := 2
				if !mode.source {
					scanSlack = 8
				}
				rendered := func() int { return e.preview.Stats().Lines }
				check := func(name string, send func()) {
					t.Helper()
					w0, p0, s0, r0 := e.stats.wrapped, e.stats.painted, e.scanner.Scanned(), rendered()
					send()
					_ = e.View()
					if work := e.stats.wrapped - w0 + e.stats.painted - p0; work > 3*h {
						t.Errorf("%s wrapped+painted %d, bound %d", name, work, 3*h)
					}
					if grown := e.scanner.Scanned() - s0; grown > h+scanSlack {
						t.Errorf("%s scanned %d lines, bound %d", name, grown, h+scanSlack)
					}
					if got := rendered() - r0; got > h {
						t.Errorf("%s rendered %d preview lines, bound %d", name, got, h)
					}
				}
				for _, k := range []string{"j", "k", "w", "b", "$", "0", "}"} {
					for range 3 {
						check(k, func() { e.Update(key(k)) })
					}
				}
				for _, k := range []string{"x", "dd", "p", "u", "J", "dw", ">>", "vjd", "Vj>", "vlU", "Vjy"} {
					check(k, func() {
						for _, r := range k {
							e.Update(key(string(r)))
						}
					})
				}
				check("i", func() { e.Update(key("i")) })
				for _, r := range "abc" {
					check("typed "+string(r), func() { e.Update(key(string(r))) })
				}
				check("esc", func() { e.Update(tea.KeyPressMsg{Code: tea.KeyEscape}) })
			})
		}
	}
}
