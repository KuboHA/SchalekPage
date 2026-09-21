package edupage

import (
	"testing"
	"time"
)

func TestParseMeal(t *testing.T) {
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	t.Run("not cooking returns nil", func(t *testing.T) {
		raw := map[string]any{"isCooking": false}
		if m := parseMeal("2", raw, "boarder1", day); m != nil {
			t.Errorf("expected nil, got %#v", m)
		}
	})

	t.Run("missing entry returns nil", func(t *testing.T) {
		if m := parseMeal("2", nil, "boarder1", day); m != nil {
			t.Errorf("expected nil, got %#v", m)
		}
	})

	t.Run("ordered meal via evidencia.obj", func(t *testing.T) {
		raw := map[string]any{
			"nazov":        "Obed",
			"druhov_jedal": float64(2),
			"evidencia":    map[string]any{"stav": "V", "obj": "A"},
			"rows": []any{
				map[string]any{
					"nazov":        "Segedin guláš",
					"alergenyStr":  "1,7",
					"hmotnostiStr": "400g",
					"menusStr":     ": I.",
				},
			},
		}

		m := parseMeal("2", raw, "boarder1", day)
		if m == nil {
			t.Fatal("expected non-nil meal")
		}
		if m.OrderedMeal != "A" {
			t.Errorf("OrderedMeal = %q, want %q", m.OrderedMeal, "A")
		}
		if m.Title != "Obed" {
			t.Errorf("Title = %q, want %q", m.Title, "Obed")
		}
		if m.MenuID != "2" {
			t.Errorf("MenuID = %q, want %q", m.MenuID, "2")
		}
		if len(m.Menus) != 1 || m.Menus[0].Number != "I." {
			t.Errorf("Menus = %#v", m.Menus)
		}
	})

	t.Run("no order recorded leaves OrderedMeal empty", func(t *testing.T) {
		raw := map[string]any{
			"nazov": "Desiata",
			"rows":  []any{},
		}
		m := parseMeal("1", raw, "boarder1", day)
		if m == nil {
			t.Fatal("expected non-nil meal")
		}
		if m.OrderedMeal != "" {
			t.Errorf("OrderedMeal = %q, want empty", m.OrderedMeal)
		}
	})
}

func TestParseMealRating(t *testing.T) {
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	meal := map[string]any{
		"hodnotenia": map[string]any{
			"I.": []any{
				map[string]any{"priemer": "4.5", "pocet": float64(10)},
				map[string]any{"priemer": "3.2", "pocet": float64(8)},
			},
		},
	}

	rating := parseMealRating(meal, "I.", day)
	if rating == nil {
		t.Fatal("expected non-nil rating")
	}
	if rating.QualityAverage != "4.5" || rating.QualityRatings != 10 {
		t.Errorf("quality = %q/%d, want 4.5/10", rating.QualityAverage, rating.QualityRatings)
	}
	if rating.BoardsAverage != "3.2" || rating.BoardsRatings != 8 {
		t.Errorf("boards = %q/%d, want 3.2/8", rating.BoardsAverage, rating.BoardsRatings)
	}
	if rating.Date != "2024-03-15" {
		t.Errorf("Date = %q, want 2024-03-15", rating.Date)
	}
}

func TestParseMealRating_Missing(t *testing.T) {
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if r := parseMealRating(map[string]any{}, "I.", day); r != nil {
		t.Errorf("expected nil, got %#v", r)
	}
}

func TestExtractDayMeals(t *testing.T) {
	page := "some html prefix\r\n" +
		`edupageData: {"myschool":{"novyListok":{"addInfo":{"stravnikid":"999"},` +
		`"2024-03-15":{"1":{"nazov":"Desiata"},"2":{"nazov":"Obed"}}}}},` + "\r\n" +
		"more trailing junk"

	meals, boarderID, ok := extractDayMeals(page, "myschool", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if boarderID != "999" {
		t.Errorf("boarderID = %q, want %q", boarderID, "999")
	}
	if len(meals) != 2 {
		t.Errorf("meals = %#v, want 2 entries", meals)
	}
}

func TestExtractDayMeals_NoDataForDay(t *testing.T) {
	page := `edupageData: {"myschool":{"novyListok":{"addInfo":{"stravnikid":"999"}}}},` + "\r\n"

	_, _, ok := extractDayMeals(page, "myschool", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	if ok {
		t.Error("expected ok=false when there is no menu entry for the day")
	}
}

func TestCanStillChangeMeal(t *testing.T) {
	day := time.Now()

	future := day.Add(24 * time.Hour).Format(eduTimeLayout)
	if !canStillChangeMeal(future, day) {
		t.Errorf("expected true for a deadline in the future (%s)", future)
	}

	past := day.Add(-24 * time.Hour).Format(eduTimeLayout)
	if canStillChangeMeal(past, day) {
		t.Errorf("expected false for a deadline in the past (%s)", past)
	}

	if canStillChangeMeal("", day) {
		t.Error("expected false for an empty deadline")
	}
	if canStillChangeMeal("garbage", day) {
		t.Error("expected false for an unparseable deadline")
	}
}
