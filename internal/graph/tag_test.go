package graph

import (
	"reflect"
	"testing"
)

func TestFindTags(t *testing.T) {
	simple := func(start, end int, name string) Tag { return Tag{Start: start, End: end, Name: name} }
	tests := []struct {
		line string
		want []Tag
	}{
		{"#s", []Tag{simple(0, 2, "s")}},
		{"#kitchen.", []Tag{simple(0, 8, "kitchen")}},
		{"(#paren)", []Tag{simple(1, 7, "paren")}},
		{"a #x", []Tag{simple(2, 4, "x")}},
		{"a\t#x", []Tag{simple(2, 4, "x")}},
		{"a\u00a0#x", []Tag{simple(3, 5, "x")}},
		{"#area/home", []Tag{simple(0, 10, "area/home")}},
		{"#a-b_c", []Tag{simple(0, 6, "a-b_c")}},
		{"#café", []Tag{simple(0, 6, "café")}},
		{"#हिन्दी", []Tag{simple(0, 19, "हिन्दी")}},
		{"#cafe\u0301", []Tag{simple(0, 7, "cafe\u0301")}},
		{"#\u0301x", nil},
		{"#日本", []Tag{simple(0, 7, "日本")}},
		{"#add #cafe #bad", []Tag{simple(0, 4, "add"), simple(5, 10, "cafe"), simple(11, 15, "bad")}},
		{"#abc12", []Tag{simple(0, 6, "abc12")}},
		{"#a1b2c3d4e", []Tag{simple(0, 10, "a1b2c3d4e")}},
		{"#[[Book Club]]", []Tag{{Start: 0, End: 14, Bracket: true}}},
		{"#[[X|alias]]", []Tag{{Start: 0, End: 12, Bracket: true}}},
		{"#foo[[Bar]]", []Tag{simple(0, 4, "foo")}},
		{"[see #x](u)", []Tag{simple(5, 7, "x")}},
		{"#x`code`", []Tag{simple(0, 2, "x")}},
		{"C#", nil},
		{"repo#12", nil},
		{"url/#frag", nil},
		{"##", nil},
		{"## x", nil},
		{"# Title", nil},
		{"#18", nil},
		{"PR #5", nil},
		{"(#12)", nil},
		{`#{"TODO"}`, nil},
		{"#!", nil},
		{"#+BEGIN_QUERY", nil},
		{"#-x", nil},
		{"#FAF3E7", nil},
		{"#f2e309", nil},
		{"#C2552F", nil},
		{"#a1b", nil},
		{"#a1b2", nil},
		{"#c0ffee00", nil},
		{"[[C#]]", nil},
		{"[[Lab #inner]]", nil},
		{"[x](#frag)", nil},
		{"[x](a #b)", nil},
		{"`#code`", nil},
		{"a `b #x ", nil},
		{"`c`#x", nil},
		{"a#[[X]]", nil},
		{"##[[X]]", nil},
	}
	for _, tc := range tests {
		if got := FindTags(tc.line); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("FindTags(%q) = %+v, want %+v", tc.line, got, tc.want)
		}
	}
}

func TestTagStartAt(t *testing.T) {
	tests := []struct {
		line string
		i    int
		want bool
	}{
		{"#a", 0, true},
		{"x #a", 2, true},
		{"(#a", 1, true},
		{"x#a", 1, false},
		{"##a", 1, false},
		{"/#a", 1, false},
		{"`#a", 1, false},
		{"[[a #b]]", 4, false},
		{"[l](#d)", 4, false},
		{"`x #a`", 3, false},
		{"abc", 1, false},
	}
	for _, tc := range tests {
		if got := TagStartAt(tc.line, tc.i); got != tc.want {
			t.Errorf("TagStartAt(%q, %d) = %v, want %v", tc.line, tc.i, got, tc.want)
		}
	}
}

var simpleTagNames = []string{"kitchen", "kb/notes", "café", "add"}

func TestIsSimpleTagName(t *testing.T) {
	for _, n := range simpleTagNames {
		if !IsSimpleTagName(n) {
			t.Errorf("IsSimpleTagName(%q) = false, want true", n)
		}
	}
	for _, n := range []string{"", "Book Club", "2026-05-24", "f2e309", "a1b", "x!"} {
		if IsSimpleTagName(n) {
			t.Errorf("IsSimpleTagName(%q) = true, want false", n)
		}
	}
}

func TestFindTagsRoundTripsSimpleNames(t *testing.T) {
	for _, n := range simpleTagNames {
		got := FindTags("#" + n)
		if len(got) != 1 || got[0].Name != n || got[0].Bracket {
			t.Errorf("FindTags(%q) = %+v, want one tag named %q", "#"+n, got, n)
		}
	}
}
