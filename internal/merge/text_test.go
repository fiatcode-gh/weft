package merge

import (
	"slices"
	"testing"
)

func TestText(t *testing.T) {
	tests := []struct {
		name               string
		base, mine, theirs string
		want               string
		wantConflict       bool
		wantMineLine       []int
	}{
		{"both append, theirs adds newline", "a", "a\nb", "a\nc\n", "a\nb\nc\n", false, []int{0, 1, 3}},
		{"only mine changed, no newline added", "a", "a\nb", "a", "a\nb", false, []int{0, 1, 2}},
		{"mine removes final newline", "a\n", "a", "a\nc\n", "a\nc", false, []int{0, 2}},
		{"theirs appends, state unchanged", "a\nb", "a\nb", "a\nb\nc", "a\nb\nc", false, []int{0, 1, 3}},
		{"empty base, mine adds line", "", "x\n", "", "x\n", false, []int{0, 1}},
		{"empty base, theirs adds text", "", "", "y", "y", false, []int{1}},
		{"CRLF, no state change", "a\r\nb", "a\r\nb\r\nm", "t\r\na\r\nb", "t\r\na\r\nb\r\nm", false, []int{1, 2, 3, 4}},
		{"CRLF both append", "a\r\nb", "a\r\nb\r\nm", "a\r\nb\r\nt", "a\r\nb\r\nm\r\nt", false, []int{0, 1, 2, 4}},
		{"overlapping edit conflicts", "a\nb\nc", "a\nX\nc", "a\nY\nc", "", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Text(tt.base, tt.mine, tt.theirs)
			if got.Conflict != tt.wantConflict {
				t.Fatalf("Conflict = %v, want %v", got.Conflict, tt.wantConflict)
			}
			if got.Text != tt.want {
				t.Errorf("Text = %q, want %q", got.Text, tt.want)
			}
			if !tt.wantConflict && !slices.Equal(got.MineLine, tt.wantMineLine) {
				t.Errorf("MineLine = %v, want %v", got.MineLine, tt.wantMineLine)
			}
		})
	}
}
