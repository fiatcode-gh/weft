// Package search runs ripgrep against a Logseq graph and parses its JSON
// output into structured hits. It performs process I/O and decoding so the UI
// layer can consume plain []Hit values.
package search

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Hit is one ripgrep result row.
type Hit struct {
	FilePath string
	Line     int
	Context  string
	// Matches are byte offsets within Context that the query matched, so the
	// view can emphasise them in the rendered list.
	Matches []Span
}

// Span is a [Start, End) byte range inside Hit.Context.
type Span struct {
	Start, End int
}

// Run executes ripgrep for query against <graphPath>/pages and
// <graphPath>/journals and returns the parsed hits. It returns (nil, nil) when
// neither subdirectory exists. A ripgrep exit code of 1 (no matches) is
// treated as success.
func Run(graphPath, query string) ([]Hit, error) {
	out, err := runRipgrep(graphPath, []string{"--smart-case"}, query)
	if err != nil {
		return nil, err
	}
	return parseJSON(out), nil
}

// Mentions finds whole-word, case-insensitive, literal occurrences of name
// across pages/ and journals/. Used to detect unlinked references to a page;
// word boundaries keep "Go" from matching "Google", and -F treats names with
// regex metacharacters literally.
func Mentions(graphPath, name string) ([]Hit, error) {
	out, err := runRipgrep(graphPath, []string{"-w", "-F", "--ignore-case"}, name)
	if err != nil {
		return nil, err
	}
	return parseJSON(out), nil
}

// runRipgrep runs `rg --json <flags...> -- <query> <pages> <journals>` and
// returns raw stdout. A ripgrep exit code of 1 (no matches) is treated as
// success. Returns (nil, nil) when neither subdirectory exists.
func runRipgrep(graphPath string, flags []string, query string) ([]byte, error) {
	args := append([]string{"--json"}, flags...)
	args = append(args, "--", query)
	baseArgs := len(args)
	for _, sub := range []string{"pages", "journals"} {
		p := filepath.Join(graphPath, sub)
		if _, err := os.Stat(p); err == nil {
			args = append(args, p)
		}
	}
	if len(args) == baseArgs {
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

func parseJSON(b []byte) []Hit {
	var out []Hit
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var env struct {
			Type string `json:"type"`
			Data struct {
				Path       struct{ Text string } `json:"path"`
				Lines      struct{ Text string } `json:"lines"`
				LineNumber int                   `json:"line_number"`
				Submatches []struct {
					Start int `json:"start"`
					End   int `json:"end"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if err := json.Unmarshal(sc.Bytes(), &env); err != nil {
			continue
		}
		if env.Type != "match" {
			continue
		}
		ctx := strings.TrimRight(env.Data.Lines.Text, "\n")
		// Strip leading whitespace from indented bullets so list rows line
		// up. Match spans are byte offsets into the *original* line, so we
		// also shift them by the number of bytes we trimmed.
		trimmed := strings.TrimLeft(ctx, " \t")
		shift := len(ctx) - len(trimmed)
		ctx = trimmed
		ctxLen := len(ctx)
		spans := make([]Span, 0, len(env.Data.Submatches))
		for _, sm := range env.Data.Submatches {
			start := sm.Start - shift
			end := sm.End - shift
			if end <= 0 || start >= ctxLen || start >= end {
				continue // span fell entirely inside the trimmed indent, or is degenerate
			}
			if start < 0 {
				start = 0
			}
			if end > ctxLen {
				end = ctxLen
			}
			spans = append(spans, Span{Start: start, End: end})
		}
		out = append(out, Hit{
			FilePath: env.Data.Path.Text,
			Line:     env.Data.LineNumber,
			Context:  ctx,
			Matches:  spans,
		})
	}
	return out
}
