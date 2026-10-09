package graph

import "regexp"

// LogbookStartRe and LogbookEndRe delimit a Logseq :LOGBOOK: drawer. They are
// the one definition shared by the index, the renderer and the painter.
var (
	LogbookStartRe = regexp.MustCompile(`(?i)^\s*:LOGBOOK:\s*$`)
	LogbookEndRe   = regexp.MustCompile(`(?i)^\s*:END:\s*$`)
)

// QueryOrEmbedRe matches the opener of a {{query ...}} or {{embed ...}} block.
// It is the one definition shared by the index, the renderer and the painter.
var QueryOrEmbedRe = regexp.MustCompile(`(?i)^\s*\{\{(query|embed)\b`)
