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
