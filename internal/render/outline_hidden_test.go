package render

import "testing"

// Children that render as nothing (a logbook, a query drawer) give no fold:
// there would be a marker and a count over rows that are not there.
func TestOutlineHiddenOnlyChildrenGiveNoFold(t *testing.T) {
	cases := map[string]string{
		"logbook only":         "- DONE task\n  :LOGBOOK:\n  CLOCK: [2026-01-01 Thu 10:00]\n  :END:\n- next\n",
		"query after a bullet": "- leaf\n{{query (page-tags x)\n}}\n- next\n",
		"heading with logbook": "## H\n:LOGBOOK:\nCLOCK: [2026-01-01 Thu 10:00]\n:END:\n## I\ntext\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			for _, f := range Outline(body) {
				if f.Line == 0 {
					t.Fatalf("Outline(%q) folds line 0: %+v", body, f)
				}
			}
		})
	}
}

// A fold ends at the last row that renders, not at hidden lines after it.
func TestOutlineFoldEndsAtLastVisibleChild(t *testing.T) {
	body := "- a\n  child\n  :LOGBOOK:\n  CLOCK: [2026-01-01 Thu 10:00]\n  :END:\n- b\n"
	got := Outline(body)
	if len(got) != 1 || got[0].Line != 0 || got[0].Start != 1 || got[0].End != 2 {
		t.Fatalf("Outline(%q) = %+v, want one fold [1,2) on line 0", body, got)
	}
}

// Level 2 of the read view folds each heading's own body only: the lines up to
// the next heading of any level, so every heading stays visible.
func TestOutlineHeadingBodyStopsAtAnyHeading(t *testing.T) {
	body := "# Title\nintro\n## A\ntext a\n### A1\ntext a1\n## B\n## C\ntext c\n"
	want := map[int][2]int{0: {1, 2}, 2: {3, 4}, 4: {5, 6}, 7: {8, 9}}
	seen := 0
	for _, f := range Outline(body) {
		if f.Kind != FoldHeading {
			continue
		}
		seen++
		w, ok := want[f.Line]
		if !ok {
			w = [2]int{} // no body of its own
		}
		if f.BodyStart != w[0] || f.BodyEnd != w[1] {
			t.Errorf("heading at line %d: body [%d,%d), want [%d,%d)", f.Line, f.BodyStart, f.BodyEnd, w[0], w[1])
		}
	}
	if seen != 4 {
		t.Errorf("%d heading folds, want 4 (B has an empty section and no fold)", seen)
	}
}
