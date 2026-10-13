package graph

import (
	"cmp"
	"path"
	"slices"
	"strings"
)

// TreeNode is one row of the page tree: a page, a group of pages, or both.
type TreeNode struct {
	Label    string      // own segment only
	Path     string      // segments joined by "/"; group identity, case-sensitive
	Page     *PageMeta   // nil for a group-only node; points into the slice the tree was built from
	Children []*TreeNode // sorted; empty for a page-only node
	Count    int         // pages strictly below this node
}

// PageTree groups the graph's non-journal pages by folder and namespace.
func (idx *Index) PageTree() []*TreeNode { return buildPageTree(idx.Pages, idx.Layout.PagesDir()) }

func buildPageTree(pages []PageMeta, pagesDir string) []*TreeNode {
	var roots []*TreeNode
	primary := map[string]*TreeNode{}
	for i := range pages {
		p := &pages[i]
		if p.IsJournal {
			continue
		}
		segs := treeSegments(p, pagesDir)
		siblings := &roots
		for j, seg := range segs {
			full := strings.Join(segs[:j+1], "/")
			n := primary[full]
			if j < len(segs)-1 {
				if n == nil {
					n = &TreeNode{Label: seg, Path: full}
					primary[full] = n
					*siblings = append(*siblings, n)
				}
				siblings = &n.Children
				continue
			}
			switch {
			case n == nil:
				n = &TreeNode{Label: seg, Path: full, Page: p}
				primary[full] = n
				*siblings = append(*siblings, n)
			case n.Page == nil:
				n.Page = p
			case p.Rel < n.Page.Rel:
				*siblings = append(*siblings, &TreeNode{Label: seg, Path: full, Page: n.Page})
				n.Page = p
			default:
				*siblings = append(*siblings, &TreeNode{Label: seg, Path: full, Page: p})
			}
		}
	}
	finishTree(roots)
	return roots
}

// treeSegments is the folder path below pagesDir (or the root for files
// outside it) followed by the namespace segments of the page name.
func treeSegments(p *PageMeta, pagesDir string) []string {
	rel := p.Rel
	if pagesDir != "" && strings.HasPrefix(rel, pagesDir+"/") {
		rel = rel[len(pagesDir)+1:]
	}
	var segs []string
	if dir := path.Dir(rel); dir != "." {
		segs = strings.Split(dir, "/")
	}
	n := len(segs)
	segs = append(segs, strings.Split(p.Name, "/")...)
	segs = slices.DeleteFunc(segs, func(s string) bool { return s == "" })
	if len(segs) == n {
		segs = append(segs, strings.TrimSuffix(path.Base(p.Rel), ".md"))
	}
	return segs
}

// finishTree sorts siblings and fills Count bottom-up.
func finishTree(nodes []*TreeNode) {
	slices.SortFunc(nodes, func(a, b *TreeNode) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)),
			cmp.Compare(a.Label, b.Label),
			cmp.Compare(nodeRel(a), nodeRel(b)),
		)
	})
	for _, n := range nodes {
		finishTree(n.Children)
		for _, c := range n.Children {
			n.Count += c.Count
			if c.Page != nil {
				n.Count++
			}
		}
	}
}

func nodeRel(n *TreeNode) string {
	if n.Page == nil {
		return ""
	}
	return n.Page.Rel
}
