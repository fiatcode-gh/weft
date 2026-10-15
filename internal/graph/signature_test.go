package graph

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fiatcode-gh/weft/v3/internal/layout"
)

func TestScanSignatureMatchesBuildIndex(t *testing.T) {
	fixture := fixturePath(t)
	vault, err := filepath.Abs("../../testdata/obsidian-vault")
	if err != nil {
		t.Fatal(err)
	}
	vaultLay, err := layout.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, path string
		lay        layout.Layout
	}{
		{"fixture-graph", fixture, layout.Default()},
		{"obsidian-vault", vault, vaultLay},
	} {
		t.Run(c.name, func(t *testing.T) {
			idx, err := BuildIndex(c.path, c.lay)
			if err != nil {
				t.Fatal(err)
			}
			a, err := ScanSignature(c.path, c.lay)
			if err != nil {
				t.Fatal(err)
			}
			b, err := ScanSignature(c.path, c.lay)
			if err != nil {
				t.Fatal(err)
			}
			if a != idx.Signature() {
				t.Errorf("ScanSignature %x != Index.Signature %x", a, idx.Signature())
			}
			if a != b {
				t.Errorf("two scans differ: %x vs %x", a, b)
			}
		})
	}
}

var sigEpoch = time.Unix(1_000_000_000, 0)

func sigTree(t *testing.T) (string, layout.Layout) {
	t.Helper()
	dir := t.TempDir()
	lay := writeTree(t, dir, map[string]string{
		"pages/Alpha.md":         "- alpha\n",
		"pages/Beta.md":          "- beta\n",
		"journals/2026_01_02.md": "- day\n",
	})
	for _, rel := range []string{"pages/Alpha.md", "pages/Beta.md", "journals/2026_01_02.md"} {
		if err := os.Chtimes(filepath.Join(dir, rel), sigEpoch, sigEpoch); err != nil {
			t.Fatal(err)
		}
	}
	return dir, lay
}

func mustScan(t *testing.T, dir string, lay layout.Layout) Signature {
	t.Helper()
	s, err := ScanSignature(dir, lay)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestScanSignatureChanges(t *testing.T) {
	later := sigEpoch.Add(time.Hour)
	write := func(t *testing.T, dir, rel, body string) string {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cases := []struct {
		name   string
		mutate func(t *testing.T, dir string)
	}{
		{"add page", func(t *testing.T, dir string) { write(t, dir, "pages/New.md", "- new\n") }},
		{"remove page", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "pages/Beta.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{"size alone", func(t *testing.T, dir string) {
			p := write(t, dir, "pages/Alpha.md", "- alpha, longer\n")
			if err := os.Chtimes(p, sigEpoch, sigEpoch); err != nil {
				t.Fatal(err)
			}
		}},
		{"mtime alone", func(t *testing.T, dir string) {
			p := write(t, dir, "pages/Alpha.md", "- ALPHA\n")
			if err := os.Chtimes(p, later, later); err != nil {
				t.Fatal(err)
			}
		}},
		{"add in subfolder", func(t *testing.T, dir string) { write(t, dir, "pages/sub/Deep.md", "- deep\n") }},
		{"rename", func(t *testing.T, dir string) {
			if err := os.Rename(filepath.Join(dir, "pages/Beta.md"), filepath.Join(dir, "pages/Gamma.md")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, lay := sigTree(t)
			before := mustScan(t, dir, lay)
			if before != mustScan(t, dir, lay) {
				t.Fatal("unchanged tree scans differently")
			}
			c.mutate(t, dir)
			if after := mustScan(t, dir, lay); after == before {
				t.Errorf("signature did not change")
			}
		})
	}
}

func TestScanSignatureIgnoresUnwalked(t *testing.T) {
	dir, lay := sigTree(t)
	before := mustScan(t, dir, lay)
	writeTree(t, dir, map[string]string{
		"pages/.hidden/X.md": "- x\n",
		"pages/notes.txt":    "text\n",
		"other/Y.md":         "- y\n",
	})
	if after := mustScan(t, dir, lay); after != before {
		t.Errorf("unwalked files changed the signature")
	}
}

func TestScanSignatureRootError(t *testing.T) {
	dir := t.TempDir()
	lay := writeTree(t, dir, map[string]string{"pages": "not a folder"})
	if _, err := BuildIndex(dir, lay); err == nil {
		t.Fatal("BuildIndex: expected error for a file root")
	}
	if _, err := ScanSignature(dir, lay); err == nil {
		t.Fatal("ScanSignature: expected error for a file root")
	}
}

func BenchmarkScanSignature(b *testing.B) {
	graph, lay := os.Getenv("WEFT_BENCH_GRAPH"), layout.Default()
	if graph != "" {
		var err error
		if lay, err = layout.Load(graph); err != nil {
			b.Fatal(err)
		}
	} else {
		var err error
		if graph, err = filepath.Abs("../../testdata/fixture-graph"); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ScanSignature(graph, lay); err != nil {
			b.Fatal(err)
		}
	}
}
