package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/search"
)

var (
	capNow  = time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	keyEnt  = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyTabK = tea.KeyPressMsg{Code: tea.KeyTab}
	keyEscK = tea.KeyPressMsg{Code: tea.KeyEscape}
)

const capJournal = "journals/2026_05_25.md"

func typeText(a *App, s string) {
	for _, r := range s {
		a.Update(key(string(r)))
	}
}

func readCapJournal(t *testing.T, a *App) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.graphPath, capJournal))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	return string(b)
}

func capJournalExists(a *App) bool {
	_, err := os.Stat(filepath.Join(a.graphPath, capJournal))
	return err == nil
}

// bootCapture boots a temp graph at 2026-05-25 with the given files.
func bootCapture(t *testing.T, files map[string]string) *App {
	t.Helper()
	if _, ok := files["pages/Anchor.md"]; !ok {
		files["pages/Anchor.md"] = "anchor\n"
	}
	return bootApp(t, bootConfig{files: files, now: capNow})
}

func TestCaptureAppendsToExistingJournal(t *testing.T) {
	a := bootApp(t, bootConfig{now: capNow})
	old := readCapJournal(t, a)
	a.Update(key("c"))
	typeText(a, "buy milk")
	a.Update(keyEnt)
	if got := readCapJournal(t, a); got != old+"- buy milk\n" {
		t.Errorf("journal = %q, want old + line", got)
	}
	if a.capture != nil {
		t.Error("prompt should close after capture")
	}
	if a.hint != "captured to 2026-05-25" {
		t.Errorf("hint = %q", a.hint)
	}
	if !strings.Contains(appText(a), "buy milk") {
		t.Errorf("read view should show the new line:\n%s", appText(a))
	}
}

func TestCaptureAddsMissingFinalNewline(t *testing.T) {
	a := bootCapture(t, map[string]string{capJournal: "a"})
	a.Update(key("c"))
	typeText(a, "x")
	a.Update(keyEnt)
	if got := readCapJournal(t, a); got != "a\n- x\n" {
		t.Errorf("journal = %q", got)
	}
}

func TestCaptureMissingJournalNoTemplate(t *testing.T) {
	a := bootCapture(t, map[string]string{})
	a.Update(key("c"))
	typeText(a, "x")
	a.Update(keyEnt)
	if got := readCapJournal(t, a); got != "- x\n" {
		t.Errorf("journal = %q", got)
	}
	if !strings.HasSuffix(a.hint, "(new journal)") {
		t.Errorf("hint = %q", a.hint)
	}
}

func TestCaptureMissingJournalWithTemplate(t *testing.T) {
	a := bootCapture(t, map[string]string{"pages/Journal Template.md": "## Log"})
	a.Update(key("c"))
	typeText(a, "x")
	a.Update(keyEnt)
	if got := readCapJournal(t, a); got != "## Log\n- x\n" {
		t.Errorf("journal = %q", got)
	}
}

func TestCaptureTodoToggle(t *testing.T) {
	a := bootCapture(t, map[string]string{})
	a.Update(key("c"))
	a.Update(keyTabK)
	typeText(a, "x")
	a.Update(keyEnt)
	if got := readCapJournal(t, a); got != "- TODO x\n" {
		t.Errorf("journal = %q", got)
	}

	b := bootCapture(t, map[string]string{})
	b.Update(key("c"))
	b.Update(keyTabK)
	b.Update(keyTabK)
	typeText(b, "x")
	b.Update(keyEnt)
	if got := readCapJournal(t, b); got != "- x\n" {
		t.Errorf("journal = %q", got)
	}
}

func TestCaptureEmptyTextDoesNothing(t *testing.T) {
	a := bootCapture(t, map[string]string{})
	a.Update(key("c"))
	a.Update(keyEnt)
	typeText(a, "   ")
	a.Update(keyEnt)
	if capJournalExists(a) {
		t.Error("blank capture must not write")
	}
	if a.capture == nil {
		t.Error("prompt should stay open")
	}
}

func TestCaptureEscCancels(t *testing.T) {
	a := bootCapture(t, map[string]string{})
	a.Update(key("c"))
	typeText(a, "x")
	a.Update(keyEscK)
	if capJournalExists(a) {
		t.Error("cancelled capture must not write")
	}
	if a.capture != nil {
		t.Error("prompt should be closed")
	}
}

func TestCaptureRetrySucceedsAfterOneChange(t *testing.T) {
	a := bootCapture(t, map[string]string{capJournal: "old\n"})
	path := filepath.Join(a.graphPath, capJournal)
	real := a.readSnapshot
	changed := false
	a.readSnapshot = func(p string) (edit.Snapshot, error) {
		s, err := real(p)
		if p == path && !changed {
			changed = true
			if werr := os.WriteFile(path, []byte("old\n- agent\n"), 0o644); werr != nil {
				t.Fatal(werr)
			}
		}
		return s, err
	}
	a.Update(key("c"))
	typeText(a, "x")
	a.Update(keyEnt)
	if got := readCapJournal(t, a); got != "old\n- agent\n- x\n" {
		t.Errorf("journal = %q", got)
	}
}

func TestCaptureSecondChangeWritesNothing(t *testing.T) {
	a := bootCapture(t, map[string]string{capJournal: "old\n"})
	path := filepath.Join(a.graphPath, capJournal)
	real := a.readSnapshot
	n := 0
	a.readSnapshot = func(p string) (edit.Snapshot, error) {
		s, err := real(p)
		if p == path {
			n++
			if werr := os.WriteFile(path, []byte("v"+string(rune('0'+n))+"\n"), 0o644); werr != nil {
				t.Fatal(werr)
			}
		}
		return s, err
	}
	a.Update(key("c"))
	typeText(a, "x")
	a.Update(keyEnt)
	if got := readCapJournal(t, a); got != "v2\n" {
		t.Errorf("journal = %q, want v2", got)
	}
	if a.capture == nil {
		t.Fatal("prompt should stay open")
	}
	if a.capture.input != "x" {
		t.Errorf("input = %q, want kept", a.capture.input)
	}
	if !strings.Contains(a.capture.errMsg, "kept changing on disk") {
		t.Errorf("errMsg = %q", a.capture.errMsg)
	}
}

func TestCaptureFileAppearsMidwayGetsNoTemplate(t *testing.T) {
	a := bootCapture(t, map[string]string{"pages/Journal Template.md": "## Log\n"})
	path := filepath.Join(a.graphPath, capJournal)
	real := a.readSnapshot
	made := false
	a.readSnapshot = func(p string) (edit.Snapshot, error) {
		s, err := real(p)
		if p == path && !made {
			made = true
			if werr := os.WriteFile(path, []byte("- agent\n"), 0o644); werr != nil {
				t.Fatal(werr)
			}
		}
		return s, err
	}
	a.Update(key("c"))
	typeText(a, "x")
	a.Update(keyEnt)
	if got := readCapJournal(t, a); got != "- agent\n- x\n" {
		t.Errorf("journal = %q", got)
	}
}

func TestCaptureRefusedWhileSyncing(t *testing.T) {
	a := bootCapture(t, map[string]string{})
	a.Update(key("c"))
	typeText(a, "x")
	a.syncing = true
	a.Update(keyEnt)
	if capJournalExists(a) {
		t.Error("must not write while syncing")
	}
	if a.capture == nil || a.capture.input != "x" {
		t.Fatal("prompt should stay open with its text")
	}
	if !strings.Contains(a.capture.errMsg, "sync in progress") {
		t.Errorf("errMsg = %q", a.capture.errMsg)
	}
}

func TestCaptureFromListOverlays(t *testing.T) {
	open := map[string]func(a *App){
		"todos":     func(a *App) { a.Update(key("T")) },
		"agenda":    func(a *App) { a.Update(key("A")) },
		"backlinks": func(a *App) { a.active = NewBacklinks(a.idx, "Hub", nil, 80, 24) },
	}
	for name, setup := range open {
		t.Run(name, func(t *testing.T) {
			a := bootApp(t, bootConfig{now: capNow})
			old := readCapJournal(t, a)
			setup(a)
			overlay := a.active
			a.Update(key("c"))
			if a.capture == nil {
				t.Fatal("c should open the capture prompt")
			}
			typeText(a, "from "+name)
			a.Update(keyEnt)
			if got := readCapJournal(t, a); got != old+"- from "+name+"\n" {
				t.Errorf("journal = %q", got)
			}
			if a.active != overlay {
				t.Error("overlay underneath should stay")
			}
			p, ok := a.active.(panelFeedback)
			if !ok {
				t.Fatal("overlay has no in-panel line")
			}
			switch o := p.(type) {
			case *Todos:
				if o.errMsg != a.hint {
					t.Errorf("errMsg = %q, want %q", o.errMsg, a.hint)
				}
			case *Agenda:
				if o.errMsg != a.hint {
					t.Errorf("errMsg = %q, want %q", o.errMsg, a.hint)
				}
			case *Backlinks:
				if o.errMsg != a.hint {
					t.Errorf("errMsg = %q, want %q", o.errMsg, a.hint)
				}
			}
		})
	}
}

func TestCaptureBacklinksConfirmKeepsC(t *testing.T) {
	a := bootApp(t, bootConfig{now: capNow})
	betaPath := filepath.Join(a.graphPath, "pages", "Beta.md")
	unl := []graph.UnlinkedRef{{
		FilePath: betaPath,
		PageName: "Beta",
		Line:     1,
		Context:  "x Hub",
		Match:    search.Span{Start: 2, End: 5},
	}}
	b := NewBacklinks(a.idx, "Hub", unl, a.width, a.height)
	selectFirstUnlinked(t, b)
	b.Update("l")
	a.active = b
	a.Update(key("c"))
	if a.capture != nil {
		t.Error("c must do nothing while confirming")
	}
	if !b.confirming {
		t.Error("still confirming")
	}
}

func TestCaptureCStaysTextInPickerAndSearch(t *testing.T) {
	a := bootApp(t, bootConfig{now: capNow})
	a.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	a.Update(key("c"))
	if p, ok := a.active.(*Picker); !ok || p.input.Value() != "c" {
		t.Errorf("picker input should be c, active=%T", a.active)
	}
	if a.capture != nil {
		t.Error("no capture from picker")
	}

	b := bootApp(t, bootConfig{now: capNow})
	b.Update(key("/"))
	b.Update(key("c"))
	if s, ok := b.active.(*SearchView); !ok || s.input.Value() != "c" {
		t.Errorf("search input should be c, active=%T", b.active)
	}
	if b.capture != nil {
		t.Error("no capture from search")
	}
}

func TestCaptureLinkCompletionInPrompt(t *testing.T) {
	a := bootApp(t, bootConfig{now: capNow})
	old := readCapJournal(t, a)
	a.Update(key("c"))
	typeText(a, "see [[Alp")
	if !a.capture.completer.active {
		t.Fatal("completer should be open")
	}
	a.Update(keyEnt)
	if a.capture.input != "see [[Alpha]]" {
		t.Errorf("input = %q", a.capture.input)
	}
	if got := readCapJournal(t, a); got != old {
		t.Error("accepting a completion must not write")
	}
	a.Update(keyEnt)
	if got := readCapJournal(t, a); got != old+"- see [[Alpha]]\n" {
		t.Errorf("journal = %q", got)
	}
}

func TestCaptureTagCompletionInPrompt(t *testing.T) {
	a := bootApp(t, bootConfig{now: capNow})
	a.Update(key("c"))
	typeText(a, "#Bet")
	if !a.capture.completer.active {
		t.Fatal("tag completer should be open")
	}
	a.Update(keyTabK)
	if a.capture.input != "#Beta" {
		t.Errorf("input = %q", a.capture.input)
	}
	if a.capture.todo {
		t.Error("tab accepted a completion; TODO must not toggle")
	}
}

func TestCaptureFrameLayersPrompt(t *testing.T) {
	a := bootApp(t, bootConfig{now: capNow})
	before := strings.Split(appText(a), "\n")
	a.Update(key("c"))
	lines := strings.Split(appText(a), "\n")
	if len(lines) != 24 {
		t.Fatalf("frame has %d lines, want 24", len(lines))
	}
	if lines[0] != before[0] {
		t.Errorf("first line = %q, want %q", lines[0], before[0])
	}
	tail := strings.Join(lines[len(lines)-6:], "\n")
	for _, want := range []string{"Capture to 2026-05-25:", "enter add · tab TODO on/off · [[ link · # tag · esc cancel"} {
		if !strings.Contains(tail, want) {
			t.Errorf("bottom rows missing %q:\n%s", want, tail)
		}
	}
	if a.View().Cursor == nil {
		t.Error("prompt needs a cursor")
	}
}

func TestCaptureCtrlCQuits(t *testing.T) {
	a := bootApp(t, bootConfig{now: capNow})
	a.Update(key("c"))
	_, cmd := a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c returned no cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c did not quit, got %T", cmd())
	}
}

func TestCompletionSplice(t *testing.T) {
	_, idx := writeGraph(t, map[string]string{"pages/Alpha.md": "x\n", "pages/Odd Name.md": "x\n"})
	cases := []struct {
		name   string
		before string
		tagOK  bool
		sel    int
		del    int
		ins    string
	}{
		{"page", "see [[Alp", false, 0, 3, "Alpha]]"},
		{"create", "see [[Zzzq", false, 0, 0, "]]"},
		{"tag", "#Alp", true, 0, 4, "#Alpha"},
		{"tag needing brackets", "#Odd", true, 0, 4, "#[[Odd Name]]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newLinkCompleter(idx)
			c.refresh(tc.before, "", true, tc.tagOK)
			del, ins, ok := c.completion()
			if !ok || del != tc.del || ins != tc.ins {
				t.Errorf("completion() = (%d, %q, %v), want (%d, %q, true)", del, ins, ok, tc.del, tc.ins)
			}
		})
	}
	if _, _, ok := newLinkCompleter(idx).completion(); ok {
		t.Error("inactive completer should report ok=false")
	}
}
