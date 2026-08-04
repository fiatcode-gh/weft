# Weft Doctor Design

## Goal

Ship `weft doctor`, a headless subcommand that prints the graph-health numbers the index already computes — unresolved links, orphan pages, unlinked mentions, and index warnings — so retrofit and linkify runs are triggered by numbers, not vibes (journal TODO, 2026-07-23). The command is read-only: it never writes to the graph.

## CLI surface

`weft doctor [--graph PATH]` runs headless and one-shot: no alt screen, no Glamour warmup, no debug log. `main()` dispatches to the subcommand after `flag.Parse()` when `flag.Arg(0) == "doctor"`, and only after the existing validation runs (graph path resolved from flag/env, ripgrep preflight, directory stat). Doctor therefore inherits those failures and their exit code 2.

Doctor carries its own `--graph` flag parsed from the remaining args, so both orderings work:

```bash
weft doctor --graph /path/to/graph
weft --graph /path/to/graph doctor
```

Resolution chain: doctor flag, then global flag, then `$WEFT_GRAPH`. A doctor-flag path receives the same directory validation as the global path; failure prints to stderr and exits 2. A doctor flag parse error prints doctor usage to stderr and exits 2; `--help` prints usage to stdout and exits 0. Unknown positional arguments after `doctor` are a parse error.

## Report model

New package `internal/doctor` composes `internal/graph` and `internal/search`:

```go
type PageMentions struct {
    Page string              // page containing bare-text mentions
    Refs []graph.UnlinkedRef // the mention locations
}

type Report struct {
    GraphPath  string
    Pages      int // non-journal page count
    Journals   int
    Unresolved []graph.UnresolvedLink
    Orphans    []string       // non-journal page names, sorted ascending
    Mentions   []PageMentions // sorted ascending by Page; only pages with >= 1 ref
    Warnings   []string       // idx.Warnings verbatim
}

func Run(graphPath string) (*Report, error)
func RunWith(graphPath string, mentions func(graphPath, name string) ([]graph.UnlinkedRef, error)) (*Report, error)
func (r *Report) HasFindings() bool // any of Unresolved, Orphans, Mentions, Warnings non-empty
func (r *Report) WriteText(w io.Writer)
```

`Run` is `RunWith` backed by the real scanner. The injected `mentions` function receives a graph path and a page name and returns that page's unlinked references; production wires it to `search.Mentions` + `graph.FilterUnlinked` with the target's own file excluded — the exact pipeline the backlinks panel uses.

Scan order and scope:

- Orphans and mention targets are non-journal pages only; journals are retrofit-irrelevant as targets. Journals remain in scope as mention sources because the scanner greps pages/ and journals/ alike.
- A page is an orphan when every backlink to it is a self-reference or there are none — the same self-ref exclusion the backlinks panel applies.
- Mention targets are scanned in ascending page-name order. Sequential ripgrep runs; no parallelism.
- A `BuildIndex` failure or any scanner error fails `Run` with the cause wrapped — a partial report would defeat the purpose.

## Index accessor

The backlinks map is private, so `internal/graph` exports one accessor:

```go
type UnresolvedLink struct {
    Target string
    Refs   []Ref
}

func (idx *Index) UnresolvedLinks() []UnresolvedLink
```

A target is unresolved when `idx.Resolve` fails for it — exact-name and case-fold lookup, matching how the TUI resolves links. Targets are sorted case-insensitively with a case-sensitive byte-order tie-break; `Refs` keeps index walk order and is cloned, matching the snapshot semantics of `BacklinksTo`. Links from journal pages are included as refs.

## Output format

`WriteText` renders plain, uncolored, greppable text. Header, then a fixed four-row summary table (zeros included, labels padded to one column, counts after), then one detail section per non-zero signal in table order. Aligned detail columns pad the leading name to the longest name in that section plus two spaces; locations join with ` · `.

```text
weft doctor · testdata/fixture-graph
7 pages · 9 journals

  unresolved links     1
  orphan pages         1
  unlinked mentions    1
  index warnings       0

Unresolved links
  DoesNotExist  Beta:3

Orphan pages
  Orphan

Unlinked mentions
  Alpha         Workbench:6
```

The unlinked-mentions count is the total number of mentions across all pages. The warnings section prints each `idx.Warnings` entry verbatim. When nothing is found, no detail sections render and the table is followed by the single line `graph is clean`.

## Exit codes

- 0 — ran and the graph is clean;
- 1 — ran and findings exist;
- 2 — operational error: missing graph path, doctor flag parse error, inaccessible graph directory, `BuildIndex` failure, scanner failure, missing ripgrep.

## Fixture and test strategy

`testdata/fixture-graph` already exercises unresolved links (`[[DoesNotExist]]` in Beta), an orphan (`Orphan.md`), and the self-ref exclusion (`[[Hub]]` in Hub). It lacks a cross-page unlinked mention: add one bare-text mention of an existing non-journal page to one existing fixture page. Fixture churn is bounded — check every fixture-backed test and golden against the new line; regenerate goldens with `-update` only after a visual diff confirms the change is the new line's effect.

Tests:

- `internal/graph`: `UnresolvedLinks` over the fixture — phantom target with its refs, resolved targets absent, case-folded link targets absent, deterministic ordering.
- `internal/doctor` unit: `RunWith` with an injected scanner builds the full report from the fixture index (orphans, mention assembly, warnings passthrough, page/journal counts); scanner errors fail `Run`.
- `internal/doctor` integration: `Run` with real ripgrep behind the existing `skipIfNoRipgrep` guard finds the fixture's cross-page mention and matches the injected-scanner result.
- `internal/doctor` render: `WriteText` exact-output assertions for a fully populated hand-built report, a clean report, and zero-section omission.
- `cmd/weft`: `runDoctor` exit-code mapping (clean 0, findings 1, operational error 2) and graph-flag resolution chain.

Every task starts with a failing test and recorded RED evidence. Before every commit run:

```bash
test -z "$(gofmt -l .)"
go vet ./...
"$(go env GOPATH)/bin/staticcheck" ./...
go test -race ./...
```

## Documentation

README gains a `weft doctor` section after Usage: invocation, the four signals in one sentence each, and the exit-code contract.

## Completion

- Before implementation, add an `Open` bullet to `${WEFT_GRAPH}/pages/weft Backlog.md` for `weft doctor`, preserving the page's voice.
- Each task receives implementer, spec-review, and quality-review passes per subagent-driven-development, with scoped re-review after fixes.
- When the branch is green, push and open one Forgejo PR via the forgejo skill.
- After merge: move the backlog bullet to `Done` and mark the `- TODO weft doctor ...` bullet in `${WEFT_GRAPH}/journals/2026_07_23.md` as DONE, preserving its wording.
- Journal the completed session through the journal-update skill.
