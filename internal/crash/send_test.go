package crash

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

const testKey = "testkey0123456789"

func TestParseDSN(t *testing.T) {
	ok := []struct{ in, endpoint string }{
		{"http://testkey0123456789@127.0.0.1:9000/3", "http://127.0.0.1:9000/api/3/envelope/"},
		{"https://k@h/sub/3", "https://h/sub/api/3/envelope/"},
	}
	for _, c := range ok {
		d, err := ParseDSN(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if d.endpoint != c.endpoint {
			t.Fatalf("%q: endpoint %q, want %q", c.in, d.endpoint, c.endpoint)
		}
	}
	bad := []string{
		"",
		"http://host/3",
		"ftp://secretkey@host/3",
		"http://secretkey@host/abc",
		"http://secretkey@host",
		"http://secretkey@host/",
		"http://secretkey@/3",
		"://secretkey@%zz",
	}
	for _, in := range bad {
		_, err := ParseDSN(in)
		if err == nil {
			t.Fatalf("%q: want error", in)
		}
		if in != "" && strings.Contains(err.Error(), in) {
			t.Fatalf("error %q contains input", err)
		}
		if strings.Contains(err.Error(), "secretkey") {
			t.Fatalf("error %q contains key", err)
		}
	}
}

func fakeDSN(t *testing.T, url string) DSN {
	t.Helper()
	d, err := ParseDSN(strings.Replace(url, "http://", "http://"+testKey+"@", 1) + "/3")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestSendEventHoldsOnlyListedFields(t *testing.T) {
	home := t.TempDir()
	graph := home + "/notes"
	var gotPath, gotType, gotAuth string
	var body []byte
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		gotPath, gotType, gotAuth = r.URL.Path, r.Header.Get("Content-Type"), r.Header.Get("X-Sentry-Auth")
		body, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	rep := capturePanic(func() { panic("open " + graph + "/pages/x.md and " + home + "/y") })
	if err := Send(fakeDSN(t, srv.URL), rep, "v1.2.3", Scrub{Home: home, Graph: graph}); err != nil {
		t.Fatal(err)
	}
	t.Logf("envelope:\n%s", body)
	if n != 1 {
		t.Fatalf("%d requests", n)
	}
	if gotPath != "/api/3/envelope/" || gotType != "application/x-sentry-envelope" {
		t.Fatalf("path %q type %q", gotPath, gotType)
	}
	if !strings.Contains(gotAuth, "sentry_key="+testKey) || !strings.Contains(gotAuth, "sentry_version=7") {
		t.Fatalf("auth %q", gotAuth)
	}

	lines := bytes.Split(body, []byte("\n"))
	if len(lines) != 4 || len(lines[3]) != 0 {
		t.Fatalf("envelope has %d lines", len(lines))
	}
	var hdr, item, ev map[string]any
	for i, p := range []*map[string]any{&hdr, &item, &ev} {
		if err := json.Unmarshal(lines[i], p); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
	}
	if !reflect.DeepEqual(keys(hdr), []string{"event_id"}) {
		t.Fatalf("header keys %v", keys(hdr))
	}
	id, _ := hdr["event_id"].(string)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) || ev["event_id"] != id {
		t.Fatalf("event_id %q vs %v", id, ev["event_id"])
	}
	if item["type"] != "event" || item["length"] != float64(len(lines[2])) {
		t.Fatalf("item header %v, event len %d", item, len(lines[2]))
	}
	want := []string{"contexts", "event_id", "exception", "level", "platform", "release", "timestamp"}
	if !reflect.DeepEqual(keys(ev), want) {
		t.Fatalf("event keys %v", keys(ev))
	}
	if ev["platform"] != "go" || ev["level"] != "fatal" || ev["release"] != "v1.2.3" {
		t.Fatalf("event %v", ev)
	}
	if _, err := time.Parse(time.RFC3339, ev["timestamp"].(string)); err != nil {
		t.Fatal(err)
	}

	ctx := ev["contexts"].(map[string]any)
	if !reflect.DeepEqual(keys(ctx), []string{"device", "os", "runtime"}) {
		t.Fatalf("contexts %v", keys(ctx))
	}
	for name, ks := range map[string][]string{
		"os": {"name"}, "device": {"arch"}, "runtime": {"name", "version"},
	} {
		if got := keys(ctx[name].(map[string]any)); !reflect.DeepEqual(got, ks) {
			t.Fatalf("contexts.%s keys %v", name, got)
		}
	}

	vals := ev["exception"].(map[string]any)["values"].([]any)
	if len(vals) != 1 {
		t.Fatalf("%d exception values", len(vals))
	}
	v := vals[0].(map[string]any)
	if !reflect.DeepEqual(keys(v), []string{"stacktrace", "type", "value"}) {
		t.Fatalf("exception keys %v", keys(v))
	}
	if v["value"] != "open "+"<graph>"+"/pages/x.md and ~/y" {
		t.Fatalf("value %q", v["value"])
	}
	frames := v["stacktrace"].(map[string]any)["frames"].([]any)
	if len(frames) == 0 {
		t.Fatal("no frames")
	}
	allowed := map[string]bool{"function": true, "module": true, "filename": true, "lineno": true, "in_app": true}
	for _, f := range frames {
		for k := range f.(map[string]any) {
			if !allowed[k] {
				t.Fatalf("frame key %q", k)
			}
		}
	}

	for _, s := range []string{home, graph} {
		if bytes.Contains(body, []byte(s)) {
			t.Fatalf("body contains %q", s)
		}
	}
	if h, err := os.Hostname(); err == nil && len(h) >= 4 &&
		!strings.Contains(strings.ToLower(fixedText), strings.ToLower(h)) &&
		bytes.Contains(body, []byte(h)) {
		t.Fatalf("body contains hostname %q", h)
	}
	for _, k := range []string{"server_name", "breadcrumbs", "modules", `"user"`, `"extra"`, `"tags"`} {
		if bytes.Contains(body, []byte(k)) {
			t.Fatalf("body contains %s", k)
		}
	}
}

// fixedText is the constant vocabulary of the event; a hostname that happens
// to be a substring of it (a host named "go") would be a false hit.
const fixedText = "go fatal event exception contexts runtime release timestamp platform stacktrace frames function module filename lineno in_app " + crashModule + " linux amd64 arm64 darwin windows"

func TestSendServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	err := Send(fakeDSN(t, srv.URL), capturePanic(panicHere), "v", Scrub{})
	if err == nil || err.Error() != "server answered 500 Internal Server Error" {
		t.Fatalf("err = %v", err)
	}
}

func TestSendConnectionRefused(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	err = Send(fakeDSN(t, "http://"+addr), capturePanic(panicHere), "v", Scrub{})
	if err == nil {
		t.Fatal("want error")
	}
	for _, s := range []string{testKey, "/api/"} {
		if strings.Contains(err.Error(), s) {
			t.Fatalf("error %q contains %q", err, s)
		}
	}
}

func TestSendHangGivesUp(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	start := time.Now()
	err := Send(fakeDSN(t, srv.URL), capturePanic(panicHere), "v", Scrub{})
	elapsed := time.Since(start)
	if err == nil || err.Error() != "no answer within 2s" {
		t.Fatalf("err = %v", err)
	}
	if elapsed < Timeout || elapsed > Timeout+time.Second {
		t.Fatalf("elapsed %v", elapsed)
	}
}

func TestTimeoutIsTwoSeconds(t *testing.T) {
	if Timeout != 2*time.Second {
		t.Fatalf("Timeout = %v, want 2s", Timeout)
	}
}

func TestSendRequestHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()
	if err := Send(fakeDSN(t, srv.URL), capturePanic(panicHere), "v1.2.3", Scrub{}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Content-Type":  "application/x-sentry-envelope",
		"X-Sentry-Auth": "Sentry sentry_version=7, sentry_client=weft/v1.2.3, sentry_key=" + testKey,
	}
	for k, v := range want {
		if got.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, got.Get(k), v)
		}
	}
	// Go adds these itself; anything else would be something weft chose to send.
	automatic := map[string]bool{"User-Agent": true, "Accept-Encoding": true, "Content-Length": true}
	for k := range got {
		if _, ok := want[k]; !ok && !automatic[k] {
			t.Errorf("unexpected header %s: %q", k, got.Values(k))
		}
	}
}

func TestScrubIgnoresUnusableGraph(t *testing.T) {
	const v = "open notes/pages/notes.md and ./x.md in /root"
	for _, graph := range []string{".", "notes", "notes/", "", "/"} {
		if got := (Scrub{Graph: graph}).apply(v); got != v {
			t.Errorf("Graph %q changed the value: %q", graph, got)
		}
	}
	if got := (Scrub{Graph: "/g/notes"}).apply("open /g/notes/pages/a.md"); got != "open <graph>/pages/a.md" {
		t.Errorf("absolute graph: %q", got)
	}
}
