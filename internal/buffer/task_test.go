package buffer

import (
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// stampBuf opens content with the cursor at (line, col).
func stampBuf(content string, line, col int) *Buffer {
	b := New(content)
	b.cursor = Pos{line, col}
	return b
}

func TestSetTaskStampPlacement(t *testing.T) {
	const d = "2026-05-25"
	cases := []struct {
		name    string
		in      string
		line    int
		kind    graph.StampKind
		want    string
		prepare func(*Buffer)
	}{
		{"scheduled under first line", "- TODO x", 0, graph.StampScheduled,
			"- TODO x\n  SCHEDULED: <2026-05-25 Mon>", nil},
		{"deadline after scheduled", "- TODO x\n  SCHEDULED: <2026-05-20 Wed>", 0, graph.StampDeadline,
			"- TODO x\n  SCHEDULED: <2026-05-20 Wed>\n  DEADLINE: <2026-05-25 Mon>", nil},
		{"deadline alone at First+1", "- TODO x", 0, graph.StampDeadline,
			"- TODO x\n  DEADLINE: <2026-05-25 Mon>", nil},
		{"scheduled before an existing deadline", "- TODO x\n  DEADLINE: <2026-05-30 Sat>", 0, graph.StampScheduled,
			"- TODO x\n  SCHEDULED: <2026-05-25 Mon>\n  DEADLINE: <2026-05-30 Sat>", nil},
		{"tab indent and priority", "\t- TODO [#A] x", 0, graph.StampScheduled,
			"\t- TODO [#A] x\n\t  SCHEDULED: <2026-05-25 Mon>", nil},
		{"nested task", "- a\n  - LATER y", 1, graph.StampScheduled,
			"- a\n  - LATER y\n    SCHEDULED: <2026-05-25 Mon>", nil},
		{"before the child bullet", "- TODO x\n  - child", 0, graph.StampScheduled,
			"- TODO x\n  SCHEDULED: <2026-05-25 Mon>\n  - child", nil},
	}
	for _, c := range cases {
		b := stampBuf(c.in, c.line, 0)
		if changed, ok := b.SetTaskStamp(c.kind, d); !changed || !ok {
			t.Errorf("%s: SetTaskStamp = (%v,%v), want (true,true)", c.name, changed, ok)
		}
		if got := b.String(); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestSetTaskStampRewriteKeepsTimeAndRepeater(t *testing.T) {
	b := stampBuf("- TODO x\n  SCHEDULED: <2026-05-25 Mon 09:30 .+1w>  ", 0, 0)
	if changed, ok := b.SetTaskStamp(graph.StampScheduled, "2026-06-01"); !changed || !ok {
		t.Fatalf("SetTaskStamp = (%v,%v)", changed, ok)
	}
	if got, want := b.String(), "- TODO x\n  SCHEDULED: <2026-06-01 Mon 09:30 .+1w>"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetTaskStampClear(t *testing.T) {
	b := stampBuf("- TODO x\n  SCHEDULED: <2026-05-25 Mon>\n  DEADLINE: <2026-05-30 Sat>\n- y", 0, 0)
	if changed, ok := b.SetTaskStamp(graph.StampScheduled, ""); !changed || !ok {
		t.Fatalf("clear = (%v,%v)", changed, ok)
	}
	if got, want := b.String(), "- TODO x\n  DEADLINE: <2026-05-30 Sat>\n- y"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	undos := len(b.undo)
	if changed, ok := b.SetTaskStamp(graph.StampScheduled, ""); changed || !ok {
		t.Errorf("clearing a missing stamp = (%v,%v), want (false,true)", changed, ok)
	}
	if len(b.undo) != undos {
		t.Error("clearing a missing stamp opened an undo group")
	}
}

func TestSetTaskStampFromContinuationLine(t *testing.T) {
	for _, line := range []int{1, 2} {
		b := stampBuf("- TODO x\n  SCHEDULED: <2026-05-25 Mon>\n  more text", line, 0)
		if d, ok := b.TaskStamp(graph.StampScheduled); !ok || d != "2026-05-25" {
			t.Errorf("line %d: TaskStamp = (%q,%v)", line, d, ok)
		}
		if changed, ok := b.SetTaskStamp(graph.StampDeadline, "2026-06-01"); !changed || !ok {
			t.Errorf("line %d: SetTaskStamp = (%v,%v)", line, changed, ok)
		}
	}
	b := stampBuf("- TODO x", 0, 0)
	if d, ok := b.TaskStamp(graph.StampDeadline); !ok || d != "" {
		t.Errorf("TaskStamp without a stamp = (%q,%v), want (\"\",true)", d, ok)
	}
}

func TestSetTaskStampNonTask(t *testing.T) {
	for name, c := range map[string]struct {
		in   string
		line int
	}{
		"plain bullet":  {"- plain", 0},
		"heading":       {"# Title", 0},
		"child bullet":  {"- TODO x\n  - child", 1},
		"before bullet": {"intro\n- TODO x", 0},
	} {
		b := stampBuf(c.in, c.line, 0)
		if _, ok := b.TaskStamp(graph.StampScheduled); ok {
			t.Errorf("%s: TaskStamp ok", name)
		}
		if changed, ok := b.SetTaskStamp(graph.StampScheduled, "2026-05-25"); changed || ok {
			t.Errorf("%s: SetTaskStamp = (%v,%v), want (false,false)", name, changed, ok)
		}
		if len(b.undo) != 0 || b.String() != c.in {
			t.Errorf("%s: buffer touched", name)
		}
	}
}

func TestSetTaskStampOneUndoStep(t *testing.T) {
	const in = "- TODO x\n  SCHEDULED: <2026-05-20 Wed>"
	for name, date := range map[string]string{"insert": "2026-05-25", "clear": ""} {
		kind := graph.StampDeadline
		if date == "" {
			kind = graph.StampScheduled
		}
		b := stampBuf(in, 0, 3)
		b.SetTaskStamp(kind, date)
		if len(b.undo) != 1 {
			t.Fatalf("%s: %d undo groups, want 1", name, len(b.undo))
		}
		b.Undo()
		if b.String() != in || b.cursor != (Pos{0, 3}) {
			t.Errorf("%s: undo gave %q at %v", name, b.String(), b.cursor)
		}
	}
}

func TestSetTaskStampCursor(t *testing.T) {
	// Insert: a cursor at or after the inserted line moves down by one.
	b := stampBuf("- TODO x\n  more", 1, 3)
	b.SetTaskStamp(graph.StampScheduled, "2026-05-25")
	if b.cursor != (Pos{2, 3}) {
		t.Errorf("insert: cursor %v, want {2 3}", b.cursor)
	}
	b = stampBuf("- TODO x\n  more", 0, 3)
	b.SetTaskStamp(graph.StampScheduled, "2026-05-25")
	if b.cursor != (Pos{0, 3}) {
		t.Errorf("insert above cursor: cursor %v, want {0 3}", b.cursor)
	}
	// Delete: later lines move up; the deleted line sends the cursor home.
	b = stampBuf("- TODO x\n  SCHEDULED: <2026-05-25 Mon>\n  more", 2, 4)
	b.SetTaskStamp(graph.StampScheduled, "")
	if b.cursor != (Pos{1, 4}) {
		t.Errorf("delete: cursor %v, want {1 4}", b.cursor)
	}
	b = stampBuf("- TODO x\n  SCHEDULED: <2026-05-25 Mon>\n  more", 1, 20)
	b.SetTaskStamp(graph.StampScheduled, "")
	if b.cursor != (Pos{0, 8}) {
		t.Errorf("delete own line: cursor %v, want {0 8}", b.cursor)
	}
	// Rewrite: the column is clamped.
	b = stampBuf("- TODO x\n  SCHEDULED: <2026-05-25 Mon>   ", 1, 40)
	b.SetTaskStamp(graph.StampScheduled, "2026-05-26")
	if want := (Pos{1, len("  SCHEDULED: <2026-05-26 Tue>")}); b.cursor != want {
		t.Errorf("rewrite: cursor %v, want %v", b.cursor, want)
	}
}

func TestSetTaskStampCRLF(t *testing.T) {
	b := stampBuf("- TODO x\r\n- y\r\n", 0, 0)
	b.SetTaskStamp(graph.StampScheduled, "2026-05-25")
	if got, want := b.String(), "- TODO x\r\n  SCHEDULED: <2026-05-25 Mon>\r\n- y\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetTaskStampMixedEOLKeepsTaskLineTerminator(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"lf task line":   {"- TODO x\n- b\r\n- c\r\n", "- TODO x\n  SCHEDULED: <2026-05-25 Mon>\n- b\r\n- c\r\n"},
		"crlf task line": {"- TODO x\r\n- b\n- c\n", "- TODO x\r\n  SCHEDULED: <2026-05-25 Mon>\r\n- b\n- c\n"},
	} {
		t.Run(name, func(t *testing.T) {
			b := stampBuf(c.in, 0, 0)
			b.SetTaskStamp(graph.StampScheduled, "2026-05-25")
			if got := b.String(); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestCyclePriority(t *testing.T) {
	for _, line := range []int{0, 1} {
		b := stampBuf("- TODO x\n  more", line, 0)
		for _, want := range []string{"- TODO [#A] x", "- TODO [#B] x", "- TODO [#C] x", "- TODO x"} {
			if !b.CyclePriority() {
				t.Fatalf("line %d: CyclePriority false", line)
			}
			if got := b.Line(0); got != want {
				t.Errorf("line %d: got %q, want %q", line, got, want)
			}
		}
		if len(b.undo) != 4 {
			t.Errorf("line %d: %d undo groups, want 4", line, len(b.undo))
		}
	}
	b := stampBuf("- DONE x", 0, 0)
	if !b.CyclePriority() || b.Line(0) != "- DONE [#A] x" {
		t.Errorf("DONE: got %q", b.Line(0))
	}
	for _, in := range []string{"- plain", "# h", "- TODO x\n  - child"} {
		line := len(New(in).lines) - 1
		b := stampBuf(in, line, 0)
		if b.CyclePriority() || len(b.undo) != 0 {
			t.Errorf("%q: cycled a non-task", in)
		}
	}
}

func TestCyclePriorityCursor(t *testing.T) {
	b := stampBuf("- TODO x", 0, 8)
	b.CyclePriority() // adds " [#A]"
	if b.cursor != (Pos{0, 13}) {
		t.Errorf("after add: cursor %v, want {0 13}", b.cursor)
	}
	b = stampBuf("- TODO [#C] x", 0, 13)
	b.CyclePriority() // removes " [#C]"
	if b.cursor != (Pos{0, 8}) {
		t.Errorf("after remove: cursor %v, want {0 8}", b.cursor)
	}
	b = stampBuf("- TODO x", 0, 3)
	b.CyclePriority()
	if b.cursor != (Pos{0, 3}) {
		t.Errorf("cursor before the marker moved to %v", b.cursor)
	}
}
