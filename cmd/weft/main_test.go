package main

import "testing"

func TestDebugLogEnabled(t *testing.T) {
	// arrange
	cases := map[string]bool{
		"":         false,
		"0":        false,
		"false":    false,
		"FALSE":    false,
		"no":       false,
		"off":      false,
		"1":        true,
		"true":     true,
		"yes":      true,
		"weft.log": true,
	}

	for v, want := range cases {
		// act
		got := debugLogEnabled(v)

		// assert
		if got != want {
			t.Errorf("debugLogEnabled(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestShortenPseudoVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"dev sentinel", "dev", "dev"},
		{"tagged release", "v0.1.3", "v0.1.3"},
		{"prerelease tag", "v0.2.0-rc1", "v0.2.0-rc1"},
		{
			"pseudo no prior tag",
			"v0.0.0-20260525115608-94836dea2daa",
			"v0.0.0+94836de",
		},
		{
			"pseudo after release",
			"v0.1.3-0.20260525115608-94836dea2daa",
			"v0.1.3-0+94836de",
		},
		{
			"pseudo after prerelease",
			"v0.1.3-pre.0.20260525115608-94836dea2daa",
			"v0.1.3-pre.0+94836de",
		},
		{
			"pseudo with dirty suffix",
			"v0.1.3-0.20260525115608-94836dea2daa+dirty",
			"v0.1.3-0+94836de+dirty",
		},
		{
			"pseudo no prior tag with dirty",
			"v0.0.0-20260525115608-94836dea2daa+dirty",
			"v0.0.0+94836de+dirty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortenPseudoVersion(tc.in); got != tc.want {
				t.Errorf("shortenPseudoVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
