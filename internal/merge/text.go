package merge

import "strings"

// Text is Lines with the final newline merged as a separate change. Each non-empty input that
// lacks a final "\n" gets one before Lines runs; the result keeps its final "\n" exactly when
// the merged final-newline state says so: mine's state if mine changed it from base, else
// theirs'. An empty input stays empty (zero lines). MineLine is Lines' mapping (adding a
// terminator never changes a line count).
func Text(base, mine, theirs string) Result {
	baseNL, mineNL, theirsNL := hasFinalNewline(base), hasFinalNewline(mine), hasFinalNewline(theirs)
	keep := theirsNL
	if mineNL != baseNL {
		keep = mineNL
	}

	res := Lines(terminated(base), terminated(mine), terminated(theirs))
	if res.Conflict || keep || res.Text == "" {
		return res
	}
	res.Text = strings.TrimSuffix(strings.TrimSuffix(res.Text, "\n"), "\r")
	return res
}

func hasFinalNewline(s string) bool { return strings.HasSuffix(s, "\n") }

// terminated appends a line terminator to a non-empty s lacking one, matching s's last line break.
func terminated(s string) string {
	if s == "" || hasFinalNewline(s) {
		return s
	}
	if i := strings.LastIndexByte(s, '\n'); i > 0 && s[i-1] == '\r' {
		return s + "\r\n"
	}
	return s + "\n"
}
