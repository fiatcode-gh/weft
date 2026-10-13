package layout

import "testing"

func TestPatternSubfolders(t *testing.T) {
	l := mustLoad(t, `journal_file_pattern = "YYYY/MM/YYYY-MM-DD"`)
	d := day(2026, 5, 24)
	if got := l.JournalRel(d); got != "journals/2026/05/2026-05-24.md" {
		t.Errorf("JournalRel = %q", got)
	}
	if got, ok := l.Journal("journals/2026/05/2026-05-24.md"); !ok || !got.Equal(d) {
		t.Errorf("Journal = %v, %v", got, ok)
	}
	for _, rel := range []string{
		"journals/2026/06/2026-05-24.md",
		"journals/2026/05/2026-02-30.md",
		"journals/2026/05/notes.md",
		"pages/2026/05/2026-05-24.md",
		"journals/2026/05/2026-05-24",
	} {
		if _, ok := l.Journal(rel); ok {
			t.Errorf("Journal(%q) must be false", rel)
		}
	}
	if got, ok := l.ParseJournalName("2026-05-24"); !ok || !got.Equal(d) {
		t.Errorf("ParseJournalName = %v, %v", got, ok)
	}
	if _, ok := l.ParseJournalName("2026_05_24"); ok {
		t.Error("underscore name must not parse")
	}
	if got := l.JournalName(d); got != "2026-05-24" {
		t.Errorf("JournalName = %q", got)
	}
}

func TestPatternLiterals(t *testing.T) {
	l := mustLoad(t, `journal_file_pattern = "DD.MM.YYYY"`)
	if got, ok := l.ParseJournalName("24.05.2026"); !ok || !got.Equal(day(2026, 5, 24)) {
		t.Errorf("got %v, %v", got, ok)
	}
	if _, ok := l.ParseJournalName("24x05x2026"); ok {
		t.Error("dot is literal, not a wildcard")
	}
	u := mustLoad(t, `journal_file_pattern = "YYYY_MM_DD"`)
	if _, ok := u.ParseJournalName("2027_02_29"); ok {
		t.Error("2027_02_29 is not a date")
	}
	if _, ok := u.ParseJournalName("2028_02_29"); !ok {
		t.Error("2028_02_29 is a date")
	}
}

func TestJournalInRootJournalsDir(t *testing.T) {
	l := mustLoad(t, `journals_dir = ""`)
	if _, ok := l.Journal("2026_05_24.md"); !ok {
		t.Error("root-level journal must match")
	}
}
