package web

import (
	"testing"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

func TestParseTermQuery(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		def  edupage.Term
		want edupage.Term
	}{
		{"first term", "P1", edupage.TermSecond, edupage.TermFirst},
		{"second term", "P2", edupage.TermFirst, edupage.TermSecond},
		{"empty falls back", "", edupage.TermSecond, edupage.TermSecond},
		{"garbage falls back", "P3", edupage.TermSecond, edupage.TermSecond},
		{"garbage falls back to first-term default", "nonsense", edupage.TermFirst, edupage.TermFirst},
		{"lowercase does not match", "p1", edupage.TermSecond, edupage.TermSecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseTermQuery(tc.raw, tc.def); got != tc.want {
				t.Errorf("parseTermQuery(%q, %q) = %q, want %q", tc.raw, tc.def, got, tc.want)
			}
		})
	}
}

func TestParseYearQuery(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		def  int
		want int
	}{
		{"valid year", "2024", 2025, 2024},
		{"empty falls back", "", 2025, 2025},
		{"non-numeric falls back", "abc", 2025, 2025},
		{"too old falls back", "1999", 2025, 2025},
		{"too far future falls back", "2101", 2025, 2025},
		{"boundary year accepted", "2000", 2025, 2000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseYearQuery(tc.raw, tc.def); got != tc.want {
				t.Errorf("parseYearQuery(%q, %d) = %d, want %d", tc.raw, tc.def, got, tc.want)
			}
		})
	}
}

func TestTermLabel(t *testing.T) {
	if got := termLabel(edupage.TermFirst); got != "First term" {
		t.Errorf("termLabel(TermFirst) = %q, want %q", got, "First term")
	}
	if got := termLabel(edupage.TermSecond); got != "Second term" {
		t.Errorf("termLabel(TermSecond) = %q, want %q", got, "Second term")
	}
}
