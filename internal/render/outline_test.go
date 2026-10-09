package render

import (
	"reflect"
	"testing"
)

func TestOutline(t *testing.T) {
	h := func(line, level, start, end, bodyStart, bodyEnd int) Fold {
		return Fold{Line: line, Kind: FoldHeading, Level: level, Start: start, End: end, BodyStart: bodyStart, BodyEnd: bodyEnd}
	}
	b := func(line, level, start, end int) Fold {
		return Fold{Line: line, Kind: FoldBullet, Level: level, Start: start, End: end}
	}
	cases := []struct {
		name string
		body string
		want []Fold
	}{
		{"ATX nesting", "## A\ntext a\n### A1\ntext a1\n## B\ntext b\n",
			[]Fold{h(0, 2, 1, 4, 1, 2), h(2, 3, 3, 4, 3, 4), h(4, 2, 5, 6, 5, 6)}},
		{"setext start skips the underline", "Title\n=====\ntext\n\nSub\n---\nmore\n",
			[]Fold{h(0, 1, 2, 7, 2, 3), h(4, 2, 6, 7, 6, 7)}},
		{"empty section has no fold", "## A\n\n## B\ntext\n", []Fold{h(2, 2, 3, 4, 3, 4)}},
		{"trailing blanks are trimmed", "## A\ntext\n\n\n## B\ntext\n", []Fold{h(0, 2, 1, 2, 1, 2), h(4, 2, 5, 6, 5, 6)}},
		{"nested bullets two levels", "- a\n  - a1\n    - a2\n- b\n",
			[]Fold{b(0, 0, 1, 3), b(1, 1, 2, 3)}},
		{"bullet without children has no fold", "- a\n- b\n", nil},
		{"ordered items", "1. one\n   - x\n2. two\n", []Fold{b(0, 0, 1, 2)}},
		{"logbook and indented fence are children",
			"- [ ] task\n  :LOGBOOK:\n  CLOCK: [2026-01-01 Thu 10:00]\n  :END:\n  ```\n  code\n  ```\n- next\n",
			[]Fold{b(0, 0, 1, 7)}},
		{"a heading ends a bullet's children", "- a\n  child\n## H\ntext\n",
			[]Fold{b(0, 0, 1, 2), {Line: 2, Kind: FoldHeading, Level: 2, Start: 3, End: 4, BodyStart: 3, BodyEnd: 4}}},
		{"unindented text ends children", "- a\n  child\nplain\n", []Fold{b(0, 0, 1, 2)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Outline(c.body)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Outline(%q)\n got %+v\nwant %+v", c.body, got, c.want)
			}
		})
	}
}
