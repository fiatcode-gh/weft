package views

import "testing"

func TestVimParseModes(t *testing.T) {
	for _, k := range []string{"h", "l", "i", "a", "I", "A", "o", "O", "u", "ctrl+r"} {
		c, st := parseVim([]string{k}, false)
		if st != vimDone || c.key != k || c.op != "" || c.count != 0 || c.arg != "" {
			t.Errorf("parseVim(%q) = %+v, %v; want key %q done", k, c, st, k)
		}
	}
	bad := [][]string{{"z"}, {"Q"}, {"esc"}, {"enter"}, {" "}, {"h", "l"}, {"i", "x"}}
	for _, toks := range bad {
		if _, st := parseVim(toks, false); st != vimBad {
			t.Errorf("parseVim(%q) = %v; want vimBad", toks, st)
		}
	}
	if _, st := parseVim(nil, false); st != vimMore {
		t.Errorf("parseVim(nil) = %v; want vimMore", st)
	}
}

func TestVimParseMotions(t *testing.T) {
	done := []struct {
		toks []string
		want vimCmd
	}{
		{[]string{"3", "w"}, vimCmd{count: 3, key: "w"}},
		{[]string{"1", "0", "j"}, vimCmd{count: 10, key: "j"}},
		{[]string{"0"}, vimCmd{key: "0"}},
		{[]string{"2", "g", "g"}, vimCmd{count: 2, key: "gg"}},
		{[]string{"g", "g"}, vimCmd{key: "gg"}},
		{[]string{"f", "x"}, vimCmd{key: "f", arg: "x"}},
		{[]string{"3", "t", "x"}, vimCmd{count: 3, key: "t", arg: "x"}},
		{[]string{"F", "é"}, vimCmd{key: "F", arg: "é"}},
		{[]string{"T", " "}, vimCmd{key: "T", arg: " "}},
		{[]string{";"}, vimCmd{key: ";"}},
		{[]string{","}, vimCmd{key: ","}},
		{[]string{"5", "G"}, vimCmd{count: 5, key: "G"}},
		{[]string{"G"}, vimCmd{key: "G"}},
		{[]string{"%"}, vimCmd{key: "%"}},
		{[]string{"5", "0", "%"}, vimCmd{count: 50, key: "%"}},
		{[]string{"ctrl+d"}, vimCmd{key: "ctrl+d"}},
		{[]string{"5", "ctrl+u"}, vimCmd{count: 5, key: "ctrl+u"}},
		{[]string{"9", "9", "9", "9", "9", "9", "j"}, vimCmd{count: 99999, key: "j"}},
		{[]string{"2", "0", "0", "$"}, vimCmd{count: 200, key: "$"}},
	}
	for _, tc := range done {
		if c, st := parseVim(tc.toks, false); st != vimDone || c != tc.want {
			t.Errorf("parseVim(%q) = %+v, %v; want %+v done", tc.toks, c, st, tc.want)
		}
	}
	for _, key := range []string{"w", "b", "e", "W", "B", "E", "^", "$", "j", "k", "{", "}"} {
		if c, st := parseVim([]string{key}, false); st != vimDone || c.key != key {
			t.Errorf("parseVim(%q) = %+v, %v; want done", key, c, st)
		}
	}
	more := [][]string{{"2"}, {"2", "0"}, {"g"}, {"2", "g"}, {"f"}, {"3", "T"}}
	for _, toks := range more {
		if _, st := parseVim(toks, false); st != vimMore {
			t.Errorf("parseVim(%q) = %v; want vimMore", toks, st)
		}
	}
	bad := [][]string{{"g", "x"}, {"2", "g", "x"}, {"f", "esc"}, {"f", "enter"}, {"t", "ab"}, {"2", "z"}, {"w", "w"}, {"0", "0"}, {"2", "0", "0", "z"}}
	for _, toks := range bad {
		if _, st := parseVim(toks, false); st != vimBad {
			t.Errorf("parseVim(%q) = %v; want vimBad", toks, st)
		}
	}
}
