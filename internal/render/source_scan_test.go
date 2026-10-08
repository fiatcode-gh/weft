package render

import (
	"reflect"
	"strings"
	"testing"
)

type strLines []string

func (s strLines) Len() int          { return len(s) }
func (s strLines) Line(i int) string { return s[i] }

func scanAll(src SourceLines) []LineInfo {
	sc := NewScanner()
	out := make([]LineInfo, src.Len())
	for i := range out {
		out[i] = sc.Info(src, i)
		// The live-preview fields have their own tests (TestChunkStartRules).
		out[i].Start, out[i].Bullet, out[i].Subst = false, 0, false
	}
	return out
}

func TestScannerClassifiesDocuments(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want []LineInfo
	}{
		{"nested bullets and continuation", "- a\n  - b\n    - c\n  more\n", []LineInfo{
			{Kind: KindBullet, Level: 0, Indent: 0, Marker: 2},
			{Kind: KindBullet, Level: 1, Indent: 2, Marker: 4},
			{Kind: KindBullet, Level: 2, Indent: 4, Marker: 6},
			{Kind: KindText, Level: 2, Indent: 2, Marker: 2, InList: true},
			{Kind: KindBlank},
		}},
		{"sibling pops the stack", "- a\n  - b\n- c\n", []LineInfo{
			{Kind: KindBullet, Marker: 2},
			{Kind: KindBullet, Level: 1, Indent: 2, Marker: 4},
			{Kind: KindBullet, Marker: 2},
			{Kind: KindBlank},
		}},
		{"tab indented child", "- a\n\t- b", []LineInfo{
			{Kind: KindBullet, Marker: 2},
			{Kind: KindBullet, Level: 1, Indent: 4, Marker: 3},
		}},
		{"unindented text clears the list", "- a\nplain\n  indented", []LineInfo{
			{Kind: KindBullet, Marker: 2},
			{Kind: KindText},
			{Kind: KindText, Indent: 2, Marker: 2},
		}},
		{"ordered", "1. one\n10) ten", []LineInfo{
			{Kind: KindOrdered, Marker: 3},
			{Kind: KindOrdered, Marker: 4},
		}},
		{"quote", "> q\n>tight", []LineInfo{
			{Kind: KindQuote, Marker: 2},
			{Kind: KindQuote, Marker: 1},
		}},
		{"headings", "## Two\n#hash\n###### Six", []LineInfo{
			{Kind: KindHeading, Level: 2, Marker: 3},
			{Kind: KindText},
			{Kind: KindHeading, Level: 6, Marker: 7},
		}},
		{"fence in a bullet", "- ```go\n  x := 1\n  ```\nafter", []LineInfo{
			{Kind: KindFence, Fence: 0, Lang: "go", Strip: 2, Indent: 0},
			{Kind: KindCode, Fence: 0, Lang: "go", Strip: 2, Indent: 2},
			{Kind: KindFence, Fence: 0, Lang: "go", Strip: 2, Indent: 2},
			{Kind: KindText},
		}},
		{"tilde fence holds backticks", "~~~\n```\n~~~\n", []LineInfo{
			{Kind: KindFence, Fence: 0},
			{Kind: KindCode, Fence: 0},
			{Kind: KindFence, Fence: 0},
			{Kind: KindBlank},
		}},
		{"logbook hides a fence line without touching fence state", ":LOGBOOK:\n```\n:END:\nx\n", []LineInfo{
			{Kind: KindHidden},
			{Kind: KindHidden},
			{Kind: KindHidden},
			{Kind: KindText},
			{Kind: KindBlank},
		}},
		{"multi-line query and single-line embed", "{{query (and\n[[a]])\n}}\n{{embed [[X]]}}\nz", []LineInfo{
			{Kind: KindHidden},
			{Kind: KindHidden},
			{Kind: KindHidden},
			{Kind: KindHidden},
			{Kind: KindText},
		}},
		{"setext", "Title\n===\nSub\n---\n", []LineInfo{
			{Kind: KindSetextHeading, Level: 1},
			{Kind: KindSetextUnderline, Level: 1},
			{Kind: KindSetextHeading, Level: 2},
			{Kind: KindSetextUnderline, Level: 2},
			{Kind: KindBlank},
		}},
		{"rule after a blank line", "para\n\n---\n* * *", []LineInfo{
			{Kind: KindText},
			{Kind: KindBlank},
			{Kind: KindRule},
			{Kind: KindRule},
		}},
		{"table", "a | b\n--|--\n1 | 2\n\nafter", []LineInfo{
			{Kind: KindTable},
			{Kind: KindTable},
			{Kind: KindTable},
			{Kind: KindBlank},
			{Kind: KindText},
		}},
		{"pipes without a delimiter row are text", "a | b\nc | d", []LineInfo{
			{Kind: KindText},
			{Kind: KindText},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scanAll(strLines(strings.Split(tc.doc, "\n")))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("infos\n got  %+v\n want %+v", got, tc.want)
			}
		})
	}
}

func TestScannerInvalidate(t *testing.T) {
	lines := make(strLines, 100)
	for i := range lines {
		lines[i] = "line"
	}
	sc := NewScanner()
	if got := sc.Info(lines, 99); got.Kind != KindText {
		t.Fatalf("line 99 = %+v", got)
	}
	if sc.Scanned() != 100 {
		t.Fatalf("Scanned = %d, want 100", sc.Scanned())
	}
	sc.Info(lines, 99)
	if sc.Scanned() != 100 {
		t.Fatalf("cached Info rescanned: %d", sc.Scanned())
	}

	lines[50] = "```"
	sc.Invalidate(50)
	if got := sc.Info(lines, 99); got.Kind != KindCode || got.Fence != 50 {
		t.Fatalf("line 99 after opening a fence = %+v", got)
	}
	// Lines 49..99 are classified again: look-ahead makes line 49 depend on 50.
	if sc.Scanned() != 100+51 {
		t.Errorf("Scanned = %d, want %d", sc.Scanned(), 151)
	}
	if got := sc.Info(lines, 10); got.Kind != KindText {
		t.Errorf("line 10 = %+v", got)
	}
	if sc.Scanned() != 151 {
		t.Errorf("untouched line rescanned: %d", sc.Scanned())
	}
}

func TestScannerInvalidateSetextLookAhead(t *testing.T) {
	lines := strLines{"Title", "plain"}
	sc := NewScanner()
	if got := sc.Info(lines, 0); got.Kind != KindText {
		t.Fatalf("before edit: %+v", got)
	}
	lines[1] = "==="
	sc.Invalidate(1)
	if got := sc.Info(lines, 0); got.Kind != KindSetextHeading {
		t.Errorf("line 0 after the underline appeared = %+v", got)
	}
}
