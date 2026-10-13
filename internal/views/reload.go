package views

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// pollInterval is how often weft stats the graph for outside changes.
const pollInterval = 2 * time.Second

// pollTickMsg is the poll clock. Exactly one tick chain exists per App: the
// first successful index load arms it and every tick or poll result re-arms it.
type pollTickMsg struct{}

// pollDoneMsg is the result of one signature walk. gen and base record the
// index state it was dispatched against; a result that no longer matches the
// App's is discarded on arrival.
type pollDoneMsg struct {
	sig  graph.Signature
	err  error
	gen  int
	base *graph.Index
}

func defaultPollTick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollTickMsg{} })
}

// indexBuilding reports whether a buildIndexCmd result is still outstanding.
func (a *App) indexBuilding() bool { return a.loadedGen != a.indexGen }

// pollCmd stats the graph off the UI goroutine. It is stat-only and
// read-only, and captures everything by value so it never reads App fields.
func (a *App) pollCmd() tea.Cmd {
	path, lay, gen, base := a.graphPath, a.lay, a.indexGen, a.idx
	return func() tea.Msg {
		sig, err := graph.ScanSignature(path, lay)
		return pollDoneMsg{sig: sig, err: err, gen: gen, base: base}
	}
}

// onPollTick dispatches a walk, or just re-arms while there is nothing to
// compare against: no index yet, a sync running (it rewrites the tree and
// rebuilds on its own), or a build already in flight.
func (a *App) onPollTick() tea.Cmd {
	if a.idx == nil || a.syncing || a.indexBuilding() {
		return a.pollTick()
	}
	return a.pollCmd()
}

// onPollDone always re-arms the clock. It starts at most one rebuild, and only
// when the walk is still current (same generation, same index, no sync) and
// the disk differs from the state last acted on. The rebuild shows no hint:
// the refresh is silent and the failure path reports itself once, because
// diskSig has already advanced.
func (a *App) onPollDone(m pollDoneMsg) tea.Cmd {
	if m.err != nil || m.gen != a.indexGen || m.base != a.idx || a.syncing || m.sig == a.diskSig {
		return a.pollTick()
	}
	a.diskSig = m.sig
	return tea.Batch(a.buildIndexCmd(), a.pollTick())
}

// refreshEditor gives the open editor the new index for completion and
// tells it whether its file now differs from what it last read or wrote.
// The buffer is never changed.
func (a *App) refreshEditor() {
	a.editor.SetIndex(a.idx)
	if snap, err := a.readSnapshot(a.editor.path); err == nil {
		a.editor.NoteDisk(snap)
	}
}
