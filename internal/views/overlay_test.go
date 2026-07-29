package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
)

func TestOverlayResultConstructors(t *testing.T) {
	// arrange
	cmd := func() tea.Msg { return "done" }
	ref := &graph.UnlinkedRef{PageName: "Note", Line: 3}
	cases := []struct {
		name    string
		result  OverlayResult
		want    OverlayResult
		wantCmd bool
	}{
		{name: "zero", result: OverlayResult{}, want: OverlayResult{kind: overlayResultNone}},
		{name: "cancel", result: overlayCancel(), want: OverlayResult{kind: overlayResultCancel}},
		{name: "command", result: overlayCommand(cmd), want: OverlayResult{kind: overlayResultCommand}, wantCmd: true},
		{name: "open", result: overlayOpen("Alpha"), want: OverlayResult{kind: overlayResultOpen, page: "Alpha"}},
		{name: "create", result: overlayCreate("New"), want: OverlayResult{kind: overlayResultCreate, page: "New"}},
		{name: "open task", result: overlayOpenTask("Alpha", 2), want: OverlayResult{kind: overlayResultOpenTask, page: "Alpha", taskOrdinal: 2}},
		{name: "focus link", result: overlayFocusLink("Note", "Alpha"), want: OverlayResult{kind: overlayResultFocusLink, page: "Note", target: "Alpha"}},
		{name: "highlight", result: overlayHighlight("Note", "Alpha"), want: OverlayResult{kind: overlayResultHighlight, page: "Note", target: "Alpha"}},
		{name: "linkify", result: overlayLinkify(ref, "Alpha"), want: OverlayResult{kind: overlayResultLinkify, ref: ref, target: "Alpha"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// act
			got := tc.result

			// assert
			if got.kind != tc.want.kind || got.page != tc.want.page || got.taskOrdinal != tc.want.taskOrdinal || got.target != tc.want.target || got.ref != tc.want.ref || (got.cmd != nil) != tc.wantCmd {
				t.Fatalf("result = %+v, want payload %+v with command present = %t", got, tc.want, tc.wantCmd)
			}
		})
	}
}
