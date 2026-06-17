package views

import (
	"regexp"
	"strings"
)

// Markdown-editing patterns for the in-app editor. Mirror the bullet/marker
// shapes used by edit_tint.go rather than importing internal/render's
// unexported symbols (the documented views/render mirror convention).
var (
	editBulletRe    = regexp.MustCompile(`^(\s*)- `)
	editEmptyBullet = regexp.MustCompile(`^\s*-\s*$`)
	editBulletBody  = regexp.MustCompile(`^(\s*)- (.*)$`)
)

// bulletPrefix reports whether line is a "- " bullet and, if so, returns the
// prefix (indent + "- ") to start a continuation line at the same indent.
func bulletPrefix(line string) (string, bool) {
	m := editBulletRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1] + "- ", true
}

// isEmptyBullet reports whether line is a bullet marker with no content
// (just "-" / "- " at some indent), i.e. an Enter here should end the list.
func isEmptyBullet(line string) bool {
	return editEmptyBullet.MatchString(line)
}

// cycleMarkerLine advances a bullet's workflow marker one step in the cycle
// plain -> TODO -> DONE -> plain, returning the rewritten line and the cursor
// column shifted by the marker-length change. ok is false when line is not a
// "- " bullet. oldCol is the caller's current cursor column on the line.
func cycleMarkerLine(line string, oldCol int) (string, int, bool) {
	m := editBulletBody.FindStringSubmatch(line)
	if m == nil {
		return "", 0, false
	}
	indent, content := m[1], m[2]
	markerCol := len([]rune(indent)) + 2 // column just after "- "

	var newContent string
	var delta int
	switch {
	case content == "TODO" || strings.HasPrefix(content, "TODO "):
		// TODO -> DONE: replace the leading "TODO" with "DONE" (length unchanged).
		newContent = "DONE" + content[len("TODO"):]
		delta = 0
	case content == "DONE" || strings.HasPrefix(content, "DONE "):
		// DONE -> plain: drop the leading "DONE " (or bare "DONE").
		if strings.HasPrefix(content, "DONE ") {
			newContent = content[len("DONE "):]
			delta = -len("DONE ")
		} else {
			newContent = ""
			delta = -len("DONE")
		}
	default:
		// plain -> TODO: prepend "TODO ".
		newContent = "TODO " + content
		delta = len("TODO ")
	}

	newLine := indent + "- " + newContent
	newCol := oldCol
	if oldCol >= markerCol {
		newCol = oldCol + delta
		if newCol < markerCol {
			newCol = markerCol
		}
	}
	if max := len([]rune(newLine)); newCol > max {
		newCol = max
	}
	return newLine, newCol, true
}

// indentLine adds one 2-space indent level to the front of line.
func indentLine(line string) string { return "  " + line }

// dedentLine removes up to one 2-space indent level from the front of line,
// returning the new line and the number of leading spaces actually removed (0-2).
func dedentLine(line string) (string, int) {
	n := 0
	for n < 2 && n < len(line) && line[n] == ' ' {
		n++
	}
	return line[n:], n
}
