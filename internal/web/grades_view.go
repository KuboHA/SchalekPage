package web

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// gradeView is a presentation-ready grade.
type gradeView struct {
	GradeDisplay string // "2" for a classic grade, or a rounded percent for point-based ones
	Comment      string
	Date         time.Time
	Title        string
	Teacher      string
	MaxPoints    float64
	Percent      float64
	IsPercent    bool
	ColorClass   string
}

// gradeSubjectView groups a subject's grades together with a computed
// average badge.
type gradeSubjectView struct {
	Subject    string
	Grades     []gradeView
	HasAverage bool
	Average    float64
	AvgClass   string
	PercentAvg float64
	HasPercent bool
}

// gradesPageView is the fully precomputed view model for grades.html.
type gradesPageView struct {
	Subjects   []gradeSubjectView
	HasOverall bool
	Overall    float64
	OverallInt int
	OverallCls string
}

// Grade badges are rendered as five steps of a grayscale ramp (see the
// .sw-grade rules in static/css/main.css): level 1 is a solid inverted
// block, level 5 a heavy hairline outline. Severity therefore survives the
// grayscale palette without relying on hue, which also keeps the badges
// legible for readers who can't distinguish red from green.
func gradeStep(level int) string {
	return fmt.Sprintf("sw-grade sw-grade--%d", level)
}

// gradeColorClass maps a 1-5 style grade onto that ramp, best to worst.
func gradeColorClass(n float64) string {
	switch {
	case n >= 5:
		return gradeStep(5)
	case n >= 4:
		return gradeStep(4)
	case n >= 3:
		return gradeStep(3)
	case n >= 2:
		return gradeStep(2)
	case n >= 1:
		return gradeStep(1)
	default:
		return "sw-grade sw-grade--none"
	}
}

// percentColorClass maps a percentage onto the same ramp. Note the
// inversion: a high percentage is good, a high grade number is not.
func percentColorClass(percent float64) string {
	switch {
	case percent >= 90:
		return gradeStep(1)
	case percent >= 75:
		return gradeStep(2)
	case percent >= 50:
		return gradeStep(3)
	default:
		return gradeStep(5)
	}
}

// averageBadgeClass maps a subject or overall average onto the ramp.
func averageBadgeClass(avg float64) string {
	switch {
	case avg >= 4:
		return gradeStep(5)
	case avg >= 3:
		return gradeStep(4)
	case avg >= 2:
		return gradeStep(3)
	default:
		return gradeStep(1)
	}
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}

// termLabel renders an edupage.Term for the page eyebrow.
func termLabel(t edupage.Term) string {
	if t == edupage.TermFirst {
		return "First term"
	}
	return "Second term"
}

// gradeNumeric parses a Grade's GradeN field as a float, ignoring verbal
// grades that don't parse cleanly (they contribute to neither average, as
// in app.py where non-numeric grade_n values fail the `<= 5` comparison via
// exception-free duck typing — here we simply skip them).
func gradeNumeric(g edupage.Grade) (float64, bool) {
	n, err := strconv.ParseFloat(g.GradeN, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// buildGradesPageView groups grades by subject (sorted alphabetically, as
// Jinja's `groupby` filter sorts before grouping) and computes per-subject
// and overall averages, porting the arithmetic embedded in grades.html.
//
// Unlike app.py's template, which references the `total_avg` accumulator
// before it is defined (a bug that would raise a Jinja UndefinedError on
// any non-empty grade list), the average is computed here in Go before
// rendering, so it is always well-defined.
func buildGradesPageView(grades []edupage.Grade) gradesPageView {
	sorted := make([]edupage.Grade, len(grades))
	copy(sorted, grades)
	// Newest first within a subject, as app.py sorts grades by date
	// descending before Jinja's groupby (a stable sort) groups them by
	// subject.
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Date.After(sorted[j].Date) })
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Subject < sorted[j].Subject })

	var view gradesPageView
	var overallSum float64
	var overallCount int

	i := 0
	for i < len(sorted) {
		j := i
		subject := sorted[i].Subject
		for j < len(sorted) && sorted[j].Subject == subject {
			j++
		}
		group := sorted[i:j]

		var sv gradeSubjectView
		sv.Subject = subject

		var sum, pointsSum, maxPointsSum float64
		var count int

		for _, g := range group {
			gv := gradeView{
				Comment:   g.Comment,
				Date:      g.Date,
				Title:     g.Title,
				Teacher:   g.Teacher,
				MaxPoints: g.MaxPoints,
				Percent:   g.Percent,
				IsPercent: g.MaxPoints > 0,
			}
			if gv.IsPercent {
				gv.GradeDisplay = strconv.FormatFloat(round2(g.Percent), 'f', -1, 64) + "%"
				gv.ColorClass = percentColorClass(g.Percent)
				pointsSum += g.MaxPoints * g.Percent / 100
				maxPointsSum += g.MaxPoints
			} else if n, ok := gradeNumeric(g); ok {
				gv.GradeDisplay = strconv.Itoa(int(n))
				gv.ColorClass = gradeColorClass(n)
				if n <= 5 {
					sum += n
					count++
				}
			} else {
				gv.GradeDisplay = g.GradeN
				gv.ColorClass = gradeColorClass(-1)
			}
			sv.Grades = append(sv.Grades, gv)
		}

		if count > 0 || maxPointsSum > 0 {
			gradeAvg := 0.0
			if count > 0 {
				gradeAvg = sum / float64(count)
			}
			if maxPointsSum > 0 {
				percent := round2(pointsSum / maxPointsSum * 100)
				sv.PercentAvg = percent
				sv.HasPercent = true

				pointsGrade := 5.0
				switch {
				case percent >= 90:
					pointsGrade = 1.0
				case percent >= 75:
					pointsGrade = 2.0
				case percent >= 50:
					pointsGrade = 3.0
				case percent >= 35:
					pointsGrade = 4.0
				}

				if count > 0 {
					gradeAvg = (gradeAvg + pointsGrade) / 2
				} else {
					gradeAvg = pointsGrade
				}
			}

			avg := round2(gradeAvg)
			sv.Average = avg
			sv.HasAverage = true
			sv.AvgClass = averageBadgeClass(avg)

			overallSum += avg
			overallCount++
		}

		view.Subjects = append(view.Subjects, sv)
		i = j
	}

	if overallCount > 0 {
		overall := round2(overallSum / float64(overallCount))
		view.Overall = overall
		view.OverallInt = int(overall)
		view.OverallCls = averageBadgeClass(overall)
		view.HasOverall = true
	}

	return view
}
