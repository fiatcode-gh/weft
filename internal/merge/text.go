package merge

import "strings"

// Text is Lines with the final newline merged as a separate change. Each non-empty input that
// lacks a final "\n" gets one before Lines runs; the result keeps its final "\n" exactly when
// the merged final-newline state says so: mine's state if mine changed it from base, else
// theirs'. An empty input stays empty (zero lines). MineLine is Lines' mapping (adding a
// terminator never changes a line count).
func Text(base, mine, theirs string) Result {
	baseNL, mineNL, theirsNL := hasFinalNewline(base), hasFinalNewline(mine), hasFinalNewline(theirs)
	keep, decider := theirsNL, theirs
	if mineNL != baseNL {
		keep, decider = mineNL, mine
	}

	res := Lines(base+terminatorIfMissing(base), mine+terminatorIfMissing(mine), theirs+terminatorIfMissing(theirs))
	if res.Conflict || keep || res.Text == "" {
		return res
	}
	// Remove the terminator Text added to the final line: its "\n" and any "\r" before it,
	// except a "\r" that is the deciding input's own last byte (content, not terminator).
	res.Text = strings.TrimSuffix(res.Text, "\n")
	if terminator(decider) == "\r\n" || !strings.HasSuffix(decider, "\r") {
		res.Text = strings.TrimSuffix(res.Text, "\r")
	}
	return res
}

func hasFinalNewline(s string) bool { return strings.HasSuffix(s, "\n") }

// terminatorIfMissing is the line terminator Text appends to s: none for an
// empty s or one already ending in "\n", else s's last line break ("\r\n" or "\n").
func terminatorIfMissing(s string) string {
	if s == "" || hasFinalNewline(s) {
		return ""
	}
	return terminator(s)
}

// terminator is the line break s's last "\n" uses: "\r\n" when that "\n" follows
// a "\r", else "\n".
func terminator(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i > 0 && s[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}
