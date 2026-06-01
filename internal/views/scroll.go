package views

// clampInt returns v constrained to the inclusive range [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// scrollWindow returns the [start, end) slice indices of a list of `count`
// items to render in a window of `rows` rows, keeping `sel` visible and
// centering it when the list overflows the window. Callers pass a positive
// `rows` (the overlays derive it from visibleRows(), clamped to a minimum);
// the result is only meaningful for rows > 0.
func scrollWindow(sel, count, rows int) (start, end int) {
	if rows >= count {
		return 0, count
	}
	half := rows / 2
	start = sel - half
	if start < 0 {
		start = 0
	}
	end = start + rows
	if end > count {
		end = count
		start = end - rows
	}
	return start, end
}
