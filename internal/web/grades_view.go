package web

import (
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

// gradeColorClass returns the Tailwind classes app.py's grades.html used for
// a 1-5 style grade badge.
func gradeColorClass(n float64) string {
	switch {
	case n >= 5:
		return "bg-red-500/20 text-red-400"
	case n >= 4:
		return "bg-orange-500/20 text-orange-400"
	case n >= 3:
		return "bg-yellow-500/20 text-yellow-400"
	case n >= 2:
		return "bg-blue-500/20 text-blue-400"
	case n >= 1:
		return "bg-green-500/20 text-green-400"
	default:
		return "bg-gray-700/50 text-gray-300"
	}
}

// percentColorClass mirrors the percent-based badge coloring in grades.html.
func percentColorClass(percent float64) string {
	switch {
	case percent >= 90:
		return "bg-green-500/20 text-green-400"
	case percent >= 75:
		return "bg-blue-500/20 text-blue-400"
	case percent >= 50:
		return "bg-yellow-500/20 text-yellow-400"
	default:
		return "bg-red-500/20 text-red-400"
	}
}

// averageBadgeClass mirrors the subject/overall average badge coloring.
func averageBadgeClass(avg float64) string {
	switch {
	case avg >= 4:
		return "bg-red-500/20 text-red-400 shadow-red-500/10"
	case avg >= 3:
		return "bg-yellow-500/20 text-yellow-400 shadow-yellow-500/10"
	case avg >= 2:
		return "bg-blue-500/20 text-blue-400 shadow-blue-500/10"
	default:
		return "bg-green-500/20 text-green-400 shadow-green-500/10"
	}
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
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
