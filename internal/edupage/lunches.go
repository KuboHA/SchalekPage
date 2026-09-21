package edupage

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MealsFor returns the meals (snack / lunch / afternoon snack) EduPage's
// canteen module has for day. It returns (nil, nil) when there is simply no
// menu data for that day.
func (c *Client) MealsFor(day time.Time) (*Meals, error) {
	path := fmt.Sprintf("/menu/?date=%s", day.Format("20060102"))
	body, err := c.Get(path)
	if err != nil {
		return nil, fmt.Errorf("edupage: fetch meals: %w", err)
	}

	dayMeals, boarderID, ok := extractDayMeals(string(body), c.Subdomain(), day)
	if !ok {
		return nil, nil
	}

	return &Meals{
		Snack:          parseMeal("1", dayMeals["1"], boarderID, day),
		Lunch:          parseMeal("2", dayMeals["2"], boarderID, day),
		AfternoonSnack: parseMeal("3", dayMeals["3"], boarderID, day),
	}, nil
}

// extractDayMeals decodes the `edupageData: {...}` JS literal embedded in the
// /menu/ page and returns the meals object for day plus the boarder id
// needed to change an order, mirroring Lunches.get_meals from the Python
// reference. ok is false when the page doesn't have the expected shape or
// there is no menu published for that day.
func extractDayMeals(page, subdomain string, day time.Time) (map[string]any, string, bool) {
	const marker = "edupageData: "
	idx := strings.Index(page, marker)
	if idx < 0 {
		return nil, "", false
	}
	rest := page[idx+len(marker):]

	end := strings.Index(rest, ",\r\n")
	if end < 0 {
		end = strings.Index(rest, ",\n")
	}
	if end < 0 {
		return nil, "", false
	}
	jsonPart := rest[:end]

	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonPart), &payload); err != nil {
		return nil, "", false
	}

	schoolData := pickMap(payload[subdomain])
	if schoolData == nil {
		return nil, "", false
	}

	novyListok := pickMap(schoolData["novyListok"])
	if novyListok == nil {
		return nil, "", false
	}

	var boarderID string
	if addInfo := pickMap(novyListok["addInfo"]); addInfo != nil {
		boarderID = pickString(addInfo["stravnikid"])
	}

	dayMeals := pickMap(novyListok[day.Format(eduDateLayout)])
	if dayMeals == nil {
		return nil, "", false
	}

	return dayMeals, boarderID, true
}

// parseMeal converts one raw meal entry (keyed "1", "2" or "3" for snack,
// lunch and afternoon snack respectively) into a *Meal. It returns nil when
// there is no entry, or when EduPage marks the meal as not being cooked that
// day (isCooking == false), mirroring Lunches.parse_meal.
func parseMeal(index string, raw any, boarderID string, day time.Time) *Meal {
	meal := pickMap(raw)
	if meal == nil {
		return nil
	}

	if cooking, has := meal["isCooking"]; has {
		if b, ok := cooking.(bool); ok && !b {
			return nil
		}
	}

	orderedMeal := ""
	if record := pickMap(meal["evidencia"]); record != nil {
		state := pickString(record["stav"])
		if state == "V" {
			orderedMeal = pickString(record["obj"])
		} else {
			orderedMeal = state
		}
	}

	amount, _ := pickInt(meal["druhov_jedal"])

	// Which key carries the ordering deadline varies by school. Observed in
	// the wild: "zmen_do" is null at some schools while "prihlas_do" (sign-up
	// by) and "odhlas_do" (sign-off by) carry the real cut-off. Reading only
	// "zmen_do" leaves CanBeChanged permanently false, which silently locks
	// ordering for every such school, so try each in turn.
	deadline := firstChangeDeadline(meal, day, "zmen_do", "prihlas_do", "odhlas_do")

	// EduPage describes the day's food twice: a flat "rows" list (every
	// course, with no menu letter) and a "menus" object keyed "1"/"2" whose
	// entries carry the letter a diner actually orders. Only the latter is
	// orderable, and at some schools "rows" has no menusStr at all, so the
	// menus object is parsed first and the flat rows only supplement it.
	menus := parseChoosableMenus(meal)

	for _, rowRaw := range pickSlice(meal["rows"]) {
		food := pickMap(rowRaw)
		if food == nil {
			continue
		}

		number := strings.ReplaceAll(pickString(food["menusStr"]), ": ", "")

		var rating *MealRating
		if number != "" {
			rating = parseMealRating(meal, number, day)
		}

		menus = append(menus, Menu{
			Name:      pickString(food["nazov"]),
			Allergens: pickString(food["alergenyStr"]),
			Weight:    pickString(food["hmotnostiStr"]),
			Number:    number,
			Rating:    rating,
		})
	}

	return &Meal{
		// The meal is only ever constructed when EduPage reports it as
		// cooking; a day with no canteen service simply has a nil *Meal.
		Served:       true,
		Amount:       amount,
		Menus:        menus,
		CanBeChanged: deadline != nil && time.Now().Before(*deadline),
		OrderedMeal:  orderedMeal,
		Title:        pickString(meal["nazov"]),
		MenuID:       index,
		Date:         day,

		BoarderID:         boarderID,
		CanBeChangedUntil: deadline,
		MealIndex:         index,
	}
}

// parseMealRating parses the boarder rating stats ("hodnotenia") for one
// specific menu number. The Python reference stores a [quality, quantity]
// pair per menu number; this ports "quantity" onto MealRating's Boards*
// fields to match this project's naming in types.go.
func parseMealRating(meal map[string]any, number string, day time.Time) *MealRating {
	ratings := pickMap(meal["hodnotenia"])
	if ratings == nil {
		return nil
	}

	entry := pickSlice(ratings[number])
	if len(entry) < 2 {
		return nil
	}

	quality := pickMap(entry[0])
	quantity := pickMap(entry[1])
	if quality == nil || quantity == nil {
		return nil
	}

	qualityAvg, _ := pickFloat(quality["priemer"])
	qualityCount, _ := pickInt(quality["pocet"])
	quantityAvg, _ := pickFloat(quantity["priemer"])
	quantityCount, _ := pickInt(quantity["pocet"])

	return &MealRating{
		Date: day.Format(eduDateLayout),
		// Boards* is a types.go naming artifact: it holds the "quantity"
		// (mnozstvo) ratings from EduPage's [quality, quantity] pair, not a
		// literal notion of "boards".
		BoardsAverage:  formatFloat(quantityAvg),
		QualityAverage: formatFloat(qualityAvg),
		BoardsRatings:  quantityCount,
		QualityRatings: qualityCount,
	}
}

// canStillChangeMeal reports whether the "zmen_do" (change-until) deadline
// EduPage sent for a meal is still in the future.
func canStillChangeMeal(raw string, day time.Time) bool {
	deadline := parseChangeDeadline(raw, day)
	return deadline != nil && time.Now().Before(*deadline)
}

// parseChangeDeadline parses the "zmen_do" (change-until) deadline EduPage
// sent for a meal into an absolute time. The Python reference keeps this as
// a raw, unparsed value; EduPage is inconsistent about its shape, so several
// plausible layouts are tried in turn. It returns nil when raw is empty or
// none of them parse, meaning "no known deadline" (treated as already passed
// by callers that need a bool, e.g. canStillChangeMeal).
func parseChangeDeadline(raw string, day time.Time) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	loc := time.Now().Location()

	if t, err := time.ParseInLocation(eduTimeLayout, raw, loc); err == nil {
		return &t
	}
	// Minute-precision timestamps ("2026-09-23 07:30") are what the
	// prihlas_do/odhlas_do fields actually carry; without this the deadline
	// silently fails to parse and ordering stays locked.
	if t, err := time.ParseInLocation("2006-01-02 15:04", raw, loc); err == nil {
		return &t
	}
	if t, err := time.ParseInLocation(eduDateLayout, raw, loc); err == nil {
		t = t.AddDate(0, 0, 1)
		return &t
	}
	if t, err := time.ParseInLocation("15:04:05", raw, loc); err == nil {
		deadline := time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
		return &deadline
	}
	if t, err := time.ParseInLocation("15:04", raw, loc); err == nil {
		deadline := time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		return &deadline
	}

	return nil
}

// mealSignOffCode is the literal choice string EduPage uses to mean "I will
// not be eating this meal", as opposed to a lettered menu choice.
const mealSignOffCode = "AX"

// menuLetters maps a 1-based menu number to EduPage's lettered menu choice
// code, as used in the "jids" map of ulozJedlaStravnika: 1 -> "A", 2 -> "B",
// … 8 -> "H". EduPage never offers more than 8 menu options.
const menuLetters = "ABCDEFGH"

// menuLetter converts a 1..8 menu number into its EduPage letter code.
func menuLetter(menuNumber int) (string, error) {
	if menuNumber < 1 || menuNumber > len(menuLetters) {
		return "", fmt.Errorf("edupage: menu number %d out of range (1-%d)", menuNumber, len(menuLetters))
	}
	return string(menuLetters[menuNumber-1]), nil
}

// ChooseMeal orders menu number menuNumber (1..8) of m for the diner it
// belongs to. It fails if menuNumber is out of range or the order can no
// longer be changed (see Meal.CanBeChangedUntil).
func (c *Client) ChooseMeal(m *Meal, menuNumber int) error {
	letter, err := menuLetter(menuNumber)
	if err != nil {
		return fmt.Errorf("edupage: choose meal: %w", err)
	}

	values, err := buildMealChoiceRequest(m, letter)
	if err != nil {
		return fmt.Errorf("edupage: choose meal: %w", err)
	}

	body, err := c.PostForm("/menu/", values)
	if err != nil {
		return fmt.Errorf("edupage: choose meal: %w", err)
	}
	if err := checkMealWriteError(body); err != nil {
		return fmt.Errorf("edupage: choose meal: %w", err)
	}
	return nil
}

// SignOffMeal cancels any order for m ("I will not eat this"). It fails if
// the order can no longer be changed (see Meal.CanBeChangedUntil).
func (c *Client) SignOffMeal(m *Meal) error {
	values, err := buildMealChoiceRequest(m, mealSignOffCode)
	if err != nil {
		return fmt.Errorf("edupage: sign off meal: %w", err)
	}

	body, err := c.PostForm("/menu/", values)
	if err != nil {
		return fmt.Errorf("edupage: sign off meal: %w", err)
	}
	if err := checkMealWriteError(body); err != nil {
		return fmt.Errorf("edupage: sign off meal: %w", err)
	}
	return nil
}

// buildMealChoiceRequest builds the form values EduPage's /menu/ endpoint
// expects for akcia=ulozJedlaStravnika: a single field, "jedlaStravnika",
// itself a JSON-encoded object naming the diner, the date, and a one-entry
// map from meal index to the chosen letter (or mealSignOffCode). Factored
// out from ChooseMeal/SignOffMeal so the request shape and the validation
// below are testable without a network round trip.
func buildMealChoiceRequest(m *Meal, choice string) (url.Values, error) {
	if m == nil {
		return nil, fmt.Errorf("meal is nil")
	}
	if m.BoarderID == "" {
		return nil, fmt.Errorf("%w: meal has no boarder id", ErrMissingData)
	}
	if m.MealIndex == "" {
		return nil, fmt.Errorf("%w: meal has no meal index", ErrMissingData)
	}
	if m.CanBeChangedUntil == nil || !time.Now().Before(*m.CanBeChangedUntil) {
		return nil, fmt.Errorf("order can no longer be changed (deadline has passed)")
	}

	payload := map[string]any{
		"stravnikid": m.BoarderID,
		"mysqlDate":  m.Date.Format(eduDateLayout),
		"jids":       map[string]string{m.MealIndex: choice},
		"view":       "pc_listok",
		"pravo":      "Student",
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode meal choice: %w", err)
	}

	return url.Values{
		"akcia":          {"ulozJedlaStravnika"},
		"jedlaStravnika": {string(encoded)},
	}, nil
}

// checkMealWriteError inspects a raw response from /menu/ for an
// application-level failure. EduPage answers this endpoint with HTTP 200
// whether or not the write succeeded; failure is signalled by a JSON object
// body with a non-empty "error" field instead. A body that isn't a JSON
// object, or is one without a non-empty "error" field, is treated as
// success — EduPage's own success replies from this endpoint are not
// reliably structured JSON either.
func checkMealWriteError(body []byte) error {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil
	}

	var v map[string]any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return nil
	}

	if msg := pickString(v["error"]); msg != "" {
		return fmt.Errorf("edupage rejected the request: %s", msg)
	}
	return nil
}

// firstChangeDeadline returns the first parseable deadline among the given
// keys, so schools that populate a different one than the reference
// implementation expects still get a working ordering window.
func firstChangeDeadline(meal map[string]any, day time.Time, keys ...string) *time.Time {
	for _, key := range keys {
		if d := parseChangeDeadline(pickString(meal[key]), day); d != nil {
			return d
		}
	}
	return nil
}

// parseChoosableMenus reads the "menus" object, whose entries are the only
// things a diner can actually order. Entries flagged false in
// "choosableMenus" are still returned (so the menu is visible) but marked
// not choosable, which keeps the UI honest about what can be picked.
func parseChoosableMenus(meal map[string]any) []Menu {
	raw := pickMap(meal["menus"])
	if raw == nil {
		return nil
	}
	choosable := pickMap(meal["choosableMenus"])

	// Keys are numeric strings ("1", "2", ...); order them so the rendered
	// list is stable rather than following Go's random map iteration.
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, errA := strconv.Atoi(keys[i])
		b, errB := strconv.Atoi(keys[j])
		if errA == nil && errB == nil {
			return a < b
		}
		return keys[i] < keys[j]
	})

	var out []Menu
	for _, k := range keys {
		entry := pickMap(raw[k])
		if entry == nil {
			continue
		}
		letter := strings.TrimSpace(pickString(entry["skratkaMenu"]))
		if letter == "" {
			continue
		}
		canChoose := true
		if choosable != nil {
			if v, present := choosable[k]; present {
				b, ok := v.(bool)
				canChoose = !ok || b
			}
		}
		out = append(out, Menu{
			Name:       strings.TrimSpace(pickString(entry["nazov"])),
			Allergens:  pickString(entry["alergenyStr"]),
			Weight:     pickString(entry["hmotnostiStr"]),
			Number:     letter,
			Label:      strings.TrimSpace(pickString(entry["nazovMenu"])),
			Choosable:  canChoose,
			OrderIndex: k,
		})
	}
	return out
}
