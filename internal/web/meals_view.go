package web

import "github.com/KuboHA/SchalekPage/internal/edupage"

// mealView is a presentation-ready meal, labelled the way app.py labelled
// its ad-hoc dicts ("Snack" / "Lunch" / "Afternoon Snack"), since
// edupage.Meal itself carries no such label.
type mealView struct {
	Type        string
	Menus       []edupage.Menu
	OrderedMeal string
}

// buildMealViews turns edupage.Meals into an ordered list of mealViews,
// skipping meals the day doesn't offer, porting app.py's meals_list
// construction (used by both the dashboard and the /lunches page).
func buildMealViews(meals *edupage.Meals) []mealView {
	if meals == nil {
		return nil
	}
	var out []mealView
	add := func(label string, m *edupage.Meal) {
		if m == nil {
			return
		}
		ordered := m.OrderedMeal
		if ordered == "" {
			ordered = "X"
		}
		out = append(out, mealView{Type: label, Menus: m.Menus, OrderedMeal: ordered})
	}
	add("Snack", meals.Snack)
	add("Lunch", meals.Lunch)
	add("Afternoon Snack", meals.AfternoonSnack)
	return out
}

// menuOrdered reports whether a menu option within a meal is the one the
// student ordered, matching app.py's comparisons against 'A'/'I.' and
// 'B'/'II.'.
func menuOrdered(m mealView, menu edupage.Menu) bool {
	return (m.OrderedMeal == "A" && menu.Number == "I.") || (m.OrderedMeal == "B" && menu.Number == "II.")
}
