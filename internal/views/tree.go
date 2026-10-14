package views

import (
	"fmt"
	"strings"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

const (
	treeVisibleRowsMax = 20
	// treeChrome is the terminal rows the panel spends outside the tree rows:
	// border (2), padding (2), title, blank line, rule, the two scroll lines,
	// blank line, error line and footer.
	treeChrome = 12
)

// treeExpansion is the session's expanded tree groups, keyed by TreeNode.Path.
// Kept on App across openings; never written anywhere.
type treeExpansion map[string]bool

type treeRow struct {
	node   *graph.TreeNode
	depth  int
	parent int // row index of the parent, -1 at top level
}

// Tree is the page-tree overlay (F).
type Tree struct {
	listBox
	roots  []*graph.TreeNode
	total  int // pages in the tree
	open   treeExpansion
	rows   []treeRow
	errMsg string
}

// NewTree builds the tree for idx with the groups in open expanded, opens the
// ancestors of the current page's node and puts the cursor on that node.
func NewTree(idx *graph.Index, current string, open treeExpansion, width, height int) *Tree {
	t := &Tree{listBox: listBox{width: width, height: height}, open: open}
	t.setRoots(idx.PageTree())
	var target *graph.TreeNode
	if p, ok := idx.Resolve(current); ok && !p.IsJournal {
		var ancestors []*graph.TreeNode
		if findTreeNode(t.roots, p.Path, &ancestors) != nil {
			target = ancestors[len(ancestors)-1]
			for _, a := range ancestors[:len(ancestors)-1] {
				open[a.Path] = true
			}
		}
	}
	t.flatten()
	if target != nil {
		for i, r := range t.rows {
			if r.node == target {
				t.sel = i
				break
			}
		}
	}
	return t
}

// setRoots installs roots and recounts the pages in the tree.
func (t *Tree) setRoots(roots []*graph.TreeNode) {
	t.roots = roots
	t.total = 0
	for _, r := range roots {
		t.total += r.Count
		if r.Page != nil {
			t.total++
		}
	}
}

// refresh rebuilds the tree from idx, keeping the expanded groups and the
// selected node (by Path); when it is gone the cursor stays on the nearest row.
func (t *Tree) refresh(idx *graph.Index) {
	old := t.sel
	path, had := "", false
	if t.sel >= 0 && t.sel < len(t.rows) {
		path, had = t.rows[t.sel].node.Path, true
	}
	t.setRoots(idx.PageTree())
	t.flatten()
	t.sel = clampInt(old, 0, max(0, len(t.rows)-1))
	if had {
		for i, r := range t.rows {
			if r.node.Path == path {
				t.sel = i
				break
			}
		}
	}
}

// findTreeNode returns the node holding the page at file path pagePath; on a
// hit, chain is the node's ancestors followed by the node itself.
func findTreeNode(nodes []*graph.TreeNode, pagePath string, chain *[]*graph.TreeNode) *graph.TreeNode {
	for _, n := range nodes {
		*chain = append(*chain, n)
		if n.Page != nil && n.Page.Path == pagePath {
			return n
		}
		if hit := findTreeNode(n.Children, pagePath, chain); hit != nil {
			return hit
		}
		*chain = (*chain)[:len(*chain)-1]
	}
	return nil
}

func (t *Tree) flatten() {
	t.rows = t.rows[:0]
	var walk func(nodes []*graph.TreeNode, depth, parent int)
	walk = func(nodes []*graph.TreeNode, depth, parent int) {
		for _, n := range nodes {
			t.rows = append(t.rows, treeRow{node: n, depth: depth, parent: parent})
			if len(n.Children) > 0 && t.open[n.Path] {
				walk(n.Children, depth+1, len(t.rows)-1)
			}
		}
	}
	walk(t.roots, 0, -1)
}

// SetError shows msg inside the panel until the next key.
func (t *Tree) SetError(msg string) { t.errMsg = msg }

func (t *Tree) Update(key string) OverlayResult {
	t.errMsg = ""
	switch key {
	case keyEsc, keyQ, keyShiftF:
		return overlayCancel()
	case "c":
		return overlayCapture()
	}
	if len(t.rows) == 0 {
		return OverlayResult{}
	}
	row := t.rows[t.sel]
	n := row.node
	isGroup := len(n.Children) > 0
	switch key {
	case keyUp, keyK:
		t.moveUp()
	case keyDown, keyJ:
		t.moveDown(len(t.rows))
	case "right", "l":
		switch {
		case !isGroup:
		case !t.open[n.Path]:
			t.open[n.Path] = true
			t.flatten()
		default:
			t.sel++
		}
	case "left", "h":
		switch {
		case isGroup && t.open[n.Path]:
			delete(t.open, n.Path)
			t.flatten()
		case row.parent >= 0:
			t.sel = row.parent
		}
	case keyEnter:
		switch {
		case n.Page != nil:
			return overlayOpen(n.Page.Key)
		case !t.open[n.Path]:
			t.open[n.Path] = true
			t.flatten()
		default:
			delete(t.open, n.Path)
			t.flatten()
		}
	}
	return OverlayResult{}
}

func (t *Tree) visibleRows() int {
	return clampInt(t.height-treeChrome, listVisibleRowsMin, treeVisibleRowsMax)
}

func (t *Tree) View() string {
	inner := t.innerWidth()
	var sb strings.Builder
	sb.WriteString(styleTitle.Render("Page tree"))
	sb.WriteString(styleFaint.Render(fmt.Sprintf("   (%d)", t.total)))
	sb.WriteString("\n\n")
	sb.WriteString(styleFaint.Render(strings.Repeat("─", inner)))
	sb.WriteString("\n")
	if len(t.rows) == 0 {
		sb.WriteString(styleFaint.Render(clamp("  no pages — journals are not listed here", inner)))
		sb.WriteString("\n")
	} else {
		start, end := scrollWindow(t.sel, len(t.rows), t.visibleRows())
		if start > 0 {
			sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↑ %d more above", start)))
			sb.WriteString("\n")
		}
		for i := start; i < end; i++ {
			t.writeRow(&sb, i, inner)
		}
		if below := len(t.rows) - end; below > 0 {
			sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↓ %d more below", below)))
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")
	if t.errMsg != "" {
		sb.WriteString(styleTitle.Render(clamp(t.errMsg, inner)))
		sb.WriteString("\n")
	}
	sb.WriteString(styleFaint.Render(clamp("↑/↓ move · →/← expand/collapse · enter open · c capture · esc close", inner)))
	return renderBordered(inner+4, sb.String())
}

func (t *Tree) writeRow(sb *strings.Builder, i, inner int) {
	r := t.rows[i]
	glyph := "  "
	if len(r.node.Children) > 0 {
		glyph = "▸ "
		if t.open[r.node.Path] {
			glyph = "▾ "
		}
	}
	count := ""
	if len(r.node.Children) > 0 {
		count = fmt.Sprintf(" (%d)", r.node.Count)
	}
	line := clamp(strings.Repeat("  ", r.depth)+glyph+r.node.Label+count, inner-3)
	if i == t.sel {
		sb.WriteString(styleSel.Render(" ▶ ") + styleSel.Render(line))
	} else {
		sb.WriteString("   " + line)
	}
	sb.WriteString("\n")
}
