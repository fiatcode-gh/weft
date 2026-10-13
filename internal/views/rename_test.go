package views

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/doctor"
	"github.com/fiatcode-gh/weft/v2/internal/edit"
	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// graphFiles reads every file under root (graph-relative, slash paths).
func graphFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func readRel(t *testing.T, a *App, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.graphPath, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeRel(t *testing.T, a *App, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(a.graphPath, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func existsRel(a *App, rel string) bool {
	_, err := os.Stat(filepath.Join(a.graphPath, filepath.FromSlash(rel)))
	return err == nil
}

// pageMeta navigates to key and returns the indexed page.
func pageMeta(t *testing.T, a *App, key string) *graph.PageMeta {
	t.Helper()
	a.navigate(key)
	meta, ok := a.idx.Resolve(a.page.Page())
	if !ok {
		t.Fatalf("%q does not resolve", key)
	}
	return meta
}

// renameApp boots a fixture, opens key, plans its rename to name and applies it.
func renameApp(t *testing.T, cfg bootConfig, key, name string) (*App, *renamePlan, renameResult) {
	t.Helper()
	a := bootApp(t, cfg)
	p, err := a.planRename(pageMeta(t, a, key), name)
	if err != nil {
		t.Fatalf("planRename(%q, %q): %v", key, name, err)
	}
	res, err := a.applyRename(p)
	if err != nil {
		t.Fatalf("applyRename: %v", err)
	}
	if res.stop != nil || res.reindexErr != nil {
		t.Fatalf("applyRename stopped: %+v", res)
	}
	return a, p, res
}

var (
	fixtureCfg = bootConfig{}
	vaultCfg   = bootConfig{fixture: "obsidian-vault"}
)

func fileRels(p *renamePlan) []string {
	var out []string
	for _, f := range p.files {
		out = append(out, f.rel)
	}
	return out
}

func TestPlanRenameListsFiles(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	before := graphFiles(t, a.graphPath)
	hub := pageMeta(t, a, "Hub")
	p, err := a.planRename(hub, "Center")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pages/Hub.md", "pages/kb___notes.md", "journals/2026_05_25.md"}
	if got := fileRels(p); !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	for _, f := range p.files {
		if f.links != 1 {
			t.Errorf("%s links = %d, want 1", f.rel, f.links)
		}
	}
	if p.links != 3 {
		t.Errorf("links = %d, want 3", p.links)
	}
	if want := strings.Replace(before["pages/Hub.md"], "[[Hub]]", "[[Center]]", 1); p.pageSeen.Content != want || !p.pageSeen.Exists {
		t.Errorf("pageSeen = %q, want Hub's rewritten content", p.pageSeen.Content)
	}
	if p.pageRead.Content != before["pages/Hub.md"] {
		t.Errorf("pageRead is not Hub's content on disk")
	}
	if p.to.Key != "Center" || p.page.Path != hub.Path {
		t.Errorf("plan target = %+v page %q", p.to, p.page.Path)
	}
	if after := graphFiles(t, a.graphPath); !reflect.DeepEqual(before, after) {
		t.Error("planRename wrote to disk")
	}
}

func TestPlanRenameRefusals(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	if _, err := a.planRename(pageMeta(t, a, "Hub"), "Beta"); err == nil || !strings.Contains(err.Error(), "already a page") {
		t.Errorf("clash error = %v", err)
	}
	hub := pageMeta(t, a, "Hub")
	if err := os.Remove(hub.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := a.planRename(hub, "Center"); err == nil || err.Error() != "pages/Hub.md no longer exists" {
		t.Errorf("missing page error = %v", err)
	}
}

func TestRenameFixtureForms(t *testing.T) {
	t.Run("Kitchen", func(t *testing.T) {
		a, _, _ := renameApp(t, fixtureCfg, "Kitchen", "Big Kitchen")
		got := readRel(t, a, "pages/Corpus.md")
		for _, want := range []string{"#[[Big Kitchen]] and (#[[Big Kitchen]]) and #s", "`#incode`"} {
			if !strings.Contains(got, want) {
				t.Errorf("Corpus lacks %q:\n%s", want, got)
			}
		}
		if !existsRel(a, "pages/Big Kitchen.md") || existsRel(a, "pages/Kitchen.md") {
			t.Error("page file not moved")
		}
	})
	t.Run("Book Club", func(t *testing.T) {
		a, _, _ := renameApp(t, fixtureCfg, "Book Club", "Readers")
		got := readRel(t, a, "pages/Corpus.md")
		if want := "#[[Readers]] and #[[Readers|the club]]"; !strings.Contains(got, want) {
			t.Errorf("Corpus lacks %q:\n%s", want, got)
		}
	})
	t.Run("Beta", func(t *testing.T) {
		a, _, _ := renameApp(t, fixtureCfg, "Beta", "Gamma")
		if got := readRel(t, a, "pages/Alpha.md"); !strings.Contains(got, "[[Gamma|the second page]]") || !strings.Contains(got, "[[Gamma]] and the namespaced") {
			t.Errorf("Alpha:\n%s", got)
		}
		if got := readRel(t, a, "journals/2026_03_15.md"); !strings.Contains(got, "Mentions [[Gamma]] in a fenced") || !strings.Contains(got, "not a link to [[NotALink]]") {
			t.Errorf("fenced journal changed:\n%s", got)
		}
	})
	t.Run("Alpha", func(t *testing.T) {
		before := graphFiles(t, cloneFixtureGraph(t))
		a, _, _ := renameApp(t, fixtureCfg, "Alpha", "Omega")
		if got := readRel(t, a, "pages/Workbench.md"); got != before["pages/Workbench.md"] {
			t.Errorf("Workbench changed:\n%s", got)
		}
		if got := readRel(t, a, "pages/Corpus.md"); got != before["pages/Corpus.md"] {
			t.Errorf("Corpus changed:\n%s", got)
		}
		if got := readRel(t, a, "journals/2026_05_15.md"); !strings.Contains(got, "[[Omega]]") {
			t.Errorf("journal:\n%s", got)
		}
	})
	t.Run("vault Alpha", func(t *testing.T) {
		a, _, _ := renameApp(t, vaultCfg, "Alpha", "Launch")
		if got := readRel(t, a, "Daily/2026-05-24.md"); got != "- worked on [[Launch#Plan]]\n" {
			t.Errorf("Daily = %q", got)
		}
		if !existsRel(a, "Projects/Launch.md") || existsRel(a, "Projects/Alpha.md") {
			t.Error("page file not moved")
		}
		if got := readRel(t, a, "Daily/Ideas.md"); got != "- an idea about Alpha\n" {
			t.Errorf("Ideas = %q", got)
		}
	})
	t.Run("Hub", func(t *testing.T) {
		a, _, _ := renameApp(t, fixtureCfg, "Hub", "Center")
		got := readRel(t, a, "pages/Center.md")
		if !strings.Contains(got, "A self-mention of [[Center]]") || !strings.Contains(got, "title:: Hub") {
			t.Errorf("Center:\n%s", got)
		}
	})
}

func TestApplyRenameCaseOnly(t *testing.T) {
	a, _, _ := renameApp(t, fixtureCfg, "Alpha", "alpha")
	entries, err := os.ReadDir(filepath.Join(a.graphPath, "pages"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Contains(names, "alpha.md") || slices.Contains(names, "Alpha.md") {
		t.Errorf("pages = %v, want alpha.md and not Alpha.md", names)
	}
	if got := readRel(t, a, "journals/2026_05_15.md"); !strings.Contains(got, "[[alpha]]") {
		t.Errorf("journal:\n%s", got)
	}
	if a.page.Page() != "alpha" {
		t.Errorf("page = %q", a.page.Page())
	}
}

func TestApplyRenameTripleLowbarNamespace(t *testing.T) {
	a, _, _ := renameApp(t, fixtureCfg, "kb/notes", "kb/Memo")
	if !existsRel(a, "pages/kb___Memo.md") || existsRel(a, "pages/kb___notes.md") {
		t.Error("page file not moved to kb___Memo.md")
	}
	if got := readRel(t, a, "journals/2026_05_01.md"); !strings.Contains(got, "[[kb/Memo]]") {
		t.Errorf("journal:\n%s", got)
	}
	if got := readRel(t, a, "pages/Corpus.md"); !strings.Contains(got, "#kb/Memo") {
		t.Errorf("Corpus:\n%s", got)
	}
}

func TestApplyRenameFoldersSubfolder(t *testing.T) {
	a, _, _ := renameApp(t, vaultCfg, "a/Note", "kb/Notes")
	if !existsRel(a, "a/kb/Notes.md") || existsRel(a, "a/Note.md") {
		t.Error("page not moved to a/kb/Notes.md")
	}
	if got := readRel(t, a, "Home.md"); !strings.HasPrefix(got, "- [[Alpha]] [[Notes]] [[B/NOTE]] [[Notes]] ") {
		t.Errorf("Home = %q", got)
	}
	if m, ok := a.idx.Resolve("Note"); !ok || m.Rel != "b/Note.md" {
		t.Errorf("Resolve(Note) = %v %v, want b/Note.md", m, ok)
	}
}

func TestApplyRenameVaultClashLoser(t *testing.T) {
	a, _, _ := renameApp(t, vaultCfg, "b/Note", "Memo")
	if !existsRel(a, "b/Memo.md") || existsRel(a, "b/Note.md") {
		t.Error("page not moved to b/Memo.md")
	}
	if got := readRel(t, a, "Home.md"); !strings.HasPrefix(got, "- [[Alpha]] [[a/Note]] [[Memo]] [[Note]] ") {
		t.Errorf("Home = %q", got)
	}
}

func TestApplyRenameStopsAtChangedFile(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	unresolvedBefore := unresolvedSet(t, a)
	p, err := a.planRename(pageMeta(t, a, "Alpha"), "Gamma")
	if err != nil {
		t.Fatal(err)
	}
	hub := readRel(t, a, "pages/Hub.md") + "- added elsewhere\n"
	writeRel(t, a, "pages/Hub.md", hub)

	res, err := a.applyRename(p)
	if err != nil || res.stop == nil {
		t.Fatalf("res = %+v err = %v, want a stop", res, err)
	}
	if want := "pages/Hub.md changed on disk — nothing overwritten"; res.stop.reason != want {
		t.Errorf("reason = %q, want %q", res.stop.reason, want)
	}
	if want := []string{"pages/Beta.md"}; !slices.Equal(res.stop.done, want) {
		t.Errorf("done = %v, want %v", res.stop.done, want)
	}
	if want := []string{"pages/Hub.md", "journals/2026_05_15.md", "journals/2026_05_24.md"}; !slices.Equal(res.stop.pending, want) {
		t.Errorf("pending = %v, want %v", res.stop.pending, want)
	}
	if got := readRel(t, a, "pages/Hub.md"); got != hub {
		t.Error("Hub.md was overwritten")
	}
	if !existsRel(a, "pages/Alpha.md") || existsRel(a, "pages/Gamma.md") {
		t.Error("page must not move after a stop")
	}
	if a.page.Page() != "Alpha" {
		t.Errorf("page = %q, want Alpha", a.page.Page())
	}

	p2, err := a.planRename(pageMeta(t, a, "Alpha"), "Gamma")
	if err != nil {
		t.Fatal(err)
	}
	if res, err := a.applyRename(p2); err != nil || res.stop != nil || res.reindexErr != nil {
		t.Fatalf("second apply: %+v %v", res, err)
	}
	if !existsRel(a, "pages/Gamma.md") || existsRel(a, "pages/Alpha.md") {
		t.Error("second attempt did not rename the page")
	}
	if got := unresolvedSet(t, a); !reflect.DeepEqual(got, unresolvedBefore) {
		t.Errorf("unresolved = %v, want %v", got, unresolvedBefore)
	}
}

func TestApplyRenameRefusesExistingDest(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	p, err := a.planRename(pageMeta(t, a, "Alpha"), "Gamma")
	if err != nil {
		t.Fatal(err)
	}
	writeRel(t, a, "pages/Gamma.md", "- taken\n")
	before := graphFiles(t, a.graphPath)
	_, err = a.applyRename(p)
	if err == nil || err.Error() != "a file already exists at pages/Gamma.md — nothing written" {
		t.Fatalf("err = %v", err)
	}
	if after := graphFiles(t, a.graphPath); !reflect.DeepEqual(before, after) {
		t.Error("files changed")
	}
}

func TestApplyRenamePageChangedBeforeWrites(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	p, err := a.planRename(pageMeta(t, a, "Alpha"), "Gamma")
	if err != nil {
		t.Fatal(err)
	}
	writeRel(t, a, "pages/Alpha.md", readRel(t, a, "pages/Alpha.md")+"- more\n")
	before := graphFiles(t, a.graphPath)
	_, err = a.applyRename(p)
	if want := "pages/Alpha.md changed on disk — nothing written; esc, then F2 to start again"; err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if after := graphFiles(t, a.graphPath); !reflect.DeepEqual(before, after) {
		t.Error("files changed")
	}
}

func TestApplyRenameRenameStepChanged(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	p, err := a.planRename(pageMeta(t, a, "Alpha"), "Gamma")
	if err != nil {
		t.Fatal(err)
	}
	modified := readRel(t, a, "pages/Alpha.md") + "- more\n"
	writeRel(t, a, "pages/Alpha.md", modified)
	read := a.readSnapshot
	a.readSnapshot = func(path string) (edit.Snapshot, error) {
		if path == p.page.Path {
			return p.pageRead, nil
		}
		return read(path)
	}
	res, err := a.applyRename(p)
	if err != nil || res.stop == nil {
		t.Fatalf("res = %+v err = %v, want a stop", res, err)
	}
	if want := "pages/Alpha.md changed on disk — page not renamed"; res.stop.reason != want {
		t.Errorf("reason = %q, want %q", res.stop.reason, want)
	}
	if want := []string{"pages/Beta.md", "pages/Hub.md", "journals/2026_05_15.md", "journals/2026_05_24.md"}; !slices.Equal(res.stop.done, want) {
		t.Errorf("done = %v, want %v", res.stop.done, want)
	}
	if len(res.stop.pending) != 0 {
		t.Errorf("pending = %v, want none", res.stop.pending)
	}
	if got := readRel(t, a, "pages/Alpha.md"); got != modified {
		t.Error("Alpha.md lost the outside change")
	}
}

func TestApplyRenameIndexBusy(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	p, err := a.planRename(pageMeta(t, a, "Alpha"), "Gamma")
	if err != nil {
		t.Fatal(err)
	}
	before := graphFiles(t, a.graphPath)
	a.indexGen++
	_, err = a.applyRename(p)
	if err == nil || err.Error() != "index is updating — press y again" {
		t.Fatalf("err = %v", err)
	}
	if after := graphFiles(t, a.graphPath); !reflect.DeepEqual(before, after) {
		t.Error("files changed")
	}
}

// unresolvedSet lists each unresolved target with its reference count.
func unresolvedSet(t *testing.T, a *App) []string {
	t.Helper()
	rep, err := doctor.RunWith(a.graphPath, a.lay, func(string) ([]graph.UnlinkedRef, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, u := range rep.Unresolved {
		out = append(out, fmt.Sprintf("%s×%d", u.Target, len(u.Refs)))
	}
	return out
}

func TestApplyRenameNoNewUnresolved(t *testing.T) {
	cases := []struct {
		cfg      bootConfig
		key, new string
	}{
		{fixtureCfg, "Hub", "Center"},
		{fixtureCfg, "kb/notes", "kb/Memo"},
		{fixtureCfg, "Book Club", "Readers"},
		{fixtureCfg, "Kitchen", "Big Kitchen"},
		{vaultCfg, "b/Note", "Memo"},
		{vaultCfg, "a/Note", "kb/Notes"},
		{vaultCfg, "Alpha", "Launch"},
	}
	for _, c := range cases {
		t.Run(c.key+"→"+c.new, func(t *testing.T) {
			a := bootApp(t, c.cfg)
			before := unresolvedSet(t, a)
			p, err := a.planRename(pageMeta(t, a, c.key), c.new)
			if err != nil {
				t.Fatal(err)
			}
			if res, err := a.applyRename(p); err != nil || res.stop != nil || res.reindexErr != nil {
				t.Fatalf("apply: %+v %v", res, err)
			}
			if after := unresolvedSet(t, a); !reflect.DeepEqual(before, after) {
				t.Errorf("unresolved after = %v, want %v", after, before)
			}
		})
	}
}

func TestApplyRenameRemapsHistory(t *testing.T) {
	a := bootApp(t, fixtureCfg)
	today := a.hist[0].page
	for _, n := range []string{"Alpha", "Hub", "Beta", "Hub"} {
		a.navigate(n)
	}
	p, err := a.planRename(pageMeta2(t, a), "Center")
	if err != nil {
		t.Fatal(err)
	}
	if res, err := a.applyRename(p); err != nil || res.stop != nil || res.reindexErr != nil {
		t.Fatalf("apply: %+v %v", res, err)
	}
	var got []string
	for _, h := range a.hist {
		got = append(got, h.page)
	}
	if want := []string{today, "Alpha", "Center", "Beta", "Center"}; !slices.Equal(got, want) {
		t.Fatalf("history = %v, want %v", got, want)
	}
	if a.page.Page() != "Center" {
		t.Fatalf("page = %q", a.page.Page())
	}
	a.historyBack()
	a.historyBack()
	if a.page.Page() != "Center" || !strings.Contains(appText(a), "The hub page") {
		t.Errorf("after back twice: page %q\n%s", a.page.Page(), appText(a))
	}
}

// pageMeta2 resolves the page the app currently shows.
func pageMeta2(t *testing.T, a *App) *graph.PageMeta {
	t.Helper()
	meta, ok := a.idx.Resolve(a.page.Page())
	if !ok {
		t.Fatalf("%q does not resolve", a.page.Page())
	}
	return meta
}

func TestApplyRenameRemapsHistoryClashFlip(t *testing.T) {
	a := bootApp(t, vaultCfg)
	a.navigate("Note")
	p, err := a.planRename(pageMeta2(t, a), "Memo2")
	if err != nil {
		t.Fatal(err)
	}
	if res, err := a.applyRename(p); err != nil || res.stop != nil || res.reindexErr != nil {
		t.Fatalf("apply: %+v %v", res, err)
	}
	last := a.hist[len(a.hist)-1].page
	if last != "Memo2" {
		t.Fatalf("history entry = %q, want Memo2", last)
	}
	if m, ok := a.idx.Resolve(last); !ok || m.Rel != "a/Memo2.md" {
		t.Errorf("entry opens %v, want a/Memo2.md", m)
	}
}

func TestApplyRenameKeepsFolds(t *testing.T) {
	for name, body := range map[string]string{
		"self-link": "- see [[Topic]]\n  - child line\n- other\n",
		"no link":   "- see this\n  - child line\n- other\n",
	} {
		t.Run(name, func(t *testing.T) {
			a := bootApp(t, bootConfig{files: map[string]string{"pages/Topic.md": body}})
			meta := pageMeta(t, a, "Topic")
			oldPath := meta.Path
			a.page.ToggleFold()
			if strings.Contains(appText(a), "child line") {
				t.Fatalf("setup: fold did not hide the child:\n%s", appText(a))
			}
			p, err := a.planRename(meta, "Subject")
			if err != nil {
				t.Fatal(err)
			}
			if res, err := a.applyRename(p); err != nil || res.stop != nil || res.reindexErr != nil {
				t.Fatalf("apply: %+v %v", res, err)
			}
			newPath := filepath.Join(a.graphPath, "pages", "Subject.md")
			e := a.folds[newPath]
			if e == nil || len(e.folded) == 0 {
				t.Fatalf("fold entry at new path = %+v", e)
			}
			if a.folds[oldPath] != nil {
				t.Error("fold entry left at the old path")
			}
			if strings.Contains(appText(a), "child line") {
				t.Errorf("fold lost after rename:\n%s", appText(a))
			}
		})
	}
}

func TestApplyRenameUpdatesDiskSig(t *testing.T) {
	a, _, _ := renameApp(t, fixtureCfg, "Hub", "Center")
	want, err := graph.ScanSignature(a.graphPath, a.lay)
	if err != nil {
		t.Fatal(err)
	}
	if a.diskSig != want {
		t.Errorf("diskSig = %+v, want %+v", a.diskSig, want)
	}
}
