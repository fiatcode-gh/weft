package views

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// v1Keys is the textarea keymap Bubbles v1 shipped; editorKeyMap must enable
// exactly these keys and nothing else.
var v1Keys = map[string][]string{
	"CharacterForward":           {"right", "ctrl+f"},
	"CharacterBackward":          {"left", "ctrl+b"},
	"WordForward":                {"alt+right", "alt+f"},
	"WordBackward":               {"alt+left", "alt+b"},
	"LineNext":                   {"down", "ctrl+n"},
	"LinePrevious":               {"up", "ctrl+p"},
	"DeleteWordBackward":         {"alt+backspace", "ctrl+w"},
	"DeleteWordForward":          {"alt+delete", "alt+d"},
	"DeleteAfterCursor":          {"ctrl+k"},
	"DeleteBeforeCursor":         {"ctrl+u"},
	"InsertNewline":              {"enter", "ctrl+m"},
	"DeleteCharacterBackward":    {"backspace", "ctrl+h"},
	"DeleteCharacterForward":     {"delete", "ctrl+d"},
	"LineStart":                  {"home", "ctrl+a"},
	"LineEnd":                    {"end", "ctrl+e"},
	"Paste":                      {"ctrl+v"},
	"InputBegin":                 {"alt+<", "ctrl+home"},
	"InputEnd":                   {"alt+>", "ctrl+end"},
	"CapitalizeWordForward":      {"alt+c"},
	"LowercaseWordForward":       {"alt+l"},
	"UppercaseWordForward":       {"alt+u"},
	"TransposeCharacterBackward": {"ctrl+t"},
}

func TestEditorKeyMapMatchesV1(t *testing.T) {
	quietTerm(t)
	km := reflect.ValueOf(editorKeyMap())
	typ := km.Type()
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		b, ok := km.Field(i).Interface().(interface {
			Keys() []string
			Enabled() bool
		})
		if !ok {
			t.Fatalf("field %s is not a key binding", name)
		}
		var got []string
		if b.Enabled() {
			got = b.Keys()
		}
		want := v1Keys[name]
		if !slices.Equal(got, want) {
			t.Errorf("%s: enabled keys %v, want %v", name, got, want)
		}
	}
	for name := range v1Keys {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("v1 table names unknown KeyMap field %s", name)
		}
	}
}

func TestEditorV2OnlyChordsAreInert(t *testing.T) {
	quietTerm(t)
	chords := []tea.KeyPressMsg{
		{Code: tea.KeyRight, Mod: tea.ModShift},
		{Code: tea.KeyLeft, Mod: tea.ModShift},
		{Code: tea.KeyUp, Mod: tea.ModShift},
		{Code: tea.KeyDown, Mod: tea.ModShift},
		{Code: 'g', Mod: tea.ModCtrl},
		{Code: 'c', Mod: tea.ModCtrl | tea.ModShift},
		{Code: tea.KeyRight, Mod: tea.ModCtrl},
		{Code: tea.KeyLeft, Mod: tea.ModCtrl},
		{Code: tea.KeyBackspace, Mod: tea.ModCtrl},
		{Code: tea.KeyDelete, Mod: tea.ModCtrl},
		{Code: tea.KeyRight, Mod: tea.ModCtrl | tea.ModShift},
		{Code: tea.KeyRight, Mod: tea.ModAlt | tea.ModShift},
		{Code: 'f', Mod: tea.ModAlt | tea.ModShift},
	}
	const content = "alpha beta gamma\nsecond\n"
	fresh := func() *EditorView {
		e := NewEditorView(nil, "P", "/tmp/p.md", content, false, 40, 10, 0)
		e.ta.SetCursorColumn(6)
		return e
	}
	for _, c := range chords {
		t.Run(c.String(), func(t *testing.T) {
			e := fresh()
			wantRow, wantCol := e.cursorRowCol()
			wantContent := e.Content()
			e.Update(c)
			row, col := e.cursorRowCol()
			if e.Content() != wantContent {
				t.Errorf("content changed: %q", e.Content())
			}
			if row != wantRow || col != wantCol {
				t.Errorf("cursor moved to %d,%d, want %d,%d", row, col, wantRow, wantCol)
			}
			if e.ta.HasSelection() {
				t.Error("chord started a selection")
			}
		})
	}

	t.Run("alt+right still moves a word", func(t *testing.T) {
		e := fresh()
		_, before := e.cursorRowCol()
		e.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
		if _, after := e.cursorRowCol(); after <= before {
			t.Errorf("column %d -> %d, want it to advance", before, after)
		}
	})
	t.Run("ctrl+w still deletes a word", func(t *testing.T) {
		e := fresh()
		e.Update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
		if want := "beta gamma\nsecond\n"; e.Content() != want {
			t.Errorf("content %q, want %q", e.Content(), want)
		}
	})
}

func TestEditorCursorIsPlainReverse(t *testing.T) {
	quietTerm(t)
	v := NewEditorView(nil, "Page", "/tmp/page.md", "intro\n", false, 40, 6, 0).View()
	if !strings.Contains(v, "\x1b[7mi") {
		t.Errorf("cursor is not plain reverse video: %q", v)
	}
	if strings.Contains(v, "\x1b[7;37m") {
		t.Errorf("cursor carries Bubbles v2's foreground colour: %q", v)
	}
}

func TestEditorEnterBeyondDefaultMaxHeight(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "P", "/tmp/p.md", strings.Repeat("line\n", 150), false, 80, 24, 149)
	e.ta.CursorEnd()
	before := e.ta.LineCount()
	e.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := e.ta.LineCount(); got != before+1 {
		t.Errorf("line count %d -> %d, want one more", before, got)
	}
	if e.LoadDiverged() {
		t.Error("buffer reports a load divergence")
	}
}
