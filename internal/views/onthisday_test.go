package views

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
)

var otdFiles = map[string]string{
	"journals/2026_05_19.md": "\n\n- week line one\n- week line two\n- third\n",
	"journals/2026_04_26.md": "",
	"journals/2025_05_26.md": "- last year\n",
	"journals/2024_05_26.md": "- two years\n",
	"journals/2026_05_20.md": "noise\n",
	"pages/Anchor.md":        "- anchor\n",
}

func otdOverlay(t *testing.T, files map[string]string, read func(string) (edit.Snapshot, error)) *OnThisDay {
	t.Helper()
	_, idx := writeGraph(t, files)
	if read == nil {
		read = edit.ReadSnapshot
	}
	return NewOnThisDay(idx, calNow, read, 80, 24)
}

func TestPreviewLines(t *testing.T) {
	got := previewLines("\n  \t\n- a  \r\n\n- b\r\n- c\n", 2)
	if len(got) != 2 || got[0] != "- a" || got[1] != "- b" {
		t.Errorf("previewLines = %q", got)
	}
	if got := previewLines("", 2); len(got) != 0 {
		t.Errorf("empty content: %q", got)
	}
}

func TestOnThisDayLists(t *testing.T) {
	quietTerm(t)
	v := plain(otdOverlay(t, otdFiles, nil).View())
	pos := 0
	for _, want := range []string{
		"1 week ago · 2026-05-19 Tue", "week line one", "week line two",
		"1 month ago · 2026-04-26 Sun", "(empty)",
		"2025 · 2025-05-26 Mon", "2024 · 2024-05-26 Sun",
	} {
		i := strings.Index(v[pos:], want)
		if i < 0 {
			t.Fatalf("missing %q (in order) in:\n%s", want, v)
		}
		pos += i + len(want)
	}
	for _, bad := range []string{"third", "noise"} {
		if strings.Contains(v, bad) {
			t.Errorf("view must not contain %q:\n%s", bad, v)
		}
	}
}

func TestOnThisDayEmptyState(t *testing.T) {
	quietTerm(t)
	v := plain(otdOverlay(t, map[string]string{"pages/Anchor.md": "- a\n"}, nil).View())
	if !strings.Contains(v, "nothing on this day") {
		t.Errorf("empty state missing:\n%s", v)
	}
}

func TestOnThisDayReadError(t *testing.T) {
	quietTerm(t)
	read := func(p string) (edit.Snapshot, error) {
		if strings.HasSuffix(p, "2026_05_19.md") {
			return edit.Snapshot{}, errors.New("boom")
		}
		return edit.ReadSnapshot(p)
	}
	v := plain(otdOverlay(t, otdFiles, read).View())
	if !strings.Contains(v, "(cannot read: boom)") {
		t.Errorf("read error missing:\n%s", v)
	}
}

func TestOnThisDayIgnoresSameNamedPage(t *testing.T) {
	quietTerm(t)
	files := map[string]string{
		"journals/2026_05_19.md": "- from journal\n",
		"pages/2026-05-19.md":    "- from page\n",
	}
	_, idx := writeGraph(t, files)
	v := plain(NewOnThisDay(idx, calNow, edit.ReadSnapshot, 80, 24).View())
	if !strings.Contains(v, "from journal") || strings.Contains(v, "from page") {
		t.Errorf("preview must come from journals/:\n%s", v)
	}
}

func TestOnThisDayGolden(t *testing.T) {
	quietTerm(t)
	teatest.RequireEqualOutput(t, []byte(plain(otdOverlay(t, otdFiles, nil).View())))
}

func otdBoot(t *testing.T) *App {
	a := bootApp(t, bootConfig{files: otdFiles, now: calNow})
	return a
}

func TestAppOnThisDayOpens(t *testing.T) {
	a := otdBoot(t)
	a.Update(key("O"))
	if _, ok := a.active.(*OnThisDay); !ok {
		t.Fatalf("O: active = %T", a.active)
	}
	a.Update(key("j"))
	a.Update(keyEnt)
	if got := a.page.Page(); got != "2026-04-26" || a.active != nil {
		t.Errorf("page %q active %v", got, a.active)
	}
}

func TestAppOnThisDayCapture(t *testing.T) {
	a := otdBoot(t)
	a.Update(key("O"))
	a.Update(key("c"))
	if a.capture == nil {
		t.Fatal("c should open capture")
	}
}
