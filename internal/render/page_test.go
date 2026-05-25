package render

import (
	"strings"
	"testing"
)

func TestRenderPageReturnsLinksWithTargets(t *testing.T) {
	body := "- See [[Alpha]] and [[Beta|the second]]."
	out, err := Render(body, 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	gotTargets := make([]string, 0, len(out.Links))
	for _, l := range out.Links {
		gotTargets = append(gotTargets, l.Target)
	}
	want := []string{"Alpha", "Beta"}
	if strings.Join(gotTargets, ",") != strings.Join(want, ",") {
		t.Errorf("link targets: want %v, got %v", want, gotTargets)
	}
	if out.Styled == "" {
		t.Error("Styled output empty")
	}
	if len(out.Links) >= 2 {
		span := out.Styled[out.Links[1].Start:out.Links[1].End]
		if !strings.Contains(span, "the second") {
			t.Errorf("aliased link span: want substring %q, got %q", "the second", span)
		}
	}
}

func TestRenderPageHandlesEmptyBody(t *testing.T) {
	out, err := Render("", 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(out.Links) != 0 {
		t.Errorf("empty body should have no links, got %v", out.Links)
	}
}
