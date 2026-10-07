package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

const goldenCorpus = "# Heading one\n\n## Heading two\n\n### Heading three\n\n" +
	"A paragraph with **bold**, *italic*, ~~struck~~, `code span`, a [markdown link](https://example.com), " +
	"a [[Wiki Target]] and an [[Wiki Target|aliased link]] in it.\n\n" +
	"- TODO top level task\n" +
	"  - DONE nested task\n" +
	"    - DOING third level with a long wrapping line that keeps going well past the sixty column width so it must wrap\n" +
	"    - plain third level bullet\n\n" +
	"> a block quote\n> over two lines\n\n" +
	"| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
	"---\n\n" +
	"```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\n\n" +
	"- LATER with logbook\n  :LOGBOOK:\n  CLOCK: [2026-01-01 Thu 10:00]\n  :END:\n"

func renderStyledGolden(t *testing.T, env map[string]string) {
	t.Helper()
	if !inFreshProcess(t, env) {
		return
	}
	res, err := Render(strings.TrimSpace(goldenCorpus)+"\n", 60)
	if err != nil {
		t.Fatal(err)
	}
	golden.RequireEqual(t, []byte(res.Styled))
}

func TestRenderStyledGoldenTerminal(t *testing.T) {
	renderStyledGolden(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": ""})
}

func TestRenderStyledGoldenDark(t *testing.T) {
	renderStyledGolden(t, map[string]string{"NO_COLOR": "", "WEFT_STYLE": "dark"})
}

func TestRenderStyledGoldenNoColor(t *testing.T) {
	renderStyledGolden(t, map[string]string{"NO_COLOR": "1", "WEFT_STYLE": ""})
}
