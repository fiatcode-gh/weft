package graph

import (
	"fmt"
	"slices"
	"strings"
)

// beats reports whether a wins a name or path clash against b: journals
// first, then fewer '/' in Rel, then the smaller Rel (byte order).
func beats(a, b *PageMeta) bool {
	if a.IsJournal != b.IsJournal {
		return a.IsJournal
	}
	if da, db := strings.Count(a.Rel, "/"), strings.Count(b.Rel, "/"); da != db {
		return da < db
	}
	return a.Rel < b.Rel
}

// relStem is Rel without its ".md" suffix.
func relStem(p *PageMeta) string { return strings.TrimSuffix(p.Rel, ".md") }

// buildNames fills ByName, ByNameFold and the filename-stem map from
// idx.Pages, warns about clashes, and assigns every page its Key. It must
// run once Pages is final, so the pointers stay stable.
func (idx *Index) buildNames() {
	byName := make(map[string][]*PageMeta)
	byFold := make(map[string][]*PageMeta)
	idx.stems = make(map[string][]*PageMeta, len(idx.Pages))
	for i := range idx.Pages {
		p := &idx.Pages[i]
		byName[p.Name] = append(byName[p.Name], p)
		fold := strings.ToLower(p.Name)
		byFold[fold] = append(byFold[fold], p)
		rel := relStem(p)
		stem := strings.ToLower(rel[strings.LastIndexByte(rel, '/')+1:])
		idx.stems[stem] = append(idx.stems[stem], p)
	}

	sortGroup := func(g []*PageMeta) {
		slices.SortFunc(g, func(a, b *PageMeta) int {
			switch {
			case beats(a, b):
				return -1
			case beats(b, a):
				return 1
			}
			return 0
		})
	}
	for name, g := range byName {
		sortGroup(g)
		idx.ByName[name] = g[0]
	}
	for fold, g := range byFold {
		sortGroup(g)
		idx.ByNameFold[fold] = g[0]
	}

	// Warnings, ordered by winner Rel so they are deterministic.
	type warn struct{ rel, text string }
	var warns []warn
	for name, g := range byName {
		if len(g) < 2 {
			continue
		}
		rels := make([]string, len(g))
		for i, p := range g {
			rels[i] = p.Rel
		}
		warns = append(warns, warn{g[0].Rel, fmt.Sprintf("page name %q is shared by %d files (%s); [[%s]] opens %s",
			name, len(g), strings.Join(rels, ", "), name, g[0].Rel)})
	}
	for fold, g := range byFold {
		winner := g[0]
		var names []string
		for _, p := range g {
			if p.Name != winner.Name && !slices.Contains(names, p.Name) {
				names = append(names, p.Name)
			}
		}
		slices.Sort(names)
		for _, n := range names {
			warns = append(warns, warn{winner.Rel + "\x00" + n, fmt.Sprintf("ambiguous page name %q vs %q (case-insensitive); [[%s]] resolves to %q",
				n, winner.Name, fold, winner.Name)})
		}
	}

	// Keys: Name when it opens the page, else the shortest path suffix that
	// resolves back to it.
	for i := range idx.Pages {
		p := &idx.Pages[i]
		if idx.ByName[p.Name] == p {
			p.Key = p.Name
			continue
		}
		rel := relStem(p)
		segs := strings.Split(rel, "/")
		for n := 2; n <= len(segs); n++ {
			cand := strings.Join(segs[len(segs)-n:], "/")
			if got, ok := idx.Resolve(cand); ok && got == p {
				p.Key = cand
				break
			}
		}
		if p.Key == "" {
			p.Key = rel
			if got, ok := idx.Resolve(rel); !ok || got != p {
				warns = append(warns, warn{p.Rel + "\x01", p.Rel + " cannot be opened by a link: every path suffix opens another page"})
			}
		}
	}
	slices.SortStableFunc(warns, func(a, b warn) int { return strings.Compare(a.rel, b.rel) })
	for _, w := range warns {
		idx.Warnings = append(idx.Warnings, w.text)
	}
}

// resolvePath finds the page whose graph-relative path (without ".md") ends
// with target's segments, comparing whole segments case-insensitively. The
// target needs at least two non-empty segments.
func (idx *Index) resolvePath(target string) (*PageMeta, bool) {
	segs := strings.Split(target, "/")
	if len(segs) < 2 || slices.Contains(segs, "") {
		return nil, false
	}
	var best *PageMeta
	for _, p := range idx.stems[strings.ToLower(segs[len(segs)-1])] {
		if hasSegSuffix(relStem(p), segs) && (best == nil || beats(p, best)) {
			best = p
		}
	}
	return best, best != nil
}

// hasSegSuffix reports whether rel ends with segs on whole '/' segments,
// ignoring case.
func hasSegSuffix(rel string, segs []string) bool {
	for i := len(segs) - 1; i >= 0; i-- {
		j := strings.LastIndexByte(rel, '/')
		if !strings.EqualFold(rel[j+1:], segs[i]) {
			return false
		}
		if j < 0 {
			return i == 0
		}
		rel = rel[:j]
	}
	return true
}
