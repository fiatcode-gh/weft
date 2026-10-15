package crash

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"
)

// Timeout bounds the whole send.
const Timeout = 2 * time.Second

// DSN is a parsed Sentry-style DSN. Its fields are never printed.
type DSN struct{ endpoint, key, raw string }

// ParseDSN parses an http(s) DSN of the form scheme://key@host[/prefix]/<id>.
// Errors are fixed strings and never contain the input.
func ParseDSN(s string) (DSN, error) {
	u, err := url.Parse(s)
	if err != nil {
		return DSN{}, errors.New("crash DSN: bad URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return DSN{}, errors.New("crash DSN: not http(s)")
	}
	if u.Host == "" {
		return DSN{}, errors.New("crash DSN: bad URL")
	}
	key := u.User.Username()
	if key == "" {
		return DSN{}, errors.New("crash DSN: no key")
	}
	path := strings.Trim(u.Path, "/")
	prefix, id := "", path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		prefix, id = "/"+path[:i], path[i+1:]
	}
	if id == "" || strings.Trim(id, "0123456789") != "" {
		return DSN{}, errors.New("crash DSN: no project id")
	}
	return DSN{
		endpoint: u.Scheme + "://" + u.Host + prefix + "/api/" + id + "/envelope/",
		key:      key,
		raw:      s,
	}, nil
}

// Scrub names the paths removed from the panic value before sending.
type Scrub struct{ Home, Graph string }

func (s Scrub) apply(v string) string {
	if s.Graph != "" {
		v = strings.ReplaceAll(v, s.Graph, "<graph>")
	}
	if s.Home != "" && s.Home != "/" {
		v = strings.ReplaceAll(v, s.Home, "~")
	}
	return v
}

type event struct {
	EventID   string    `json:"event_id"`
	Timestamp string    `json:"timestamp"`
	Platform  string    `json:"platform"`
	Level     string    `json:"level"`
	Release   string    `json:"release"`
	Exception exception `json:"exception"`
	Contexts  contexts  `json:"contexts"`
}

type exception struct {
	Values []exceptionValue `json:"values"`
}

type exceptionValue struct {
	Type       string     `json:"type"`
	Value      string     `json:"value"`
	Stacktrace stacktrace `json:"stacktrace"`
}

type stacktrace struct {
	Frames []frame `json:"frames"`
}

type contexts struct {
	OS struct {
		Name string `json:"name"`
	} `json:"os"`
	Device struct {
		Arch string `json:"arch"`
	} `json:"device"`
	Runtime struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"runtime"`
}

func newEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func buildEnvelope(r *Report, release string, s Scrub) ([]byte, error) {
	id, err := newEventID()
	if err != nil {
		return nil, err
	}
	frames := r.frames
	if frames == nil {
		frames = []frame{}
	}
	ev := event{
		EventID:   id,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Platform:  "go",
		Level:     "fatal",
		Release:   release,
		Exception: exception{Values: []exceptionValue{{
			Type:       r.Type,
			Value:      s.apply(r.Value),
			Stacktrace: stacktrace{Frames: frames},
		}}},
	}
	ev.Contexts.OS.Name = runtime.GOOS
	ev.Contexts.Device.Arch = runtime.GOARCH
	ev.Contexts.Runtime.Name = "go"
	ev.Contexts.Runtime.Version = runtime.Version()

	body, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "{\"event_id\":%q}\n{\"type\":\"event\",\"length\":%d}\n", id, len(body))
	buf.Write(body)
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// Send posts one event for r, bounded by Timeout, with no retries. Returned
// errors never contain the DSN key or URL.
func Send(d DSN, r *Report, release string, s Scrub) error {
	payload, err := buildEnvelope(r, release, s)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("crash DSN: bad URL")
	}
	req.Header.Set("Content-Type", "application/x-sentry-envelope")
	req.Header.Set("X-Sentry-Auth", "Sentry sentry_version=7, sentry_client=weft/"+release+", sentry_key="+d.key)

	resp, err := (&http.Client{Timeout: Timeout}).Do(req)
	if err != nil {
		var ue *url.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ue) && ue.Timeout()) {
			return errors.New("no answer within 2s")
		}
		if errors.As(err, &ue) {
			return ue.Err
		}
		return errors.New("request failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("server answered %s", resp.Status)
	}
	return nil
}
