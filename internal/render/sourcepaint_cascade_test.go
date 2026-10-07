package render

import (
	"testing"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestCascadeChildOverridesSetFieldsAndInheritsTheRest(t *testing.T) {
	parent := ansi.StylePrimitive{Color: strPtr("1"), BackgroundColor: strPtr("2"), Bold: boolPtr(true), Italic: boolPtr(true)}
	child := ansi.StylePrimitive{Color: strPtr("3"), Bold: boolPtr(false), Underline: boolPtr(true)}
	got := cascade(parent, child)

	if *got.Color != "3" {
		t.Errorf("Color = %q, want the child's 3", *got.Color)
	}
	if *got.BackgroundColor != "2" {
		t.Errorf("BackgroundColor = %q, want the inherited 2", *got.BackgroundColor)
	}
	if got.Bold == nil || *got.Bold {
		t.Errorf("Bold = %v, want an explicit false from the child", got.Bold)
	}
	if got.Italic == nil || !*got.Italic {
		t.Errorf("Italic = %v, want inherited true", got.Italic)
	}
	if got.Underline == nil || !*got.Underline {
		t.Errorf("Underline = %v, want the child's true", got.Underline)
	}
	if got.CrossedOut != nil {
		t.Errorf("CrossedOut = %v, want unset", got.CrossedOut)
	}
}

func TestCascadeChainsLeftToRight(t *testing.T) {
	a := ansi.StylePrimitive{Color: strPtr("1"), Bold: boolPtr(true)}
	b := ansi.StylePrimitive{Color: strPtr("2")}
	c := ansi.StylePrimitive{Italic: boolPtr(true)}
	got := cascade(cascade(a, b), c)
	if *got.Color != "2" || !*got.Bold || !*got.Italic {
		t.Errorf("cascade(a,b,c) = colour %q bold %v italic %v, want 2 true true", *got.Color, *got.Bold, *got.Italic)
	}
}

func TestToUVMapsEveryAttributeGlamourRenders(t *testing.T) {
	yes := boolPtr(true)
	tests := []struct {
		name string
		in   ansi.StylePrimitive
		want uv.Style
	}{
		{"zero", ansi.StylePrimitive{}, uv.Style{}},
		{"colour", ansi.StylePrimitive{Color: strPtr("12")}, uv.Style{Fg: lipgloss.Color("12")}},
		{"background", ansi.StylePrimitive{BackgroundColor: strPtr("#102030")}, uv.Style{Bg: lipgloss.Color("#102030")}},
		{"bold", ansi.StylePrimitive{Bold: yes}, uv.Style{Attrs: uv.AttrBold}},
		{"italic", ansi.StylePrimitive{Italic: yes}, uv.Style{Attrs: uv.AttrItalic}},
		{"crossed out", ansi.StylePrimitive{CrossedOut: yes}, uv.Style{Attrs: uv.AttrStrikethrough}},
		{"inverse", ansi.StylePrimitive{Inverse: yes}, uv.Style{Attrs: uv.AttrReverse}},
		{"blink", ansi.StylePrimitive{Blink: yes}, uv.Style{Attrs: uv.AttrBlink}},
		{"underline", ansi.StylePrimitive{Underline: yes}, uv.Style{Underline: uv.UnderlineSingle}},
		{"false attributes", ansi.StylePrimitive{Bold: boolPtr(false), Italic: boolPtr(false)}, uv.Style{}},
		// Glamour's renderText ignores these; so does the editor.
		{"faint ignored", ansi.StylePrimitive{Faint: yes}, uv.Style{}},
		{"conceal ignored", ansi.StylePrimitive{Conceal: yes}, uv.Style{}},
		{"upper lower title ignored", ansi.StylePrimitive{Upper: yes, Lower: yes, Title: yes}, uv.Style{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toUV(tt.in)
			if !got.Equal(&tt.want) {
				t.Errorf("toUV = %+v, want %+v", got, tt.want)
			}
		})
	}
}
