package buffer

import "strings"

type coalesceKind uint8

const (
	coalesceNone coalesceKind = iota
	coalesceType
	coalesceBackspace
	coalesceDelete
)

// edit is one replacement: removed and inserted carry exact terminators.
type edit struct {
	at                Pos
	removed, inserted string
}

type state struct {
	cursor, anchor Pos
	hasAnchor      bool
}

// group is one undo step: its edits plus the cursor/selection around them.
type group struct {
	edits         []edit
	before, after state
}

func (b *Buffer) state() state { return state{b.cursor, b.anchor, b.hasAnchor} }

func (b *Buffer) setState(s state) {
	b.cursor, b.anchor, b.hasAnchor = b.clampPos(s.cursor), b.clampPos(s.anchor), s.hasAnchor
}

// beginGroup starts (or nests into) an undo group; every replace until the matching
// endGroup joins it. Starting a group closes the coalescing run.
func (b *Buffer) beginGroup() {
	if b.depth == 0 {
		b.kind = coalesceNone
		b.pending = &group{before: b.state()}
	}
	b.depth++
}

// endGroup closes the group; an outermost group with edits lands on the undo stack
// and clears redo.
func (b *Buffer) endGroup() {
	b.depth--
	if b.depth > 0 {
		return
	}
	g := b.pending
	b.pending = nil
	if len(g.edits) == 0 {
		return
	}
	g.after = b.state()
	b.undo = append(b.undo, g)
	b.redo = nil
	b.last = g
}

// edit runs fn as exactly one undo group.
func (b *Buffer) edit(fn func()) {
	b.beginGroup()
	fn()
	b.endGroup()
}

// extend runs fn as more edits of the last group (coalescing).
func (b *Buffer) extend(fn func()) {
	b.pending = b.last
	b.depth = 1
	fn()
	b.depth = 0
	b.pending = nil
	b.last.after = b.state()
}

func (b *Buffer) canExtend(k coalesceKind) bool {
	return b.kind == k && b.depth == 0 && len(b.undo) > 0 && b.undo[len(b.undo)-1] == b.last
}

// replace is the only mutation primitive: it swaps r for raw (exact terminators),
// records the edit in the open group, and bumps the version. It clamps the cursor
// and anchor but does not otherwise move them; callers set the cursor afterwards.
func (b *Buffer) replace(r Range, raw string) {
	r = b.clampRange(r)
	if r.Start == r.End && raw == "" {
		return
	}
	if b.depth == 0 {
		b.beginGroup()
		defer b.endGroup()
	}
	b.pending.edits = append(b.pending.edits, edit{at: r.Start, removed: b.raw(r), inserted: raw})
	b.splice(r, raw)
	b.version++
}

// Break closes the open coalescing group.
func (b *Buffer) Break() { b.kind = coalesceNone }

// toRaw converts text with "\n", "\r\n" or lone "\r" breaks into raw text using EOL.
func (b *Buffer) toRaw(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			sb.WriteString(b.eol)
		case '\n':
			sb.WriteString(b.eol)
		default:
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

// target is the selection if active, else the empty range at the cursor.
func (b *Buffer) target() Range {
	if r, ok := b.Selection(); ok {
		return r
	}
	return Range{b.cursor, b.cursor}
}

// Type inserts typed text (no line breaks), replacing the selection. Consecutive
// calls, each starting where the last ended, form one undo group.
func (b *Buffer) Type(s string) {
	if s == "" {
		return
	}
	if strings.ContainsAny(s, "\r\n") {
		b.Insert(s)
		return
	}
	if _, sel := b.Selection(); !sel && b.canExtend(coalesceType) && b.next == b.cursor {
		b.extend(func() {
			c := b.cursor
			b.replace(Range{c, c}, s)
			b.cursor = Pos{c.Line, c.Col + len(s)}
			b.hasAnchor = false
		})
	} else {
		b.edit(func() {
			r := b.target()
			b.replace(r, s)
			b.cursor = endPos(r.Start, s)
			b.hasAnchor = false
		})
	}
	b.kind, b.next = coalesceType, b.cursor
}

// Insert inserts s at the cursor (replacing the selection) as one group; line breaks
// in s (LF, CRLF or CR) become EOL().
func (b *Buffer) Insert(s string) {
	b.edit(func() {
		r := b.target()
		raw := b.toRaw(s)
		b.replace(r, raw)
		b.cursor = endPos(r.Start, raw)
		b.hasAnchor = false
	})
}

// Backspace deletes the selection, else one grapheme or the line break before the
// cursor. A run of backspaces is one group.
func (b *Buffer) Backspace() {
	if r, ok := b.Selection(); ok {
		b.DeleteRange(r)
		return
	}
	from := b.Left(b.cursor)
	if from == b.cursor {
		return
	}
	del := func() {
		b.replace(Range{from, b.cursor}, "")
		b.cursor, b.hasAnchor = from, false
	}
	if b.canExtend(coalesceBackspace) && b.next == b.cursor {
		b.extend(del)
	} else {
		b.edit(del)
	}
	b.kind, b.next = coalesceBackspace, b.cursor
}

// Delete is the forward twin of Backspace. A run of deletes is one group.
func (b *Buffer) Delete() {
	if r, ok := b.Selection(); ok {
		b.DeleteRange(r)
		return
	}
	to := b.Right(b.cursor)
	if to == b.cursor {
		return
	}
	del := func() {
		b.replace(Range{b.cursor, to}, "")
		b.hasAnchor = false
	}
	if b.canExtend(coalesceDelete) && b.next == b.cursor {
		b.extend(del)
	} else {
		b.edit(del)
	}
	b.kind, b.next = coalesceDelete, b.cursor
}

// DeleteRange removes r as one group and puts the cursor at r.Start.
func (b *Buffer) DeleteRange(r Range) {
	r = b.clampRange(r)
	if r.Start == r.End {
		return
	}
	b.edit(func() {
		b.replace(r, "")
		b.cursor, b.hasAnchor = r.Start, false
	})
}

// ReplaceRange swaps r for s (breaks as in Insert) as one group, leaving the cursor
// after the inserted text.
func (b *Buffer) ReplaceRange(r Range, s string) {
	r = b.clampRange(r)
	raw := b.toRaw(s)
	if r.Start == r.End && raw == "" {
		return
	}
	b.edit(func() {
		b.replace(r, raw)
		b.cursor, b.hasAnchor = endPos(r.Start, raw), false
	})
}

// ReplaceAll makes the buffer content equal to content as one group, recording only
// the differing middle lines. The cursor and selection are clamped, not moved.
func (b *Buffer) ReplaceAll(content string) {
	if b.EqualString(content) {
		return
	}
	nl, nf := parseRaw(content)
	ol, of := b.lines, b.crlf
	// Terminators are comparable only on non-final lines.
	p := 0
	for p < len(ol) && p < len(nl) && ol[p] == nl[p] && (p == len(ol)-1) == (p == len(nl)-1) &&
		(p == len(ol)-1 || of[p] == nf[p]) {
		p++
	}
	s := 0
	for s < len(ol)-p && s < len(nl)-p {
		oi, ni := len(ol)-1-s, len(nl)-1-s
		if ol[oi] != nl[ni] || of[oi] != nf[ni] {
			break
		}
		s++
	}
	oldEnd, newEnd := len(ol)-s, len(nl)-s
	var r Range
	if s > 0 {
		r = Range{Pos{p, 0}, Pos{oldEnd, 0}}
	} else {
		r = Range{Pos{p, 0}, b.End()}
	}
	var sb strings.Builder
	for i := p; i < newEnd; i++ {
		sb.WriteString(nl[i])
		if i < len(nl)-1 {
			if nf[i] {
				sb.WriteString("\r\n")
			} else {
				sb.WriteByte('\n')
			}
		}
	}
	b.edit(func() { b.replace(r, sb.String()) })
}

// Undo reverts the last group, restoring the cursor and selection from before it.
func (b *Buffer) Undo() bool {
	if b.depth > 0 || len(b.undo) == 0 {
		return false
	}
	g := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	for i := len(g.edits) - 1; i >= 0; i-- {
		e := g.edits[i]
		b.splice(Range{e.at, endPos(e.at, e.inserted)}, e.removed)
	}
	b.setState(g.before)
	b.redo = append(b.redo, g)
	b.afterHistory()
	return true
}

// Redo reapplies the last undone group, restoring the cursor from after it.
func (b *Buffer) Redo() bool {
	if b.depth > 0 || len(b.redo) == 0 {
		return false
	}
	g := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	for _, e := range g.edits {
		b.splice(Range{e.at, endPos(e.at, e.removed)}, e.inserted)
	}
	b.setState(g.after)
	b.undo = append(b.undo, g)
	b.afterHistory()
	return true
}

func (b *Buffer) afterHistory() {
	b.kind = coalesceNone
	b.last = nil
	b.version++
}

// UndoMark closes the typing run and returns the depth of the undo stack, for
// a later Squash.
func (b *Buffer) UndoMark() int {
	b.kind = coalesceNone
	return len(b.undo)
}

// Squash merges every undo step recorded since mark into one, so a command
// made of several edits undoes and redoes in one step. The merged step
// restores the cursor and selection from before its first part and after its
// last. With fewer than two steps since mark, or while a group is open
// (depth > 0), it changes nothing but closing the typing run.
func (b *Buffer) Squash(mark int) {
	b.kind = coalesceNone
	if b.depth > 0 {
		return
	}
	mark = max(0, min(mark, len(b.undo)))
	steps := b.undo[mark:]
	if len(steps) < 2 {
		return
	}
	n := 0
	for _, g := range steps {
		n += len(g.edits)
	}
	merged := &group{
		edits:  make([]edit, 0, n),
		before: steps[0].before,
		after:  steps[len(steps)-1].after,
	}
	for _, g := range steps {
		merged.edits = append(merged.edits, g.edits...)
	}
	b.undo = append(b.undo[:mark], merged)
	b.last = nil
}
