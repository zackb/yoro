package model

import (
	"testing"
	"time"
)

// TestOccurrenceDays verifies an occurrence is expanded to every calendar day it
// spans, with end-of-day boundaries treated as exclusive.
func TestOccurrenceDays(t *testing.T) {
	day := func(s string) time.Time {
		tm, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return tm
	}
	cases := []struct {
		name       string
		start, end string
		want       []string
	}{
		{"all-day single", "2026-06-10 00:00", "2026-06-11 00:00", []string{"2026-06-10"}},
		{"all-day three days", "2026-06-10 00:00", "2026-06-13 00:00", []string{"2026-06-10", "2026-06-11", "2026-06-12"}},
		{"timed crossing days", "2026-06-10 14:00", "2026-06-13 10:00", []string{"2026-06-10", "2026-06-11", "2026-06-12", "2026-06-13"}},
		{"timed same day", "2026-06-10 09:00", "2026-06-10 10:00", []string{"2026-06-10"}},
		{"timed ending at midnight", "2026-06-10 22:00", "2026-06-11 00:00", []string{"2026-06-10"}},
		{"zero duration", "2026-06-10 09:00", "2026-06-10 09:00", []string{"2026-06-10"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := Occurrence{Start: day(c.start), End: day(c.end)}
			var got []string
			for _, d := range o.Days() {
				got = append(got, d.Format("2006-01-02"))
			}
			if len(got) != len(c.want) {
				t.Fatalf("Days() = %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("Days() = %v, want %v", got, c.want)
				}
			}
		})
	}
}
