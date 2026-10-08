package views

import (
	"flag"
	"os"
	"os/exec"
	"regexp"
	"testing"
)

// sourceModeEnv makes a test binary run with the editor starting in source
// mode; TestSuiteInSourceMode sets it in a child run of the whole package.
const sourceModeEnv = "WEFT_TEST_EDITOR_SOURCE"

func TestMain(m *testing.M) {
	editorStartsInSource = os.Getenv(sourceModeEnv) == "1"
	os.Exit(m.Run())
}

// TestSuiteInSourceMode re-runs every other test of the package with the
// editor opening in source mode, so each behaviour test proves both looks:
// the parent run is live preview, the child is the unit 3 source look.
func TestSuiteInSourceMode(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if os.Getenv(sourceModeEnv) != "" {
		t.Skip("already the source-mode run")
	}
	if f := flag.Lookup("test.run"); f != nil && f.Value.String() != "" {
		if ok, _ := regexp.MatchString(f.Value.String(), t.Name()); !ok {
			t.Skip("not selected by -run")
		}
	}
	cmd := exec.Command(os.Args[0], "-test.run=.", "-test.skip=^TestSuiteInSourceMode$", "-test.count=1")
	cmd.Env = append(os.Environ(), sourceModeEnv+"=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("source-mode run failed: %v\n%s", err, out)
	}
}

// startInLive makes Apps built by the calling test open live preview, whatever
// look the suite runs in; for tests about the default and about switching.
func startInLive(t *testing.T) {
	t.Helper()
	orig := editorStartsInSource
	editorStartsInSource = false
	t.Cleanup(func() { editorStartsInSource = orig })
}

// skipInSourceRun skips a test about live preview itself when the suite runs in
// source mode: the live run has already proved it.
func skipInSourceRun(t *testing.T) {
	t.Helper()
	if editorStartsInSource {
		t.Skip("live preview only; covered by the default run")
	}
}
