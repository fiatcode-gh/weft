# Rebrand: peekseq → weft

**Date:** 2026-06-16
**Status:** approved (pre-approved for subagent-driven execution)

## Why

`peekseq` was named as a *viewer* — "peek" = look. The tool has since grown its
own interaction grammar (in-app editor, `[[`-completion, unlinked references,
one-key linkify) and become a terminal **knowledge base** that reads the Logseq
format but is its own thing, inspired by Logseq's linking model. `weft` — the
cross-thread in weaving — names what made it more than a viewer: the woven
cross-links (backlinks / linkify). It sits well over a graph dir literally named
`fiat-codex`: the threads woven through the codex.

The on-disk format stays Logseq-compatible. This is a rename of the **tool's
identity**, not a fork of the **data format**. README/AGENTS keep framing weft as
"reads a Logseq-format graph."

## Scope

A wide-but-shallow mechanical rename. Three categories: things that **change**,
things that **stay** (historical record), and one **external** surface (the graph).

### Changes (in-repo)

1. **Module path** — `git.fiatcode.dev/fiatcode/peekseq` → `git.fiatcode.dev/fiatcode/weft`
   (`go.mod` + all internal import lines across `cmd/` and `internal/`).
2. **Command dir / binary** — `cmd/peekseq/` → `cmd/weft/`; built binary becomes
   `weft`. Updates the `go install …/cmd/weft@latest` target.
3. **Env vars (hard rename, no legacy fallback):** `PEEKSEQ_GRAPH` → `WEFT_GRAPH`,
   `PEEKSEQ_DEBUG` → `WEFT_DEBUG`, `PEEKSEQ_STYLE` → `WEFT_STYLE`.
4. **Runtime user-visible strings:**
   - `internal/views/app.go` — title bar `"weft"` / `"weft — failed to index %s"`.
   - `internal/views/help.go` — footer `"weft " + version`.
   - `cmd/peekseq/main.go` (→ `cmd/weft/main.go`) — stderr prefix `weft:`, debug
     log filename `weft.log`, `tea.LogToFile` prefix.
   - `internal/graph/index.go` — stderr `weft: skipping …`.
5. **Test-side assertions** that hard-code the name/strings:
   - `internal/views/help_test.go` — `"peekseq v0.1.2"` and the `"peekseq "`
     prefix checks → `weft`.
   - `internal/views/app_dispatch_test.go:98` — `"peekseq"` title contains-check.
6. **Golden:** `internal/views/testdata/TestHelpGolden.golden` — regenerate with
   `go test ./internal/views -run TestHelpGolden -update` (footer now reads
   `weft <version>`); visually diff before staging.
7. **Doc comments in code** — `edit/editor.go`, `render/page.go`, `views/editor.go`,
   `views/app.go` package/inline comments naming peekseq.
8. **Project docs (living):** `README.md`, `AGENTS.md`, `CHANGELOG.md` (intro line
   + a new release entry), `LICENSE` (`the weft authors`).
9. **`.gitignore`** — `/peekseq` → `/weft` (ignored binary name).
10. **`NEXT-SESSION.md`** — local untracked scratch (excluded via
    `.git/info/exclude`); update references for coherence, not committed.

### Stays (historical record — do NOT rewrite)

- `docs/superpowers/plans/*` and `docs/superpowers/specs/*` (except this new spec)
  — dated artifacts describing what was built under the old name.
- The 9 Logseq journal entries mentioning peekseq, and prior release notes/tags.

Rationale: same logic as not rewriting git history — these were `peekseq` at the
time, and rewriting them misrepresents the record and adds churn.

### External (separate write surface — the graph)

- `~/Documents/fiat-codex/pages/peekseq.md` → `pages/weft.md`; retarget the
  `[[peekseq]]` wiki-link and live cross-references in `pages/AI Memory.md`,
  `pages/Lifecycle-Aware State Versioning.md`, `pages/Fedora Kinoite Installation.md`,
  and the AI-memory bullet's `scope::`/`source::`. Done via the `logseq:retrofit`
  skill, **not** a generic code subagent. Journals stay as-is (dated record).
- `ai-stack` references to peekseq are historical examples — leave.

## Sequencing (each step leaves the tree green)

The rename is atomic-per-step so `go vet ./... && go test ./...` passes at every
checkpoint:

1. **Module + dir rename** — `go.mod`, every import line, `cmd/peekseq/` →
   `cmd/weft/`. Runtime strings unchanged, so existing string assertions still
   pass. Verify: build `./cmd/weft`, vet, full test suite green.
2. **Runtime-facing rename** — env vars + user-visible strings + the test
   assertions that mirror them, together (they move in lockstep), then regenerate
   the golden. Verify: vet + test green; eyeball the golden diff.
3. **Docs + comments + gitignore** — no build impact. Verify: vet + test still
   green (sanity).
4. **Graph retrofit** — `logseq:retrofit`, separate skill, separate surface.
5. **Release** — see Versioning; confirmed separately (outward-facing).

## Testing

- Per AGENTS.md: `go vet ./... && go test ./...` must pass before every commit.
- Golden regen only via `-update`, with a visual diff before staging.
- No new behavior — this is a rename. No new tests; existing tests must stay green
  with updated expectations.

## Versioning

The rename is **user-facing breaking**: binary name, env vars, and the
`go install` path all change. Recommendation: cut **v2.0.0** with a CHANGELOG
`### Changed` entry documenting the rename and the env-var migration. The actual
tag/release is an outward-facing action confirmed separately from the code work.

## Risks / gotchas

- **Half-done module rename doesn't compile** — keep step 1 atomic; don't commit a
  partial import sweep.
- **Golden drift** — the only golden touching the name is `TestHelpGolden`; regen
  deliberately, never hand-edit.
- **Don't over-reach into history** — the temptation is to sed the whole repo; the
  `docs/superpowers/**` historical plans/specs must stay `peekseq`.
- **Env-var break is intentional** — the one live consumer is the user's shell
  config (`PEEKSEQ_GRAPH=~/Documents/fiat-codex`); they edit it once. No dual-read
  fallback code.
