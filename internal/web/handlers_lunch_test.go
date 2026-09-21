package web

import (
	"testing"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

func TestParseMenuNumber(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{"lower bound", "1", 1, false},
		{"upper bound", "8", 8, false},
		{"mid range", "3", 3, false},
		{"padded with whitespace", " 2 ", 2, false},
		{"zero rejected", "0", 0, true},
		{"nine rejected: EduPage never offers more than 8", "9", 0, true},
		{"negative rejected", "-1", 0, true},
		{"empty rejected", "", 0, true},
		{"non-numeric rejected", "abc", 0, true},
		{"float rejected", "1.5", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMenuNumber(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseMenuNumber(%q): expected an error, got %d", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMenuNumber(%q): unexpected error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("parseMenuNumber(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestMealByIndex(t *testing.T) {
	snack := &edupage.Meal{MenuID: "1"}
	lunch := &edupage.Meal{MenuID: "2"}
	afternoon := &edupage.Meal{MenuID: "3"}
	meals := &edupage.Meals{Snack: snack, Lunch: lunch, AfternoonSnack: afternoon}

	cases := []struct {
		name  string
		meals *edupage.Meals
		index string
		want  *edupage.Meal
	}{
		{"snack", meals, "1", snack},
		{"lunch", meals, "2", lunch},
		{"afternoon snack", meals, "3", afternoon},
		{"unknown index", meals, "4", nil},
		{"empty index", meals, "", nil},
		{"whitespace is trimmed", meals, " 2 ", lunch},
		{"nil meals", nil, "2", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mealByIndex(tc.meals, tc.index); got != tc.want {
				t.Errorf("mealByIndex(...) = %p, want %p", got, tc.want)
			}
		})
	}

	t.Run("a day that simply doesn't offer that meal", func(t *testing.T) {
		if got := mealByIndex(&edupage.Meals{Lunch: lunch}, "1"); got != nil {
			t.Errorf("expected nil for an unserved snack, got %#v", got)
		}
	})
}
