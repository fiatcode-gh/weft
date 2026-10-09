package graph

import (
	"errors"
	"strings"
	"testing"
)

func TestParseTaskPrefix(t *testing.T) {
	cases := []struct {
		line   string
		ok     bool
		marker string
		ms, me int
		prio   string
		ps, pe int
	}{
		{"- TODO x", true, "TODO", 2, 6, "", 0, 0},
		{"- DOING x", true, "DOING", 2, 7, "", 0, 0},
		{"- LATER x", true, "LATER", 2, 7, "", 0, 0},
		{"- WAITING x", true, "WAITING", 2, 9, "", 0, 0},
		{"- NOW x", true, "NOW", 2, 5, "", 0, 0},
		{"- DONE x", true, "DONE", 2, 6, "", 0, 0},
		{"- CANCELED x", true, "CANCELED", 2, 10, "", 0, 0},
		{"- CANCELLED x", true, "CANCELLED", 2, 11, "", 0, 0},
		{"  - TODO x", true, "TODO", 4, 8, "", 0, 0},
		{"\t- TODO x", true, "TODO", 3, 7, "", 0, 0},
		{"- TODO [#A] x", true, "TODO", 2, 6, "A", 7, 11},
		{"- LATER [#B] x", true, "LATER", 2, 7, "B", 8, 12},
		{"- NOW  [#C] x", true, "NOW", 2, 5, "C", 7, 11},
		{"- TODO [#A]x", true, "TODO", 2, 6, "", 0, 0},
		{"- TODO [#A]", true, "TODO", 2, 6, "A", 7, 11},
		{"- TODO [#D] x", true, "TODO", 2, 6, "", 0, 0},
		{"- DONE\r", true, "DONE", 2, 6, "", 0, 0},
		{"- TODO: x", false, "", 0, 0, "", 0, 0},
		{"- TODOx", false, "", 0, 0, "", 0, 0},
		{"* TODO x", false, "", 0, 0, "", 0, 0},
		{"TODO x", false, "", 0, 0, "", 0, 0},
	}
	for _, c := range cases {
		got, ok := ParseTaskPrefix(c.line)
		if ok != c.ok {
			t.Errorf("%q: ok = %v, want %v", c.line, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		want := TaskPrefix{c.marker, c.ms, c.me, c.prio, c.ps, c.pe}
		if got != want {
			t.Errorf("%q: got %+v, want %+v", c.line, got, want)
		}
	}
}

func TestNextMarker(t *testing.T) {
	cases := map[string]string{
		"TODO": "DONE", "LATER": "DONE", "DOING": "DONE", "WAITING": "DONE", "NOW": "DONE",
		"DONE": "", "CANCELED": "", "CANCELLED": "",
		"": "TODO",
	}
	for in, want := range cases {
		if got := NextMarker(in); got != want {
			t.Errorf("NextMarker(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetTaskMarker(t *testing.T) {
	cases := []struct {
		name, line, marker string
		out                string
		at, delta          int
		ok                 bool
	}{
		{"priority kept", "- LATER [#A] x", "DONE", "- DONE [#A] x", 2, -1, true},
		{"tab and CR kept", "\t- TODO x\r", "DONE", "\t- DONE x\r", 3, 0, true},
		{"done to plain", "- DONE x", "", "- x", 2, -5, true},
		{"done to plain with priority", "- DONE [#A] x", "", "- [#A] x", 2, -5, true},
		{"bare done", "- DONE", "", "- ", 2, -4, true},
		{"marker followed by tab", "- DONE\tx", "", "- x", 2, -5, true},
		{"plain to todo", "- x", "TODO", "- TODO x", 2, 5, true},
		{"nested plain to todo", "  - x", "TODO", "  - TODO x", 4, 5, true},
		{"plain stays plain", "- x", "", "- x", 2, 0, true},
		{"not a bullet", "# h", "TODO", "# h", 0, 0, false},
		{"star bullet", "* x", "TODO", "* x", 0, 0, false},
	}
	for _, c := range cases {
		out, at, delta, ok := SetTaskMarker(c.line, c.marker)
		if out != c.out || at != c.at || delta != c.delta || ok != c.ok {
			t.Errorf("%s: got (%q,%d,%d,%v), want (%q,%d,%d,%v)", c.name, out, at, delta, ok, c.out, c.at, c.delta, c.ok)
		}
	}
}

func TestNextPriority(t *testing.T) {
	for in, want := range map[string]string{"": "A", "A": "B", "B": "C", "C": ""} {
		if got := NextPriority(in); got != want {
			t.Errorf("NextPriority(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetTaskPriority(t *testing.T) {
	cases := []struct {
		name, line, p, out string
		at, delta          int
		ok                 bool
	}{
		{"add", "- TODO x", "A", "- TODO [#A] x", 6, 5, true},
		{"add at end of line", "- TODO", "A", "- TODO [#A]", 6, 5, true},
		{"change", "- LATER [#A] x", "C", "- LATER [#C] x", 7, 0, true},
		{"remove", "- TODO [#B] x", "", "- TODO x", 6, -5, true},
		{"remove at end of line", "- TODO [#A]", "", "- TODO", 6, -5, true},
		{"remove from none", "- TODO x", "", "- TODO x", 6, 0, true},
		{"CR kept", "- DONE x\r", "B", "- DONE [#B] x\r", 6, 5, true},
		{"non-task bullet", "- x", "A", "- x", 0, 0, false},
		{"non-task line", "# h", "A", "# h", 0, 0, false},
	}
	for _, c := range cases {
		out, at, delta, ok := SetTaskPriority(c.line, c.p)
		if out != c.out || at != c.at || delta != c.delta || ok != c.ok {
			t.Errorf("%s: got (%q,%d,%d,%v), want (%q,%d,%d,%v)", c.name, out, at, delta, ok, c.out, c.at, c.delta, c.ok)
		}
	}
}

func TestTaskTail(t *testing.T) {
	if got := TaskTail("A", "x y"); got != "[#A] x y" {
		t.Errorf("with priority: %q", got)
	}
	if got := TaskTail("", "x y"); got != "x y" {
		t.Errorf("without priority: %q", got)
	}
}

func TestMarkTask(t *testing.T) {
	t.Run("at the hint", func(t *testing.T) {
		out, line, err := MarkTask("- a\n- TODO x\n- b", 2, "TODO", "DONE", "x")
		if err != nil || line != 2 || out != "- a\n- DONE x\n- b" {
			t.Fatalf("got (%q,%d,%v)", out, line, err)
		}
	})
	t.Run("hint shifted by an inserted line picks the nearest", func(t *testing.T) {
		out, line, err := MarkTask("new\n- a\n- TODO x\n- b", 2, "TODO", "DONE", "x")
		if err != nil || line != 3 || out != "new\n- a\n- DONE x\n- b" {
			t.Fatalf("got (%q,%d,%v)", out, line, err)
		}
	})
	t.Run("identical tasks pick the hint", func(t *testing.T) {
		out, line, err := MarkTask("- TODO x\n- TODO x\n- TODO x", 2, "TODO", "DONE", "x")
		if err != nil || line != 2 || out != "- TODO x\n- DONE x\n- TODO x" {
			t.Fatalf("got (%q,%d,%v)", out, line, err)
		}
	})
	t.Run("tie goes to the earlier line", func(t *testing.T) {
		_, line, err := MarkTask("- TODO x\nmid\n- TODO x", 2, "TODO", "DONE", "x")
		if err != nil || line != 1 {
			t.Fatalf("got line %d err %v, want 1", line, err)
		}
	})
	t.Run("fenced lookalike ignored", func(t *testing.T) {
		body := "```\n- TODO x\n```\n- TODO x"
		out, line, err := MarkTask(body, 2, "TODO", "DONE", "x")
		if err != nil || line != 4 || out != "```\n- TODO x\n```\n- DONE x" {
			t.Fatalf("got (%q,%d,%v)", out, line, err)
		}
	})
	t.Run("whitespace variant tail matches", func(t *testing.T) {
		out, _, err := MarkTask("- TODO  [#A]   a   b ", 1, "TODO", "DONE", "[#A] a b")
		if err != nil || out != "- DONE  [#A]   a   b " {
			t.Fatalf("got (%q,%v)", out, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		for _, c := range []struct{ body, from, tail string }{
			{"- TODO y", "TODO", "x"},
			{"- DONE x", "TODO", "x"},
			{"x", "TODO", "x"},
		} {
			if _, _, err := MarkTask(c.body, 1, c.from, "DONE", c.tail); !errors.Is(err, ErrTaskNotFound) {
				t.Errorf("%q: err = %v", c.body, err)
			}
		}
	})
	t.Run("undo direction", func(t *testing.T) {
		out, line, err := MarkTask("- DONE x", 1, "DONE", "LATER", "x")
		if err != nil || line != 1 || out != "- LATER x" {
			t.Fatalf("got (%q,%d,%v)", out, line, err)
		}
	})
	t.Run("CRLF body changes only the task line", func(t *testing.T) {
		body := "# h\r\n- TODO x\r\nplain\r\n"
		out, line, err := MarkTask(body, 2, "TODO", "DONE", "x")
		if err != nil || line != 2 {
			t.Fatalf("got (%d,%v)", line, err)
		}
		in, got := strings.Split(body, "\n"), strings.Split(out, "\n")
		if len(in) != len(got) {
			t.Fatalf("line count %d -> %d", len(in), len(got))
		}
		for i := range in {
			if i == 1 {
				if got[i] != "- DONE x\r" {
					t.Errorf("task line %q", got[i])
				}
				continue
			}
			if got[i] != in[i] {
				t.Errorf("line %d: %q != %q", i, got[i], in[i])
			}
		}
	})
}
