package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

var paletteNow = time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

// typePalette feeds s to the palette one key at a time.
func typePalette(p *Palette, s string) {
	for _, r := range s {
		p.Update(string(r))
	}
}

func paletteKeys(p *Palette) []string {
	keys := make([]string, len(p.matches))
	for i, m := range p.matches {
		keys[i] = paletteActions[m.Index].key
	}
	return keys
}

func TestPaletteListsActionsInHelpOrder(t *testing.T) {
	quietTerm(t)
	got := paletteKeys(NewPalette(80, 24))
	want := strings.Fields("tab z enter [ ] . < > C O ctrl-p F / b T A S c e E R F2 1 2 3 4 5 | m ? q")
	if len(want) != 31 || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("palette keys\n got: %v\nwant: %v", got, want)
	}

	inPalette := map[string]bool{}
	for _, act := range paletteActions {
		inPalette[act.key] = true
	}
	var fromHelp []string
	for _, g := range helpGroups {
		if g.title == "Editor" || g.title == "Find" || g.title == vimHelp.title {
			continue
		}
		for _, r := range g.rows {
			if inPalette[r.key] {
				fromHelp = append(fromHelp, r.key)
			}
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(fromHelp) {
		t.Errorf("palette order differs from help order\n got: %v\nhelp: %v", got, fromHelp)
	}

	for _, absent := range []string{"j / k", "ctrl-d", "ctrl-u", "g / G", "n / N", ":", "esc"} {
		if inPalette[absent] {
			t.Errorf("palette lists %q", absent)
		}
	}
}

func TestPaletteFiltersByTitle(t *testing.T) {
	quietTerm(t)
	for query, want := range map[string]string{"rename": "F2", "backlinks": "b"} {
		p := NewPalette(80, 24)
		typePalette(p, query)
		if len(p.matches) == 0 || paletteActions[p.matches[0].Index].key != want {
			t.Errorf("query %q: matches %v, want %q first", query, paletteKeys(p), want)
		}
	}
}

func TestPaletteFiltersByKey(t *testing.T) {
	quietTerm(t)
	for query, want := range map[string]string{"F2": "F2", "ctrl-p": "ctrl-p"} {
		p := NewPalette(80, 24)
		typePalette(p, query)
		if len(p.matches) == 0 || paletteActions[p.matches[0].Index].key != want {
			t.Errorf("query %q: matches %v, want %q first", query, paletteKeys(p), want)
		}
	}
	p := NewPalette(80, 24)
	typePalette(p, "zzqqxx")
	if len(p.matches) != 0 {
		t.Errorf("zzqqxx matched %v", paletteKeys(p))
	}
	if res := p.Update(keyEnter); res.kind != overlayResultNone {
		t.Errorf("enter with no match returned kind %d", res.kind)
	}
}

// viaPalette opens the palette on a, moves to the action labelled label and
// presses enter, returning the command the App answered with.
func viaPalette(t *testing.T, a *App, label string) tea.Cmd {
	t.Helper()
	a.Update(key(":"))
	p, ok := a.active.(*Palette)
	if !ok {
		t.Fatalf(": opened %T, want *Palette", a.active)
	}
	pos := -1
	for i, m := range p.matches {
		if paletteLabels[m.Index] == label {
			pos = i
		}
	}
	if pos < 0 {
		t.Fatalf("palette lacks %q", label)
	}
	for range pos {
		a.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	_, cmd := a.Update(keyEnt)
	return cmd
}

func TestPaletteRunEqualsKey(t *testing.T) {
	cases := []struct {
		name  string
		key   tea.KeyPressMsg
		label string
		setup func(a *App)
		want  func(t *testing.T, a *App)
	}{
		{name: "panel", key: key("b"), label: "b backlinks + unlinked refs for the current page",
			setup: func(a *App) { a.navigate("Alpha") },
			want: func(t *testing.T, a *App) {
				if _, ok := a.active.(*Backlinks); !ok {
					t.Errorf("active %T, want *Backlinks", a.active)
				}
			}},
		{name: "navigate", key: key("["), label: "[ back",
			setup: func(a *App) { a.navigate("Alpha") },
			want: func(t *testing.T, a *App) {
				if got := a.page.Page(); got != "2026_05_24" {
					t.Errorf("page %q, want 2026_05_24", got)
				}
			}},
		{name: "cursor row", key: keyEnt, label: "enter follow link ([[Page#Heading]] opens at the heading)",
			setup: func(a *App) { a.navigate("Alpha"); a.Update(key("n")) },
			want: func(t *testing.T, a *App) {
				if got := a.page.Page(); got != "Beta" {
					t.Errorf("page %q, want Beta", got)
				}
			}},
		{name: "refused", key: key("e"), label: "e edit in-app at the cursor row",
			setup: func(a *App) { a.navigate("Alpha"); a.syncing = true },
			want: func(t *testing.T, a *App) {
				if a.editor != nil || a.hint != "⟳ "+syncBusyMsg {
					t.Errorf("editor %v, hint %q", a.editor != nil, a.hint)
				}
			}},
		{name: "sidebar", key: key("3"), label: "3 sidebar: todos",
			setup: func(a *App) {
				a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
				a.navigate("Alpha")
			},
			want: func(t *testing.T, a *App) {
				if !a.side.shown || a.side.section != sectionTodos || !a.side.focused {
					t.Errorf("sidebar shown %v section %d focused %v", a.side.shown, a.side.section, a.side.focused)
				}
			}},
		{name: "quit", key: key("q"), label: "q quit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			byKey := bootApp(t, bootConfig{now: paletteNow})
			byPalette := bootApp(t, bootConfig{now: paletteNow})
			if tc.setup != nil {
				tc.setup(byKey)
				tc.setup(byPalette)
			}
			_, keyCmd := byKey.Update(tc.key)
			palCmd := viaPalette(t, byPalette, tc.label)

			if tc.want != nil {
				tc.want(t, byKey)
				tc.want(t, byPalette)
			}
			if a, b := byKey.page.Page(), byPalette.page.Page(); a != b {
				t.Errorf("page: key %q, palette %q", a, b)
			}
			if a, b := fmt.Sprintf("%T", byKey.active), fmt.Sprintf("%T", byPalette.active); a != b {
				t.Errorf("active: key %s, palette %s", a, b)
			}
			if byKey.hint != byPalette.hint {
				t.Errorf("hint: key %q, palette %q", byKey.hint, byPalette.hint)
			}
			if (byKey.editor == nil) != (byPalette.editor == nil) {
				t.Errorf("editor differs")
			}
			if (byKey.capture == nil) != (byPalette.capture == nil) {
				t.Errorf("capture differs")
			}
			if (keyCmd == nil) != (palCmd == nil) {
				t.Errorf("cmd nil-ness: key %v, palette %v", keyCmd == nil, palCmd == nil)
			}
			if a, b := appText(byKey), appText(byPalette); a != b {
				t.Errorf("frame differs\nkey:\n%s\npalette:\n%s", a, b)
			}
			if tc.name == "quit" {
				for name, cmd := range map[string]tea.Cmd{"key": keyCmd, "palette": palCmd} {
					if _, ok := cmd().(tea.QuitMsg); !ok {
						t.Errorf("%s cmd did not quit", name)
					}
				}
			}
		})
	}
}

func TestColonOnlyOpensPaletteInReadView(t *testing.T) {
	a := bootApp(t, bootConfig{now: paletteNow})
	a.Update(key(":"))
	if _, ok := a.active.(*Palette); !ok {
		t.Fatalf("read view: active %T, want *Palette", a.active)
	}

	a = bootApp(t, bootConfig{now: paletteNow})
	a.navigate("Alpha")
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	before := strings.Count(text(a.editor), ":")
	a.Update(key(":"))
	if a.editor == nil || a.active != nil {
		t.Errorf("editor %v, active %T: ':' must stay in the editor", a.editor != nil, a.active)
	}
	if got := strings.Count(text(a.editor), ":"); got != before+1 {
		t.Errorf("':' count %d, want %d", got, before+1)
	}

	a = bootApp(t, bootConfig{now: paletteNow})
	a.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	a.Update(key(":"))
	if p, ok := a.active.(*Picker); !ok || p.input.Value() != ":" {
		t.Errorf("picker: active %T, want *Picker holding \":\"", a.active)
	}
}

func TestPaletteKeys(t *testing.T) {
	quietTerm(t)
	p := NewPalette(80, 24)
	press := func(k string) { p.Update(k) }
	press(keyDown)
	press(keyCtrlJ)
	if p.sel != 2 {
		t.Errorf("after down, ctrl+j: sel %d, want 2", p.sel)
	}
	press(keyUp)
	press(keyCtrlK)
	if p.sel != 0 {
		t.Errorf("after up, ctrl+k: sel %d, want 0", p.sel)
	}
	press(keyUp)
	if p.sel != 0 {
		t.Errorf("up at top: sel %d, want 0", p.sel)
	}

	a := bootApp(t, bootConfig{now: paletteNow})
	a.Update(key(":"))
	a.Update(keyEscK)
	if a.active != nil {
		t.Errorf("esc left %T open", a.active)
	}
	a.Update(key(":"))
	_, cmd := a.Update(keyCtrlC)
	if cmd == nil {
		t.Fatal("ctrl+c in the palette returned no cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c in the palette did not quit")
	}
}

func TestPaletteGolden(t *testing.T) {
	quietTerm(t)
	teatest.RequireEqualOutput(t, []byte(plain(NewPalette(80, 24).View())))
}
