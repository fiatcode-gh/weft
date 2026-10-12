package graph

import "strings"

// Resolve returns the page a link target opens: the page whose name matches
// exactly, else whose case-folded name matches, else (target of two or more
// non-empty '/' segments) the page whose graph-relative path without ".md"
// ends with the target on whole segments, case-insensitively. A clash is won
// by a journal, then the shallower path, then the smaller path. The
// case-preserving PageMeta is always returned.
func (idx *Index) Resolve(name string) (*PageMeta, bool) {
	if p, ok := idx.ByName[name]; ok {
		return p, true
	}
	if p, ok := idx.ByNameFold[strings.ToLower(name)]; ok {
		return p, true
	}
	return idx.resolvePath(name)
}

// LinkDest is where a link target leads: a page, and a heading on it when
// the target names one.
type LinkDest struct {
	Page    *PageMeta
	Heading string // fragment as written, trimmed; "" = the page itself
}

// ResolveLink resolves a link target to a page, or to a heading on a page for
// [[Page#Heading]]. A page named by the whole target wins; otherwise the target
// splits at its last '#' and the fragment must match a heading of the page
// (HeadingKey). Anything else is unresolved.
func (idx *Index) ResolveLink(target string) (LinkDest, bool) {
	if p, ok := idx.Resolve(target); ok {
		return LinkDest{Page: p}, true
	}
	i := strings.LastIndexByte(target, '#')
	if i <= 0 {
		return LinkDest{}, false
	}
	name, frag := strings.TrimSpace(target[:i]), strings.TrimSpace(target[i+1:])
	if name == "" || frag == "" {
		return LinkDest{}, false
	}
	p, ok := idx.Resolve(name)
	if !ok {
		return LinkDest{}, false
	}
	key := HeadingKey(frag)
	if key == "" {
		return LinkDest{}, false
	}
	if _, ok := idx.headingKeys[p.Path][key]; !ok {
		return LinkDest{}, false
	}
	return LinkDest{Page: p, Heading: frag}, true
}
