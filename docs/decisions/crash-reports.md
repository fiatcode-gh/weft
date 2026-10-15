# Crash reports are opt-in and sent by hand-written code

A release build of weft can send one crash report to the author's Bugsink when
weft panics. It sends only when the user config has `crash_reports = true`. First
run asks once (`Send crash reports to the weft author? [y/N]`, Enter means no).
A build from source has no endpoint, so it never asks and never sends. A report
holds the panic message, stack trace, weft version, OS, architecture and Go
version, and nothing else.

Why: the author cannot see a crash that happens on someone else's machine. A
report is the cheapest way to hear about it, and consent keeps it the user's
choice.

Out of scope: reporting anything but panics; tracing, sessions, profiling,
breadcrumbs, user feedback; a queue, a retry or a send on the next start;
reporting from builds without an endpoint; a setting inside the UI; any new
dependency.

## Decisions

1. **No SDK.** `internal/crash` posts one Sentry envelope by hand, so `go.mod`
   does not change. `sentry-go` fills the host name when it is empty, adds the
   module list, CPU count, goroutine counts, trace context and breadcrumbs, and
   pulls in extra modules. Keeping "nothing else" would mean undoing those
   defaults in a hook and still not controlling the envelope headers. About 250
   lines of our own send exactly the listed fields and are easy to audit.
2. **Bubble Tea keeps catching panics.** `tea.WithoutCatchPanics` is never used:
   Bubble Tea restores the terminal and prints the panic and stack. weft wraps
   the root model in `crash.Guard`, which records the panic value and stack in a
   `crash.Recorder` (first panic only) and lets Bubble Tea handle it. In `Init`,
   `Update` and `View` the deferred recover records and re-panics the same
   value on the `Run` goroutine. Every command returned by `Init` and `Update`
   is wrapped, and a batch has each child wrapped. A panicking command records
   and returns an internal `panicMsg`; `Guard.Update` re-panics with the report
   on the UI loop. Bubble Tea's own recovery of a command prints after `Run` may
   have returned, so this route makes the stack print synchronous.
3. **Not covered.** `tea.Sequence` children (the message type is unexported;
   weft uses none) and `tea.ExecProcess` callbacks (weft has one,
   `App.editCurrent`, built as a struct literal). A panic there still restores
   the terminal. Nothing is sent, because nothing was recorded.
4. **Send after restore.** `cmd/weft` sends only after `Program.Run` returned,
   or, for a panic outside the UI, from `recoverMain` in `main` (the terminal was
   never taken; exit 2). One send, at most `crash.Timeout` (2 seconds), no
   retry. A failure prints one line, `weft: crash report not sent: <reason>`;
   success prints nothing. A recorded panic exits 1 even when `Run` returned
   nil.
5. **Consent gate.** A report is sent only when the build's DSN parses and
   `crash_reports = true`. An empty or bad `main.crashDSN` turns reporting off
   everywhere: no question, no connection. Panics during `weft doctor` and the
   first-run prompt are muted. A panic before the user config was loaded reads
   it then (`userconfig.Load`, read-only); any error means no send.
6. **Exact event fields.** The envelope header holds `event_id`. The event has
   `event_id`, `timestamp`, `platform` (`go`), `level` (`fatal`), `release` (the
   weft version), `exception` and `contexts`. The exception has `type`, the
   scrubbed `value` and the frames (`function`, `module`, `filename`, `lineno`,
   `in_app`). `contexts` has only `os.name`, `device.arch`, `runtime.name` and
   `runtime.version`. Key-set tests pin all of it. The scrub replaces the graph
   path with `<graph>` and then the home folder with `~` in the value, and cuts
   an absolute frame file name to its base name. The value can still quote
   text from a page; the README says so. The server also sees the client IP.
7. **User config.** `crash_reports` is a bool; absent means false and a
   non-bool is a config error naming the file and key. `SaveFirstRun` replaces
   `SaveGraph`: it appends `graph` and, when asked, `crash_reports` in one
   guarded write. `Config.CrashReportsSet` tells first run the key is already
   in the file, so it neither asks nor appends a duplicate key.
8. **Release.** `.goreleaser.yaml` passes
   `-X main.crashDSN={{ envOrDefault "SENTRY_DSN" "" }}`, and the release step
   gets the repository secret `SENTRY_DSN`, which Actions masks. Without the
   secret (a fork, a local snapshot) the build succeeds with reporting off.
   Release builds use `-trimpath`, so frames carry module paths, not runner
   paths.

## Traps

- Never add `tea.WithoutCatchPanics`: the terminal would stay in raw mode and
  nothing would print the stack.
- A new `tea.Sequence` is unguarded (decision 3). A panic in it is not reported.
- The DSN is public in every release binary (`go version -m`, `strings`).
  Anyone can post junk events to the project. This is how Sentry and Bugsink
  keys work, and Bugsink quotas bound it.
- Ctrl+C or end of input at the crash question can leave the folder the user
  just confirmed creating. The config is not written. This amends decision 5 of
  `public-readiness.md`, "every write happens after the last answer".
- An unparsable baked DSN silently disables reporting. Only a real send to the
  endpoint shows that the event is accepted.
- Tests never contact the real host. They use a local `httptest` endpoint and a
  fake DSN, and re-exec the test binary through `WEFT_TEST_CRASH`.
