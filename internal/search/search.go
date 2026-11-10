// Package search runs ripgrep over a set of graph files and parses its JSON
// output into structured hits. It performs process I/O and decoding so the UI
// layer can consume plain []Hit values.
package search

import (
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
// success for a batch. Exit code 2 is also success when rg got as far as its
// summary record: it finished searching and only some listed files failed to
// open, as happens when a file vanished since the last index. Exit 2 with no
// summary (a bad regex, say) is an error. Returns (nil, nil) when files is
// empty.
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
		if exitErr, ok := err.(*exec.ExitError); ok {
			switch exitErr.ExitCode() {
			case 1:
				err = nil
			case 2:
				if bytes.Contains(stdout.Bytes(), summaryMarker) {
					err = nil
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("rg failed: %w (%s)", err, stderr.String())
		}
		all = append(all, stdout.Bytes()...)
	}
	return all, nil
}

// summaryMarker identifies rg's closing JSON record. Text inside other records
// is JSON-escaped, so these raw bytes only occur in a real summary record.
var summaryMarker = []byte(`"type":"summary"`)

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

// rgData is rg's encoding of arbitrary data: {"text":...} when it is valid
// UTF-8, otherwise {"bytes":"<base64>"}.
type rgData struct {
	Text  string `json:"text"`
	Bytes []byte `json:"bytes"`
}

func (d rgData) String() string {
	if d.Text != "" {
		return d.Text
	}
	return string(d.Bytes)
}

// parseJSON decodes rg's newline-delimited JSON records into hits. Records are
// split on newlines rather than scanned with a bounded buffer, so a single
// huge matching line (a pasted base64 image) cannot abort the parse; records
// that fail to decode are skipped.
func parseJSON(b []byte) ([]Hit, error) {
	var out []Hit
	for len(b) > 0 {
		var line []byte
		line, b, _ = bytes.Cut(b, []byte{'\n'})
		var env struct {
			Type string `json:"type"`
			Data struct {
				Path       rgData `json:"path"`
				Lines      rgData `json:"lines"`
				LineNumber int    `json:"line_number"`
				Submatches []struct {
					Start int `json:"start"`
					End   int `json:"end"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		if env.Type != "match" {
			continue
		}
		ctx := strings.TrimRight(env.Data.Lines.String(), "\r\n")
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
			FilePath: env.Data.Path.String(),
			Line:     env.Data.LineNumber,
			Context:  ctx,
			Matches:  spans,
		})
	}
	return out, nil
}
