package render

import (
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2/quick"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// fenceKey identifies a highlighted fence: where it opens and what it holds.
type fenceKey struct {
	opener     int
	code, lang string
}

// fenceEntry is a fence's code with the style of every byte, and where each
// content line starts in it.
type fenceEntry struct {
	styles []uv.Style
	starts []int
}

// fenceCacheLimit is how many fences one generation holds.
const fenceCacheLimit = 16

// fenceCache keeps two generations of highlighted fences: a lookup promotes a
// hit to the current one, and a full current generation replaces the previous
// one. Fences the last frames did not touch fall out; highlighting never reruns
// for a fence that is still on screen.
type fenceCache struct{ cur, prev map[fenceKey]*fenceEntry }

func (c *fenceCache) get(k fenceKey) *fenceEntry {
	if e, ok := c.cur[k]; ok {
		return e
	}
	if e, ok := c.prev[k]; ok {
		c.put(k, e)
		return e
	}
	return nil
}

func (c *fenceCache) put(k fenceKey, e *fenceEntry) {
	if c.cur == nil || len(c.cur) >= fenceCacheLimit {
		c.prev, c.cur = c.cur, make(map[fenceKey]*fenceEntry, fenceCacheLimit)
	}
	c.cur[k] = e
}

// chromaTheme is the chroma style Glamour highlights fences with: its
// registered "charm" entry when the style carries a Chroma block, else the
// style's named theme. Empty means Glamour draws fences in one style.
func (p *Painter) chromaTheme() string {
	if p.theme.Config.CodeBlock.Chroma != nil {
		return "charm"
	}
	return p.theme.Config.CodeBlock.Theme
}

func (p *Painter) quickHighlight(code, lang string) (string, error) {
	var out strings.Builder
	err := quick.Highlight(&out, code, lang, p.theme.Formatter, p.chromaTheme())
	return out.String(), err
}

// paintCode copies the style of a code line out of its fence's highlight.
func (p *Painter) paintCode(b *lineBuf, src SourceLines, sc *Scanner, i int, info LineInfo) {
	var code strings.Builder
	var starts []int
	for j := info.Fence + 1; j < src.Len(); j++ {
		if ji := sc.Info(src, j); ji.Kind != KindCode || ji.Fence != info.Fence {
			break
		}
		starts = append(starts, code.Len())
		code.WriteString(stripFence(src.Line(j), info.Strip))
		code.WriteByte('\n')
	}
	key := fenceKey{info.Fence, code.String(), info.Lang}
	e := p.fences.get(key)
	if e == nil {
		e = &fenceEntry{styles: p.highlightStyles(key.code, key.lang), starts: starts}
		p.fences.put(key, e)
	}
	j := i - info.Fence - 1
	line := src.Line(i)
	strip := len(line) - len(stripFence(line, info.Strip))
	if j < 0 || j >= len(e.starts) {
		return
	}
	for k := 0; strip+k < len(line) && e.starts[j]+k < len(e.styles); k++ {
		b.st[strip+k], b.cov[strip+k] = e.styles[e.starts[j]+k], true
	}
}

// stripFence removes up to n leading spaces: the indentation of the fence's
// container, which Glamour's code text does not have.
func stripFence(line string, n int) string {
	k := 0
	for k < n && k < len(line) && line[k] == ' ' {
		k++
	}
	return line[k:]
}

// highlightStyles styles every byte of code as Glamour's CodeBlockElement
// draws it: chroma's output when it is the code plus SGR, else the one style
// of Glamour's fallback rendering.
func (p *Painter) highlightStyles(code, lang string) []uv.Style {
	styles := make([]uv.Style, len(code))
	if p.chromaTheme() != "" {
		if out, err := p.highlight(code, lang); err == nil && sgrStyles(out, code, styles) {
			return styles
		}
	}
	fallback := toUV(cascade(p.doc, p.theme.Config.CodeBlock.StylePrimitive))
	for i := range styles {
		styles[i] = fallback
	}
	return styles
}

// sgrStyles lays the SGR styles of out, highlighted code, onto the bytes of
// code. It reports false when out's text is not code.
func sgrStyles(out, code string, styles []uv.Style) bool {
	var pen uv.Style
	cur := 0
	ok := true
	p := ansi.NewParser()
	p.SetHandler(ansi.Handler{
		Print: func(r rune) {
			n := utf8.RuneLen(r)
			if !ok || n < 0 || !strings.HasPrefix(code[cur:], string(r)) {
				ok = false
				return
			}
			for k := range n {
				styles[cur+k] = pen
			}
			cur += n
		},
		Execute: func(c byte) {
			if !ok || cur >= len(code) || code[cur] != c {
				ok = false
				return
			}
			styles[cur] = pen
			cur++
		},
		HandleCsi: func(cmd ansi.Cmd, params ansi.Params) {
			if cmd.Final() == 'm' && cmd.Prefix() == 0 && cmd.Intermediate() == 0 {
				uv.ReadStyle(params, &pen)
			}
		},
	})
	p.Parse([]byte(out))
	return ok && cur == len(code)
}
