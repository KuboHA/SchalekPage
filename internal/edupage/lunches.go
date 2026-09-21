package edupage

import (
	"encoding/json"
	"fmt"
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

	var menus []Menu
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
		CanBeChanged: canStillChangeMeal(pickString(meal["zmen_do"]), day),
		OrderedMeal:  orderedMeal,
		Title:        pickString(meal["nazov"]),
		MenuID:       index,
		Date:         day,
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
// EduPage sent for a meal is still in the future. The Python reference keeps
// this as a raw, unparsed value; this project's Meal.CanBeChanged is a bool,
// so several plausible timestamp shapes are tried and the deadline defaults
// to "already passed" when none of them parse or the field is empty.
func canStillChangeMeal(raw string, day time.Time) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}

	now := time.Now()

	if t, err := time.ParseInLocation(eduTimeLayout, raw, now.Location()); err == nil {
		return now.Before(t)
	}
	if t, err := time.ParseInLocation(eduDateLayout, raw, now.Location()); err == nil {
		return now.Before(t.AddDate(0, 0, 1))
	}
	if t, err := time.ParseInLocation("15:04:05", raw, now.Location()); err == nil {
		deadline := time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
		return now.Before(deadline)
	}
	if t, err := time.ParseInLocation("15:04", raw, now.Location()); err == nil {
		deadline := time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
		return now.Before(deadline)
	}

	return false
}
