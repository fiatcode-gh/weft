package render

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

type levelAt struct{ line, level int }

// graphHeadings and scannerHeadings must agree: graph.Headings is the one
// definition of a heading (link resolution, folding), and the Scanner is what
// the renderer and painter classify with.
func graphHeadings(body string) []levelAt {
	var out []levelAt
	for _, h := range graph.Headings(body) {
		out = append(out, levelAt{h.Line, h.Level})
	}
	return out
}

func scannerHeadings(body string) []levelAt {
	src := strLines(strings.Split(body, "\n"))
	sc := NewScanner()
	var out []levelAt
	for i := range src {
		if k := sc.Info(src, i); k.Kind == KindHeading || k.Kind == KindSetextHeading {
			out = append(out, levelAt{i, k.Level})
		}
	}
	return out
}

func TestGraphHeadingsAgreeWithScanner(t *testing.T) {
	corpus := map[string]string{
		"edge": "# a\n## b ##\n#\n    # x\n  ## y\nOne\n===\n\nTwo\n---\n- item\ntext\n---\n\na | b\n---|---\n1 | 2\n\n" +
			"```\n# no\n```\n:LOGBOOK:\n# no\n:END:\n{{query\n# no\n}}\n{{embed [[X]]}}\n# c\n#tag\n#!/bin/sh\n#+BEGIN_QUERY\n- ## x\n",
		"list continuation": "- a\n  text\n  ---\n- b\nPara\n---\n> quote\n---\n1. one\n   more\n   ===\n",
		"rule and setext":   "text\n***\n\ntext\n- - -\n\nTitle\n=\nTitle2\n--\n",
		"tilde fence":       "~~~\n# no\n```\n# no\n~~~\n# yes\n",
	}
	files, err := filepath.Glob("../../testdata/fixture-graph/*/*.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("fixture graph files: %v (%d)", err, len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		corpus[f] = string(b)
	}
	for name, body := range corpus {
		if got, want := graphHeadings(body), scannerHeadings(body); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: graph.Headings %v, Scanner %v", name, got, want)
		}
	}
}
