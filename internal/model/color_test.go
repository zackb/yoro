package model

import "testing"

func TestParseColor(t *testing.T) {
	cases := map[string]Color{
		"#FECF0FFF": "#fecf0f", // RGBA from real data → alpha dropped
		"#B90E28FF": "#b90e28",
		"#34aadc":   "#34aadc",
		"#abc":      "#aabbcc",
		"":          "",
		"red":       "",
		"#12":       "",
		"#zzzzzz":   "",
	}
	for in, want := range cases {
		if got := ParseColor(in); got != want {
			t.Errorf("ParseColor(%q) = %q, want %q", in, got, want)
		}
	}
}
