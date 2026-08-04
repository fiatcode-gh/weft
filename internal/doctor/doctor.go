// Package doctor computes a read-only health report over a weft graph:
// unresolved wiki-links, orphan pages, unlinked mentions, and index
// warnings. It never writes to the graph.
package doctor

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
	"git.fiatcode.dev/fiatcode/weft/v2/internal/search"
)

// MentionScanner returns the unlinked (bare-text) references to one page.
type MentionScanner func(graphPath, name string) ([]graph.UnlinkedRef, error)

// PageMentions collects one page's bare-text mentions elsewhere in the graph.
type PageMentions struct {
	Page string              // page that is mentioned
	Refs []graph.UnlinkedRef // mention locations
}

// Report is the full graph-health result.
type Report struct {
	GraphPath  string
	Pages      int // non-journal page count
	Journals   int
	Unresolved []graph.UnresolvedLink
	Orphans    []string       // non-journal page names with no backlinks from other pages
	Mentions   []PageMentions // ascending by Page; only pages with at least one ref
	Warnings   []string       // index warnings verbatim
}

// HasFindings reports whether any health signal fired.
func (r *Report) HasFindings() bool {
	return len(r.Unresolved) > 0 || len(r.Orphans) > 0 || len(r.Mentions) > 0 || len(r.Warnings) > 0
}

// Run computes the report using the ripgrep-backed mention scanner — the
// exact pipeline the backlinks panel uses (search.Mentions +
// graph.FilterUnlinked, target's own file excluded).
func Run(graphPath string) (*Report, error) {
	idx, err := graph.BuildIndex(graphPath)
	if err != nil {
		return nil, err
	}
	return run(idx, graphPath, func(gp, name string) ([]graph.UnlinkedRef, error) {
		hits, err := search.Mentions(gp, name)
		if err != nil {
			return nil, err
		}
		targetPath := ""
		if meta, ok := idx.Resolve(name); ok {
			targetPath = meta.Path
		}
		return graph.FilterUnlinked(hits, targetPath, readFile), nil
	})
}

// RunWith computes the report with a caller-supplied mention scanner.
func RunWith(graphPath string, mentions MentionScanner) (*Report, error) {
	idx, err := graph.BuildIndex(graphPath)
	if err != nil {
		return nil, err
	}
	return run(idx, graphPath, mentions)
}

// run assembles the report from a built index. Orphans and mention targets
// are non-journal pages only; journals stay in scope as mention sources
// because the scanner greps pages/ and journals/ alike.
func run(idx *graph.Index, graphPath string, mentions MentionScanner) (*Report, error) {
	rep := &Report{
		GraphPath:  graphPath,
		Unresolved: idx.UnresolvedLinks(),
		Warnings:   idx.Warnings,
	}
	var names []string
	for _, p := range idx.Pages {
		if p.IsJournal {
			rep.Journals++
			continue
		}
		rep.Pages++
		names = append(names, p.Name)
		if !hasOtherBacklink(idx, p.Name) {
			rep.Orphans = append(rep.Orphans, p.Name)
		}
	}
	sort.Strings(rep.Orphans)
	sort.Strings(names)
	for _, name := range names {
		refs, err := mentions(graphPath, name)
		if err != nil {
			return nil, fmt.Errorf("scan unlinked mentions for %q: %w", name, err)
		}
		if len(refs) > 0 {
			rep.Mentions = append(rep.Mentions, PageMentions{Page: name, Refs: refs})
		}
	}
	return rep, nil
}

// hasOtherBacklink reports whether any page other than name links to it —
// the same self-ref exclusion the backlinks panel applies.
func hasOtherBacklink(idx *graph.Index, name string) bool {
	for _, r := range idx.BacklinksTo(name) {
		if r.FromPage != name {
			return true
		}
	}
	return false
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

// WriteText renders the report as plain, uncolored text: header, a fixed
// four-row summary table (zeros included), then one detail section per
// non-zero signal in table order. Detail columns pad the leading name to
// the longest name in that section plus two spaces; locations join with
// " · ". A clean report ends with the single line "graph is clean".
func (r *Report) WriteText(w io.Writer) {
	fmt.Fprintf(w, "weft doctor · %s\n", r.GraphPath)
	fmt.Fprintf(w, "%d pages · %d journals\n\n", r.Pages, r.Journals)
	fmt.Fprintf(w, "  %-17s %d\n", "unresolved links", len(r.Unresolved))
	fmt.Fprintf(w, "  %-17s %d\n", "orphan pages", len(r.Orphans))
	fmt.Fprintf(w, "  %-17s %d\n", "unlinked mentions", r.mentionTotal())
	fmt.Fprintf(w, "  %-17s %d\n", "index warnings", len(r.Warnings))
	if !r.HasFindings() {
		fmt.Fprintln(w, "\ngraph is clean")
		return
	}
	if len(r.Unresolved) > 0 {
		fmt.Fprintln(w, "\nUnresolved links")
		width := 0
		for _, u := range r.Unresolved {
			if len(u.Target) > width {
				width = len(u.Target)
			}
		}
		for _, u := range r.Unresolved {
			fmt.Fprintf(w, "  %-*s  %s\n", width, u.Target, joinRefLocations(u.Refs))
		}
	}
	if len(r.Orphans) > 0 {
		fmt.Fprintln(w, "\nOrphan pages")
		for _, name := range r.Orphans {
			fmt.Fprintf(w, "  %s\n", name)
		}
	}
	if len(r.Mentions) > 0 {
		fmt.Fprintln(w, "\nUnlinked mentions")
		width := 0
		for _, m := range r.Mentions {
			if len(m.Page) > width {
				width = len(m.Page)
			}
		}
		for _, m := range r.Mentions {
			fmt.Fprintf(w, "  %-*s  %s\n", width, m.Page, joinMentionLocations(m.Refs))
		}
	}
	if len(r.Warnings) > 0 {
		fmt.Fprintln(w, "\nIndex warnings")
		for _, msg := range r.Warnings {
			fmt.Fprintf(w, "  %s\n", msg)
		}
	}
}

func (r *Report) mentionTotal() int {
	n := 0
	for _, m := range r.Mentions {
		n += len(m.Refs)
	}
	return n
}

func joinRefLocations(refs []graph.Ref) string {
	locs := make([]string, 0, len(refs))
	for _, r := range refs {
		locs = append(locs, fmt.Sprintf("%s:%d", r.FromPage, r.LineNumber))
	}
	return strings.Join(locs, " · ")
}

func joinMentionLocations(refs []graph.UnlinkedRef) string {
	locs := make([]string, 0, len(refs))
	for _, r := range refs {
		locs = append(locs, fmt.Sprintf("%s:%d", r.PageName, r.Line))
	}
	return strings.Join(locs, " · ")
}
