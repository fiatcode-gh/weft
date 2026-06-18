package search

import (
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

// writePage seeds a page file under dir/pages/name with the given body and
// returns the graph root (dir). It is the shared arrange step for tests that
// grep over a freshly built temp graph.
func writePage(t *testing.T, dir, name, body string) {
	t.Helper()
	pages := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
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
	hits := parseJSON([]byte(in))

	// assert
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

func TestRunFindsHits(t *testing.T) {
	skipIfNoRipgrep(t)

	// arrange
	graph := fixtureGraph(t)

	// act
	hits, err := Run(graph, "Beta")

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
	hits, err := Run(graph, "thisstringshouldnotexistanywhere_xyzzy_1234")

	// assert
	if err != nil {
		t.Fatalf("no-match should not error, got: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("no-match: want 0 hits, got %d", len(hits))
	}
}

func TestRunMissingDirsReturnsNil(t *testing.T) {
	// arrange — an empty temp dir has neither pages/ nor journals/.
	tmp := t.TempDir()

	// act
	hits, err := Run(tmp, "anything")

	// assert
	if err != nil {
		t.Errorf("missing dirs: want nil err, got %v", err)
	}
	if hits != nil {
		t.Errorf("missing dirs: want nil hits, got %+v", hits)
	}
}

func TestMentionsWholeWordCaseInsensitive(t *testing.T) {
	skipIfNoRipgrep(t)

	// arrange — "Alpha" whole word (kept), "alpha" case variant (kept),
	// "Alphabet" substring (must NOT match under -w).
	tmp := t.TempDir()
	writePage(t, tmp, "Note.md", "see Alpha here\nan alpha mention\nAlphabet soup\n")

	// act
	hits, err := Mentions(tmp, "Alpha")

	// assert
	if err != nil {
		t.Fatalf("Mentions: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 whole-word hits (Alpha, alpha), got %d: %+v", len(hits), hits)
	}
}
