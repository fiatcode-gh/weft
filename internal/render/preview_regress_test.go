package render

import "testing"

// Documents the chunker once got wrong: each splits (or loses a definition)
// where the read view's render of the whole page does not.
func TestPreviewParityRegressions(t *testing.T) {
	cases := []struct{ name, doc string }{
		{"definition title holding a tag", "[r]: /u \"about #tag\"\n\nsee [r]"},
		{"definition title holding a wiki link", "[r]: /u \"about [[Page]]\"\n\nsee [r]"},
		{"ordered list indented one to three spaces", "x\n\n  1. one\n\n1. two"},
		{"fence line inside a type 6 html block", "</details>\n```\n\n```\n- b"},
		{"fence line inside a type 7 html block", "<span>\n```\n\n```\n- b"},
	}
	if !inFreshProcess(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""}) {
		return
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, w := range []int{100, 61, 60, 23} {
				checkPreviewMatchesRender(t, tc.doc, w)
			}
		})
	}
}
