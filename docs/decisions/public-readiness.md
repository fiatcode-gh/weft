# weft is ready for a first user

A new user runs `weft`, is asked for a notes folder, and lands in the app. The
folder is saved as `graph` in the user config, so later runs need no flag or
environment variable. `--graph` and `$WEFT_GRAPH` still work and win over the
config. The Go module path is now `github.com/fiatcode-gh/weft/v3`, the help
footer shows the project URL, and the README starts with Getting started.

Why: until now a graph path was required on every start, and the module path
(`/v2`) did not match the next release.

Out of scope: a first-run screen inside the TUI, a choice of layout at first
run, writing `<graph>/.weft/config.toml`, welcome or sample pages, changing the
graph from inside weft, `weft init`, Homebrew or install scripts, Windows,
separate user pages under `docs/`, and editing a saved `graph` line (first run
only happens when the key is absent).

## Decisions

1. **Precedence and lazy read.** `--graph` > `$WEFT_GRAPH` > `graph` in the user
   config. `resolveGraph` reads the user config only when the flag and the env
   are both empty, so a bad user config does not affect a run that names its
   graph some other way.
2. **One source, no fallback.** The first non-empty source is used. If its path
   is missing or not a folder, the error names the source (`--graph`,
   `$WEFT_GRAPH` or the config file path) and weft exits 2. Falling back to a
   later source would hide a typo.
3. **One no-graph message.** `noGraphError` is shared by a non-terminal run and
   `weft doctor`, and lists the three ways to set a graph.
4. **Doctor before resolution.** `main` dispatches `doctor` before it resolves
   the graph, so `weft doctor --graph PATH` works without `$WEFT_GRAPH`, and
   `weft --graph PATH doctor` still works. Doctor's own flag wins, then the
   global flag, the env, the config key. Doctor reads the user config only when
   none of the first three names a graph, and then a bad file exits 2. The
   contract said a malformed user config is "still" a doctor error; at the
   parent head doctor did not read the file at all, and `--graph` and
   `$WEFT_GRAPH` behaviour had to stay as it was, so this is the narrowest
   reading. Doctor never prompts.
5. **First run.** Only when no source names a graph and both stdin and stdout are
   terminals (`x/term`; `ModeCharDevice` would count `/dev/null`). It asks
   `Notes folder [~/notes]:`; Enter takes `~/notes`. `~/x` and absolute answers
   are taken as typed; a relative answer is made absolute from the current
   folder; `~user/…` is refused. An existing folder is used; a file is refused.
   A missing folder gets `Create <abs>? [Y/n]` and is made only after a yes;
   `n` asks for the folder again. End of input ends the dialogue with exit 2 and
   nothing saved. Ctrl+C keeps Go's default SIGINT behaviour (the shell shows
   130): every write happens after the last answer, so an interrupt writes
   nothing.
6. **Stored form.** `userconfig.GraphValue` stores `~/rel` when the folder is
   strictly inside the home folder, else the absolute path, so a config copied
   between machines keeps working.
7. **`SaveGraph` appends.** It adds one `graph = "…"` line at the end of the text
   it read, TOML-encoded, and writes through `edit.WriteFileIfUnchanged` against
   that snapshot. Appending at the end is always a top-level key because the
   strict parser rejects tables, so the file cannot end up with the key inside
   a table. If the file changed while weft was asking, the save is refused
   (`edit.ErrChanged`), nothing is overwritten, and weft exits 2.
8. **Writes.** First run writes the folder (after yes) and the user config, and
   nothing else: no `.weft/`, no pages, no journals.
9. **`/v3`.** The module path moves to `/v3` everywhere except released
   CHANGELOG sections and `docs/superpowers/`. The README keeps the `/v2`
   install line for people who need v2.
10. **Help URL.** The help overlay footer shows the version and the project URL.
11. **Speed test reference.** `TestEditorSpeed10000` bounds each operation as a
    ratio to a reference (`perfReference`: build the buffer and scan to the
    anchor) timed in the same run, with separate tables for race and non-race.
    A slow or loaded machine stretches both sides, so the test no longer scales
    by `raceFactor`.

## Traps

- Tests must never inherit `WEFT_GRAPH` or read the real user config. Set a temp
  `HOME` and `XDG_CONFIG_HOME`, and drop `WEFT_GRAPH` from the environment of
  re-exec tests.
- A new user-config key must keep `SaveGraph`'s append valid: top-level keys
  only, no tables.
- The prompt reads through a `bufio.Reader`, which can read ahead. Keys typed
  before the UI starts on a first run may be swallowed. This was accepted: it
  happens once.
