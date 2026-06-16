# Rebrand peekseq → weft — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the tool from `peekseq` to `weft` across the codebase, docs, and the Logseq graph, without changing behavior or the on-disk Logseq format.

**Architecture:** A wide-but-shallow mechanical rename, sequenced so `go vet ./... && go test ./...` passes at every commit. Module/dir rename first (structural, no string changes), then all in-`.go` name occurrences + golden, then non-code docs, then the external graph retrofit, then the release. Verification-driven, not test-first: this is a rename of existing, tested behavior — the existing suite + the `TestHelpGolden` golden are the safety net (per AGENTS.md, both must pass before every commit).

**Tech Stack:** Go 1.26, Bubble Tea, ripgrep, teatest goldens. `git mv`, `sed`. `logseq:retrofit` skill for the graph.

**Spec:** `docs/superpowers/specs/2026-06-16-rebrand-weft-design.md`

**Branch:** `rename/weft` (already created; spec already committed there).

---

### Task 1: Module path + command-dir rename

Structural only — no string/comment/test changes. Renames the Go module, every
import line, and the command directory (which renames the built binary).

**Files:**
- Modify: `go.mod` (module line)
- Modify: every `*.go` importing `git.fiatcode.dev/fiatcode/peekseq/...` (21 files across `cmd/`, `internal/`)
- Rename: `cmd/peekseq/` → `cmd/weft/`

- [ ] **Step 1: Rename the module path in `go.mod`**

```bash
sed -i 's#^module git\.fiatcode\.dev/fiatcode/peekseq#module git.fiatcode.dev/fiatcode/weft#' go.mod
```

- [ ] **Step 2: Sweep every Go import of the old module path**

```bash
grep -rl 'git\.fiatcode\.dev/fiatcode/peekseq' --include='*.go' . \
  | xargs sed -i 's#git\.fiatcode\.dev/fiatcode/peekseq#git.fiatcode.dev/fiatcode/weft#g'
```

- [ ] **Step 3: Rename the command directory (renames the binary)**

```bash
git mv cmd/peekseq cmd/weft
```

- [ ] **Step 4: Verify nothing still imports the old path**

```bash
grep -rn 'git\.fiatcode\.dev/fiatcode/peekseq' --include='*.go' . ; echo "exit:$?"
```
Expected: no matches (grep prints nothing, `exit:1`).

- [ ] **Step 5: Build, vet, test**

```bash
go build ./cmd/weft && go vet ./... && go test ./...
```
Expected: binary `./weft` builds; vet clean; **all tests PASS** (runtime strings
are unchanged this task, so existing `"peekseq"` string assertions still hold).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: rename Go module and command dir peekseq → weft"
```

---

### Task 2: Rename every in-`.go` name occurrence + regenerate golden

Covers runtime user-visible strings (title bar, help footer, stderr, debug log
filename), env vars (`PEEKSEQ_* → WEFT_*`), doc comments, and the test assertions
that mirror those strings. After Task 1 the only remaining `peekseq`/`PEEKSEQ`
tokens in `*.go` are exactly these — two scoped seds cover them, and the test
suite + golden are the guard.

**Files (all `*.go` still containing the name):**
- Modify: `cmd/weft/main.go` (stderr prefix, `WEFT_DEBUG`, `WEFT_GRAPH`, `weft.log`, `LogToFile` prefix)
- Modify: `internal/views/app.go` (title `"weft"` / `"weft — failed to index %s"`, comment)
- Modify: `internal/views/help.go` (footer `"weft " + version`)
- Modify: `internal/graph/index.go` (stderr `weft: skipping …`)
- Modify: `internal/render/page.go` (`WEFT_STYLE`, comment)
- Modify: `internal/edit/editor.go`, `internal/views/editor.go` (package/doc comments)
- Test: `internal/views/help_test.go` (`"weft v0.1.2"`, `"weft "` prefix checks)
- Test: `internal/views/app_dispatch_test.go:98` (`strings.Contains(v, "weft")`)
- Regenerate: `internal/views/testdata/TestHelpGolden.golden`

- [ ] **Step 1: Sweep the env-var prefix across all Go files**

```bash
grep -rl 'PEEKSEQ_' --include='*.go' . | xargs sed -i 's/PEEKSEQ_/WEFT_/g'
```
(Safe: no `*_test.go` sets these env vars — verified — so no test fixture breaks.)

- [ ] **Step 2: Sweep the bare word across all Go files (strings, comments, assertions)**

```bash
grep -rl 'peekseq' --include='*.go' . | xargs sed -i 's/peekseq/weft/g'
```
This turns `"peekseq:"` → `"weft:"`, `"peekseq.log"` → `"weft.log"`, the title/footer
strings, the doc comments, and the `help_test.go`/`app_dispatch_test.go` expectations
all to `weft` in lockstep. (Only lowercase `peekseq` and uppercase-prefix `PEEKSEQ_`
exist — no mixed-case `Peekseq` — so these two seds are exhaustive.)

- [ ] **Step 3: Confirm no name tokens remain in Go**

```bash
grep -rni 'peekseq' --include='*.go' . ; echo "exit:$?"
```
Expected: no matches (`exit:1`).

- [ ] **Step 4: Run the suite to find the golden mismatch**

```bash
go vet ./... && go test ./internal/views -run TestHelpGolden
```
Expected: `TestHelpGolden` **FAILS** — the rendered footer now reads `weft <version>`
but the golden still says `peekseq v1.0.0`. This failure is the cue to regenerate.

- [ ] **Step 5: Regenerate the golden and eyeball the diff**

```bash
go test ./internal/views -run TestHelpGolden -update
git --no-pager diff internal/views/testdata/TestHelpGolden.golden
```
Expected diff: the footer line changes `peekseq v1.0.0` → `weft v1.0.0` and nothing
else. If anything else moved, stop and investigate.

- [ ] **Step 6: Full vet + test**

```bash
go vet ./... && go test ./...
```
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: rename runtime strings, env vars, and golden peekseq → weft"
```

---

### Task 3: Non-code docs, license, gitignore

No build impact. Living docs only — historical `docs/superpowers/plans/*` and
`specs/*` (except this rename pair) stay `peekseq` on purpose.

**Files:**
- Modify: `README.md` (title, install command `cmd/weft@latest`, clone URL `…/weft`, env-var table `WEFT_*`, all prose)
- Modify: `AGENTS.md` (intro, all `go build ./cmd/weft` / `./weft` commands, layer bullets, env vars, debug `WEFT_DEBUG`/`weft.log`)
- Modify: `CHANGELOG.md` (intro line `weft`; add the rename release entry — see Step 3)
- Modify: `LICENSE` (`the weft authors`)
- Modify: `.gitignore` (`/peekseq` → `/weft`)
- Modify: `NEXT-SESSION.md` (untracked scratch; update references — NOT committed)

- [ ] **Step 1: Sweep the living docs + gitignore**

```bash
sed -i 's/peekseq/weft/g; s/PEEKSEQ_/WEFT_/g' README.md AGENTS.md CHANGELOG.md LICENSE .gitignore NEXT-SESSION.md
```

- [ ] **Step 2: Verify the historical record was untouched**

```bash
git status --short docs/superpowers/   # expect: no changes under plans/ or specs/ except the rename pair (already committed)
grep -rl 'peekseq' docs/superpowers/plans docs/superpowers/specs | grep -v 'rebrand-weft' | head
```
Expected: the older dated plans/specs still contain `peekseq` and show no diff.

- [ ] **Step 3: Add the CHANGELOG release entry**

Insert above the `## [1.4.0]` section (replace `<NEW_VERSION>`/date per the
Versioning decision in Task 5; default `2.0.0`):

```markdown
## [<NEW_VERSION>] - 2026-06-16

### Changed

- **Renamed the project from `peekseq` to `weft`.** This is a breaking change for
  installed users: the binary, the `go install` path (`…/cmd/weft@latest`), and
  the environment variables all change.
  - `PEEKSEQ_GRAPH` → `WEFT_GRAPH`, `PEEKSEQ_DEBUG` → `WEFT_DEBUG`,
    `PEEKSEQ_STYLE` → `WEFT_STYLE` (no backward-compatible fallback).
  - Debug log file is now `weft.log`.
  - The on-disk Logseq graph format is unchanged.
```

- [ ] **Step 4: Sanity build/test (docs shouldn't affect it) and commit the tracked files**

```bash
go vet ./... && go test ./...
git add README.md AGENTS.md CHANGELOG.md LICENSE .gitignore
git commit -m "docs: rebrand README, AGENTS, CHANGELOG, LICENSE peekseq → weft"
```
(`NEXT-SESSION.md` is git-excluded — its edit lands but is not committed; that's expected.)

---

### Task 4: Retrofit the Logseq graph (separate skill, separate surface)

Run **in the main session via the `logseq:retrofit` skill** — not a generic code
subagent. This is the user's mutable graph at `~/Documents/fiat-codex`, a write
surface outside the repo.

- [ ] **Step 1:** Invoke `logseq:retrofit` to rename `pages/peekseq.md` → `pages/weft.md` and retarget every live `[[peekseq]]` reference.
- [ ] **Step 2:** Confirmed cross-reference targets: `pages/AI Memory.md` (the `## [[peekseq]]` section header + the rename bullet's `scope::`/`source::`), `pages/Lifecycle-Aware State Versioning.md`, `pages/Fedora Kinoite Installation.md`.
- [ ] **Step 3:** Leave the 9 dated journal entries as historical record (do NOT retrofit).
- [ ] **Step 4:** Do not commit or sync the graph (graph convention).

---

### Task 5: Pull request + release (outward-facing — confirm before pushing)

- [ ] **Step 1:** Push `rename/weft` and open a PR on `git.fiatcode.dev/fiatcode/weft` via the `forgejo` skill.
- [ ] **Step 2:** Confirm the version number with the user (recommended **v2.0.0** — breaking binary/env/install changes). Apply it to the CHANGELOG entry from Task 3, Step 3.
- [ ] **Step 3:** After merge, tag and cut the release via `forgejo` (build with `-ldflags "-X main.Version=<NEW_VERSION>"`). This is the outward-facing step — confirm explicitly before tagging/publishing.

---

## Self-Review

**Spec coverage:** Module path ✓ (T1), cmd dir/binary ✓ (T1), env vars ✓ (T2 S1),
runtime strings ✓ (T2 S2), test assertions ✓ (T2 S2), golden ✓ (T2 S4–5), code
comments ✓ (T2 S2), README/AGENTS/CHANGELOG/LICENSE ✓ (T3), gitignore ✓ (T3),
NEXT-SESSION ✓ (T3), "stays historical" guard ✓ (T3 S2), graph retrofit ✓ (T4),
versioning/release ✓ (T5). No gaps.

**Placeholder scan:** `<NEW_VERSION>` is an intentional, documented decision point
resolved in T5 S2, not a placeholder gap. No TBD/TODO/"handle edge cases".

**Consistency:** env sweep `PEEKSEQ_ → WEFT_` and word sweep `peekseq → weft` are
the same two transforms used consistently in T2 (code) and T3 (docs); the "no
mixed-case `Peekseq`" fact (verified in the spec scan) is what makes them
exhaustive. Verification greps (`exit:1` expected) gate each sweep.
