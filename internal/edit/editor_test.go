package edit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeLookPath returns a lookPath that resolves a fixed set of names and
// returns ErrNotFound for everything else. errNotFound matches the
// surface of exec.LookPath on missing binaries.
var errNotFound = errors.New("not found")

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
