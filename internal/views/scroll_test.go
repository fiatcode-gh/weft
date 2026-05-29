package views

import "testing"

func TestClampInt(t *testing.T) {
	cases := []struct {
		v, lo, hi, want int
	}{
		{5, 0, 10, 5},
		{-1, 0, 10, 0},
		{11, 0, 10, 10},
		{0, 0, 10, 0},
		{10, 0, 10, 10},
	}
	for _, c := range cases {
		if got := clampInt(c.v, c.lo, c.hi); got != c.want {
			t.Errorf("clampInt(%d,%d,%d): want %d, got %d", c.v, c.lo, c.hi, c.want, got)
		}
	}
}

func TestScrollWindowAllFit(t *testing.T) {
	start, end := scrollWindow(2, 3, 10)
	if start != 0 || end != 3 {
		t.Errorf("count <= rows: want [0,3), got [%d,%d)", start, end)
	}
}

func TestScrollWindowCenters(t *testing.T) {
	const count, rows = 20, 6
	cases := []struct {
		name      string
		sel       int
		wantStart int
	}{
		{"top", 0, 0},
		{"middle", 10, 10 - rows/2},
		{"bottom", 19, count - rows},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := scrollWindow(c.sel, count, rows)
			if end-start != rows {
				t.Errorf("window size: want %d, got %d ([%d,%d))", rows, end-start, start, end)
			}
			if start != c.wantStart {
				t.Errorf("start: want %d, got %d", c.wantStart, start)
			}
			if c.sel < start || c.sel >= end {
				t.Errorf("sel %d not in [%d,%d)", c.sel, start, end)
			}
		})
	}
}
