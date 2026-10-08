package buffer

import (
	"reflect"
	"testing"
)

func TestMatchInAgreesWithFind(t *testing.T) {
	cases := []struct{ name, line, query string }{
		{"fold lower", "Alpha alpha ALPHA", "alpha"},
		{"exact upper", "Alpha alpha ALPHA", "Alpha"},
		{"exact all caps", "Alpha alpha ALPHA", "ALPHA"},
		{"combining marks", "cafe\u0301 cafe caf\u00e9", "cafe"},
		{"combining query", "cafe\u0301 cafe", "e\u0301"},
		{"wide runes", "日本語 日本 日本語", "日本"},
		{"repeated", "aaaa aaa", "aa"},
		{"adjacent", "abab ab", "ab"},
		{"unicode fold", "Straße STRASSE", "straße"},
		{"no match", "hello", "xyz"},
		{"empty query", "hello", ""},
		{"newline query", "hello\nworld", "o\nw"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var want [][2]int
			for _, r := range New(c.line).Find(c.query) {
				if r.Start.Line == 0 {
					want = append(want, [2]int{r.Start.Col, r.End.Col})
				}
			}
			if got := MatchIn(c.line, c.query); !reflect.DeepEqual(got, want) {
				t.Errorf("MatchIn(%q, %q) = %v, Find says %v", c.line, c.query, got, want)
			}
		})
	}
}
