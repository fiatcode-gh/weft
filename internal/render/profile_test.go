package render

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestColorProfile(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want colorprofile.Profile
	}{
		{"NO_COLOR=1 beats truecolor", map[string]string{"NO_COLOR": "1", "TTY_FORCE": "1", "TERM": "xterm-256color", "COLORTERM": "truecolor"}, colorprofile.Ascii},
		{"any non-empty NO_COLOR", map[string]string{"NO_COLOR": "yes", "TTY_FORCE": "1", "TERM": "xterm-256color"}, colorprofile.Ascii},
		{"256-colour terminal", map[string]string{"TTY_FORCE": "1", "TERM": "xterm-256color"}, colorprofile.TrueColor},
		{"16-colour terminal", map[string]string{"TTY_FORCE": "1", "TERM": "xterm"}, colorprofile.TrueColor},
		{"dumb terminal", map[string]string{"TTY_FORCE": "1", "TERM": "dumb"}, colorprofile.NoTTY},
		{"not a TTY", map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}, colorprofile.NoTTY},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"NO_COLOR", "TERM", "COLORTERM", "TMUX", "CLICOLOR", "CLICOLOR_FORCE", "TTY_FORCE"} {
				t.Setenv(k, tc.env[k])
			}
			if got := ColorProfile(&bytes.Buffer{}); got != tc.want {
				t.Fatalf("ColorProfile = %v, want %v", got, tc.want)
			}
		})
	}
}
