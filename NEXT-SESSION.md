# Next Session — Kickoff Prompt

Paste the block below into a fresh Claude Code session opened from `~/Development/Projects/fiatcode/logseq-tui` to resume work.

---

Execute the implementation plan at `docs/superpowers/plans/2026-05-24-logseq-tui.md`. The spec it implements is at `docs/superpowers/specs/2026-05-24-logseq-tui-design.md` — read it first for context.

Use the `superpowers:subagent-driven-development` skill: dispatch one fresh subagent per task in the plan, in order, and review the output between tasks before moving on. Tasks are TDD-shaped (red → green → commit) and each ends with a Conventional Commit per my global instructions.

Ground rules:
- Working dir: `~/Development/Projects/fiatcode/logseq-tui`.
- Run `go vet ./... && go test ./...` before every commit; both must pass.
- Do not skip the failing-test step — write the test, confirm RED, then implement.
- If a task uncovers a real problem with the plan, stop and flag it instead of silently improvising. Small corrections (typos, obvious method-name fixes) are fine to make inline.
- The fixture graph lives at `testdata/fixture-graph/` once Task 2 lands; don't touch the real graph at `~/Documents/fiat-codex` during development.
- External dep: `rg` (ripgrep) must be on PATH. Verify with `which rg` early.

Start with Task 1 (Bootstrap module). When all 15 tasks are green and committed, build the binary, run it once against the fixture as a final smoke test, and report back.
