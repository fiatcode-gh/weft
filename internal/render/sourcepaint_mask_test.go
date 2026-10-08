package render

import (
	"reflect"
	"testing"
)

func TestMaskLinks(t *testing.T) {
	cases := []struct {
		name, line, masked string
		spans              []linkSpan
	}{
		{"simple tag", "a #topic b", "a xxxxxx b", []linkSpan{{2, 8, 2, 8}}},
		{"bracket tag", "#[[B]] x", "xxxxxx x", []linkSpan{{0, 1, 0, 1}, {1, 6, 3, 4}}},
		{"not tags", "C# `#x` [x](#f)", "C# `#x` [x](#f)", nil},
		{"emphasis", "_a #b_c_ d_", "_a xxxxx d_", []linkSpan{{3, 8, 3, 8}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			masked, spans := maskLinks(tc.line)
			if masked != tc.masked || !reflect.DeepEqual(spans, tc.spans) {
				t.Fatalf("maskLinks(%q) = %q, %v; want %q, %v", tc.line, masked, spans, tc.masked, tc.spans)
			}
		})
	}
}
