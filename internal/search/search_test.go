package search

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// skipIfNoRipgrep skips the test when the rg binary is not on PATH; the search
// package shells out to ripgrep, so these tests cannot run without it.
func skipIfNoRipgrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run search tests")
	}
}

// fixtureGraph returns the absolute path to the checked-in fixture graph that
// the Run tests grep over; a path failure is fatal.
func fixtureGraph(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// writePage seeds a page file under dir/pages/name with the given body. It is
// the shared arrange step for tests that grep over a freshly built temp graph.
func writePage(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, "pages", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// graphFiles lists the .md files one level under dir's subdirectories, the way
// the index would hand them to Run.
func graphFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestParseJSON(t *testing.T) {
	// arrange — a representative rg --json stream: a begin/end pair for one
	// file, a match with submatches, and a second match with none.
	in := `{"type":"begin","data":{"path":{"text":"/g/pages/Alpha.md"}}}
{"type":"match","data":{"path":{"text":"/g/pages/Alpha.md"},"lines":{"text":"links to Beta\n"},"line_number":3,"absolute_offset":42,"submatches":[{"match":{"text":"Beta"},"start":9,"end":13}]}}
{"type":"match","data":{"path":{"text":"/g/journals/2026_05_24.md"},"lines":{"text":"references Alpha\n"},"line_number":1,"absolute_offset":0,"submatches":[]}}
{"type":"end","data":{"path":{"text":"/g/pages/Alpha.md"}}}
`

	// act
	hits, err := parseJSON([]byte(in))

	// assert
	if err != nil {
		t.Fatalf("parseJSON err: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d: %+v", len(hits), hits)
	}
	if hits[0].Line != 3 || hits[0].Context != "links to Beta" {
		t.Errorf("hit[0]: %+v", hits[0])
	}
	if len(hits[0].Matches) != 1 || hits[0].Matches[0].Start != 9 || hits[0].Matches[0].End != 13 {
		t.Errorf("hit[0].Matches: want [{9 13}], got %+v", hits[0].Matches)
	}
	if hits[1].FilePath != "/g/journals/2026_05_24.md" {
		t.Errorf("hit[1]: %+v", hits[1])
	}
	if len(hits[1].Matches) != 0 {
		t.Errorf("hit[1].Matches: want 0, got %d", len(hits[1].Matches))
	}
}

func TestParseJSONStripsTrailingCR(t *testing.T) {
	// arrange — rg reports a CRLF file's line with \r\n intact
	input := []byte(`{"type":"match","data":{"path":{"text":"pages/A.md"},"lines":{"text":"- hit here\r\n"},"line_number":1,"submatches":[{"start":2,"end":5}]}}` + "\n")

	// act
	hits, err := parseJSON(input)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Context != "- hit here" {
		t.Fatalf("Context = %q, want trailing CR stripped", hits[0].Context)
	}
}

// TestParseJSONSurfacesScannerOverflow guards against silent truncation: a
// single rg output line longer than the scanner's buffer cap (a pasted log
// blob inside any note, for instance) must abort with an error, not just
// stop scanning and quietly drop every hit after it.
func TestParseJSONSurfacesScannerOverflow(t *testing.T) {
	// arrange — one line over the 1MB scanner cap, then a valid match
	huge := `{"type":"match","data":{"path":{"text":"pages/A.md"},"lines":{"text":"` +
		strings.Repeat("x", 1100*1024) + `"},"line_number":1,"submatches":[]}}`
	valid := `{"type":"match","data":{"path":{"text":"pages/B.md"},"lines":{"text":"hit"},"line_number":2,"submatches":[{"start":0,"end":3}]}}`
	input := []byte(huge + "\n" + valid + "\n")

	// act
	_, err := parseJSON(input)

	// assert — truncation must be an error, not silently missing hits
	if err == nil {
		t.Fatal("scanner overflow was swallowed; hits after the long line are silently dropped")
	}
}

func TestRunFindsHits(t *testing.T) {
	skipIfNoRipgrep(t)

	// arrange
	graph := fixtureGraph(t)

	// act
	hits, err := Run(graphFiles(t, graph), "Beta")

	// assert
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected at least one hit for \"Beta\"")
	}
	for _, h := range hits {
		if !strings.HasPrefix(h.FilePath, graph) {
			t.Errorf("hit path outside fixture: %s", h.FilePath)
		}
	}
}

func TestRunNoMatchReturnsEmpty(t *testing.T) {
	skipIfNoRipgrep(t)

	// arrange
	graph := fixtureGraph(t)

	// act
	hits, err := Run(graphFiles(t, graph), "thisstringshouldnotexistanywhere_xyzzy_1234")

	// assert
	if err != nil {
		t.Fatalf("no-match should not error, got: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("no-match: want 0 hits, got %d", len(hits))
	}
}

func TestRunMalformedRegexSurfacesRipgrepFailure(t *testing.T) {
	skipIfNoRipgrep(t)

	// arrange
	graph := fixtureGraph(t)

	// act
	_, err := Run(graphFiles(t, graph), "[")

	// assert
	if err == nil || !strings.Contains(err.Error(), "rg failed") {
		t.Fatalf("Run malformed regex error = %v, want rg failed", err)
	}
}

func TestRunEmptyFileListReturnsNil(t *testing.T) {
	// act — no rg needed: nothing to search short-circuits
	hits, err := Run(nil, "anything")

	// assert
	if err != nil {
		t.Errorf("empty file list: want nil err, got %v", err)
	}
	if hits != nil {
		t.Errorf("empty file list: want nil hits, got %+v", hits)
	}
}

func TestRunSearchesOnlyGivenFiles(t *testing.T) {
	skipIfNoRipgrep(t)

	// arrange — a non-.md sibling and a nested folder both contain the needle
	tmp := t.TempDir()
	writePage(t, tmp, "a.md", "needle\n")
	writePage(t, tmp, "b.txt", "needle\n")
	writePage(t, tmp, "sub/c.md", "needle\n")
	a := filepath.Join(tmp, "pages", "a.md")

	// act
	hits, err := Run([]string{a}, "needle")

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].FilePath != a {
		t.Fatalf("hits = %+v, want exactly one hit in %s", hits, a)
	}
}

// The index can go stale: a listed file deleted outside weft must not fail
// the search, and the other files' hits still come back.
func TestRunAndMentionsSkipVanishedFiles(t *testing.T) {
	skipIfNoRipgrep(t)

	// arrange — b.md is deleted after the file list was built
	tmp := t.TempDir()
	writePage(t, tmp, "a.md", "needle here\n")
	writePage(t, tmp, "b.md", "needle there\n")
	a := filepath.Join(tmp, "pages", "a.md")
	b := filepath.Join(tmp, "pages", "b.md")
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}

	// act
	runHits, runErr := Run([]string{a, b}, "needle")
	mentionHits, mentionErr := Mentions([]string{a, b}, "needle")

	// assert
	if runErr != nil {
		t.Fatalf("Run error = %v, want nil", runErr)
	}
	if len(runHits) != 1 || runHits[0].FilePath != a {
		t.Fatalf("Run hits = %+v, want exactly one hit in %s", runHits, a)
	}
	if mentionErr != nil {
		t.Fatalf("Mentions error = %v, want nil", mentionErr)
	}
	if len(mentionHits) != 1 || mentionHits[0].FilePath != a {
		t.Fatalf("Mentions hits = %+v, want exactly one hit in %s", mentionHits, a)
	}
}

func TestRunBatchesLargeFileLists(t *testing.T) {
	skipIfNoRipgrep(t)

	// arrange — 10 files with a match each plus a matchless one; a tiny budget
	// forces several rg invocations, including a batch with no match.
	tmp := t.TempDir()
	var files []string
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("p%02d.md", i)
		writePage(t, tmp, name, "needle\n")
		files = append(files, filepath.Join(tmp, "pages", name))
	}
	writePage(t, tmp, "none.md", "nothing\n")
	files = append(files[:5], append([]string{filepath.Join(tmp, "pages", "none.md")}, files[5:]...)...)
	old := argBudget
	argBudget = len(files[0]) * 3
	t.Cleanup(func() { argBudget = old })

	// act
	hits, err := Run(files, "needle")

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 10 {
		t.Fatalf("hits = %d, want 10: %+v", len(hits), hits)
	}
}

func TestBatchesRespectBudget(t *testing.T) {
	old := argBudget
	argBudget = 10
	t.Cleanup(func() { argBudget = old })

	got := batches([]string{"aaaa", "bbbb", "cccc", "waytoolongpath"})

	if len(got) != 3 || len(got[0]) != 2 || len(got[1]) != 1 || len(got[2]) != 1 {
		t.Fatalf("batches = %v, want [[aaaa bbbb] [cccc] [waytoolongpath]]", got)
	}
}

func TestMentionsWholeWordCaseInsensitive(t *testing.T) {
	skipIfNoRipgrep(t)

	// arrange — "Alpha" whole word (kept), "alpha" case variant (kept),
	// "Alphabet" substring (must NOT match under -w).
	tmp := t.TempDir()
	writePage(t, tmp, "Note.md", "see Alpha here\nan alpha mention\nAlphabet soup\n")

	// act
	hits, err := Mentions(graphFiles(t, tmp), "Alpha")

	// assert
	if err != nil {
		t.Fatalf("Mentions: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 whole-word hits (Alpha, alpha), got %d: %+v", len(hits), hits)
	}
}

func TestParseJSONClampsIndentShiftedSpans(t *testing.T) {
	// arrange: 4 leading spaces are trimmed from the context, shifting
	// spans left by 4. After the shift: {0,3}→{-4,-1} falls entirely in
	// the indent (dropped), {2,6}→{-2,2} straddles it (clamped to {0,2}),
	// {8,99}→{4,95} overhangs the line (clamped to {4,6}), {5,5}→{1,1}
	// is degenerate (dropped). A regression here slices Hit.Context out
	// of range and panics inside the alt-screen.
	line := `{"type":"match","data":{"path":{"text":"pages/X.md"},"lines":{"text":"    abcdef\n"},"line_number":3,"submatches":[{"start":0,"end":3},{"start":2,"end":6},{"start":8,"end":99},{"start":5,"end":5}]}}` + "\n"

	// act
	hits, err := parseJSON([]byte(line))

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	want := []Span{{Start: 0, End: 2}, {Start: 4, End: 6}}
	got := hits[0].Matches
	if len(got) != len(want) {
		t.Fatalf("spans = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	for _, sp := range got {
		if sp.Start < 0 || sp.End > len(hits[0].Context) || sp.Start >= sp.End {
			t.Errorf("span %+v can slice Context %q out of range", sp, hits[0].Context)
		}
	}
}
