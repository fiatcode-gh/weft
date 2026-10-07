package buffer

import (
	"strings"
	"testing"
)

func manyLines(n int, final bool) string {
	var sb strings.Builder
	for i := range n {
		sb.WriteString("line ")
		sb.WriteString(strings.Repeat("x", i%17))
		if i < n-1 || final {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func TestRoundTrip(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"a":             "a",
		"a nl":          "a\n",
		"crlf trailing": "a\r\nb\r\n",
		"crlf no final": "a\r\nb",
		"mixed":         "a\r\nb\nc",
		"lone cr":       "x\ry\n",
		"tabs":          "\ta\t\tb\n\t",
		"invalid utf8":  "\xff\n",
		"nul":           "a\x00b\n\x00",
		"only nl":       "\n",
		"only crlf":     "\r\n",
		"blank lines":   "\n\n\n",
		"cr at end":     "a\r",
		"many final":    manyLines(20000, true),
		"many no final": manyLines(20000, false),
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			b := New(s)
			if got := b.String(); got != s {
				t.Fatalf("String() mismatch for %q", name)
			}
			if !b.EqualString(s) {
				t.Fatalf("EqualString(s) = false")
			}
			if b.EqualString(s + "x") {
				t.Fatalf("EqualString(s+x) = true")
			}
			if len(s) > 0 && b.EqualString(s[:len(s)-1]) {
				t.Fatalf("EqualString(prefix) = true")
			}
		})
	}
}

func TestLines(t *testing.T) {
	b := New("a\r\nb\n")
	if b.Len() != 3 {
		t.Fatalf("Len = %d, want 3", b.Len())
	}
	for i, want := range []string{"a", "b", ""} {
		if got := b.Line(i); got != want {
			t.Errorf("Line(%d) = %q, want %q", i, got, want)
		}
	}
	if got := b.End(); got != (Pos{2, 0}) {
		t.Errorf("End = %v", got)
	}
	if New("").Len() != 1 {
		t.Errorf("empty buffer must have one line")
	}
	if got := New("x\ry").Line(0); got != "x\ry" {
		t.Errorf("lone CR must stay text, got %q", got)
	}
}

func TestEOL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "\n"},
		{"a", "\n"},
		{"a\nb\n", "\n"},
		{"a\r\nb\r\n", "\r\n"},
		{"a\r\nb\nc\n", "\n"},
		{"a\r\nb\r\nc\n", "\r\n"},
		{"a\r\nb\n", "\n"}, // tie → LF
	}
	for _, c := range cases {
		if got := New(c.in).EOL(); got != c.want {
			t.Errorf("EOL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEqualStringDoesNotAllocate(t *testing.T) {
	s := manyLines(10000, true)
	b := New(s)
	allocs := testing.AllocsPerRun(20, func() {
		if !b.EqualString(s) {
			t.Fatal("not equal")
		}
	})
	if allocs != 0 {
		t.Fatalf("EqualString allocated %v times", allocs)
	}
}

func TestVersionBumpsPerMutation(t *testing.T) {
	b := New("")
	v := b.Version()
	b.Type("a")
	if b.Version() != v+1 {
		t.Fatalf("Type: version %d, want %d", b.Version(), v+1)
	}
	b.Undo()
	if b.Version() != v+2 {
		t.Fatalf("Undo: version %d, want %d", b.Version(), v+2)
	}
	b.Redo()
	if b.Version() != v+3 {
		t.Fatalf("Redo: version %d, want %d", b.Version(), v+3)
	}
	b.MoveTo(Pos{0, 0}, false)
	if b.Version() != v+3 {
		t.Fatalf("MoveTo must not bump the version")
	}
}
