package views

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/search"
	syncpkg "github.com/fiatcode-gh/weft/v2/internal/sync"
)

// indexLoadedMsg carries the result of an asynchronous graph.BuildIndex
// run. gen identifies which buildIndexCmd produced it; the handler drops
// results from any generation but the latest, so a slow walk delivered
// late can't overwrite a newer index.
type indexLoadedMsg struct {
	idx *graph.Index
	err error
	gen int
}

// syncRunner runs a git sync against repoDir. Injected on App so view tests
// stub it instead of shelling out to git.
type syncRunner func(repoDir string) syncpkg.Result

// syncDoneMsg carries the outcome of an async sync.
type syncDoneMsg struct{ res syncpkg.Result }

// statusProbedMsg carries the outcome of a read-only graph sync-state probe.
type statusProbedMsg struct {
	st  syncpkg.WorktreeStatus
	err error
}

// editorExitedMsg is delivered when the child editor process returns. Only
// the E ($EDITOR) handoff produces this; the in-app editor (e) schedules an
// async reindex in the res.Exit branch and never goes through here.
// path is the file we handed to the editor; t0 is the pre-edit mtime
// snapshot (zero if the file did not exist before EnsureFile ran).
// err is non-nil when the editor exited non-zero or failed to launch.
type editorExitedMsg struct {
	path string
	t0   time.Time
	err  error
}

// hintTTL is how long a status-bar hint stays before fading on its own.
const hintTTL = 3 * time.Second

// hintExpireMsg is delivered by the timer started in setHint. The handler
// ignores it when gen doesn't match the current hintGen — i.e. when a newer
// hint or keystroke has superseded the one that scheduled this tick.
type hintExpireMsg struct{ gen int }

type App struct {
	graphPath string

	idx     *graph.Index
	loadErr error
	page    *PageView

	// editor is the full-screen in-app editor, or nil when not editing.
	// When non-nil it owns all keys and the whole screen.
	editor *EditorView

	// active is the overlay layered over the page, or nil when the page has
	// focus. Set when an open-overlay key is pressed; cleared by terminal outcomes.
	active Overlay

	width  int
	height int

	// nowFunc returns "now" for today-journal resolution. Defaults to
	// time.Now; tests inject a fixed clock.
	nowFunc func() time.Time

	// hint is a transient right-side status replacement. Set via setHint
	// (which schedules a hintTTL tick) and cleared either at the top of the
	// next tea.KeyMsg or by a matching hintExpireMsg. hintGen is bumped each
	// time a hint is set so stale ticks ignore themselves.
	hint    string
	hintGen int

	// lastIndexWarnings remembers the most recently seen index-warning set
	// (joined by newlines) so newIndexWarnings can tell a standing warning
	// from a new one.
	lastIndexWarnings string

	// syncFunc runs the git sync; defaults to sync.Run with App's clock.
	// syncing is the single-flight guard: a second `s` mid-sync is a no-op.
	syncFunc syncRunner
	syncing  bool

	// statusProbe reads the graph repo's sync state; injected so view tests
	// stub it instead of shelling out to git. unsynced caches the last
	// successful probe's verdict and drives the status-bar indicator.
	statusProbe func(repoDir string) (syncpkg.WorktreeStatus, error)
	unsynced    bool

	// readSnapshot reads a file's content for the enter-editor and linkify
	// paths; injected so tests can simulate a change between read and write,
	// like statusProbe.
	readSnapshot func(path string) (edit.Snapshot, error)

	// Browser-style page history. hist[histIdx] is the entry currently on
	// screen. histIdx == -1 before the first page is shown.
	hist    []historyEntry
	histIdx int

	// version is the binary version string shown in the help overlay
	// footer. Empty hides the version segment.
	version string

	// indexGen counts buildIndexCmd invocations. Bumped synchronously at
	// call time (not inside the returned closure) so two concurrent
	// reindexes always get distinct generations even if their disk walks
	// finish out of order. The indexLoadedMsg handler drops any result
	// whose gen doesn't match the current indexGen.
	indexGen int
}

type historyEntry struct {
	page   string
	offset int
	cursor int
	// taskOrdinal is the 0-based open-todo deep-link target recorded when the
	// entry was created via navigateToTask (e.g. picking a TODO from the
	// dashboard). -1 means "no deep-link" — Restore ignores it and the page
	// stays at its stored offset.
	taskOrdinal int
}

// New returns an App that has not yet built its index. The index is built
// asynchronously in Init so the first frame can render a "loading" splash
// instead of freezing the terminal while a large graph is walked.
func New(graphPath, version string) *App {
	a := &App{
		graphPath: graphPath,
		histIdx:   -1,
		version:   version,
		nowFunc:   time.Now,
	}
	a.syncFunc = func(repoDir string) syncpkg.Result {
		return syncpkg.Run(repoDir, a.nowFunc())
	}
	a.statusProbe = syncpkg.Status
	a.readSnapshot = edit.ReadSnapshot
	return a
}

func (a *App) todayJournalName() string { return a.nowFunc().Format("2006-01-02") }

// setHint stores s as the active status-bar hint, bumps hintGen, and returns
// a tea.Cmd that delivers a hintExpireMsg after hintTTL. The handler clears
// the hint only when the message's gen still matches hintGen — keystrokes or
// follow-up hints between now and the tick make the message a no-op.
func (a *App) setHint(s string) tea.Cmd {
	a.hint = s
	a.hintGen++
	gen := a.hintGen
	return tea.Tick(hintTTL, func(time.Time) tea.Msg { return hintExpireMsg{gen: gen} })
}

func (a *App) Init() tea.Cmd { return a.buildIndexCmd() }

func (a *App) buildIndexCmd() tea.Cmd {
	// Stamp the generation now, synchronously, not inside the closure
	// below — two overlapping calls must get distinct gens regardless of
	// which of their disk walks finishes first.
	a.indexGen++
	gen := a.indexGen
	path := a.graphPath
	return func() tea.Msg {
		idx, err := graph.BuildIndex(path)
		return indexLoadedMsg{idx: idx, err: err, gen: gen}
	}
}

// statusProbeCmd reads the graph repo's sync state off the UI thread and
// delivers a statusProbedMsg. Run it after writes, reindexes, and syncs —
// the moments git state can change.
func (a *App) statusProbeCmd() tea.Cmd {
	probe := a.statusProbe
	dir := a.graphPath
	return func() tea.Msg {
		st, err := probe(dir)
		return statusProbedMsg{st: st, err: err}
	}
}

// tryInitPage constructs PageView the first time both the index and a real
// terminal size are available. Constructing at the real width avoids the
// "flash from 80 cols to actual width" the previous synchronous path showed.
func (a *App) tryInitPage() {
	if a.page == nil && a.idx != nil && a.loadErr == nil && a.width > 0 {
		name := a.todayJournalName()
		a.page = NewPageView(a.idx, name, a.width, a.height)
		a.hist = []historyEntry{{page: name, offset: 0, cursor: -1, taskOrdinal: -1}}
		a.histIdx = 0
	}
}

// navigate switches the page view to name and records the transition in
// history. The departing page's offset/cursor are captured into the current
// history entry, any forward history is truncated, then a fresh entry for
// the destination is pushed and becomes current.
func (a *App) navigate(name string) {
	a.navigateToTask(name, -1)
}

// canonicalName maps a raw wiki-link target to the indexed page's exact
// (filename-derived) name, so downstream exact-match lookups agree with
// the case-insensitive Resolve the read view uses. Unindexed names pass
// through unchanged — they name pages that don't exist yet.
func (a *App) canonicalName(name string) string {
	if meta, ok := a.idx.Resolve(name); ok {
		return meta.Name
	}
	return name
}

// pushHistory captures the departing page's position into the current
// history entry, truncates any forward history, and pushes a fresh
// entry for name (which becomes current). The invariant every navigation
// relies on: the page being left always has its offset/cursor saved.
func (a *App) pushHistory(name string, taskOrdinal int) {
	if a.histIdx >= 0 && a.histIdx < len(a.hist) {
		a.hist[a.histIdx].offset = a.page.Offset()
		a.hist[a.histIdx].cursor = a.page.Cursor()
	}
	a.hist = append(a.hist[:a.histIdx+1], historyEntry{
		page:        name,
		offset:      0,
		cursor:      -1,
		taskOrdinal: taskOrdinal,
	})
	a.histIdx = len(a.hist) - 1
}

// navigateToTask is navigate plus an open-todo deep-link target. When
// ordinal >= 0 the new history entry stores it and the page is scrolled to
// that todo on first display. Restore ignores it — it stays a one-shot jump.
func (a *App) navigateToTask(name string, ordinal int) {
	name = a.canonicalName(name)
	a.pushHistory(name, ordinal)
	a.page.SetPage(name)
	if ordinal >= 0 {
		a.page.ScrollToTask(ordinal)
	}
}

// navigateFocusingLink is navigate plus positioning the destination page's link
// cursor on the first link back to backTarget — so jumping from a backlink lands
// on (and highlights) the referencing link. The resulting cursor is stored in
// the new history entry so it survives [ / ] history navigation.
func (a *App) navigateFocusingLink(name, backTarget string) {
	name = a.canonicalName(name)
	a.pushHistory(name, -1)
	a.page.SetPage(name)
	a.page.FocusLinkTo(backTarget)
	a.hist[a.histIdx].cursor = a.page.Cursor()
}

// navigateHighlighting navigates to name and highlights occurrences of term
// (the page navigated from) on the destination, scrolling to the first — for
// unlinked references, which have no link to focus a cursor on. One-shot: the
// new history entry stores no emphasis, so [ / ] restore lands without it.
func (a *App) navigateHighlighting(name, term string) {
	name = a.canonicalName(name)
	a.pushHistory(name, -1)
	a.page.SetPageEmphasizing(name, term)
}

// historyBack walks one step backward in the history stack, restoring the
// stored offset/cursor for that entry. The departing page's current
// offset/cursor are saved into the current entry so a subsequent forward
// step lands where the user left off. No-op at the start of history.
func (a *App) historyBack() {
	if a.histIdx <= 0 {
		return
	}
	a.hist[a.histIdx].offset = a.page.Offset()
	a.hist[a.histIdx].cursor = a.page.Cursor()
	a.histIdx--
	target := a.hist[a.histIdx]
	a.page.SetPage(target.page)
	a.page.Restore(target.offset, target.cursor)
}

// historyForward walks one step forward in the history stack. Mirrors
// historyBack. No-op at the tail of history.
func (a *App) historyForward() {
	if a.histIdx < 0 || a.histIdx >= len(a.hist)-1 {
		return
	}
	a.hist[a.histIdx].offset = a.page.Offset()
	a.hist[a.histIdx].cursor = a.page.Cursor()
	a.histIdx++
	target := a.hist[a.histIdx]
	a.page.SetPage(target.page)
	a.page.Restore(target.offset, target.cursor)
}

// createJournalAndReindex creates the on-disk file for journal page `name`
// via the internal/edit hook, rebuilds the index synchronously, and rebinds
// the current PageView to it. Shared by the `.` and `e` handlers when they
// land on a today's-journal page whose file doesn't exist yet. Returns an
// error whose message is ready for setHint.
func (a *App) createJournalAndReindex(name string) error {
	journalPath := filepath.Join(a.graphPath, "journals", graph.FilenameFromPageName(name))
	if _, err := edit.EnsureFile(journalPath); err != nil {
		return fmt.Errorf("cannot create journal: %w", err)
	}
	if err := a.reindex(); err != nil {
		return fmt.Errorf("reindex failed: %w", err)
	}
	return nil
}

// reindex rebuilds the in-memory index synchronously and rebinds the current
// PageView so it reflects new links/todos. Shared by createJournalAndReindex
// and the linkify path, both of which mutate the graph while a view is open and
// need the refresh before returning (no indexLoadedMsg round-trip).
func (a *App) reindex() error {
	idx, err := graph.BuildIndex(a.graphPath)
	if err != nil {
		return err
	}
	a.idx = idx
	if a.page != nil {
		// Same-page rebuild: keep the reader's place, exactly like the
		// async indexLoadedMsg path. Restore clamps if the page shrank.
		off, cur := a.page.Offset(), a.page.Cursor()
		a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
		a.page.Restore(off, cur)
	}
	if fresh := a.newIndexWarnings(idx.Warnings); fresh && len(idx.Warnings) > 0 {
		// Best-effort: nothing to degrade to here — an overlay usually
		// covers the status bar when this path runs (linkify, journal
		// create), so persist the details and move on.
		_ = a.logIndexWarnings(idx.Warnings)
	}
	return nil
}

// editCurrent snapshots the current page's file mtime, ensures the
// file exists (creating an empty one for today's journal if needed),
// resolves the user's editor, and returns a tea.ExecProcess cmd that
// hands the file off. The child editor's exit yields an
// editorExitedMsg, which the Update case below mtime-gates against a
// reindex.
func (a *App) editCurrent() tea.Cmd {
	page := a.page.Page()
	meta, ok := a.idx.Resolve(page)
	if !ok {
		// Page is not in the index. The realistic case is a cold
		// start landing on today's journal whose file doesn't exist
		// yet — the user wants to start journaling and `e` is the
		// natural next step. Bootstrap the journal file on demand
		// rather than requiring a separate `.` press, and reindex
		// so the rest of the flow has a populated idx. Non-journal
		// pages that aren't in the index still fall through to the
		// "page not in index" hint — those are unreachable in
		// normal navigation (picker / wiki-links only point to
		// indexed pages) and a missing journal is the only one
		// worth handling automatically.
		if !graph.IsJournalPageName(page) {
			return a.setHint("page not in index: " + page)
		}
		if err := a.createJournalAndReindex(page); err != nil {
			return a.setHint(err.Error())
		}
		newMeta, ok := a.idx.Resolve(page)
		if !ok {
			return a.setHint("reindex dropped page: " + page)
		}
		meta = newMeta
	}
	path := meta.Path

	t0, statErr := edit.SnapshotMtime(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return a.setHint("cannot stat: " + statErr.Error())
	}

	if t0.IsZero() {
		// Defensive: meta was in the index (so the file existed at
		// BuildIndex time) but is gone now. Recreate the empty stub
		// so the editor has something to open. Reachable only on a
		// narrow race with an external tool deleting the file
		// between boot and `e`.
		if _, err := edit.EnsureFile(path); err != nil {
			return a.setHint("cannot create journal: " + err.Error())
		}
	}

	resolved, err := edit.Resolve(
		edit.Env{Visual: os.Getenv("VISUAL"), Editor: os.Getenv("EDITOR")},
		exec.LookPath,
	)
	if err != nil {
		return a.setHint("cannot resolve editor: " + err.Error())
	}

	return tea.ExecProcess(exec.Command(resolved.Binary, path), func(cmdErr error) tea.Msg {
		return editorExitedMsg{path: path, t0: t0, err: cmdErr}
	})
}

// enterEditor opens the in-app editor on the current page. The target file
// path is computed but NOT created — a brand-new page is written to disk
// only on save (Ctrl+S). Journals route to journals/, every other name to
// pages/ (flat, "/" mangled to "___" by FilenameFromPageName). The cursor
// opens on the read view's anchor line.
func (a *App) enterEditor() tea.Cmd {
	name := a.page.Page()
	var path string
	if meta, ok := a.idx.Resolve(name); ok {
		path = meta.Path
	} else {
		sub := "pages"
		if graph.IsJournalPageName(name) {
			sub = "journals"
		}
		path = filepath.Join(a.graphPath, sub, graph.FilenameFromPageName(name))
	}
	snap, err := a.readSnapshot(path)
	if err != nil {
		return a.setHint("cannot read: " + err.Error())
	}
	content, isNew := snap.Content, !snap.Exists
	anchor := 0
	if line, ok := a.page.AnchorSourceLine(); ok {
		anchor = line + leadingTrimmedLines(content)
	}
	e := NewEditorView(a.idx, name, path, content, isNew, a.width, a.height, anchor)
	if e.LoadDiverged() {
		return a.setHint("in-app editor would alter this file (CRLF, tabs, or >10000 lines) — press E to edit externally")
	}
	a.editor = e
	return a.editor.Focus()
}

// unlinkedRefs finds bare-text mentions of `name` elsewhere in the graph that
// aren't already links. Best-effort: a ripgrep failure yields no unlinked refs
// rather than breaking the backlinks panel — and an error hint would be
// invisible behind the overlay anyway (cf. the slice-1 hidden-hint lesson).
func (a *App) unlinkedRefs(name string) []graph.UnlinkedRef {
	hits, err := search.Mentions(a.graphPath, name)
	if err != nil {
		return nil
	}
	targetPath := ""
	if meta, ok := a.idx.ByName[name]; ok {
		targetPath = meta.Path
	}
	return graph.FilterUnlinked(hits, targetPath, func(p string) (string, error) {
		b, err := os.ReadFile(p)
		return string(b), err
	})
}

// linkify wraps the unlinked reference's mention as a [[link]] in its source
// file, then reindexes and rebuilds the backlinks overlay so the reference
// moves from Unlinked to Linked. The file is re-read and re-matched here (not
// trusting the offset captured at panel-open) and written back only if the
// file is still byte-identical to what was read (edit.WriteFileIfUnchanged),
// so a file that changed since detection, or mid-linkify, fails safely.
// Failures render inside the panel via SetError; a
// status-bar hint would be invisible behind the overlay. Returns nil — the
// reindex is synchronous, so there is no command to run.
func (a *App) linkify(ref *graph.UnlinkedRef, target string) tea.Cmd {
	bl, _ := a.active.(*Backlinks)
	fail := func(msg string) tea.Cmd {
		if bl != nil {
			bl.SetError("linkify failed: " + msg)
		}
		return nil
	}
	snap, err := a.readSnapshot(ref.FilePath)
	if err != nil {
		return fail("cannot read " + ref.PageName + ": " + err.Error())
	}
	if !snap.Exists {
		return fail("cannot read " + ref.PageName + ": file not found")
	}
	newBody, _, err := graph.LinkifyMention(snap.Content, ref.Line, target)
	if err != nil {
		return fail("mention no longer found in " + ref.PageName)
	}
	if err := edit.WriteFileIfUnchanged(ref.FilePath, snap, []byte(newBody)); err != nil {
		if errors.Is(err, edit.ErrChanged) {
			return fail(ref.PageName + " changed on disk — try again")
		}
		return fail("write failed: " + err.Error())
	}
	if err := a.reindex(); err != nil {
		// The write already landed, so surface the failure in the panel (a
		// status-bar hint is invisible behind the overlay) rather than rebuild
		// from the unchanged index — the stale panel keeps showing the now
		// already-linked mention as unlinked.
		return fail("reindex failed: " + err.Error())
	}
	a.active = NewBacklinks(a.idx, target, a.unlinkedRefs(target), a.width, a.height)
	// The linkify write changed the working tree — refresh the indicator.
	return a.statusProbeCmd()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case indexLoadedMsg:
		if m.gen != a.indexGen {
			return a, nil // a newer reindex is in flight; drop the stale walk
		}
		if m.err != nil {
			if a.page != nil {
				// Mid-session reindex failed — keep the old index and
				// tell the user via a hint. The working page stays on
				// screen; the boot path (a.page == nil) still surfaces
				// the splash so the user can retry.
				return a, a.setHint("reindex failed: " + m.err.Error())
			}
			a.loadErr = m.err
			return a, nil
		}
		a.idx = m.idx
		a.loadErr = nil
		if a.page != nil {
			// Refresh path (R, sync-pull, editor save-exit): rebuild PageView
			// for the same page so it picks up new links/todos — but keep the
			// user's place. Restore clamps if the page shrank.
			off, cur := a.page.Offset(), a.page.Cursor()
			a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
			a.page.Restore(off, cur)
		} else {
			a.tryInitPage()
		}
		// Every reindex (boot, R, post-edit, post-$EDITOR) is a natural
		// moment to refresh the sync indicator.
		return a, tea.Batch(a.statusProbeCmd(), a.indexWarningsHint(m.idx.Warnings))
	case syncDoneMsg:
		a.syncing = false
		if m.res.Err != nil {
			hint := "✗ " + m.res.Stage + " failed — see " + DebugLogPath()
			if err := a.logSyncFailure(m.res); err != nil {
				hint = "✗ " + m.res.Stage + " failed: " + m.res.Err.Error()
			}
			// A failed sync may still have changed state (e.g. committed
			// then failed to push), so re-probe.
			return a, tea.Batch(a.setHint(hint), a.statusProbeCmd())
		}
		cmds := []tea.Cmd{a.setHint("✓ synced"), a.statusProbeCmd()}
		if m.res.Pulled {
			cmds = append(cmds, a.buildIndexCmd())
		}
		return a, tea.Batch(cmds...)

	case statusProbedMsg:
		// A failed probe says nothing about sync state; keep the last
		// known value rather than clearing the ● to a false all-clear.
		if m.err == nil {
			a.unsynced = m.st.Unsynced()
		}
		return a, nil
	case searchDoneMsg:
		if s, ok := a.active.(*SearchView); ok && s == m.view {
			s.Apply(m)
		}
		return a, nil
	case editorExitedMsg:
		// A non-zero exit (vim :cq, a crashed wrapper) does not mean nothing
		// was written — surface it, but still run the mtime-gated refresh
		// below so a real save isn't left rendering stale.
		var exitHint tea.Cmd
		if m.err != nil {
			exitHint = a.setHint("editor exited: " + m.err.Error())
		}
		info, err := os.Stat(m.path)
		if err != nil {
			if os.IsNotExist(err) {
				return a, tea.Batch(exitHint, a.statusProbeCmd())
			}
			return a, tea.Batch(exitHint, a.setHint("cannot stat: "+err.Error()))
		}
		if info.ModTime().Equal(m.t0) {
			// Unchanged by the editor — but editCurrent may have just
			// created a journal stub, so still refresh the indicator.
			return a, tea.Batch(exitHint, a.statusProbeCmd())
		}
		// Changed: the reindex's indexLoadedMsg refreshes the indicator.
		return a, tea.Batch(exitHint, a.buildIndexCmd())
	case hintExpireMsg:
		if m.gen == a.hintGen {
			a.hint = ""
		}
		return a, nil
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		if a.page != nil {
			a.page.SetSize(m.Width, m.Height)
		} else {
			a.tryInitPage()
		}
		// Propagate the new size to any open overlay so its scroll-window
		// budget and inner-width tracking stay correct on resize.
		if a.active != nil {
			a.active.SetSize(m.Width, m.Height)
		}
		if a.editor != nil {
			a.editor.SetSize(m.Width, m.Height)
		}
		return a, nil
	case tea.KeyMsg:
		key := m.String()
		a.hint = ""
		// While loading or in an error state, only quit + retry are honoured.
		if a.page == nil {
			switch key {
			case keyQ, "ctrl+c":
				return a, tea.Quit
			case "R":
				if a.loadErr != nil {
					a.loadErr = nil
					return a, a.buildIndexCmd()
				}
			}
			return a, nil
		}
		if a.editor != nil {
			res, taCmd := a.editor.Update(m)
			cmds := []tea.Cmd{taCmd}
			if res.Save {
				content := a.editor.Content()
				if err := edit.WriteFile(a.editor.path, []byte(content)); err != nil {
					a.editor.SetError(err.Error())
					return a, taCmd
				}
				a.editor.MarkSaved(content)
			}
			if res.Exit {
				saved := a.editor.saved
				a.editor = nil
				if saved {
					// Reindex picks up the saved file; its indexLoadedMsg
					// then refreshes the indicator.
					cmds = append(cmds, a.buildIndexCmd())
				} else {
					off, cur := a.page.Offset(), a.page.Cursor()
					a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
					a.page.Restore(off, cur)
				}
			} else if res.Save {
				// A save without exit doesn't reindex, so probe directly.
				cmds = append(cmds, a.statusProbeCmd())
			}
			return a, tea.Batch(cmds...)
		}
		// An open overlay swallows all keys until it accepts or cancels —
		// except ctrl+c, which must always quit (Bubble Tea convention; it
		// works in every other mode).
		if a.active != nil {
			if key == "ctrl+c" {
				return a, tea.Quit
			}
			res := a.active.Update(key)
			switch res.kind {
			case overlayResultCancel:
				a.active = nil
			case overlayResultCommand:
				return a, res.cmd
			case overlayResultLinkify:
				if cmd, blocked := a.blockIfSyncing(); blocked {
					return a, cmd
				}
				return a, a.linkify(res.ref, res.target)
			case overlayResultCreate:
				if cmd, blocked := a.blockIfSyncing(); blocked {
					return a, cmd
				}
				a.navigate(res.page)
				a.active = nil
				return a, a.enterEditor()
			case overlayResultOpen:
				if res.page != "" {
					a.navigate(res.page)
				}
				a.active = nil
			case overlayResultOpenTask:
				a.navigateToTask(res.page, res.taskOrdinal)
				a.active = nil
			case overlayResultFocusLink:
				a.navigateFocusingLink(res.page, res.target)
				a.active = nil
			case overlayResultHighlight:
				a.navigateHighlighting(res.page, res.target)
				a.active = nil
			}
			return a, nil
		}
		switch key {
		case keyQ, "ctrl+c":
			return a, tea.Quit
		case "ctrl+p":
			a.active = NewPicker(a.idx, a.width, a.height)
		case "/":
			a.active = NewSearchView(a.idx, a.width, a.height)
		case "b":
			name := a.page.Page()
			a.active = NewBacklinks(a.idx, name, a.unlinkedRefs(name), a.width, a.height)
		case "T":
			a.active = NewTodos(a.idx, a.width, a.height)
		case "?":
			a.active = NewHelp(a.version, a.width, a.height)
		case "[":
			a.historyBack()
		case "]":
			a.historyForward()
		case ".":
			if cmd, blocked := a.blockIfSyncing(); blocked {
				return a, cmd
			}
			today := a.todayJournalName()
			var probe tea.Cmd
			if _, ok := a.idx.ByName[today]; !ok {
				if err := a.createJournalAndReindex(today); err != nil {
					return a, a.setHint(err.Error())
				}
				// Creating the journal wrote a new file — refresh the indicator.
				probe = a.statusProbeCmd()
			}
			if a.page.Page() != today {
				a.navigate(today)
			}
			return a, probe
		case "<":
			page := a.page.Page()
			if name, ok := a.idx.JournalNeighbor(page, -1); ok {
				a.navigate(name)
			} else if graph.IsJournalPageName(page) {
				return a, a.setHint("no earlier journal")
			}
		case ">":
			page := a.page.Page()
			if name, ok := a.idx.JournalNeighbor(page, +1); ok {
				a.navigate(name)
			} else if graph.IsJournalPageName(page) {
				return a, a.setHint("no later journal")
			}
		case "g":
			a.page.GotoTop()
		case "G":
			a.page.GotoBottom()
		case "S":
			if a.syncing {
				return a, a.setHint("⟳ already syncing")
			}
			a.syncing = true
			run := a.syncFunc
			dir := a.graphPath
			return a, tea.Batch(
				a.setHint("⟳ syncing…"),
				func() tea.Msg { return syncDoneMsg{res: run(dir)} },
			)
		case "R":
			// Async reindex — the response lands as indexLoadedMsg and
			// rebuilds PageView for the current page. The hint makes a
			// swallowed or racing R visible instead of looking like a
			// silent no-op. Errors surface in loadErr which the splash
			// overlay renders.
			return a, tea.Batch(a.setHint("⟳ reindexing…"), a.buildIndexCmd())
		case keyE:
			if cmd, blocked := a.blockIfSyncing(); blocked {
				return a, cmd
			}
			return a, a.enterEditor()
		case keyShiftE:
			if cmd, blocked := a.blockIfSyncing(); blocked {
				return a, cmd
			}
			return a, a.editCurrent()
		case "n":
			a.page.CycleLink(+1)
		case "N":
			a.page.CycleLink(-1)
		case keyEnter:
			if t := a.page.FollowCursor(); t != "" {
				a.navigate(t)
			}
		case keyJ, keyDown:
			a.page.LineDown()
		case keyK, keyUp:
			a.page.LineUp()
		case "ctrl+d":
			a.page.HalfPageDown()
		case "ctrl+u":
			a.page.HalfPageUp()
		}
	}
	return a, nil
}

func (a *App) View() string {
	if a.loadErr != nil {
		return styleTitle.Render(fmt.Sprintf("weft — failed to index %s", a.graphPath)) +
			"\n\n" + a.loadErr.Error() +
			"\n\n" + styleFaint.Render("R to retry · q to quit")
	}
	if a.page == nil {
		return styleTitle.Render("weft") +
			"\n\n" + styleFaint.Render(fmt.Sprintf("Loading %s ...", a.graphPath)) +
			"\n\n" + styleFaint.Render("q to quit")
	}
	if a.editor != nil {
		return a.editor.View()
	}
	if a.active != nil {
		return a.centerOverlay(a.active.View())
	}
	return a.page.View() + "\n" + a.statusBar()
}

// centerOverlay places content in the middle of the terminal. Falls back to
// the raw content when the terminal size hasn't arrived yet.
func (a *App) centerOverlay(content string) string {
	if a.width <= 0 || a.height <= 0 {
		return content
	}
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}

// statusBar renders a one-line bottom bar: page title + meta on the left,
// help hint on the right, separated by enough whitespace to span the
// terminal width. A faint horizontal rule sits above it so the bar reads
// as a distinct strip even on terminals without colour.
//
// When the page title is too long to fit alongside the right segment, the
// left segment is truncated with an ellipsis. Without this clamp the bar
// overflowed the terminal width and wrapped onto a second line.
func (a *App) statusBar() string {
	left := a.page.StatusLine()
	var right string
	if a.hint != "" {
		right = styleFaint.Render(a.hint)
	} else {
		rightText := "? help"
		if ind := a.page.ScrollIndicator(); ind != "" {
			rightText = ind + "  " + rightText
		}
		right = styleFaint.Render(rightText)
		// An unsynced graph shows a leading attention dot — hidden while a
		// transient hint occupies the right side.
		if a.unsynced {
			right = styleSyncDirty.Render("●") + " " + right
		}
	}
	width := a.width
	if width <= 0 {
		width = lipgloss.Width(left) + 2 + lipgloss.Width(right)
	}
	rightW := lipgloss.Width(right)
	// Reserve at least one space between left and right.
	leftBudget := width - rightW - 1
	if leftBudget < 1 {
		leftBudget = 1
	}
	left = clamp(left, leftBudget)
	rule := styleFaint.Render(strings.Repeat("─", width))
	gap := width - lipgloss.Width(left) - rightW
	if gap < 1 {
		gap = 1
	}
	return rule + "\n" + left + strings.Repeat(" ", gap) + right
}

// blockIfSyncing gates the graph-mutating entry points while the async git
// sync goroutine is rewriting the worktree: a save landing mid
// `pull --rebase` is overwritten by the rebase checkout and silently lost,
// and `add -A` can stage editor temp files. Reports blocked=true while a
// sync runs. Feedback goes into the open overlay when it has an in-panel
// channel — the status bar is hidden behind an overlay — falling back to a
// status-bar hint; cmd is non-nil only for that fallback.
func (a *App) blockIfSyncing() (cmd tea.Cmd, blocked bool) {
	if !a.syncing {
		return nil, false
	}
	const msg = "sync in progress — retry when it finishes"
	switch o := a.active.(type) {
	case *Backlinks:
		o.SetError(msg)
		return nil, true
	case *Picker:
		o.SetError(msg)
		return nil, true
	default:
		return a.setHint("⟳ " + msg), true
	}
}

// logSyncFailure appends a failing sync's stage, error, and captured git
// output to DebugLogPath(), the same cache-dir path WEFT_DEBUG mirrors to —
// written regardless of the flag so the hint's "see <path>" pointer is
// valid. res.Err matters most: a timed-out push often has no output,
// and the error is the only way to tell timeout from rejection. Returns
// the write error so the caller can degrade the hint instead of pointing
// at a log that was never written.
func (a *App) logSyncFailure(res syncpkg.Result) error {
	f, err := os.OpenFile(DebugLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "sync %s failed at %s: %v\n%s\n",
		res.Stage, a.nowFunc().Format(time.RFC3339), res.Err, res.Output)
	return err
}

// logIndexWarnings appends index-build warnings to DebugLogPath() — written
// regardless of WEFT_DEBUG so the hint's "see <path>" pointer is valid
// (mirrors logSyncFailure). Returns the write error so the caller can
// degrade the hint instead of pointing at a log that was never written.
func (a *App) logIndexWarnings(warnings []string) error {
	f, err := os.OpenFile(DebugLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, w := range warnings {
		if _, err := fmt.Fprintf(f, "index warning at %s: %s\n", a.nowFunc().Format(time.RFC3339), w); err != nil {
			return err
		}
	}
	return nil
}

// newIndexWarnings reports whether the warning set differs from the last
// set it saw, remembering the new one either way (so a set that clears and
// later returns re-notifies). Standing graph warnings — a permanent
// subdirectory, a long-lived case collision — would otherwise re-log and
// re-hint on every reindex: boot, R, save-exit, sync-pull.
func (a *App) newIndexWarnings(warnings []string) bool {
	key := strings.Join(warnings, "\n")
	if key == a.lastIndexWarnings {
		return false
	}
	a.lastIndexWarnings = key
	return true
}

// indexWarningsHint logs warnings and returns the status-bar hint command,
// or nil when there are none, or when this is the same warning set already
// surfaced by a prior reindex (see newIndexWarnings).
func (a *App) indexWarningsHint(warnings []string) tea.Cmd {
	if !a.newIndexWarnings(warnings) {
		return nil
	}
	n := len(warnings)
	if n == 0 {
		return nil
	}
	noun := "warnings"
	if n == 1 {
		noun = "warning"
	}
	hint := fmt.Sprintf("indexed with %d %s — see %s", n, noun, DebugLogPath())
	if err := a.logIndexWarnings(warnings); err != nil {
		hint = fmt.Sprintf("indexed with %d %s: %s", n, noun, warnings[0])
	}
	return a.setHint(hint)
}
