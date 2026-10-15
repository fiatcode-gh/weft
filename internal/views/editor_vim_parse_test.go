package views

import "testing"

func TestVimParseModes(t *testing.T) {
	for _, k := range []string{"h", "l", "i", "a", "I", "A", "o", "O", "u", "ctrl+r"} {
		c, st := parseVim([]string{k}, false)
		if st != vimDone || c.key != k || c.op != "" || c.count != 0 || c.arg != "" {
			t.Errorf("parseVim(%q) = %+v, %v; want key %q done", k, c, st, k)
		}
	}
	bad := [][]string{{"z"}, {"Q"}, {"d"}, {"esc"}, {"enter"}, {" "}, {"h", "l"}, {"i", "x"}}
	for _, toks := range bad {
		if _, st := parseVim(toks, false); st != vimBad {
			t.Errorf("parseVim(%q) = %v; want vimBad", toks, st)
		}
	}
	if _, st := parseVim(nil, false); st != vimMore {
		t.Errorf("parseVim(nil) = %v; want vimMore", st)
	}
}
