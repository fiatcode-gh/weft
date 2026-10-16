package crash

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const crashModule = "github.com/fiatcode-gh/weft/v3/internal/crash"

func capturePanic(f func()) (r *Report) {
	defer func() {
		if v := recover(); v != nil {
			r = Capture(v)
		}
	}()
	f()
	return nil
}

func panicHere() { panic("boom") }

func indexOutOfRange() int {
	s := []int{1}
	i := 5
	return s[i]
}

func TestCaptureStartsAtPanicSite(t *testing.T) {
	r := capturePanic(panicHere)
	if r == nil {
		t.Fatal("no report")
	}
	if r.Value != "boom" || r.Type != "string" {
		t.Fatalf("Value=%q Type=%q", r.Value, r.Type)
	}
	if len(r.frames) == 0 {
		t.Fatal("no frames")
	}
	last := r.frames[len(r.frames)-1]
	if last.Function != "panicHere" || !last.InApp || last.Module != crashModule {
		t.Fatalf("last frame = %+v", last)
	}
	for _, f := range r.frames {
		if strings.HasPrefix(f.Function, "runtime.gopanic") || f.Function == "Capture" {
			t.Fatalf("unexpected frame %+v", f)
		}
	}
}

func TestCaptureRuntimeError(t *testing.T) {
	r := capturePanic(func() { indexOutOfRange() })
	if r == nil {
		t.Fatal("no report")
	}
	if r.Type != "runtime.boundsError" {
		t.Fatalf("Type = %q", r.Type)
	}
	last := r.frames[len(r.frames)-1]
	if last.Function != "indexOutOfRange" {
		t.Fatalf("last frame = %+v", last)
	}
}

func TestCaptureFilenamesNotAbsolute(t *testing.T) {
	r := capturePanic(panicHere)
	for _, f := range r.frames {
		if filepath.IsAbs(f.Filename) {
			t.Fatalf("absolute filename %q", f.Filename)
		}
	}
}

func TestReportString(t *testing.T) {
	r := &Report{Value: "v", Stack: []byte("stack")}
	if got := r.String(); got != "v\n\nstack" {
		t.Fatalf("String = %q", got)
	}
}

func TestToFrameMainModuleIsInApp(t *testing.T) {
	f := toFrame(runtime.Frame{Function: "main.main", File: "/src/cmd/weft/main.go", Line: 7})
	if !f.InApp || f.Module != "main" || f.Function != "main" {
		t.Fatalf("frame = %+v", f)
	}
	if toFrame(runtime.Frame{Function: "net/http.(*Client).Do"}).InApp {
		t.Fatal("net/http frame marked in-app")
	}
}
