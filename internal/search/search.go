// Package search runs ripgrep over a set of graph files and parses its JSON
// output into structured hits. It performs process I/O and decoding so the UI
// layer can consume plain []Hit values.
package search

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
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

// argBudget caps the summed length of the file paths passed to one rg
// invocation, keeping the command line well under the OS argument limit.
var argBudget = 128 << 10

// Run executes ripgrep for query over exactly the given files and returns the
// parsed hits. It returns (nil, nil) when files is empty. A ripgrep exit code
// of 1 (no matches) is treated as success.
func Run(files []string, query string) ([]Hit, error) {
	out, err := runRipgrep(files, []string{"--smart-case"}, query)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// Mentions finds whole-word, case-insensitive, literal occurrences of name
// across exactly the given files. Used to detect unlinked references to a
// page; word boundaries keep "Go" from matching "Google", and -F treats names
// with regex metacharacters literally.
func Mentions(files []string, name string) ([]Hit, error) {
	out, err := runRipgrep(files, []string{"-w", "-F", "--ignore-case"}, name)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// runRipgrep runs `rg --json <flags...> -- <query> <files...>` and returns the
// raw stdout. Large file lists are split into batches under argBudget and the
// outputs concatenated. A ripgrep exit code of 1 (no matches) is treated as
// success for a batch. Returns (nil, nil) when files is empty.
func runRipgrep(files []string, flags []string, query string) ([]byte, error) {
	var all []byte
	for _, batch := range batches(files) {
		args := append([]string{"--json"}, flags...)
		args = append(args, "--", query)
		args = append(args, batch...)
		cmd := exec.Command("rg", args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			err = nil
		}
		if err != nil {
			return nil, fmt.Errorf("rg failed: %w (%s)", err, stderr.String())
		}
		all = append(all, stdout.Bytes()...)
	}
	return all, nil
}

// batches splits files into consecutive groups whose summed len(path)+1 stays
// within argBudget; a group always holds at least one file.
func batches(files []string) [][]string {
	var out [][]string
	start, size := 0, 0
	for i, f := range files {
		n := len(f) + 1
		if i > start && size+n > argBudget {
			out = append(out, files[start:i])
			start, size = i, 0
		}
		size += n
	}
	if start < len(files) {
		out = append(out, files[start:])
	}
	return out
}

func parseJSON(b []byte) ([]Hit, error) {
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
		ctx := strings.TrimRight(env.Data.Lines.Text, "\r\n")
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
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("scan rg output: %w", err)
	}
	return out, nil
}
