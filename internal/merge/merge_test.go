package merge

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

func TestLines(t *testing.T) {
	tests := []struct {
		name                string
		base, mine, theirs  string
		want                string
		wantConflict        bool
		wantSingleTrailingN bool
	}{
		{name: "append at end while editing above", base: "a\nb\nc\n", mine: "A\nb\nc\n", theirs: "a\nb\nc\nd\n", want: "A\nb\nc\nd\n"},
		{name: "distant separate lines", base: "1\n2\n3\n4\n5\n", mine: "X\n2\n3\n4\n5\n", theirs: "1\n2\n3\n4\nY\n", want: "X\n2\n3\n4\nY\n"},
		{name: "same line", base: "1\n2\n3\n", mine: "1\nM\n3\n", theirs: "1\nT\n3\n", wantConflict: true},
		{name: "adjacent lines", base: "1\n2\n3\n4\n", mine: "1\nM\n3\n4\n", theirs: "1\n2\nT\n4\n", wantConflict: true},
		{name: "insert next to change", base: "a\nb\n", mine: "a\nB\n", theirs: "a\nb\nc\n", wantConflict: true},
		{name: "insert at the point of a change", base: "a\nb\n", mine: "a\nm\nb\n", theirs: "a\nB\n", wantConflict: true},
		{name: "insert right after a change", base: "a\nb\n", mine: "a\nm\nb\n", theirs: "A\nb\n", wantConflict: true},
		{name: "one untouched line apart", base: "1\n2\n3\n", mine: "M\n2\n3\n", theirs: "1\n2\nT\n", want: "M\n2\nT\n"},
		{name: "identical change both sides", base: "1\n2\n3\n", mine: "1\nX\n3\n", theirs: "1\nX\n3\n", want: "1\nX\n3\n"},
		{name: "identical append both sides", base: "a\n", mine: "a\nz\n", theirs: "a\nz\n", want: "a\nz\n"},
		{name: "both append at end", base: "a\nb\n", mine: "a\nb\nm\n", theirs: "a\nb\nt1\nt2\n", want: "a\nb\nm\nt1\nt2\n"},
		{name: "both append a copy of the last line", base: "a\n", mine: "a\na\nm\n", theirs: "a\nt\n", want: "a\na\nm\nt\n"},
		{name: "both insert in middle", base: "a\nb\n", mine: "a\nm\nb\n", theirs: "a\nt\nb\n", want: "a\nm\nt\nb\n"},
		{name: "same-spot overlapping inserts not deduped", base: "a\n", mine: "a\nx\n", theirs: "a\nx\ny\n", want: "a\nx\nx\ny\n"},
		{name: "same-spot insert plus edit elsewhere", base: "a\nb\nc\n", mine: "A\nb\nc\nm\n", theirs: "a\nb\nc\nt\n", want: "A\nb\nc\nm\nt\n"},
		{name: "trailing newline kept", base: "a\nb\nc\n", mine: "A\nb\nc\n", theirs: "a\nb\nc\nd\n", want: "A\nb\nc\nd\n", wantSingleTrailingN: true},
		{name: "no trailing newline kept", base: "a\nb\nc", mine: "A\nb\nc", theirs: "a\nb\nc", want: "A\nb\nc"},
		{name: "empty base, both insert", base: "", mine: "x\n", theirs: "y\n", want: "x\ny\n"},
		{name: "empty base, identical insert", base: "", mine: "x\n", theirs: "x\n", want: "x\n"},
		{name: "empty base, theirs only", base: "", mine: "", theirs: "y\n", want: "y\n"},
		{name: "theirs unchanged", base: "a\nb\n", mine: "a\nB\n", theirs: "a\nb\n", want: "a\nB\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lines(tt.base, tt.mine, tt.theirs)

			if got.Conflict != tt.wantConflict {
				t.Fatalf("Conflict = %v, want %v (text %q)", got.Conflict, tt.wantConflict, got.Text)
			}
			if tt.wantConflict {
				if got.Text != "" || got.MineLine != nil {
					t.Fatalf("conflict result carries data: %+v", got)
				}
				return
			}
			if got.Text != tt.want {
				t.Fatalf("Text = %q, want %q", got.Text, tt.want)
			}
			if tt.wantSingleTrailingN && (!strings.HasSuffix(got.Text, "\n") || strings.HasSuffix(got.Text, "\n\n")) {
				t.Fatalf("want exactly one trailing newline, got %q", got.Text)
			}
			if n := len(splitLines(tt.mine)) + 1; len(got.MineLine) != n {
				t.Fatalf("len(MineLine) = %d, want %d", len(got.MineLine), n)
			}
		})
	}
}

func TestLinesMineLine(t *testing.T) {
	t.Run("edit below a theirs insertion at the top", func(t *testing.T) {
		base := "l1\nl2\nl3\nl4\nl5\n"
		mine := "l1\nl2\nl3\nXl4\nl5\n"
		theirs := "n1\nn2\nl1\nl2\nl3\nl4\nl5\n"

		got := Lines(base, mine, theirs)

		if got.Conflict {
			t.Fatal("unexpected conflict")
		}
		if got.MineLine[3] != 5 || got.MineLine[0] != 2 || got.MineLine[len(got.MineLine)-1] != 7 {
			t.Fatalf("MineLine = %v", got.MineLine)
		}
	})
	t.Run("theirs-only replaced line maps to the replacement start", func(t *testing.T) {
		base := "1\n2\n3\n4\n5\n"
		mine := "M\n2\n3\n4\n5\n"
		theirs := "1\n2\nT1\nT2\n4\n5\n"

		got := Lines(base, mine, theirs)

		if got.Text != "M\n2\nT1\nT2\n4\n5\n" {
			t.Fatalf("Text = %q", got.Text)
		}
		if got.MineLine[2] != 2 || got.MineLine[3] != 4 {
			t.Fatalf("MineLine = %v", got.MineLine)
		}
	})
	t.Run("theirs-deleted line maps to the following merged line", func(t *testing.T) {
		base := "1\n2\n3\n4\n5\n"
		mine := "M\n2\n3\n4\n5\n"
		theirs := "1\n2\n4\n5\n"

		got := Lines(base, mine, theirs)

		if got.Text != "M\n2\n4\n5\n" {
			t.Fatalf("Text = %q", got.Text)
		}
		if got.MineLine[2] != 2 {
			t.Fatalf("MineLine = %v", got.MineLine)
		}
	})
	t.Run("same-spot group keeps mine lines first", func(t *testing.T) {
		got := Lines("a\n", "a\nm1\nm2\n", "a\nt\n")

		if got.Text != "a\nm1\nm2\nt\n" {
			t.Fatalf("Text = %q", got.Text)
		}
		if want := []int{0, 1, 2, 4}; !reflect.DeepEqual(got.MineLine, want) {
			t.Fatalf("MineLine = %v, want %v", got.MineLine, want)
		}
	})
}

func TestLinesEditCapIsConflict(t *testing.T) {
	var base, mine, theirs strings.Builder
	for i := range 600 {
		fmt.Fprintf(&base, "base%d\n", i)
		fmt.Fprintf(&mine, "mine%d\n", i)
		fmt.Fprintf(&theirs, "theirs%d\n", i)
	}

	got := Lines(base.String(), mine.String(), theirs.String())

	if !got.Conflict {
		t.Fatal("want Conflict when the edit distance exceeds the cap")
	}
}

func TestDiffProperty(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	randSeq := func() []int {
		s := make([]int, rng.IntN(31))
		for i := range s {
			s[i] = rng.IntN(3)
		}
		return s
	}
	for i := range 500 {
		a, b := randSeq(), randSeq()

		hunks, ok := diff(a, b)

		if !ok {
			t.Fatalf("case %d: diff refused a small input", i)
		}
		var got []int
		pos := 0
		for h, hk := range hunks {
			if h > 0 {
				prev := hunks[h-1]
				ga, gb := hk.aLo-prev.aHi, hk.bLo-prev.bHi
				if ga != gb || ga < 1 {
					t.Fatalf("case %d: gaps a=%d b=%d between hunks %v", i, ga, gb, hunks)
				}
			}
			got = append(got, a[pos:hk.aLo]...)
			got = append(got, b[hk.bLo:hk.bHi]...)
			pos = hk.aHi
		}
		got = append(got, a[pos:]...)
		if !reflect.DeepEqual(append([]int(nil), got...), append([]int(nil), b...)) && !(len(got) == 0 && len(b) == 0) {
			t.Fatalf("case %d: applying %v to %v gave %v, want %v", i, hunks, a, got, b)
		}
	}
}
