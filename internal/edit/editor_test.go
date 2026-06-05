package edit

import (
	"errors"
	"testing"
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
