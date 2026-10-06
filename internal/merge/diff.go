package merge

// hunk means a[aLo:aHi] is replaced by b[bLo:bHi].
type hunk struct{ aLo, aHi, bLo, bHi int }

// maxEditDistance caps insertions+deletions after prefix/suffix trimming. The
// Myers trace costs about (D+1)² ints, so the cap bounds memory at a few MB.
const maxEditDistance = 1000

// diff returns the maximal hunks turning a into b. Consecutive hunks always
// have at least one common line between them. ok is false when the edit
// distance exceeds maxEditDistance.
func diff(a, b []int) (hunks []hunk, ok bool) {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	ma, mb := a[p:len(a)-s], b[p:len(b)-s]
	n, m := len(ma), len(mb)
	if n == 0 && m == 0 {
		return nil, true
	}

	dmax := min(n+m, maxEditDistance)
	off := dmax + 1
	v := make([]int, 2*dmax+3)
	trace := make([][]int, 0, dmax+1)
	found := -1
search:
	for d := 0; d <= dmax; d++ {
		trace = append(trace, append([]int(nil), v[off-d:off+d+1]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && ma[x] == mb[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				found = d
				break search
			}
		}
	}
	if found < 0 {
		return nil, false
	}

	// Backtrack, collecting single-step edits last-to-first. Each edit starts
	// at (ai, bi) and ends one step later: down (insertion) or right (deletion).
	type edit struct {
		ai, bi int
		insert bool
	}
	edits := make([]edit, 0, found)
	for d, k := found, n-m; d > 0; d-- {
		prev := trace[d] // v after step d-1, indexed k+d
		insert := k == -d || (k != d && prev[k-1+d] < prev[k+1+d])
		pk := k - 1
		if insert {
			pk = k + 1
		}
		px := prev[pk+d]
		edits = append(edits, edit{px, px - pk, insert})
		k = pk
	}

	// Walk forward, merging edits with no common line between them.
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		ai, bi := e.ai+p, e.bi+p
		last := len(hunks) - 1
		if last < 0 || hunks[last].aHi != ai || hunks[last].bHi != bi {
			hunks = append(hunks, hunk{ai, ai, bi, bi})
			last++
		}
		if e.insert {
			hunks[last].bHi++
		} else {
			hunks[last].aHi++
		}
	}
	return hunks, true
}
