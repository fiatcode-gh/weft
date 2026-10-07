package render

import (
	"io"
	"os"

	"github.com/charmbracelet/colorprofile"
)

// noColor reports whether NO_COLOR asks for colourless output. Any
// non-empty value counts (no-color.org), unlike colorprofile, which needs a
// boolean.
func noColor() bool { return os.Getenv("NO_COLOR") != "" }

// ColorProfile is the colour profile weft's Bubble Tea program renders
// with. NO_COLOR drops colour but keeps text attributes (bold, italic,
// underline, reverse, ...): the Ascii profile, which Bubble Tea's renderer
// applies to every cell. Any terminal with colour gets TrueColor: weft's own colours are ANSI 0-15,
// which every colour profile passes through unchanged, and Glamour's named
// WEFT_STYLE themes rendered in TrueColor under v1. A terminal without
// colour (TERM=dumb or unset, or not a TTY) keeps what Detect found.
func ColorProfile(out io.Writer) colorprofile.Profile {
	if noColor() {
		return colorprofile.Ascii
	}
	p := colorprofile.Detect(out, os.Environ())
	if p >= colorprofile.ANSI {
		return colorprofile.TrueColor
	}
	return p
}
