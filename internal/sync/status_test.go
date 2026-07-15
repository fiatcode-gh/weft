package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFile creates dir/name with content, failing the test on error.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatusCleanSyncedRepo(t *testing.T) {
	// Arrange: a fresh clone whose seed commit is already pushed.
	work := newRepoWithRemote(t)

	// Act
	st, err := Status(work)

	// Assert
	if err != nil {
		t.Fatalf("Status: unexpected error: %v", err)
	}
	if st.Dirty || st.Ahead != 0 || st.Unsynced() {
		t.Errorf("clean synced repo should be in sync, got %+v (Unsynced=%v)", st, st.Unsynced())
	}
}

func TestStatusDirtyWorktree(t *testing.T) {
	// Arrange: an uncommitted new file.
	work := newRepoWithRemote(t)
	writeFile(t, work, "new.md", "hi\n")

	// Act
	st, err := Status(work)

	// Assert
	if err != nil {
		t.Fatalf("Status: unexpected error: %v", err)
	}
	if !st.Dirty || st.Ahead != 0 || !st.Unsynced() {
		t.Errorf("uncommitted change should read dirty (ahead 0) + unsynced, got %+v", st)
	}
}

func TestStatusAheadOfUpstream(t *testing.T) {
	// Arrange: a commit that exists locally but was never pushed.
	work := newRepoWithRemote(t)
	writeFile(t, work, "new.md", "hi\n")
	git(t, work, "add", "-A")
	git(t, work, "commit", "-m", "local only")

	// Act
	st, err := Status(work)

	// Assert
	if err != nil {
		t.Fatalf("Status: unexpected error: %v", err)
	}
	if st.Dirty {
		t.Errorf("a committed tree should not read dirty, got %+v", st)
	}
	if st.Ahead != 1 {
		t.Errorf("Ahead = %d, want 1", st.Ahead)
	}
	if !st.Unsynced() {
		t.Error("an unpushed commit should read unsynced")
	}
}

func TestStatusNonRepoErrors(t *testing.T) {
	// Arrange: a plain directory that is not a git work tree.
	dir := t.TempDir()

	// Act
	_, err := Status(dir)

	// Assert
	if err == nil {
		t.Fatal("Status on a non-repo should return an error")
	}
}

func TestStatusTimesOutOnSlowGit(t *testing.T) {
	// Arrange: a valid repo, but an effectively-zero deadline so the very
	// first git call (the preflight) exceeds it deterministically.
	work := newRepoWithRemote(t)
	withShortSyncTimeout(t, time.Nanosecond)

	// Act
	start := time.Now()
	_, err := Status(work)

	// Assert
	if err == nil {
		t.Fatal("want timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Status blocked %v; timeout did not apply", elapsed)
	}
}
