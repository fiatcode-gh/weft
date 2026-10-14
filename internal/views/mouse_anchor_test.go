package views

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// anchorCase is a list panel a click can land on while the list overflows.
type anchorCase struct {
	name  string
	open  func(t *testing.T) *App
	panel func(a *App) rowPanel
	maxX  int  // 0: the whole frame; else the sidebar column
	tree  bool // the target is a group row; the second click toggles it
}

// manyTodos is one page of 40 open tasks: every task costs one row, so the
// window's last row sits past the half-budget point that makes a key centre.
func manyTodos() map[string]string {
	var sb strings.Builder
	for i := range 40 {
		fmt.Fprintf(&sb, "- TODO task %02d\n", i)
	}
	return map[string]string{"pages/Many.md": sb.String(), "pages/Alpha.md": "- a\n"}
}

func anchorCases() []anchorCase {
	overlayPanel := func(a *App) rowPanel { return a.active.(rowPanel) }
	sidePanel := func(a *App) rowPanel { return a.side.panel().(rowPanel) }
	sidebar := func(cfg bootConfig, w, h int, section string) func(t *testing.T) *App {
		return func(t *testing.T) *App {
			cfg.now = paletteNow
			a := bootApp(t, cfg)
			resize(a, w, h)
			a.Update(key(section))
			if !a.sidebarFocused() {
				t.Fatalf("sidebar section %s not focused", section)
			}
			return a
		}
	}
	overlay := func(cfg bootConfig, k string) func(t *testing.T) *App {
		return func(t *testing.T) *App {
			cfg.now = paletteNow
			a := bootApp(t, cfg)
			resize(a, 100, 30)
			a.Update(key(k))
			if a.active == nil {
				t.Fatalf("%s opened no overlay", k)
			}
			return a
		}
	}
	todos := bootConfig{files: manyTodos()}
	return []anchorCase{
		{name: "picker overlay", open: overlay(bootConfig{}, "ctrl+p"), panel: overlayPanel},
		{name: "todos overlay", open: overlay(todos, "T"), panel: overlayPanel},
		{name: "todos sidebar", open: sidebar(todos, 120, 30, "3"), panel: sidePanel, maxX: sidebarCols},
		{name: "tree sidebar", open: sidebar(bootConfig{}, 120, 20, "1"), panel: sidePanel, maxX: sidebarCols, tree: true},
	}
}

// bodyItems is the item on each body line of p's last render (-1: none).
func bodyItems(p rowPanel) []int {
	out := make([]int, 80)
	for i := range out {
		out[i] = -1
		if item, ok := p.drawnRows().itemAt(i); ok {
			out[i] = item
		}
	}
	return out
}

// framed renders a and returns the item on each body line of its panel.
func framed(a *App, c anchorCase) []int {
	appText(a)
	return bodyItems(c.panel(a))
}

// anchorTarget is the screen cell of the last item row drawn before the
// "more below" line, and its item. The list must overflow and the target not
// be selected.
func anchorTarget(t *testing.T, a *App, c anchorCase) (x, y, item int) {
	t.Helper()
	p := c.panel(a)
	appText(a) // renders the panel, so its row map is current
	mx, my := mustCell(t, a, "more below", c.maxX)
	mbLine, found := 0, false
	for i, line := range strings.Split(ansi.Strip(p.View()), "\n") {
		if strings.Contains(line, "more below") {
			mbLine, found = i, true
			break
		}
	}
	if !found {
		t.Fatalf("%s: the list does not overflow (no more below line)", c.name)
	}
	off := styleBorder.GetBorderTopSize() + styleBorder.GetPaddingTop()
	for line := mbLine - 1 - off; line >= 0; line-- {
		if it, ok := p.drawnRows().itemAt(line); ok {
			if it == p.selected() {
				t.Fatalf("%s: the last drawn row is the selected one", c.name)
			}
			return mx, my - (mbLine - off - line), it
		}
	}
	t.Fatalf("%s: no item row above more below", c.name)
	return 0, 0, 0
}

// twinAt is a fresh app of the case with the keys moved to item.
func twinAt(t *testing.T, c anchorCase, item int) *App {
	t.Helper()
	tw := c.open(t)
	stepTo(t, tw, func() Overlay { return c.panel(tw) }, item)
	return tw
}

func TestMouseClickKeepsWindow(t *testing.T) {
	for _, c := range anchorCases() {
		t.Run(c.name, func(t *testing.T) {
			a := c.open(t)
			before := framed(a, c)
			x, y, item := anchorTarget(t, a, c)
			if twin := twinAt(t, c, item); slices.Equal(framed(twin, c), before) {
				t.Fatalf("fixture: the keyboard twin renders the same window as the clicked frame (%v)", before)
			}

			a.Update(click(x, y))
			if got := c.panel(a).selected(); got != item {
				t.Fatalf("first click: selected %d, want %d", got, item)
			}
			if got := framed(a, c); !slices.Equal(got, before) {
				t.Fatalf("first click scrolled the list:\n before %v\n after  %v", before, got)
			}

			if c.tree {
				tr := c.panel(a).(*Tree)
				path := tr.rows[item].node.Path
				rows := len(tr.rows)
				a.Update(click(x, y))
				if !tr.open[path] || len(tr.rows) <= rows {
					t.Fatalf("second click: open %v rows %d (was %d), want expanded", tr.open[path], len(tr.rows), rows)
				}
				if got := framed(a, c); !slices.Equal(got[:item+1], before[:item+1]) {
					t.Fatalf("second click moved rows up to the group:\n before %v\n after  %v", before, got)
				}
				a.Update(click(x, y))
				if tr.open[path] || len(tr.rows) != rows {
					t.Fatalf("third click: open %v rows %d (was %d), want collapsed", tr.open[path], len(tr.rows), rows)
				}
				if got := framed(a, c); !slices.Equal(got, before) {
					t.Fatalf("third click scrolled the list:\n before %v\n after  %v", before, got)
				}
				return
			}

			a.Update(click(x, y))
			tw := twinAt(t, c, item)
			tw.Update(keyEnt)
			if got, want := listStateOf(a).noFrame(), listStateOf(tw).noFrame(); got != want {
				t.Errorf("second click != enter:\n got %+v\nwant %+v", got, want)
			}
			if got, want := a.sidebarFocused(), tw.sidebarFocused(); got != want {
				t.Errorf("sidebar focus %v, want %v", got, want)
			}
		})
	}
}

func TestMouseAnchorDroppedByKeys(t *testing.T) {
	for _, c := range anchorCases() {
		if c.name != "picker overlay" && c.name != "todos sidebar" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			probe := c.open(t)
			clickedWindow := framed(probe, c) // a click keeps this window
			x, y, item := anchorTarget(t, probe, c)
			clicked := func() *App {
				a := c.open(t)
				a.Update(click(x, y))
				if got := c.panel(a).selected(); got != item {
					t.Fatalf("click: selected %d, want %d", got, item)
				}
				return a
			}
			twin := func(keys ...string) string {
				tw := twinAt(t, c, item)
				for _, k := range keys {
					tw.Update(key(k))
				}
				return appText(tw)
			}
			up := twinAt(t, c, item)
			up.Update(listUp)
			if slices.Equal(framed(up, c), clickedWindow) {
				t.Fatal("fixture: up renders the clicked window, a kept anchor would not show")
			}

			t.Run("up", func(t *testing.T) {
				a := clicked()
				a.Update(listUp)
				if got, want := appText(a), twin("up"); got != want {
					t.Errorf("up after a click != keyboard twin:\n got:\n%s\nwant:\n%s", got, want)
				}
			})
			t.Run("wheel", func(t *testing.T) {
				a := clicked()
				a.Update(wheel(x, y, -1))
				if got, want := appText(a), twin("up"); got != want {
					t.Errorf("wheel up after a click != keyboard twin:\n got:\n%s\nwant:\n%s", got, want)
				}
			})
			if c.name == "picker overlay" {
				t.Run("typing", func(t *testing.T) {
					a := clicked()
					a.Update(key("a"))
					if got, want := appText(a), twin("a"); got != want {
						t.Errorf("typing after a click != keyboard twin:\n got:\n%s\nwant:\n%s", got, want)
					}
				})
			}
		})
	}
}
