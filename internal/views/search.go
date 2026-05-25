package views

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

// SearchHit is one rg result row.
type SearchHit struct {
	FilePath string
	Line     int
	Context  string
}

type SearchView struct {
	idx     *graph.Index
	query   string
	hits    []SearchHit
	sel     int
	running bool
	err     error
}

func NewSearchView(idx *graph.Index) *SearchView {
	return &SearchView{idx: idx}
}

func (s *SearchView) Query() string { return s.query }

func (s *SearchView) SetQuery(q string) { s.query = q }

// SearchCmd returns a tea.Cmd that runs rg and returns a searchDoneMsg.
func (s *SearchView) SearchCmd(graphPath string) tea.Cmd {
	q := s.query
	return func() tea.Msg {
		out, err := runRipgrep(graphPath, q)
		if err != nil {
			return searchDoneMsg{err: err}
		}
		return searchDoneMsg{hits: parseRipgrepJSON(out)}
	}
}

type searchDoneMsg struct {
	hits []SearchHit
	err  error
}

func (s *SearchView) Apply(msg searchDoneMsg) {
	s.running = false
	s.err = msg.err
	s.hits = msg.hits
	if s.sel >= len(s.hits) {
		s.sel = 0
	}
}

// Update handles a key. Returns (selected hit, accept, cancel, cmd to run).
func (s *SearchView) Update(key string, graphPath string) (hit *SearchHit, accept, cancel bool, cmd tea.Cmd) {
	switch key {
	case "esc":
		return nil, false, true, nil
	case "enter":
		if s.running {
			return nil, false, false, nil
		}
		if s.query == "" {
			return nil, false, false, nil
		}
		if len(s.hits) == 0 {
			s.running = true
			return nil, false, false, s.SearchCmd(graphPath)
		}
		if s.sel >= 0 && s.sel < len(s.hits) {
			h := s.hits[s.sel]
			return &h, true, false, nil
		}
	case "up", "ctrl+k":
		if s.sel > 0 {
			s.sel--
		}
	case "down", "ctrl+j":
		if s.sel < len(s.hits)-1 {
			s.sel++
		}
	case "backspace":
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.hits = nil
		}
	default:
		if len(key) == 1 {
			s.query += key
			s.hits = nil
		}
	}
	return nil, false, false, nil
}

var (
	searchBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	searchTitle  = lipgloss.NewStyle().Bold(true)
	searchPrompt = lipgloss.NewStyle().Faint(true)
	searchSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Bold(true)
	searchFaint  = lipgloss.NewStyle().Faint(true)
	searchHitPos = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

const searchInnerWidth = 72

func (s *SearchView) View() string {
	var b strings.Builder
	b.WriteString(searchTitle.Render("Search the graph"))
	b.WriteString("\n\n")
	b.WriteString(searchPrompt.Render("/ "))
	b.WriteString(s.query)
	switch {
	case s.err != nil:
		b.WriteString(searchFaint.Render(fmt.Sprintf("   error: %v", s.err)))
	case s.running:
		b.WriteString(searchFaint.Render("   searching…"))
	case len(s.hits) == 0 && s.query != "":
		b.WriteString(searchFaint.Render("   press enter to search"))
	}
	b.WriteString("\n")
	b.WriteString(searchFaint.Render(strings.Repeat("─", searchInnerWidth)))
	b.WriteString("\n")
	if len(s.hits) == 0 && s.query == "" {
		b.WriteString(searchFaint.Render("  type a query and press enter"))
		b.WriteString("\n")
	}
	for i, h := range s.hits {
		marker := "  "
		pos := searchHitPos.Render(fmt.Sprintf("%s:%d", shortPath(h.FilePath), h.Line))
		line := pos + searchFaint.Render("  · ") + h.Context
		if i == s.sel {
			marker = searchSel.Render(" ▶ ")
			line = searchSel.Render(fmt.Sprintf("%s:%d  · %s", shortPath(h.FilePath), h.Line, h.Context))
		}
		b.WriteString(marker)
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(searchFaint.Render("↑/↓ select · enter search/open · esc cancel"))
	return searchBorder.Render(b.String())
}

func shortPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

func runRipgrep(graphPath, query string) ([]byte, error) {
	args := []string{"--json", "--", query}
	for _, sub := range []string{"pages", "journals"} {
		p := filepath.Join(graphPath, sub)
		if _, err := os.Stat(p); err == nil {
			args = append(args, p)
		}
	}
	if len(args) == 3 {
		// No pages/ or journals/ dir — nothing to search.
		return nil, nil
	}
	cmd := exec.Command("rg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return stdout.Bytes(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("rg failed: %w (%s)", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func parseRipgrepJSON(b []byte) []SearchHit {
	var out []SearchHit
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var env struct {
			Type string `json:"type"`
			Data struct {
				Path       struct{ Text string } `json:"path"`
				Lines      struct{ Text string } `json:"lines"`
				LineNumber int                   `json:"line_number"`
			} `json:"data"`
		}
		if err := json.Unmarshal(sc.Bytes(), &env); err != nil {
			continue
		}
		if env.Type != "match" {
			continue
		}
		out = append(out, SearchHit{
			FilePath: env.Data.Path.Text,
			Line:     env.Data.LineNumber,
			Context:  strings.TrimRight(env.Data.Lines.Text, "\n"),
		})
	}
	return out
}
