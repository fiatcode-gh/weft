# Invariants Sweep Design

## Goal

Remove three duplicated or caller-owned invariants at the top of the weft backlog without changing user-visible behavior:

1. graph extraction and page rendering must agree exactly on which bullets are open tasks;
2. overlays must report one legal outcome at a time;
3. backlink folding and journal ordering must be owned by `graph.Index`, not remembered by views.

Bundle the adjacent documentation and test-helper cleanup plus only source-adjacent coverage extras. The Glamour v1.0.0 upgrade is explicitly out of scope.

## Open-task classification

`internal/graph` will export:

```go
func IsOpenTask(line string) bool
```

The predicate is backed by the existing `todoRe` grammar. `parseBody` will use the same predicate before extracting task fields, and `internal/render.preprocessTaskMarkers` will call it on the original markdown line when deciding whether to record a deep-link position. Rendering will continue to style all recognized workflow markers; only open-task position recording changes ownership.

This keeps dashboard `TodoBullet.Ordinal` values and rendered `Result.Tasks` offsets aligned by construction, including priority markers, punctuation-adjacent pseudo-markers, empty task text, indentation, and fenced code.

## Overlay outcomes

`OverlayResult` will become a tagged value with unexported payload fields. The valid outcomes and construction paths are:

- `OverlayResult{}` for no action;
- `overlayCancel()`;
- `overlayCommand(cmd)`;
- `overlayOpen(name)`;
- `overlayCreate(name)`;
- `overlayOpenTask(name, ordinal)`;
- `overlayFocusLink(name, target)`;
- `overlayHighlight(name, text)`;
- `overlayLinkify(ref, target)`.

`App.Update` will switch on the result tag rather than interpreting independent booleans in precedence order.

Existing behavior remains unchanged:

- `ctrl+c` still quits through any overlay;
- cancel closes the overlay;
- open outcomes close after navigation;
- an empty search-derived page name closes without navigating;
- create and linkify remain blocked while sync is running and leave the overlay visible with its in-panel error;
- search commands keep the overlay open;
- linked and unlinked backlink jumps retain their focus/highlight behavior.

## Index-owned invariants

Folded backlink storage and sorted journal storage will become implementation details of `graph.Index`.

```go
func (idx *Index) BacklinksTo(name string) []Ref
func (idx *Index) JournalNeighbor(current string, dir int) (string, bool)
```

`BacklinksTo` folds the supplied page name internally before lookup. Callers no longer import `strings` merely to access the map.

`JournalNeighbor` owns journal-name validation and binary search over the ascending journal list. It preserves current semantics for an indexed journal, a journal-shaped phantom date, invalid names, and movement beyond either end. Views no longer import `sort` or depend directly on journal ordering.

The existing `Backlinks` and `Journals` fields will become unexported backing fields so callers cannot bypass these methods. Exact-name lookup remains available through the existing `Resolve`/index behavior needed by the rest of the graph.

## Adjacent cleanup

- Correct `stripQueryAndEmbedBlocks` documentation to cover both a bare own-line closer and the same-line self-closing form.
- Correct `PageMeta.Path` documentation to say the path preserves the supplied graph-path rootedness; it is absolute only when the graph path is absolute. Do not change path behavior because `FilterUnlinked` relies on both compared paths sharing the same prefix form.
- Replace `bootApp`, `bootAppAt`, and `bootAppWithGraph` with one boot sequence accepting an optional test configuration for fixture files and pinned time.
- Do not upgrade Glamour or refresh dependencies.

## Source-adjacent coverage

Add the three backlog coverage cases whose production surfaces are already touched by this sweep:

- picker selection can move down past fuzzy matches to the create row;
- malformed regular-expression search reports an `rg failed` error;
- emphasis whole-word matching rejects a match without a valid left boundary.

Defer the PageView disappearance/cursor golden, completion-strip `moveUp`, and CLI helper tables because their production surfaces are otherwise untouched.

## TDD and task boundaries

Implementation is split into four independently reviewed commits:

1. shared open-task predicate plus render agreement and emphasis-boundary coverage;
2. tagged overlay outcomes plus picker and malformed-search coverage;
3. `Index.BacklinksTo` and `Index.JournalNeighbor` plus caller migration;
4. boot-helper consolidation and documentation corrections.

Each task starts with a failing characterization or API test and recorded RED evidence, then receives the minimal implementation. Before every commit run:

```bash
test -z "$(gofmt -l .)"
go vet ./...
"$(go env GOPATH)/bin/staticcheck" ./...
go test -race ./...
```

Each task receives implementer, spec-review, and quality-review passes, with scoped re-review after fixes. Teatest goldens are not regenerated unless an intentional output change is identified and visually reviewed; this refactor is expected to leave them byte-identical.

After all task commits, a whole-branch final review checks cross-task behavior and plan-level omissions. Any fixes receive the same full gate before commit.

## Completion

When the branch is green and final review is clear:

- move completed backlog bullets from `Open` to `Done` in `${WEFT_GRAPH}/pages/weft Backlog.md`, preserving its voice;
- leave the Glamour item at the top of `Open` for a separate session;
- push the feature branch and open one Forgejo PR using the linked-worktree curl fallback if `tea pr create` fails;
- journal the completed session through the `journal-update` skill.
