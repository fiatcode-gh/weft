package render

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

func TestCurrentTheme(t *testing.T) {
	cases := []struct {
		name          string
		noColor       string
		weftS         string
		wantName      string
		wantFormatter string
		wantTerminal  bool
		wantErr       string
	}{
		{"defaults to terminal palette", "", "", "", "terminal16", true, ""},
		{"NO_COLOR forces notty", "1", "", "notty", "terminal256", false, ""},
		{"WEFT_STYLE selects named", "", "dracula", "dracula", "terminal256", false, ""},
		{"NO_COLOR beats WEFT_STYLE", "1", "dracula", "notty", "terminal256", false, ""},
		{"unknown name falls back to notty with error", "", "nope", "notty", "terminal256", false, "nope: style not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tc.noColor)
			t.Setenv("WEFT_STYLE", tc.weftS)

			got, err := CurrentTheme()

			if tc.wantErr == "" && err != nil {
				t.Fatalf("CurrentTheme() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr) {
				t.Fatalf("CurrentTheme() error = %v, want %q", err, tc.wantErr)
			}
			if got.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tc.wantName)
			}
			if got.Formatter != tc.wantFormatter {
				t.Errorf("Formatter = %q, want %q", got.Formatter, tc.wantFormatter)
			}
			want := terminalStyleConfig
			switch {
			case got.Name == "notty":
				want = noColorStyleConfig
			case !tc.wantTerminal:
				want = *styles.DefaultStyles[got.Name]
			}
			if !reflect.DeepEqual(got.Config, want) {
				t.Errorf("Config is not the %q style config", got.Name)
			}
			if _, ok := got.Markers["TODO"]; !ok {
				t.Error("Markers lacks TODO")
			}
		})
	}
}

// A renderer built for one style must not be served after the environment
// selects another.
func TestRendererCacheFollowsStyle(t *testing.T) {
	const body = "# Title\n"

	t.Setenv("NO_COLOR", "1")
	plain, err := Render(body, 60)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ansi.Strip(plain.Styled), "# Title") {
		t.Fatalf("NO_COLOR render lacks the ASCII heading prefix: %q", plain.Styled)
	}

	t.Setenv("NO_COLOR", "")
	coloured, err := Render(body, 60)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ansi.Strip(coloured.Styled), "# Title") {
		t.Fatalf("coloured render still has the ASCII heading prefix (stale cached renderer?): %q", coloured.Styled)
	}
}
