package web

import (
	"net/url"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// mealView is a presentation-ready meal, labelled the way app.py labelled
// its ad-hoc dicts ("Snack" / "Lunch" / "Afternoon Snack"), since
// edupage.Meal itself carries no such label.
type mealView struct {
	Type        string
	Menus       []edupage.Menu
	OrderedMeal string

	// The fields below drive the ordering controls (feature 14). Index is
	// EduPage's meal index ("1"/"2"/"3") and is what a write call needs.
	Index string
	Date  time.Time
	// Changeable is false once the canteen's deadline has passed; the page
	// must then render the current choice read-only rather than offering
	// controls that are guaranteed to fail server-side.
	Changeable    bool
	DeadlineLabel string
	HasOrder      bool
	// IsSignedOff is true when the recorded choice is an explicit "I will
	// not eat this" (EduPage's mealSignOffCode, "AX") rather than a chosen
	// menu — the two must never be displayed as the same "Ordered" state.
	IsSignedOff bool
}

// mealSignOffCode mirrors edupage's own (unexported) mealSignOffCode: the
// literal OrderedMeal value EduPage reports once the student has explicitly
// signed off a meal, as opposed to simply never having ordered one (which
// reports as "").
const mealSignOffCode = "AX"

// mealChangeable reports whether a meal's order can still be changed at
// `now`. It respects an explicit deadline when EduPage published one and
// otherwise falls back to the coarse CanBeChanged flag.
func mealChangeable(m *edupage.Meal, now time.Time) bool {
	if m == nil {
		return false
	}
	if m.CanBeChangedUntil != nil {
		return now.Before(*m.CanBeChangedUntil)
	}
	return m.CanBeChanged
}

// urlQueryEscape is a small wrapper so handlers can build redirect targets
// without importing net/url directly.
func urlQueryEscape(s string) string { return url.QueryEscape(s) }

// buildMealViews turns edupage.Meals into an ordered list of mealViews,
// skipping meals the day doesn't offer, porting app.py's meals_list
// construction (used by both the dashboard and the /lunches page).
func buildMealViews(meals *edupage.Meals, now time.Time) []mealView {
	if meals == nil {
		return nil
	}
	var out []mealView
	add := func(label, index string, m *edupage.Meal) {
		if m == nil {
			return
		}
		raw := m.OrderedMeal
		ordered := raw
		if ordered == "" {
			ordered = "X"
		}
		mv := mealView{
			Type:        label,
			Menus:       m.Menus,
			OrderedMeal: ordered,
			Index:       index,
			Date:        m.Date,
			Changeable:  mealChangeable(m, now),
			HasOrder:    raw != "",
			IsSignedOff: raw == mealSignOffCode,
		}
		if m.CanBeChangedUntil != nil {
			mv.DeadlineLabel = m.CanBeChangedUntil.Format("02 Jan 2006 15:04")
		}
		out = append(out, mv)
	}
	add("Snack", "1", meals.Snack)
	add("Lunch", "2", meals.Lunch)
	add("Afternoon Snack", "3", meals.AfternoonSnack)
	return out
}

// menuOrdered reports whether a menu option within a meal is the one the
// student ordered. edupage.Menu.Number now carries EduPage's own letter
// code ("A", "B", ...) for a real choosable menu variant — the same
// alphabet Meal.OrderedMeal is recorded in — so a direct comparison works
// for all 8 possible menus, not just the first two. A plain course row
// (soup, side dish, ...) has no OrderIndex and an empty or roman-numeral
// Number, which never matches a recorded letter, so it is correctly never
// reported as "ordered".
func menuOrdered(m mealView, menu edupage.Menu) bool {
	return menu.Number != "" && menu.Number == m.OrderedMeal
}

// choosableMenuCount returns how many of a meal's Menus are real, pickable
// order targets rather than one of the flat course rows edupage.Meal.Menus
// also carries. edupage.ChooseMeal's menuNumber argument is a 1-based
// position among the choosable entries only, so this — not len(menus) — is
// the upper bound a submitted choice must be validated against.
func choosableMenuCount(menus []edupage.Menu) int {
	n := 0
	for _, m := range menus {
		if m.OrderIndex != "" && m.Choosable {
			n++
		}
	}
	return n
}
