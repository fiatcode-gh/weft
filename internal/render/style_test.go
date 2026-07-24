package render

import (
	"reflect"
	"regexp"
	"strconv"
	"testing"
)

// collectStyleColors walks v recursively and sorts every non-nil *string field
// named "Color" or "BackgroundColor" into two buckets: those inside the
// CodeBlock.Chroma subtree (idx=false → chroma, hex-encoded) and those outside
// it (idx=true → termenv-rendered, ANSI 0-15 index). Chroma colors are hex
// because chroma parses color strings as hex RGB; the terminal16 formatter maps
// them to the terminal palette (see style.go / page.go).
func collectStyleColors(v reflect.Value, inChroma bool, ansiIdx, chromaHex *[]string) {
	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			collectStyleColors(v.Elem(), inChroma, ansiIdx, chromaHex)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			fv := v.Field(i)
			if (f.Name == "Color" || f.Name == "BackgroundColor") &&
				fv.Kind() == reflect.Ptr && fv.Type().Elem().Kind() == reflect.String {
				if !fv.IsNil() {
					if inChroma {
						*chromaHex = append(*chromaHex, fv.Elem().String())
					} else {
						*ansiIdx = append(*ansiIdx, fv.Elem().String())
					}
				}
				continue
			}
			// Everything reachable from the Chroma field is a chroma color.
			collectStyleColors(fv, inChroma || f.Name == "Chroma", ansiIdx, chromaHex)
		}
	}
}

func TestTerminalStyleConfigColors(t *testing.T) {
	// arrange
	var ansiIdx, chromaHex []string
	collectStyleColors(reflect.ValueOf(terminalStyleConfig), false, &ansiIdx, &chromaHex)

	// assert: termenv colors are bare ANSI 0-15 indices
	if len(ansiIdx) == 0 {
		t.Fatal("no ANSI-index colors found; walker or config is wrong")
	}
	for _, c := range ansiIdx {
		n, err := strconv.Atoi(c)
		if err != nil || n < 0 || n > 15 {
			t.Errorf("non-chroma color %q is not an ANSI 0-15 index", c)
		}
	}

	// assert: chroma colors are #rrggbb hex anchors
	if len(chromaHex) == 0 {
		t.Fatal("no chroma colors found; walker or config is wrong")
	}
	hexRe := regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	for _, c := range chromaHex {
		if !hexRe.MatchString(c) {
			t.Errorf("chroma color %q is not a #rrggbb hex anchor", c)
		}
	}
}

func TestSelectStyle(t *testing.T) {
	cases := []struct {
		name    string
		noColor string
		weftS   string
		want    styleSelection
	}{
		{"defaults to terminal palette", "", "", styleSelection{terminal: true}},
		{"NO_COLOR forces notty", "1", "", styleSelection{name: "notty"}},
		{"WEFT_STYLE selects named", "", "dracula", styleSelection{name: "dracula"}},
		{"NO_COLOR beats WEFT_STYLE", "1", "dracula", styleSelection{name: "notty"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			t.Setenv("NO_COLOR", tc.noColor)
			t.Setenv("WEFT_STYLE", tc.weftS)

			// act
			got := selectStyle()

			// assert
			if got != tc.want {
				t.Errorf("selectStyle() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestStyleOptionsChromaFormatterScope guards the subtle invariant that the
// terminal16 chroma formatter is pinned only on the default terminal-palette
// path: a WEFT_STYLE user must keep their theme's full-fidelity formatter. The
// default path returns two options (WithStyles + WithChromaFormatter); the
// named-style and notty paths return one (WithStandardStyle only). The options
// are opaque funcs, so count is the observable proxy for "formatter added".
func TestStyleOptionsChromaFormatterScope(t *testing.T) {
	cases := []struct {
		name    string
		noColor string
		weftS   string
		want    int
	}{
		{"default path adds terminal16 formatter", "", "", 2},
		{"named style: no formatter override", "", "dracula", 1},
		{"notty: no formatter override", "1", "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			t.Setenv("NO_COLOR", tc.noColor)
			t.Setenv("WEFT_STYLE", tc.weftS)

			// act
			got := len(styleOptions())

			// assert
			if got != tc.want {
				t.Errorf("len(styleOptions()) = %d, want %d", got, tc.want)
			}
		})
	}
}
