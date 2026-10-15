package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fiatcode-gh/weft/v3/internal/crash"

	tea "charm.land/bubbletea/v2"
)

const (
	fakeKey     = "testkey0123456789"
	crashRel    = "v0.0.0-crashtest"
	altScreenOn = "\x1b[?1049h"
	altScreenOf = "\x1b[?1049l"
)

type crashArm struct{}

func updatePanics()           { panic(crashValue) }
func viewPanics()             { panic(crashValue) }
func panicCmd() tea.Msg       { panic(crashValue) }
func mainPanics(graph string) { panic(crashValue) }

// crashValue is set by crashChild before any panic can fire.
var crashValue string

// crashModel panics at where once crashArm arrives.
type crashModel struct {
	where string
	armed bool
}

func (m *crashModel) Init() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return crashArm{} })
}

func (m *crashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(crashArm); !ok {
		return m, nil
	}
	switch m.where {
	case "update":
		updatePanics()
	case "view":
		m.armed = true
	case "cmd":
		return m, panicCmd
	}
	return m, nil
}

func (m *crashModel) View() tea.View {
	if m.armed && m.where == "view" {
		viewPanics()
	}
	v := tea.NewView("crash fixture")
	v.AltScreen = true
	return v
}

// crashChild mirrors main's crash wiring around a panicking model and returns
// the process exit code.
func crashChild(where string) int {
	rep := newReporter(os.Getenv("WEFT_TEST_CRASH_DSN"), crashRel, os.Stderr)
	defer rep.recoverMain()
	cfg, ok := loadUserConfig(os.Stderr)
	if !ok {
		return 2
	}
	graph := os.Getenv("WEFT_TEST_CRASH_GRAPH")
	home, _ := os.UserHomeDir()
	crashValue = fmt.Sprintf("crash fixture %s at %s and %s", where,
		filepath.Join(graph, "pages", "x.md"), filepath.Join(home, "elsewhere"))
	rep.setGraph(graph)
	rep.setConsent(cfg.CrashReports)
	if where == "main" {
		mainPanics(graph)
	}
	out, err := os.Create(os.Getenv("WEFT_TEST_CRASH_OUT"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer out.Close()
	opts := append(programOptions(out),
		tea.WithInput(strings.NewReader("")),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
		tea.WithWindowSize(80, 24))
	return rep.runUI(&crashModel{where: where}, opts...)
}

type recordedReq struct {
	path, auth string
	body       []byte
}

// fakeSentry is a local endpoint that records requests and counts connections.
type fakeSentry struct {
	srv   *httptest.Server
	mu    sync.Mutex
	reqs  []*recordedReq
	conns int
	code  int
}

func newFakeSentry(t *testing.T, code int) *fakeSentry {
	t.Helper()
	f := &fakeSentry{code: code}
	f.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, &recordedReq{r.URL.Path, r.Header.Get("X-Sentry-Auth"), body})
		f.mu.Unlock()
		w.WriteHeader(f.code)
		io.WriteString(w, `{"id":"0123456789abcdef0123456789abcdef"}`)
	}))
	f.srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			f.mu.Lock()
			f.conns++
			f.mu.Unlock()
		}
	}
	f.srv.Start()
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSentry) dsn() string {
	return "http://" + fakeKey + "@" + f.srv.Listener.Addr().String() + "/3"
}

func (f *fakeSentry) snapshot() ([]*recordedReq, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*recordedReq(nil), f.reqs...), f.conns
}

// runCrash runs the crash child for where against a fake endpoint.
func runCrash(t *testing.T, where string, config *string, withDSN bool) (code int, stderr string, out []byte, reqs []*recordedReq, conns int, home, graph string) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	fake := newFakeSentry(t, 200)
	home = t.TempDir()
	xdg := t.TempDir()
	graph = t.TempDir()
	outPath := filepath.Join(t.TempDir(), "out.bin")
	if config != nil {
		p := filepath.Join(xdg, "weft", "config.toml")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(*config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "WEFT_GRAPH=") {
			env = append(env, e)
		}
	}
	env = append(env, "WEFT_TEST_CRASH="+where, "HOME="+home, "XDG_CONFIG_HOME="+xdg,
		"WEFT_TEST_CRASH_GRAPH="+graph, "WEFT_TEST_CRASH_OUT="+outPath)
	if withDSN {
		env = append(env, "WEFT_TEST_CRASH_DSN="+fake.dsn())
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = env
	var errb bytes.Buffer
	cmd.Stderr = &errb
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		code = ee.ExitCode()
	default:
		t.Fatal(err)
	}
	out, _ = os.ReadFile(outPath)
	reqs, conns = fake.snapshot()
	return code, errb.String(), out, reqs, conns, home, graph
}

// eventOf decodes the event line of a Sentry envelope body.
func eventOf(t *testing.T, body []byte) map[string]any {
	t.Helper()
	lines := strings.Split(string(body), "\n")
	if len(lines) < 3 {
		t.Fatalf("envelope has %d lines: %q", len(lines), body)
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &ev); err != nil {
		t.Fatalf("event json: %v", err)
	}
	return ev
}

func TestCrashReports(t *testing.T) {
	on, off, absent := "crash_reports = true\n", "crash_reports = false\n", ""
	modes := []struct {
		name    string
		config  *string
		withDSN bool
		sends   bool
	}{
		{"on", &on, true, true},
		{"off", &off, true, false},
		{"absent", &absent, true, false},
		{"nodsn", &on, false, false},
	}
	wheres := []struct{ where, fn string }{
		{"update", "updatePanics"},
		{"view", "viewPanics"},
		{"cmd", "panicCmd"},
		{"main", "mainPanics"},
	}
	for _, w := range wheres {
		for _, m := range modes {
			t.Run(w.where+"/"+m.name, func(t *testing.T) {
				code, stderr, out, reqs, conns, home, graph := runCrash(t, w.where, m.config, m.withDSN)
				wantCode := 1
				if w.where == "main" {
					wantCode = 2
				}
				if code != wantCode {
					t.Errorf("exit code = %d, want %d\nstderr:\n%s", code, wantCode, stderr)
				}
				raw := fmt.Sprintf("crash fixture %s at %s and %s", w.where,
					filepath.Join(graph, "pages", "x.md"), filepath.Join(home, "elsewhere"))
				for _, want := range []string{raw, "goroutine ", w.fn} {
					if !strings.Contains(stderr, want) {
						t.Errorf("stderr lacks %q:\n%s", want, stderr)
					}
				}
				if w.where != "main" {
					i := bytes.LastIndex(out, []byte(altScreenOn))
					if i < 0 || !bytes.Contains(out[i:], []byte(altScreenOf)) {
						t.Errorf("terminal not restored: alt-screen enter at %d, no exit after it", i)
					}
				}
				if !m.sends {
					if len(reqs) != 0 || conns != 0 {
						t.Errorf("requests = %d, connections = %d, want none", len(reqs), conns)
					}
					return
				}
				if len(reqs) != 1 {
					t.Fatalf("requests = %d, want 1\nstderr:\n%s", len(reqs), stderr)
				}
				r := reqs[0]
				if r.path != "/api/3/envelope/" {
					t.Errorf("path = %q", r.path)
				}
				if !strings.Contains(r.auth, "sentry_key="+fakeKey) {
					t.Errorf("auth = %q", r.auth)
				}
				if strings.Contains(stderr, "crash report not sent") {
					t.Errorf("unexpected send failure:\n%s", stderr)
				}
				ev := eventOf(t, r.body)
				if ev["release"] != crashRel {
					t.Errorf("release = %v", ev["release"])
				}
				vals := ev["exception"].(map[string]any)["values"].([]any)
				v0 := vals[0].(map[string]any)
				wantVal := fmt.Sprintf("crash fixture %s at %s/pages/x.md and ~/elsewhere", w.where, "<graph>")
				if v0["value"] != wantVal {
					t.Errorf("value = %q, want %q", v0["value"], wantVal)
				}
				var fns []string
				for _, f := range v0["stacktrace"].(map[string]any)["frames"].([]any) {
					fns = append(fns, f.(map[string]any)["function"].(string))
				}
				if !strings.Contains(strings.Join(fns, "\n"), w.fn) {
					t.Errorf("frames lack %s: %v", w.fn, fns)
				}
				if w.where == "update" {
					t.Logf("recorded event body (fake key): %s", r.body)
				}
			})
		}
	}
}

func TestReporterMutedSendsNothing(t *testing.T) {
	fake := newFakeSentry(t, 200)
	var errb bytes.Buffer
	r := newReporter(fake.dsn(), crashRel, &errb)
	r.setConsent(true)
	r.mute()
	r.send(nil)
	if _, conns := fake.snapshot(); conns != 0 {
		t.Errorf("connections = %d, want 0", conns)
	}
}

func TestReporterSendFailureLine(t *testing.T) {
	fake := newFakeSentry(t, 500)
	var errb bytes.Buffer
	r := newReporter(fake.dsn(), crashRel, &errb)
	r.setConsent(true)
	r.send(captureForTest())
	want := "weft: crash report not sent: server answered 500 Internal Server Error\n"
	if errb.String() != want {
		t.Errorf("stderr = %q, want %q", errb.String(), want)
	}
	if strings.Contains(errb.String(), fakeKey) {
		t.Error("stderr leaks the key")
	}
}

func TestNewReporterBadDSN(t *testing.T) {
	for _, dsn := range []string{"not a dsn", ""} {
		if newReporter(dsn, crashRel, io.Discard).hasDSN() {
			t.Errorf("hasDSN(%q) = true", dsn)
		}
	}
}

func TestReporterLazyConsent(t *testing.T) {
	tests := []struct {
		name, config string
		wantReqs     int
	}{
		{"enabled", "crash_reports = true\n", 1},
		{"malformed", "crash_reports = \n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeUserConfig(t, tt.config)
			fake := newFakeSentry(t, 200)
			r := newReporter(fake.dsn(), crashRel, io.Discard)
			r.send(captureForTest())
			if reqs, _ := fake.snapshot(); len(reqs) != tt.wantReqs {
				t.Errorf("requests = %d, want %d", len(reqs), tt.wantReqs)
			}
		})
	}
}

// captureForTest returns a real panic report built the way production builds it.
func captureForTest() (rep *crash.Report) {
	defer func() { rep = crash.Capture(recover()) }()
	panic("unit fixture")
}
