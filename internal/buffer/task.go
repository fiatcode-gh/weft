package buffer

import (
	"strings"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// taskBlock is the task block (graph.TaskBlockAt) owning the cursor line.
func (b *Buffer) taskBlock() (graph.TaskBlock, bool) {
	return graph.TaskBlockAt(b.Len(), b.Line, b.cursor.Line)
}

// stampIndex is the line of blk's stamp of kind, -1 when it has none.
func stampIndex(blk graph.TaskBlock, kind graph.StampKind) int {
	if kind == graph.StampDeadline {
		return blk.Deadline
	}
	return blk.Scheduled
}

// TaskStamp is the date of the cursor task's stamp of kind ("" when it has
// none). ok is false when the cursor is not on a task or one of its own lines.
func (b *Buffer) TaskStamp(kind graph.StampKind) (date string, ok bool) {
	blk, ok := b.taskBlock()
	if !ok {
		return "", false
	}
	if idx := stampIndex(blk, kind); idx >= 0 {
		if s, isStamp := graph.ParseStampLine(b.lines[idx]); isStamp {
			date = s.Date
		}
	}
	return date, true
}

// SetTaskStamp sets the cursor task's SCHEDULED/DEADLINE stamp to date
// (YYYY-MM-DD), or removes its line when date is "". An existing stamp keeps
// its time and repeater; a new one goes right under the task's first line
// (a DEADLINE under an existing SCHEDULED), indented to the task text. The
// change is one undo step and clears the selection. ok is false, opening no
// group, when the cursor is not on a task; changed is false when nothing had
// to change.
func (b *Buffer) SetTaskStamp(kind graph.StampKind, date string) (changed, ok bool) {
	blk, ok := b.taskBlock()
	if !ok {
		return false, false
	}
	idx := stampIndex(blk, kind)
	cur := b.cursor
	switch {
	case date == "" && idx < 0:
		return false, true
	case date == "":
		b.edit(func() {
			b.replace(Range{Pos{idx - 1, len(b.lines[idx-1])}, Pos{idx, len(b.lines[idx])}}, "")
			switch {
			case cur.Line > idx:
				cur.Line--
			case cur.Line == idx:
				cur = Pos{blk.First, min(cur.Col, len(b.lines[blk.First]))}
			}
			b.cursor, b.hasAnchor = cur, false
		})
		return true, true
	case idx >= 0:
		old := b.lines[idx]
		s, _ := graph.ParseStampLine(old)
		text := old[:s.Start] + graph.StampText(kind, date, s.Time, s.Repeater)
		if text == old {
			return false, true
		}
		b.edit(func() {
			b.replace(Range{Pos{idx, 0}, Pos{idx, len(old)}}, text)
			cur.Col = min(cur.Col, len(text))
			b.cursor, b.hasAnchor = cur, false
		})
		return true, true
	}
	k := blk.First + 1
	if kind == graph.StampDeadline && blk.Scheduled >= 0 {
		k = blk.Scheduled + 1
	}
	first := b.lines[blk.First]
	p, _ := graph.ParseTaskPrefix(first)
	lead := first[:len(first)-len(strings.TrimLeft(first, " \t"))]
	indent := lead + strings.Repeat(" ", max(0, p.MarkerStart-len(lead)))
	b.edit(func() {
		end := len(b.lines[k-1])
		b.replace(Range{Pos{k - 1, end}, Pos{k - 1, end}}, b.eol+indent+graph.StampText(kind, date, "", ""))
		if cur.Line >= k {
			cur.Line++
		}
		b.cursor, b.hasAnchor = cur, false
	})
	return true, true
}

// CyclePriority advances the cursor task's priority (graph.NextPriority:
// none → A → B → C → none) as one undo step. It reports false, opening no
// group, when the cursor is not on a task or one of its own lines.
func (b *Buffer) CyclePriority() bool {
	blk, ok := b.taskBlock()
	if !ok {
		return false
	}
	line := b.lines[blk.First]
	p, ok := graph.ParseTaskPrefix(line)
	if !ok {
		return false
	}
	out, at, delta, ok := graph.SetTaskPriority(line, graph.NextPriority(p.Priority))
	if !ok {
		return false
	}
	cur := b.cursor
	if cur.Line == blk.First && cur.Col > at {
		cur.Col = max(cur.Col+delta, at)
	}
	b.edit(func() {
		b.replace(Range{Pos{blk.First, 0}, Pos{blk.First, len(line)}}, out)
		cur.Col = min(cur.Col, len(b.lines[cur.Line]))
		b.cursor, b.hasAnchor = cur, false
	})
	return true
}
