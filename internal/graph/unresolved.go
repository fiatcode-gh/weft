package graph

import (
	"slices"
	"sort"
	"strings"
)

// UnresolvedLink is one wiki-link target that resolves to no page, with
// every reference to it across the graph.
type UnresolvedLink struct {
	Target string
	Refs   []Ref
}

// UnresolvedLinks returns every link target that Resolve cannot find —
// exact and case-folded lookup, matching how the TUI resolves links —
// each with a cloned snapshot of its references in index walk order.
// Targets are sorted case-insensitively; case-sensitive byte order
// breaks ties (defensive: folded keys are unique, so ties do not arise
// from the backlinks map).
func (idx *Index) UnresolvedLinks() []UnresolvedLink {
	var out []UnresolvedLink
	for key, refs := range idx.backlinks {
		if _, ok := idx.Resolve(idx.targetSpell[key]); ok {
			continue
		}
		out = append(out, UnresolvedLink{
			Target: idx.targetSpell[key],
			Refs:   slices.Clone(refs),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		fi, fj := strings.ToLower(out[i].Target), strings.ToLower(out[j].Target)
		if fi != fj {
			return fi < fj
		}
		return out[i].Target < out[j].Target
	})
	return out
}
