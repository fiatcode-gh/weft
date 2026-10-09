package graph

import (
	"errors"
	"regexp"
	"strings"
)

// taskPrefixRe is the one grammar for a task line's marker and optional
// priority. Go's RE2 prefers the backtracking-first submatch, so "- TODO [#A]x"
// has no priority (the priority group would need whitespace or the line end
// after it) while "- TODO [#A]" has priority A.
var taskPrefixRe = regexp.MustCompile(`^(\s*-\s+)(TODO|DOING|LATER|WAITING|NOW|DONE|CANCELED|CANCELLED)(\s+\[#([ABC])\])?(?:\s|$)`)

// TaskPrefix locates the marker and priority of a task line. Priority spans are
// valid only when Priority != "".
type TaskPrefix struct {
	Marker                     string
	MarkerStart, MarkerEnd     int
	Priority                   string // "", "A", "B", "C"
	PriorityStart, PriorityEnd int    // span of "[#X]" itself, not the gap before it
}

// ParseTaskPrefix reports whether line is a "- MARKER" bullet with any of the
// eight workflow markers, and where its parts are.
func ParseTaskPrefix(line string) (TaskPrefix, bool) {
	m := taskPrefixRe.FindStringSubmatchIndex(line)
	if m == nil {
		return TaskPrefix{}, false
	}
	p := TaskPrefix{
		Marker:      line[m[4]:m[5]],
		MarkerStart: m[4],
		MarkerEnd:   m[5],
	}
	if m[6] >= 0 {
		p.Priority = line[m[8]:m[9]]
		p.PriorityEnd = m[7]
		p.PriorityStart = m[7] - len("[#X]")
	}
	return p, true
}

// NextMarker is the Ctrl+T cycle: open markers become DONE, closed ones
// (DONE, CANCELED, CANCELLED) become plain, plain becomes TODO.
func NextMarker(marker string) string {
	switch marker {
	case "TODO", "LATER", "DOING", "WAITING", "NOW":
		return "DONE"
	case "DONE", "CANCELED", "CANCELLED":
		return ""
	}
	return "TODO"
}

var plainBulletRe = regexp.MustCompile(`^(\s*)- `)

// SetTaskMarker rewrites line's marker. A "" marker removes it (with one
// following space or tab); a marker on a plain "- " bullet is inserted after the
// bullet. at is the marker column and delta the byte-length change. ok is false
// when line is not a "- " bullet at all. Every other byte is preserved.
func SetTaskMarker(line, marker string) (out string, at, delta int, ok bool) {
	if p, isTask := ParseTaskPrefix(line); isTask {
		end := p.MarkerEnd
		if marker == "" && end < len(line) && (line[end] == ' ' || line[end] == '\t') {
			end++
		}
		out = line[:p.MarkerStart] + marker + line[end:]
		return out, p.MarkerStart, len(out) - len(line), true
	}
	m := plainBulletRe.FindStringSubmatch(line)
	if m == nil {
		return line, 0, 0, false
	}
	at = len(m[1]) + 2
	if marker == "" {
		return line, at, 0, true
	}
	out = line[:at] + marker + " " + line[at:]
	return out, at, len(out) - len(line), true
}

// NextPriority cycles "" → A → B → C → "".
func NextPriority(p string) string {
	switch p {
	case "":
		return "A"
	case "A":
		return "B"
	case "B":
		return "C"
	}
	return ""
}

// SetTaskPriority adds, changes or (p == "") removes a task line's [#X]
// priority. at is the column just after the marker. ok is false for a line
// without a task prefix.
func SetTaskPriority(line, p string) (out string, at, delta int, ok bool) {
	t, isTask := ParseTaskPrefix(line)
	if !isTask {
		return line, 0, 0, false
	}
	switch {
	case t.Priority == "" && p == "":
		out = line
	case t.Priority == "":
		out = line[:t.MarkerEnd] + " [#" + p + "]" + line[t.MarkerEnd:]
	case p == "":
		out = line[:t.MarkerEnd] + line[t.PriorityEnd:]
	default:
		out = line[:t.PriorityEnd-2] + p + line[t.PriorityEnd-1:]
	}
	return out, t.MarkerEnd, len(out) - len(line), true
}

// TaskTail is what follows a task marker on its line: "[#P] text" with a
// priority, else text. Relocation compares it whitespace-normalised.
func TaskTail(priority, text string) string {
	if priority == "" {
		return text
	}
	return "[#" + priority + "] " + text
}

// ErrTaskNotFound means MarkTask found no line matching the expected task.
var ErrTaskNotFound = errors.New("task not found")

func normSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// MarkTask rewrites the marker of the task whose marker is from and whose tail
// matches tail, preferring the 1-based hint line, else the nearest candidate
// (earlier on a tie), skipping fenced lines. It returns the new body and the
// 1-based line it changed. Only "\n" splits lines, so "\r" stays with its line.
func MarkTask(body string, hint int, from, to, tail string) (out string, line int, err error) {
	lines := strings.Split(body, "\n")
	want := normSpace(tail)
	best, bestDist := -1, 0
	var fence FenceState
	for i, l := range lines {
		if fence.Step(l) {
			continue
		}
		p, ok := ParseTaskPrefix(l)
		if !ok || p.Marker != from || normSpace(l[p.MarkerEnd:]) != want {
			continue
		}
		d := i + 1 - hint
		if d < 0 {
			d = -d
		}
		if best < 0 || d < bestDist {
			best, bestDist = i, d
		}
	}
	if best < 0 {
		return body, 0, ErrTaskNotFound
	}
	newLine, _, _, ok := SetTaskMarker(lines[best], to)
	if !ok {
		return body, 0, ErrTaskNotFound
	}
	lines[best] = newLine
	return strings.Join(lines, "\n"), best + 1, nil
}
