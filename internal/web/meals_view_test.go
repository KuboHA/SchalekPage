package web

import (
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// ptrTime returns a pointer to t, for constructing edupage.Meal/RingingTime
// literals in tests (e.g. CanBeChangedUntil) without a temporary variable.
// Shared across this package's tests — see render_smoke_test.go.
func ptrTime(t time.Time) *time.Time { return &t }

func TestMealChangeable(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	t.Run("nil meal", func(t *testing.T) {
		if mealChangeable(nil, now) {
			t.Error("expected false for a nil meal")
		}
	})

	t.Run("explicit deadline in the future", func(t *testing.T) {
		deadline := now.Add(time.Hour)
		m := &edupage.Meal{CanBeChangedUntil: &deadline, CanBeChanged: false}
		if !mealChangeable(m, now) {
			t.Error("expected true: deadline has not passed yet")
		}
	})

	t.Run("explicit deadline in the past", func(t *testing.T) {
		deadline := now.Add(-time.Hour)
		m := &edupage.Meal{CanBeChangedUntil: &deadline, CanBeChanged: true}
		if mealChangeable(m, now) {
			t.Error("expected false: deadline has passed, regardless of the coarse flag")
		}
	})

	t.Run("no deadline at all falls back to CanBeChanged", func(t *testing.T) {
		if !mealChangeable(&edupage.Meal{CanBeChanged: true}, now) {
			t.Error("expected true when CanBeChanged is true and there is no deadline")
		}
		if mealChangeable(&edupage.Meal{CanBeChanged: false}, now) {
			t.Error("expected false when CanBeChanged is false and there is no deadline")
		}
	})
}

func TestBuildMealViews(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	t.Run("nil meals yields nil", func(t *testing.T) {
		if got := buildMealViews(nil, now); got != nil {
			t.Errorf("expected nil, got %#v", got)
		}
	})

	t.Run("skips meals the day doesn't offer", func(t *testing.T) {
		views := buildMealViews(&edupage.Meals{Lunch: &edupage.Meal{MenuID: "2"}}, now)
		if len(views) != 1 {
			t.Fatalf("expected 1 view, got %d: %#v", len(views), views)
		}
		if views[0].Type != "Lunch" || views[0].Index != "2" {
			t.Errorf("got %#v", views[0])
		}
	})

	t.Run("no order recorded", func(t *testing.T) {
		views := buildMealViews(&edupage.Meals{Lunch: &edupage.Meal{}}, now)
		v := views[0]
		if v.HasOrder {
			t.Error("expected HasOrder = false")
		}
		if v.IsSignedOff {
			t.Error("expected IsSignedOff = false")
		}
		if v.OrderedMeal != "X" {
			t.Errorf("OrderedMeal = %q, want the empty-order sentinel %q", v.OrderedMeal, "X")
		}
	})

	t.Run("menu ordered", func(t *testing.T) {
		views := buildMealViews(&edupage.Meals{Lunch: &edupage.Meal{OrderedMeal: "A"}}, now)
		v := views[0]
		if !v.HasOrder {
			t.Error("expected HasOrder = true")
		}
		if v.IsSignedOff {
			t.Error("expected IsSignedOff = false for a real menu choice")
		}
		if v.OrderedMeal != "A" {
			t.Errorf("OrderedMeal = %q, want %q", v.OrderedMeal, "A")
		}
	})

	t.Run("signed off is distinguished from an ordered menu", func(t *testing.T) {
		views := buildMealViews(&edupage.Meals{Lunch: &edupage.Meal{OrderedMeal: mealSignOffCode}}, now)
		v := views[0]
		if !v.HasOrder {
			t.Error("expected HasOrder = true: a sign-off is still a recorded choice")
		}
		if !v.IsSignedOff {
			t.Error("expected IsSignedOff = true")
		}
	})

	t.Run("deadline label only set when EduPage published a deadline", func(t *testing.T) {
		deadline := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
		withDeadline := buildMealViews(&edupage.Meals{Lunch: &edupage.Meal{CanBeChangedUntil: &deadline}}, now)
		if withDeadline[0].DeadlineLabel == "" {
			t.Error("expected a non-empty DeadlineLabel")
		}

		withoutDeadline := buildMealViews(&edupage.Meals{Lunch: &edupage.Meal{}}, now)
		if withoutDeadline[0].DeadlineLabel != "" {
			t.Errorf("expected empty DeadlineLabel, got %q", withoutDeadline[0].DeadlineLabel)
		}
	})

	t.Run("Changeable reflects mealChangeable against now", func(t *testing.T) {
		past := now.Add(-time.Hour)
		views := buildMealViews(&edupage.Meals{Lunch: &edupage.Meal{CanBeChangedUntil: &past}}, now)
		if views[0].Changeable {
			t.Error("expected Changeable = false: deadline has passed")
		}
	})

	t.Run("all three meal slots in order", func(t *testing.T) {
		views := buildMealViews(&edupage.Meals{
			Snack:          &edupage.Meal{},
			Lunch:          &edupage.Meal{},
			AfternoonSnack: &edupage.Meal{},
		}, now)
		if len(views) != 3 {
			t.Fatalf("expected 3 views, got %d", len(views))
		}
		wantTypes := []string{"Snack", "Lunch", "Afternoon Snack"}
		wantIndices := []string{"1", "2", "3"}
		for i, v := range views {
			if v.Type != wantTypes[i] || v.Index != wantIndices[i] {
				t.Errorf("view %d = %+v, want Type=%q Index=%q", i, v, wantTypes[i], wantIndices[i])
			}
		}
	})
}

func TestMenuOrdered(t *testing.T) {
	cases := []struct {
		name string
		m    mealView
		menu edupage.Menu
		want bool
	}{
		{"first menu ordered", mealView{OrderedMeal: "A"}, edupage.Menu{Number: "A"}, true},
		{"third menu ordered (beyond the old A/B-only bug)", mealView{OrderedMeal: "C"}, edupage.Menu{Number: "C"}, true},
		{"mismatched letter", mealView{OrderedMeal: "A"}, edupage.Menu{Number: "B"}, false},
		{"nothing ordered", mealView{OrderedMeal: "X"}, edupage.Menu{Number: "A"}, false},
		{"signed off never matches a menu", mealView{OrderedMeal: mealSignOffCode}, edupage.Menu{Number: "A"}, false},
		{"a plain course row (no menu letter) never matches", mealView{OrderedMeal: "A"}, edupage.Menu{Number: ""}, false},
		{"a roman-numeral course label never matches a letter order", mealView{OrderedMeal: "A"}, edupage.Menu{Number: "I."}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := menuOrdered(tc.m, tc.menu); got != tc.want {
				t.Errorf("menuOrdered(%+v, %+v) = %v, want %v", tc.m, tc.menu, got, tc.want)
			}
		})
	}
}

func TestChoosableMenuCount(t *testing.T) {
	cases := []struct {
		name  string
		menus []edupage.Menu
		want  int
	}{
		{"nil", nil, 0},
		{"only course rows, no real menu variants", []edupage.Menu{
			{Name: "Soup", Number: "I."},
			{Name: "Main course", Number: "II."},
		}, 0},
		{"two choosable menus plus their course rows", []edupage.Menu{
			{Name: "Menu A", Number: "A", OrderIndex: "1", Choosable: true},
			{Name: "Menu B", Number: "B", OrderIndex: "2", Choosable: true},
			{Name: "Soup", Number: "I."},
			{Name: "Side dish", Number: ""},
		}, 2},
		{"a menu variant the school marked not choosable is excluded", []edupage.Menu{
			{Name: "Menu A", Number: "A", OrderIndex: "1", Choosable: true},
			{Name: "Menu B", Number: "B", OrderIndex: "2", Choosable: false},
		}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := choosableMenuCount(tc.menus); got != tc.want {
				t.Errorf("choosableMenuCount(%+v) = %d, want %d", tc.menus, got, tc.want)
			}
		})
	}
}
