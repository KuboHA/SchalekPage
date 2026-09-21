package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// mealByIndex returns the meal with the given EduPage meal index ("1" snack,
// "2" lunch, "3" afternoon snack) from a day's meals.
func mealByIndex(meals *edupage.Meals, index string) *edupage.Meal {
	if meals == nil {
		return nil
	}
	switch strings.TrimSpace(index) {
	case "1":
		return meals.Snack
	case "2":
		return meals.Lunch
	case "3":
		return meals.AfternoonSnack
	default:
		return nil
	}
}

// errInvalidMenuNumber is returned by parseMenuNumber for anything that
// isn't one of EduPage's supported 1..8 menu choices.
var errInvalidMenuNumber = errors.New("choose one of the listed menus")

// parseMenuNumber validates the "menu" form field submitted by the order
// form: it must be an integer in EduPage's 1..8 range (edupage.ChooseMeal
// rejects anything else outright). Kept separate from handleLunchOrder so
// it's unit-testable without a live *edupage.Client.
func parseMenuNumber(raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 || n > 8 {
		return 0, errInvalidMenuNumber
	}
	return n, nil
}

// lunchRedirect sends the user back to the lunches page for a date, carrying
// an outcome message. Ordering is a POST, so redirecting afterwards keeps a
// refresh from re-submitting the order (POST/redirect/GET).
func (s *Server) lunchRedirect(w http.ResponseWriter, r *http.Request, day time.Time, status, msg string) {
	target := "/lunches/" + day.Format(dateLayout) + "?" + status + "=" + urlQueryEscape(msg)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// resolveOrderTarget parses the shared form fields of both ordering actions
// and loads the meal being changed. It returns a user-facing error message
// when the request cannot be honoured.
func (s *Server) resolveOrderTarget(r *http.Request, sess *Session) (day time.Time, meal *edupage.Meal, errMsg string) {
	now := s.now()
	day = parseDateOrToday(r.FormValue("date"), now)

	meals, err := sess.Client.MealsFor(day)
	if err != nil {
		s.logger.Warn("lunch order: fetch meals failed", "date", day, "error", err)
		return day, nil, "Could not load the menu for that day."
	}

	meal = mealByIndex(meals, r.FormValue("meal"))
	if meal == nil {
		return day, nil, "That meal is not served on this day."
	}
	if !mealChangeable(meal, now) {
		return day, nil, "The ordering deadline for this meal has passed."
	}
	return day, meal, ""
}

// handleLunchOrder serves POST /lunches/order. This places a REAL order in
// the school canteen, so it is deliberately a POST with an explicit confirm
// in the template, never a link.
func (s *Server) handleLunchOrder(w http.ResponseWriter, r *http.Request, sess *Session) {
	if err := r.ParseForm(); err != nil {
		s.lunchRedirect(w, r, s.now(), "error", "Malformed request.")
		return
	}

	number, err := parseMenuNumber(r.FormValue("menu"))
	if err != nil {
		s.lunchRedirect(w, r, parseDateOrToday(r.FormValue("date"), s.now()), "error", err.Error())
		return
	}

	day, meal, errMsg := s.resolveOrderTarget(r, sess)
	if errMsg != "" {
		s.lunchRedirect(w, r, day, "error", errMsg)
		return
	}

	if number > choosableMenuCount(meal.Menus) {
		s.lunchRedirect(w, r, day, "error", "That menu option is no longer available.")
		return
	}

	if err := sess.Client.ChooseMeal(meal, number); err != nil {
		// EduPage reports write failures in the body with HTTP 200; the
		// client layer turns that into an error, so never swallow it.
		s.logger.Warn("lunch order failed", "date", day, "menu", number, "error", err)
		s.lunchRedirect(w, r, day, "error", "EduPage rejected the order: "+err.Error())
		return
	}

	s.lunchRedirect(w, r, day, "ok", "Order placed.")
}

// handleLunchSignOff serves POST /lunches/signoff, cancelling an order.
func (s *Server) handleLunchSignOff(w http.ResponseWriter, r *http.Request, sess *Session) {
	if err := r.ParseForm(); err != nil {
		s.lunchRedirect(w, r, s.now(), "error", "Malformed request.")
		return
	}

	day, meal, errMsg := s.resolveOrderTarget(r, sess)
	if errMsg != "" {
		s.lunchRedirect(w, r, day, "error", errMsg)
		return
	}

	if err := sess.Client.SignOffMeal(meal); err != nil {
		s.logger.Warn("lunch sign-off failed", "date", day, "error", err)
		s.lunchRedirect(w, r, day, "error", "EduPage rejected the cancellation: "+err.Error())
		return
	}

	s.lunchRedirect(w, r, day, "ok", "Order cancelled.")
}
