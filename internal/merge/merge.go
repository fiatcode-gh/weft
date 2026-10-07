// Package merge is a pure, git-style three-way line merge used by the in-app editor's save; it never touches disk.
package merge

import "strings"

// Result is the outcome of Lines.
type Result struct {
	Text     string // merged text; "" when Conflict
	Conflict bool
	// MineLine[i] is the 0-based merged line holding mine's line i.
	// len == number of lines in mine + 1; the last entry is the position just
	// past the end of the merged text.
	MineLine []int
}

// Lines merges mine and theirs, both derived from base. Changes conflict when
// they touch the same or adjacent base lines; insertions at exactly the same
// spot are both kept, mine first. The merge is byte-faithful: it never adds or
// drops a final newline.
func Lines(base, mine, theirs string) Result {
	baseL, mineL, theirsL := splitLines(base), splitLines(mine), splitLines(theirs)
	ids := map[string]int{}
	intern := func(ls []string) []int {
		out := make([]int, len(ls))
		for i, l := range ls {
			id, ok := ids[l]
			if !ok {
				id = len(ids)
				ids[l] = id
			}
			out[i] = id
		}
		return out
	}
	baseI, mineI, theirsI := intern(baseL), intern(mineL), intern(theirsL)

	hm, ok := diff(baseI, mineI)
	if !ok {
		return Result{Conflict: true}
	}
	ht, ok := diff(baseI, theirsI)
	if !ok {
		return Result{Conflict: true}
	}

	out := make([]string, 0, len(baseL)+len(mineL)+len(theirsL))
	mineLine := make([]int, len(mineL)+1)
	pos := 0     // next unemitted base line
	deltaM := 0  // mine index minus base index before the current group
	i, j := 0, 0 // next hunk in hm, ht
	emitUntouched := func(to int) {
		for ; pos < to; pos++ {
			mineLine[pos+deltaM] = len(out)
			out = append(out, baseL[pos])
		}
	}

	for i < len(hm) || j < len(ht) {
		var lo int
		switch {
		case j >= len(ht) || (i < len(hm) && hm[i].aLo <= ht[j].aLo):
			lo = hm[i].aLo
		default:
			lo = ht[j].aLo
		}
		hi := lo
		i0, j0 := i, j
		for {
			switch {
			case i < len(hm) && hm[i].aLo <= hi:
				hi = max(hi, hm[i].aHi)
				i++
			case j < len(ht) && ht[j].aLo <= hi:
				hi = max(hi, ht[j].aHi)
				j++
			default:
				goto grouped
			}
		}
	grouped:
		emitUntouched(lo)

		mStart := lo + deltaM
		mChunk, tChunk := baseL[lo:hi], baseL[lo:hi]
		if i > i0 {
			mChunk = mineL[hm[i0].bLo-(hm[i0].aLo-lo) : hm[i-1].bHi+(hi-hm[i-1].aHi)]
		}
		if j > j0 {
			tChunk = theirsL[ht[j0].bLo-(ht[j0].aLo-lo) : ht[j-1].bHi+(hi-ht[j-1].aHi)]
		}
		mineChanged, theirsChanged := i > i0, j > j0
		outStart := len(out)

		switch {
		case mineChanged && !theirsChanged, mineChanged && equal(mChunk, tChunk):
			out = append(out, mChunk...)
			mapMine(mineLine, mStart, len(mChunk), outStart)
		case theirsChanged && !mineChanged:
			out = append(out, tChunk...)
			for k := range hi - lo {
				mineLine[mStart+k] = outStart + min(k, max(len(tChunk)-1, 0))
			}
		case lo == hi:
			out = append(out, mChunk...)
			out = append(out, tChunk...)
			mapMine(mineLine, mStart, len(mChunk), outStart)
		default:
			return Result{Conflict: true}
		}

		if i > i0 {
			deltaM = hm[i-1].bHi - hm[i-1].aHi
		}
		pos = hi
	}
	emitUntouched(len(baseL))
	mineLine[len(mineL)] = len(out)

	return Result{Text: strings.Join(out, ""), MineLine: mineLine}
}

func mapMine(mineLine []int, mStart, n, outStart int) {
	for k := range n {
		mineLine[mStart+k] = outStart + k
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// splitLines splits s into lines that keep their "\n". A trailing unterminated
// segment is its own line; "" has no lines.
func splitLines(s string) []string {
	var lines []string
	for s != "" {
		n := strings.IndexByte(s, '\n') + 1
		if n == 0 {
			n = len(s)
		}
		lines = append(lines, s[:n])
		s = s[n:]
	}
	return lines
}
