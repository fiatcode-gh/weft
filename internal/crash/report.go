// Package crash turns a panic into a report and sends it to the build's
// Bugsink endpoint. It is weft's only network surface for reporting; see
// docs/decisions/crash-reports.md.
package crash

import (
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
)

// Report is a recovered panic.
type Report struct {
	Value  string  // fmt.Sprint of the panic value, unscrubbed (stderr is local)
	Type   string  // fmt.Sprintf("%T", v)
	Stack  []byte  // debug.Stack() at capture, printed to stderr
	frames []frame // oldest first
}

type frame struct {
	Function string `json:"function"`
	Module   string `json:"module"`
	Filename string `json:"filename"`
	Lineno   int    `json:"lineno"`
	InApp    bool   `json:"in_app"`
}

const appModule = "github.com/fiatcode-gh/weft/v3"

// Capture records v. Call it directly inside the deferred function that
// recovered v, while the panicking stack has not yet unwound.
func Capture(v any) *Report {
	r := &Report{
		Value: fmt.Sprint(v),
		Type:  fmt.Sprintf("%T", v),
		Stack: debug.Stack(),
	}

	pcs := make([]uintptr, 100)
	n := runtime.Callers(1, pcs)
	it := runtime.CallersFrames(pcs[:n])
	var all []runtime.Frame
	for {
		f, more := it.Next()
		all = append(all, f)
		if !more {
			break
		}
	}

	start := 2 // skip Capture and its caller (the deferred func)
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Function == "runtime.gopanic" {
			start = i + 1
			break
		}
	}
	if start > len(all) {
		start = len(all)
	}
	kept := all[start:]
	for len(kept) > 0 && strings.HasPrefix(kept[0].Function, "runtime.") {
		kept = kept[1:]
	}
	for _, f := range kept {
		r.frames = append(r.frames, toFrame(f))
	}
	slices.Reverse(r.frames)
	return r
}

func toFrame(f runtime.Frame) frame {
	module, function := "", f.Function
	slash := strings.LastIndex(f.Function, "/")
	if dot := strings.Index(f.Function[slash+1:], "."); dot >= 0 {
		dot += slash + 1
		module, function = f.Function[:dot], f.Function[dot+1:]
	}
	file := f.File
	if filepath.IsAbs(file) {
		file = filepath.Base(file)
	}
	return frame{
		Function: function,
		Module:   module,
		Filename: file,
		Lineno:   f.Line,
		InApp:    module == "main" || strings.HasPrefix(module, appModule),
	}
}

// String is what weft prints to stderr.
func (r *Report) String() string { return r.Value + "\n\n" + string(r.Stack) }
