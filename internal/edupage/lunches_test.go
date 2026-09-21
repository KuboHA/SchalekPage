package edupage

import (
	"encoding/json"
	"errors"
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

func TestParseMealPopulatesWriteFields(t *testing.T) {
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	future := time.Now().Add(24 * time.Hour).Format(eduTimeLayout)

	raw := map[string]any{
		"nazov":   "Obed",
		"zmen_do": future,
	}

	m := parseMeal("2", raw, "999", day)
	if m == nil {
		t.Fatal("expected non-nil meal")
	}
	if m.BoarderID != "999" {
		t.Errorf("BoarderID = %q, want %q", m.BoarderID, "999")
	}
	if m.MealIndex != "2" {
		t.Errorf("MealIndex = %q, want %q", m.MealIndex, "2")
	}
	if m.CanBeChangedUntil == nil {
		t.Fatal("expected non-nil CanBeChangedUntil")
	}
	if !m.CanBeChanged {
		t.Error("expected CanBeChanged = true for a future deadline")
	}
}

func TestMenuLetter(t *testing.T) {
	cases := []struct {
		n       int
		want    string
		wantErr bool
	}{
		{1, "A", false},
		{2, "B", false},
		{8, "H", false},
		{0, "", true},
		{9, "", true},
		{-1, "", true},
	}
	for _, tc := range cases {
		got, err := menuLetter(tc.n)
		if tc.wantErr {
			if err == nil {
				t.Errorf("menuLetter(%d): expected error, got %q", tc.n, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("menuLetter(%d): unexpected error: %v", tc.n, err)
		}
		if got != tc.want {
			t.Errorf("menuLetter(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func changeableMeal(deadline time.Time) *Meal {
	return &Meal{
		BoarderID:         "999",
		MealIndex:         "2",
		Date:              time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		CanBeChangedUntil: &deadline,
	}
}

func TestBuildMealChoiceRequest_JidsLetterMapping(t *testing.T) {
	m := changeableMeal(time.Now().Add(time.Hour))

	values, err := buildMealChoiceRequest(m, "C")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := values.Get("akcia"); got != "ulozJedlaStravnika" {
		t.Errorf("akcia = %q, want %q", got, "ulozJedlaStravnika")
	}

	var payload struct {
		StravnikID string            `json:"stravnikid"`
		MysqlDate  string            `json:"mysqlDate"`
		Jids       map[string]string `json:"jids"`
		View       string            `json:"view"`
		Pravo      string            `json:"pravo"`
	}
	if err := json.Unmarshal([]byte(values.Get("jedlaStravnika")), &payload); err != nil {
		t.Fatalf("jedlaStravnika is not valid JSON: %v", err)
	}
	if payload.StravnikID != "999" {
		t.Errorf("stravnikid = %q, want %q", payload.StravnikID, "999")
	}
	if payload.MysqlDate != "2024-03-15" {
		t.Errorf("mysqlDate = %q, want %q", payload.MysqlDate, "2024-03-15")
	}
	if payload.Jids["2"] != "C" {
		t.Errorf("jids[\"2\"] = %q, want %q", payload.Jids["2"], "C")
	}
	if payload.View != "pc_listok" || payload.Pravo != "Student" {
		t.Errorf("view/pravo = %q/%q, want pc_listok/Student", payload.View, payload.Pravo)
	}
}

func TestBuildMealChoiceRequest_SignOffUsesAX(t *testing.T) {
	m := changeableMeal(time.Now().Add(time.Hour))

	values, err := buildMealChoiceRequest(m, mealSignOffCode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload struct {
		Jids map[string]string `json:"jids"`
	}
	if err := json.Unmarshal([]byte(values.Get("jedlaStravnika")), &payload); err != nil {
		t.Fatalf("jedlaStravnika is not valid JSON: %v", err)
	}
	if payload.Jids["2"] != "AX" {
		t.Errorf("jids[\"2\"] = %q, want %q", payload.Jids["2"], "AX")
	}
}

func TestBuildMealChoiceRequest_DeadlinePassed(t *testing.T) {
	m := changeableMeal(time.Now().Add(-time.Hour))

	if _, err := buildMealChoiceRequest(m, "A"); err == nil {
		t.Error("expected an error for a meal whose change deadline has passed")
	}
}

func TestBuildMealChoiceRequest_NoDeadlineAtAll(t *testing.T) {
	m := &Meal{BoarderID: "999", MealIndex: "2", Date: time.Now()}

	if _, err := buildMealChoiceRequest(m, "A"); err == nil {
		t.Error("expected an error for a meal with no CanBeChangedUntil at all")
	}
}

func TestBuildMealChoiceRequest_MissingBoarderID(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	m := &Meal{MealIndex: "2", Date: time.Now(), CanBeChangedUntil: &deadline}

	_, err := buildMealChoiceRequest(m, "A")
	if err == nil {
		t.Fatal("expected an error for a meal with no boarder id")
	}
	if !errors.Is(err, ErrMissingData) {
		t.Errorf("expected ErrMissingData, got %v", err)
	}
}

func TestCheckMealWriteError(t *testing.T) {
	t.Run("empty body is success", func(t *testing.T) {
		if err := checkMealWriteError([]byte("")); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("non-JSON body is success", func(t *testing.T) {
		if err := checkMealWriteError([]byte("OK")); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("JSON without error field is success", func(t *testing.T) {
		if err := checkMealWriteError([]byte(`{"status":"ok"}`)); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("HTTP 200 with a non-empty error field fails", func(t *testing.T) {
		err := checkMealWriteError([]byte(`{"error":"Termin na zmenu objednavky vypršal"}`))
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("empty error field is success", func(t *testing.T) {
		if err := checkMealWriteError([]byte(`{"error":""}`)); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
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

// TestParseChangeDeadlineMinutePrecision guards the format that the real
// prihlas_do/odhlas_do fields use. Without it the deadline silently fails to
// parse, CanBeChanged stays false and ordering is locked at every school
// whose payload uses this shape.
func TestParseChangeDeadlineMinutePrecision(t *testing.T) {
	day := time.Date(2026, 9, 23, 0, 0, 0, 0, time.Local)

	got := parseChangeDeadline("2026-09-23 07:30", day)
	if got == nil {
		t.Fatal("expected a parsed deadline for a minute-precision timestamp")
	}
	if got.Hour() != 7 || got.Minute() != 30 || got.Day() != 23 {
		t.Errorf("parsed %v, want 2026-09-23 07:30", got)
	}
}

// TestFirstChangeDeadlineFallsBack covers the observed case where zmen_do is
// null but prihlas_do carries the real cut-off.
func TestFirstChangeDeadlineFallsBack(t *testing.T) {
	day := time.Date(2026, 9, 23, 0, 0, 0, 0, time.Local)

	meal := map[string]any{
		"zmen_do":    nil,
		"prihlas_do": "2026-09-23 07:30",
		"odhlas_do":  "2026-09-23 07:30",
	}
	got := firstChangeDeadline(meal, day, "zmen_do", "prihlas_do", "odhlas_do")
	if got == nil {
		t.Fatal("expected the prihlas_do fallback to supply a deadline")
	}

	if none := firstChangeDeadline(map[string]any{}, day, "zmen_do", "prihlas_do"); none != nil {
		t.Errorf("expected nil when no key carries a deadline, got %v", none)
	}
}

// TestParseChoosableMenusUsesMenusObject checks that orderable menus come
// from the "menus" object (which carries the letter) rather than the flat
// "rows" course list, and that choosableMenus is honoured.
func TestParseChoosableMenusUsesMenusObject(t *testing.T) {
	meal := map[string]any{
		"menus": map[string]any{
			"1": map[string]any{"skratkaMenu": "A", "nazovMenu": "Menu A", "nazov": "Soup\nChicken"},
			"2": map[string]any{"skratkaMenu": "B", "nazovMenu": "Menu B", "nazov": "Salad"},
		},
		"choosableMenus": map[string]any{"1": true, "2": false},
	}

	menus := parseChoosableMenus(meal)
	if len(menus) != 2 {
		t.Fatalf("expected 2 menus, got %d", len(menus))
	}
	if menus[0].Number != "A" || menus[0].OrderIndex != "1" || !menus[0].Choosable {
		t.Errorf("menu 0 = %+v, want orderable Menu A", menus[0])
	}
	if menus[1].Number != "B" || menus[1].Choosable {
		t.Errorf("menu 1 = %+v, want Menu B marked not choosable", menus[1])
	}

	if got := parseChoosableMenus(map[string]any{}); got != nil {
		t.Errorf("expected nil when there is no menus object, got %v", got)
	}
}
