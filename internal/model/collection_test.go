package model

import "testing"

// TestNamespaceIDRoundTrip verifies a backend-native identifier survives a
// NamespaceID/NativeID round-trip, even when it contains the path separators
// used by local ("/") and DAV ("/...") backends.
func TestNamespaceIDRoundTrip(t *testing.T) {
	cases := []struct{ source, native string }{
		{"local", "fastmail/personal"},
		{"icloud", "/123456/calendars/work/"},
		{"nc", ""},
	}
	for _, c := range cases {
		id := NamespaceID(c.source, c.native)
		if id == c.native {
			t.Errorf("NamespaceID(%q,%q) did not namespace", c.source, c.native)
		}
		if got := NativeID(c.source, id); got != c.native {
			t.Errorf("NativeID(NamespaceID(%q,%q)) = %q, want %q", c.source, c.native, got, c.native)
		}
	}
}
