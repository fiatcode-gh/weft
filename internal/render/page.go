package render

import (
	"regexp"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// Link is one wiki-link position inside the styled output.
type Link struct {
	Target string
	Start  int // byte offset in Styled
	End    int // byte offset in Styled (exclusive)
}

// Result is the rendered page.
type Result struct {
	Styled string
	Links  []Link
}

var wikiLinkRe = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)

var linkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)

// Render returns Glamour-rendered markdown with wiki-link positions annotated.
// width is the target terminal column count.
func Render(body string, width int) (Result, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return Result{}, err
	}
	styled, err := r.Render(body)
	if err != nil {
		// Fallback: plain text if Glamour chokes
		styled = body
	}

	var links []Link
	out := wikiLinkRe.ReplaceAllStringFunc(styled, func(match string) string {
		m := wikiLinkRe.FindStringSubmatch(match)
		display := m[1]
		if m[2] != "" {
			display = m[2]
		}
		return linkStyle.Render(display)
	})

	// Re-scan the original styled string to locate each link in the final output.
	// Because ReplaceAllStringFunc rewrites the buffer, we compute positions on `out`.
	idx := 0
	for _, m := range wikiLinkRe.FindAllStringSubmatchIndex(styled, -1) {
		target := styled[m[2]:m[3]]
		display := target
		if m[4] != -1 {
			display = styled[m[4]:m[5]]
		}
		rendered := linkStyle.Render(display)
		pos := indexFrom(out, rendered, idx)
		if pos < 0 {
			continue
		}
		links = append(links, Link{
			Target: target,
			Start:  pos,
			End:    pos + len(rendered),
		})
		idx = pos + len(rendered)
	}

	return Result{Styled: out, Links: links}, nil
}

func indexFrom(s, sub string, from int) int {
	if from < 0 || from >= len(s) {
		return -1
	}
	rel := -1
	if i := indexOf(s[from:], sub); i >= 0 {
		rel = from + i
	}
	return rel
}

func indexOf(s, sub string) int {
	// thin wrapper to keep the call site readable and avoid strings import name clash
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
