package views

// vimOpenSearch opens the find bar for `/` (dir +1) or `?` (dir -1): empty,
// origin at the cursor, the bar's current match the nearest one in dir.
func (e *EditorView) vimOpenSearch(dir int) {
	b := e.buf
	b.Break()
	e.find = &findBar{origin: b.Cursor(), cur: -1, dir: dir}
	e.completer.dismiss()
	e.findRun(e.find.origin, true)
}

// vimSearchEnter is Enter in a vim bar. An empty field takes the last query.
// With a match the bar closes with the cursor on its start and the query and
// direction recorded; without one the cursor goes back to the origin and the
// notice says so.
func (e *EditorView) vimSearchEnter() {
	f, v, b := e.find, e.vim, e.buf
	q := f.query
	if q == "" {
		q = e.lastQuery
	}
	if q == "" {
		b.MoveTo(f.origin, false)
		e.closeFind()
		e.notice = "no previous search"
		return
	}
	r, wrapped, ok := b.Search(q, f.origin, f.dir < 0)
	f.query = q
	if !ok {
		b.MoveTo(f.origin, false)
		e.closeFind()
		e.notice = "pattern not found: " + q
		return
	}
	b.MoveTo(r.Start, false)
	e.lastQuery = q
	e.closeFind()
	e.goalOK = false
	e.syncReveal()
	e.ensureVisible()
	v.searchDir = f.dir
	if wrapped {
		e.notice = "search wrapped"
	}
}

// vimSearchNext is n and N: count times the last query from the cursor, in the
// direction of the last search or, for N, against it.
func (e *EditorView) vimSearchNext(c vimCmd) {
	b := e.buf
	q := e.lastQuery
	if q == "" {
		e.notice = "no previous search"
		return
	}
	back := e.vim.searchDir < 0
	if c.key == "N" {
		back = !back
	}
	p := b.Cursor()
	wrapped := false
	for range max(c.count, 1) {
		r, w, ok := b.Search(q, p, back)
		if !ok {
			e.notice = "pattern not found: " + q
			return
		}
		p, wrapped = r.Start, wrapped || w
	}
	b.MoveTo(p, false)
	e.goalOK = false
	if wrapped {
		e.notice = "search wrapped"
	}
}
