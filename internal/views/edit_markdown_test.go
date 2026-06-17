package views

import "testing"

func TestBulletPrefix(t *testing.T) {
	cases := []struct {
		line       string
		wantPrefix string
		wantOK     bool
	}{
		{"- foo", "- ", true},
		{"  - nested", "  - ", true},
		{"- TODO task", "- ", true},
		{"# heading", "", false},
		{"plain text", "", false},
		{"-no space", "", false},
	}
	for _, c := range cases {
		gotPrefix, gotOK := bulletPrefix(c.line)
		if gotPrefix != c.wantPrefix || gotOK != c.wantOK {
			t.Errorf("bulletPrefix(%q) = (%q,%v), want (%q,%v)", c.line, gotPrefix, gotOK, c.wantPrefix, c.wantOK)
		}
	}
}

func TestIsEmptyBullet(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"- ", true},
		{"  - ", true},
		{"-", true},
		{"  -", true},
		{"- x", false},
		{"  - foo", false},
		{"plain", false},
	}
	for _, c := range cases {
		if got := isEmptyBullet(c.line); got != c.want {
			t.Errorf("isEmptyBullet(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestCycleMarkerLine(t *testing.T) {
	// oldCol is the cursor column; pick one past the marker so deltas apply.
	cases := []struct {
		name        string
		line        string
		oldCol      int
		wantNewLine string
		wantNewCol  int
		wantOK      bool
	}{
		{"plain to TODO", "- foo", 5, "- TODO foo", 10, true},
		{"TODO to DONE", "- TODO foo", 10, "- DONE foo", 10, true},
		{"DONE to plain", "- DONE foo", 10, "- foo", 5, true},
		{"nested plain to TODO preserves indent", "  - bar", 7, "  - TODO bar", 12, true},
		{"non-bullet no-op", "# heading", 3, "", 0, false},
		{"cursor before marker unaffected on insert", "- foo", 1, "- TODO foo", 1, true},
	}
	for _, c := range cases {
		gotLine, gotCol, gotOK := cycleMarkerLine(c.line, c.oldCol)
		if gotOK != c.wantOK || (c.wantOK && (gotLine != c.wantNewLine || gotCol != c.wantNewCol)) {
			t.Errorf("%s: cycleMarkerLine(%q,%d) = (%q,%d,%v), want (%q,%d,%v)",
				c.name, c.line, c.oldCol, gotLine, gotCol, gotOK, c.wantNewLine, c.wantNewCol, c.wantOK)
		}
	}
}

func TestIndentLine(t *testing.T) {
	if got := indentLine("- foo"); got != "  - foo" {
		t.Errorf("indentLine(%q) = %q, want %q", "- foo", got, "  - foo")
	}
	if got := indentLine("  - bar"); got != "    - bar" {
		t.Errorf("indentLine(%q) = %q, want %q", "  - bar", got, "    - bar")
	}
}

func TestDedentLine(t *testing.T) {
	cases := []struct {
		line        string
		wantLine    string
		wantRemoved int
	}{
		{"  - foo", "- foo", 2},
		{" - foo", "- foo", 1},
		{"- foo", "- foo", 0},
		{"    x", "  x", 2},
	}
	for _, c := range cases {
		gotLine, gotRemoved := dedentLine(c.line)
		if gotLine != c.wantLine || gotRemoved != c.wantRemoved {
			t.Errorf("dedentLine(%q) = (%q,%d), want (%q,%d)", c.line, gotLine, gotRemoved, c.wantLine, c.wantRemoved)
		}
	}
}
