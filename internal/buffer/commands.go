package buffer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// DeleteWordBackward deletes the selection, else back to WordLeft (at column 0 that
// is the line break before the cursor). One undo group.
func (b *Buffer) DeleteWordBackward() {
	if r, ok := b.Selection(); ok {
		b.DeleteRange(r)
		return
	}
	b.DeleteRange(Range{b.WordLeft(b.cursor), b.cursor})
}

// DeleteWordForward deletes the selection, else forward to WordRight (at line end
// that is the line break). One undo group.
func (b *Buffer) DeleteWordForward() {
	if r, ok := b.Selection(); ok {
		b.DeleteRange(r)
		return
	}
	b.DeleteRange(Range{b.cursor, b.WordRight(b.cursor)})
}

// DeleteToLineEnd deletes from the cursor to the end of its line; at line end it
// deletes the line break instead.
func (b *Buffer) DeleteToLineEnd() {
	end := b.LineEnd(b.cursor)
	if end == b.cursor {
		end = b.Right(b.cursor)
	}
	b.DeleteRange(Range{b.cursor, end})
}

// DeleteToLineStart deletes from the start of the cursor's line to the cursor.
func (b *Buffer) DeleteToLineStart() {
	b.DeleteRange(Range{b.LineStart(b.cursor), b.cursor})
}

// CaseMode selects how CaseWord rewrites a word.
type CaseMode int

const (
	CaseCapitalize CaseMode = iota
	CaseLower
	CaseUpper
)

// CaseWord rewrites the word after the cursor (spaces skipped) and leaves the cursor
// at its end. It never crosses a line break and leaves invalid UTF-8 words alone.
func (b *Buffer) CaseWord(m CaseMode) {
	line := b.lines[b.cursor.Line]
	end := b.WordRight(b.cursor)
	if end.Line != b.cursor.Line {
		return
	}
	start := b.cursor.Col
	for start < end.Col {
		r, n := utf8.DecodeRuneInString(line[start:])
		if !unicode.IsSpace(r) {
			break
		}
		start += n
	}
	word := line[start:end.Col]
	out := word
	if utf8.ValidString(word) {
		switch m {
		case CaseUpper:
			out = strings.ToUpper(word)
		case CaseLower:
			out = strings.ToLower(word)
		default:
			r, n := utf8.DecodeRuneInString(word)
			out = string(unicode.ToUpper(r)) + strings.ToLower(word[n:])
		}
	}
	if out == word {
		b.MoveTo(end, false)
		return
	}
	b.ReplaceRange(Range{Pos{end.Line, start}, end}, out)
}

// Copy returns the selection's text; without a selection, the cursor line plus "\n"
// and linewise = true. Line breaks are "\n".
func (b *Buffer) Copy() (text string, linewise bool) {
	if r, ok := b.Selection(); ok {
		return b.Text(r), false
	}
	return b.lines[b.cursor.Line] + "\n", true
}

// Cut is Copy followed by removing what it returned, as one group.
func (b *Buffer) Cut() (text string, linewise bool) {
	text, linewise = b.Copy()
	if !linewise {
		r, _ := b.Selection()
		b.DeleteRange(r)
		return text, false
	}
	l, col := b.cursor.Line, b.cursor.Col
	var r Range
	switch {
	case l < len(b.lines)-1:
		r = Range{Pos{l, 0}, Pos{l + 1, 0}}
	case l > 0:
		r = Range{Pos{l - 1, len(b.lines[l-1])}, Pos{l, len(b.lines[l])}}
	default:
		r = Range{Pos{0, 0}, Pos{0, len(b.lines[0])}}
	}
	b.edit(func() {
		b.replace(r, "")
		nl := min(l, len(b.lines)-1)
		b.cursor, b.hasAnchor = Pos{nl, min(col, len(b.lines[nl]))}, false
	})
	return text, true
}

// Paste inserts text: over a selection it replaces it; else linewise text goes before
// the cursor line (the cursor keeps its column on its moved line); else it is inserted
// at the cursor. One undo group.
func (b *Buffer) Paste(text string, linewise bool) {
	if text == "" {
		return
	}
	if _, ok := b.Selection(); ok || !linewise {
		b.Insert(text)
		return
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	raw := b.toRaw(text)
	l, col := b.cursor.Line, b.cursor.Col
	b.edit(func() {
		b.replace(Range{Pos{l, 0}, Pos{l, 0}}, raw)
		b.cursor, b.hasAnchor = Pos{l + strings.Count(raw, "\n"), col}, false
	})
}
