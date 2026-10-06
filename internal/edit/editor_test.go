package edit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// errNotFound matches the surface of exec.LookPath on missing binaries.
var errNotFound = errors.New("not found")

// fakeLookPath returns a lookPath that resolves a fixed set of names and
// returns errNotFound for everything else.
func fakeLookPath(present map[string]string) func(string) (string, error) {
	return func(bin string) (string, error) {
		if p, ok := present[bin]; ok {
			return p, nil
		}
		return "", errNotFound
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name string
		env  Env
		lp   func(string) (string, error)
		want string
	}{
		{
			name: "VISUAL set, present",
			env:  Env{Visual: "vim", Editor: "emacs"},
			lp:   fakeLookPath(map[string]string{"vim": "/usr/bin/vim", "emacs": "/usr/bin/emacs"}),
			want: "/usr/bin/vim",
		},
		{
			name: "VISUAL set, missing; EDITOR present",
			env:  Env{Visual: "nvim", Editor: "emacs"},
			lp:   fakeLookPath(map[string]string{"emacs": "/usr/bin/emacs"}),
			want: "/usr/bin/emacs",
		},
		{
			name: "VISUAL and EDITOR empty, vi present",
			env:  Env{},
			lp:   fakeLookPath(map[string]string{"/usr/bin/vi": "/usr/bin/vi"}),
			want: "/usr/bin/vi",
		},
		{
			name: "VISUAL empty, EDITOR present, vi also present — EDITOR wins",
			env:  Env{Editor: "micro"},
			lp:   fakeLookPath(map[string]string{"micro": "/usr/bin/micro", "/usr/bin/vi": "/usr/bin/vi"}),
			want: "/usr/bin/micro",
		},
		{
			name: "VISUAL set but missing; EDITOR set but missing; vi missing",
			env:  Env{Visual: "nvim", Editor: "nano"},
			lp:   fakeLookPath(map[string]string{}),
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.env, tc.lp)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
				if !errors.Is(err, errNoEditor) {
					t.Errorf("want errNoEditor, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Binary != tc.want {
				t.Errorf("Binary: want %q, got %q", tc.want, got.Binary)
			}
		})
	}
}

func TestEnsureFile(t *testing.T) {
	t.Run("missing file is created empty", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "new.md")

		created, err := EnsureFile(path)
		if err != nil {
			t.Fatalf("EnsureFile: %v", err)
		}
		if !created {
			t.Errorf("want created=true, got false")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Size() != 0 {
			t.Errorf("want empty file, got %d bytes", info.Size())
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("want mode 0o644, got %v", info.Mode().Perm())
		}
	})

	t.Run("existing file is left alone", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "existing.md")
		if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		before, _ := os.Stat(path)

		created, err := EnsureFile(path)
		if err != nil {
			t.Fatalf("EnsureFile: %v", err)
		}
		if created {
			t.Errorf("want created=false, got true")
		}
		after, _ := os.Stat(path)
		if before.ModTime() != after.ModTime() {
			t.Errorf("mtime changed: before=%v after=%v", before.ModTime(), after.ModTime())
		}
		body, _ := os.ReadFile(path)
		if string(body) != "hello\n" {
			t.Errorf("content changed: got %q", body)
		}
	})

	t.Run("path is a directory — error", func(t *testing.T) {
		dir := t.TempDir()
		_, err := EnsureFile(dir)
		if err == nil {
			t.Errorf("want error, got nil")
		}
	})
}

func TestEnsureFileNeverTruncatesExisting(t *testing.T) {
	// arrange
	dir := t.TempDir()
	path := filepath.Join(dir, "journal.md")
	if err := os.WriteFile(path, []byte("- precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	created, err := EnsureFile(path)

	// assert
	if err != nil || created {
		t.Fatalf("EnsureFile = (%v, %v), want (false, nil)", created, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "- precious\n" {
		t.Fatalf("existing content changed: %q", got)
	}
}

// seedFile writes an existing file at dir/name with the given content and an
// explicit mode (chmod is umask-proof, unlike the WriteFile perm arg). It is
// the shared arrange step for the overwrite-path WriteFile tests.
func seedFile(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// readBack returns the file's content; a read failure is fatal to the test.
func readBack(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back %s: %v", path, err)
	}
	return string(body)
}

// permOf returns the file's permission bits; a stat failure is fatal.
func permOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// fileNames lists the entry names in dir, sorted by ReadDir.
func fileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestWriteFile(t *testing.T) {
	t.Run("writes content and creates parent dir", func(t *testing.T) {
		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "pages", "New Page.md")

		// act
		err := WriteFile(path, []byte("hello\n"))

		// assert
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if got := readBack(t, path); got != "hello\n" {
			t.Errorf("content: got %q, want %q", got, "hello\n")
		}
		if got := permOf(t, path); got != 0o644 {
			t.Errorf("mode: got %v, want 0o644 (new-file default)", got)
		}
	})

	t.Run("overwrites an existing file's content", func(t *testing.T) {
		// arrange
		dir := t.TempDir()
		path := seedFile(t, dir, "p.md", "old\n", 0o644)

		// act
		err := WriteFile(path, []byte("new\n"))

		// assert
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if got := readBack(t, path); got != "new\n" {
			t.Errorf("content: got %q, want %q", got, "new\n")
		}
	})

	t.Run("preserves an existing file's custom mode", func(t *testing.T) {
		// arrange — 0o640 differs from both the 0o644 default and the
		// 0o600 a temp file is born with, so a naive temp+rename that
		// skips the chmod step fails this assertion.
		dir := t.TempDir()
		path := seedFile(t, dir, "p.md", "old\n", 0o640)

		// act
		err := WriteFile(path, []byte("new\n"))

		// assert
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if got := permOf(t, path); got != 0o640 {
			t.Errorf("mode: got %v, want 0o640 (preserved)", got)
		}
	})

	t.Run("leaves no temp files behind on success", func(t *testing.T) {
		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "p.md")

		// act
		err := WriteFile(path, []byte("data\n"))

		// assert
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if got := fileNames(t, dir); len(got) != 1 || got[0] != "p.md" {
			t.Errorf("dir entries: got %v, want [p.md] (temp cleaned up)", got)
		}
	})

	t.Run("a failed write leaves the original intact and strands no temp", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses directory permission bits")
		}
		// arrange — seed a file, then make its directory read-only so the
		// temp create fails. This is the whole point of the atomic rewrite:
		// a botched write must not destroy the file that was already there.
		dir := t.TempDir()
		path := seedFile(t, dir, "p.md", "original\n", 0o644)
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0o700) }) // let t.TempDir clean up

		// act
		err := WriteFile(path, []byte("replacement\n"))

		// assert
		if err == nil {
			t.Fatal("WriteFile: want error writing into a read-only dir, got nil")
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if got := readBack(t, path); got != "original\n" {
			t.Errorf("content: got %q, want %q (original must survive a failed write)", got, "original\n")
		}
		if got := fileNames(t, dir); len(got) != 1 || got[0] != "p.md" {
			t.Errorf("dir entries: got %v, want [p.md] (no temp stranded)", got)
		}
	})
}

func TestSnapshotMtime(t *testing.T) {
	t.Run("existing file returns ModTime", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "page.md")
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := SnapshotMtime(path)
		if err != nil {
			t.Fatalf("SnapshotMtime: %v", err)
		}
		if got.IsZero() {
			t.Errorf("want non-zero mtime, got zero")
		}
	})

	t.Run("missing file returns zero mtime and ENOENT", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nope.md")
		got, err := SnapshotMtime(path)
		if !os.IsNotExist(err) {
			t.Errorf("want ENOENT, got %v", err)
		}
		if !got.IsZero() {
			t.Errorf("want zero mtime, got %v", got)
		}
	})

	t.Run("mtime advances after write", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "page.md")
		if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		first, err := SnapshotMtime(path)
		if err != nil {
			t.Fatal(err)
		}
		// Sleep just past the filesystem mtime resolution (1s on
		// FAT/vfat, 1ns on ext4/tmpfs/APFS; 10ms is a safe margin).
		time.Sleep(10 * time.Millisecond)
		if err := os.WriteFile(path, []byte("bb\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		second, err := SnapshotMtime(path)
		if err != nil {
			t.Fatal(err)
		}
		if !second.After(first) {
			t.Errorf("want second > first; first=%v second=%v", first, second)
		}
	})
}

func TestReadSnapshot(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := ReadSnapshot(path)

		if err != nil {
			t.Fatal(err)
		}
		if want := (Snapshot{Content: "hello\n", Exists: true}); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
	t.Run("missing file", func(t *testing.T) {
		got, err := ReadSnapshot(filepath.Join(t.TempDir(), "nope.md"))

		if err != nil {
			t.Fatal(err)
		}
		if got != (Snapshot{}) {
			t.Errorf("got %+v, want zero Snapshot", got)
		}
	})
	t.Run("directory", func(t *testing.T) {
		if _, err := ReadSnapshot(t.TempDir()); err == nil {
			t.Error("reading a directory should return an error")
		}
	})
}

func TestWriteFileIfUnchanged(t *testing.T) {
	t.Run("unchanged writes", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		seen, _ := ReadSnapshot(path)

		err := WriteFileIfUnchanged(path, seen, []byte("new\n"))

		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); string(got) != "new\n" {
			t.Errorf("file = %q, want new content", got)
		}
	})
	t.Run("content changed → ErrChanged, file untouched", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		seen, _ := ReadSnapshot(path)
		if err := os.WriteFile(path, []byte("outside\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		err := WriteFileIfUnchanged(path, seen, []byte("new\n"))

		if !errors.Is(err, ErrChanged) {
			t.Fatalf("err = %v, want ErrChanged", err)
		}
		if got, _ := os.ReadFile(path); string(got) != "outside\n" {
			t.Errorf("file = %q, want untouched", got)
		}
	})
	t.Run("seen missing, file now exists → ErrChanged, untouched", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		seen, _ := ReadSnapshot(path)
		if err := os.WriteFile(path, []byte("appeared\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		err := WriteFileIfUnchanged(path, seen, []byte("new\n"))

		if !errors.Is(err, ErrChanged) {
			t.Fatalf("err = %v, want ErrChanged", err)
		}
		if got, _ := os.ReadFile(path); string(got) != "appeared\n" {
			t.Errorf("file = %q, want untouched", got)
		}
	})
	t.Run("seen existing, file deleted → ErrChanged, not recreated", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		seen, _ := ReadSnapshot(path)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}

		err := WriteFileIfUnchanged(path, seen, []byte("new\n"))

		if !errors.Is(err, ErrChanged) {
			t.Fatalf("err = %v, want ErrChanged", err)
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("file must not be recreated; stat err = %v", statErr)
		}
	})
	t.Run("seen missing and still missing → creates file and parent dir", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sub", "a.md")

		err := WriteFileIfUnchanged(path, Snapshot{}, []byte("new\n"))

		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); string(got) != "new\n" {
			t.Errorf("file = %q, want new content", got)
		}
	})
	t.Run("mtime-only change still writes", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		seen, _ := ReadSnapshot(path)
		later := time.Now().Add(time.Hour)
		if err := os.Chtimes(path, later, later); err != nil {
			t.Fatal(err)
		}

		err := WriteFileIfUnchanged(path, seen, []byte("new\n"))

		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); string(got) != "new\n" {
			t.Errorf("file = %q, want new content", got)
		}
	})
}
