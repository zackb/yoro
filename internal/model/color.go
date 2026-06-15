package model

import "strings"

// Color is a normalized #RRGGBB hex color (alpha discarded). The empty Color
// means "no color set".
type Color string

// ParseColor normalizes a vdirsyncer/khal color string. It accepts #RGB,
// #RRGGBB, and #RRGGBBAA (alpha is dropped) and returns a #RRGGBB Color. An
// unrecognized input yields the empty Color.
func ParseColor(s string) Color {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '#' {
		return ""
	}
	hex := s[1:]
	switch len(hex) {
	case 3: // #RGB -> #RRGGBB
		return Color("#" + dup(hex[0]) + dup(hex[1]) + dup(hex[2]))
	case 6, 8: // #RRGGBB or #RRGGBBAA
		if !isHex(hex[:6]) {
			return ""
		}
		return Color("#" + strings.ToLower(hex[:6]))
	default:
		return ""
	}
}

// Hex returns the color as a #RRGGBB string, or "" if unset.
func (c Color) Hex() string { return string(c) }

func dup(b byte) string { return string([]byte{b, b}) }

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
