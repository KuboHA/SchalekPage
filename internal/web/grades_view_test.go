package web

import (
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

func TestBuildGradesPageViewClassicGrades(t *testing.T) {
	grades := []edupage.Grade{
		{Subject: "Math", GradeN: "1", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Subject: "Math", GradeN: "3", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
	view := buildGradesPageView(grades)

	if len(view.Subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(view.Subjects))
	}
	sv := view.Subjects[0]
	if sv.Subject != "Math" {
		t.Fatalf("expected Math, got %q", sv.Subject)
	}
	if !sv.HasAverage {
		t.Fatal("expected an average to be computed")
	}
	if sv.Average != 2 {
		t.Errorf("expected average 2, got %v", sv.Average)
	}
	if !view.HasOverall || view.OverallInt != 2 {
		t.Errorf("expected overall 2, got HasOverall=%v Overall=%v", view.HasOverall, view.Overall)
	}
}

func TestBuildGradesPageViewPercentGrades(t *testing.T) {
	grades := []edupage.Grade{
		{Subject: "Biology", MaxPoints: 100, Percent: 95, Date: time.Now()},
	}
	view := buildGradesPageView(grades)
	sv := view.Subjects[0]
	if !sv.HasPercent {
		t.Fatal("expected a percent average")
	}
	if sv.PercentAvg != 95 {
		t.Errorf("expected percent avg 95, got %v", sv.PercentAvg)
	}
	// >=90% maps to points_grade 1.0, and with no classic grades in the
	// subject, the subject average is exactly that points grade.
	if sv.Average != 1 {
		t.Errorf("expected average 1, got %v", sv.Average)
	}
}

func TestBuildGradesPageViewGroupsAlphabetically(t *testing.T) {
	grades := []edupage.Grade{
		{Subject: "Zoology", GradeN: "1", Date: time.Now()},
		{Subject: "Art", GradeN: "2", Date: time.Now()},
		{Subject: "Math", GradeN: "3", Date: time.Now()},
	}
	view := buildGradesPageView(grades)
	if len(view.Subjects) != 3 {
		t.Fatalf("expected 3 subjects, got %d", len(view.Subjects))
	}
	want := []string{"Art", "Math", "Zoology"}
	for i, w := range want {
		if view.Subjects[i].Subject != w {
			t.Errorf("subject[%d] = %q, want %q", i, view.Subjects[i].Subject, w)
		}
	}
}

func TestBuildGradesPageViewEmpty(t *testing.T) {
	view := buildGradesPageView(nil)
	if len(view.Subjects) != 0 {
		t.Fatalf("expected no subjects, got %d", len(view.Subjects))
	}
	if view.HasOverall {
		t.Fatal("expected no overall average for an empty grade list")
	}
}

func TestBuildGradesPageViewVerbalGradeDoesNotCrash(t *testing.T) {
	grades := []edupage.Grade{
		{Subject: "Art", GradeN: "excellent", Verbal: true, Date: time.Now()},
	}
	view := buildGradesPageView(grades)
	sv := view.Subjects[0]
	if sv.HasAverage {
		t.Error("a purely verbal grade should not produce a numeric average")
	}
	if sv.Grades[0].GradeDisplay != "excellent" {
		t.Errorf("expected verbal grade text preserved, got %q", sv.Grades[0].GradeDisplay)
	}
}
