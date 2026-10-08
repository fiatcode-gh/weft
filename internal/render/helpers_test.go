package render

import (
	"flag"
	"os"
	"os/exec"
	"testing"
)

// inFreshProcess runs the calling test in a child process with env applied.
// The renderer cache and Glamour's style registry are process state, so a
// golden pinned per environment must not share a process with another one.
// It returns true in the child, where the test body should run, and false in
// the parent, after the child has passed.
func inFreshProcess(t *testing.T, env map[string]string) bool {
	t.Helper()
	if os.Getenv("WEFT_TEST_CHILD") == t.Name() {
		return true
	}
	// The parent only waits for its child, so the three looks run side by side.
	t.Parallel()
	args := []string{"-test.run=^" + t.Name() + "$"}
	if f := flag.Lookup("update"); f != nil && f.Value.String() == "true" {
		args = append(args, "-update")
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "WEFT_TEST_CHILD="+t.Name())
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child process failed: %v\n%s", err, out)
	}
	return false
}
