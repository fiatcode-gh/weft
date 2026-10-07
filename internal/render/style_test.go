package render

import (
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
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

var styleAttrFields = []string{"Underline", "Bold", "Italic", "CrossedOut", "Faint", "Inverse", "Blink"}

// TestNoColorStyleConfig pins noColorStyleConfig to the ASCII layout plus exactly the terminal style's
// attributes: no colour anywhere, nothing but attributes added, and no code
// highlighting.
func TestNoColorStyleConfig(t *testing.T) {
	ascii := reflect.ValueOf(styles.ASCIIStyleConfig)
	term := reflect.ValueOf(terminalStyleConfig)
	got := reflect.ValueOf(noColorStyleConfig)
	if got.Type() != ascii.Type() {
		t.Fatalf("noColorStyleConfig has type %v", got.Type())
	}
	attrSet := map[string]bool{}
	for _, n := range styleAttrFields {
		attrSet[n] = true
	}
	primitive := reflect.TypeFor[ansi.StylePrimitive]()
	sawAttr := 0

	var walk func(path string, got, ascii, term reflect.Value)
	walk = func(path string, got, ascii, term reflect.Value) {
		switch {
		case got.Type() == primitive:
			for i := 0; i < primitive.NumField(); i++ {
				name := primitive.Field(i).Name
				g, a, tm := got.Field(i), ascii.Field(i), term.Field(i)
				p := path + "." + name
				switch {
				case name == "Color" || name == "BackgroundColor":
					if !g.IsNil() {
						t.Errorf("%s = %v, want nil (no colour)", p, g.Elem())
					}
				case attrSet[name]:
					want := a
					if !tm.IsNil() {
						want = tm
						sawAttr++
					}
					if !reflect.DeepEqual(g.Interface(), want.Interface()) {
						t.Errorf("%s = %v, want %v", p, derefPtr(g), derefPtr(want))
					}
				default:
					if !reflect.DeepEqual(g.Interface(), a.Interface()) {
						t.Errorf("%s = %v, want ASCII's %v", p, g.Interface(), a.Interface())
					}
				}
			}
		case got.Kind() == reflect.Struct:
			for i := 0; i < got.NumField(); i++ {
				walk(path+"."+got.Type().Field(i).Name, got.Field(i), ascii.Field(i), term.Field(i))
			}
		case got.Type() == reflect.TypeFor[*ansi.Chroma]():
			if !got.IsNil() {
				t.Errorf("%s is set; NO_COLOR has no code highlighting", path)
			}
		default:
			if !reflect.DeepEqual(got.Interface(), ascii.Interface()) {
				t.Errorf("%s = %v, want ASCII's %v", path, got.Interface(), ascii.Interface())
			}
		}
	}
	walk("StyleConfig", got, ascii, term)

	if sawAttr == 0 {
		t.Fatal("walk found no attribute in the terminal config; the test is not reaching StylePrimitive values")
	}
}

// derefPtr renders a possibly-nil pointer value readably in failures.
func derefPtr(v reflect.Value) any {
	if v.IsNil() {
		return nil
	}
	return v.Elem().Interface()
}
