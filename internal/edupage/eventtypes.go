package edupage

import "strings"

// eventTypeNames ports the Python reference's EVENT_TYPE_MAP: a human-readable
// label for each EduPage timeline event type key. EduPage's timeline entries
// carry both the raw enum value (e.g. "sprava", "znamka") and, in some UI
// paths, a snake_cased name (e.g. "big_exam"); the original map contained
// keys in both forms, so this port keeps every key exactly as found.
var eventTypeNames = map[string]string{
	"album":                  "Album",
	"arrival_to_school":      "Arrival to School",
	"bee":                    "Bee",
	"big_exam":               "Big Exam",
	"booked_room":            "Booked Room",
	"change_room":            "Change Room",
	"classification_meeting": "Classification Meeting",
	"class_book":             "Class Book",
	"class_teacher_event":    "Class Teacher Event",
	"class_teacher_lesson":   "Class Teacher Lesson",
	"confirmation":           "Confirmation",
	"contest":                "Contest",
	"culture":                "Culture",
	"distant_learning":       "Distant Learning",
	"event":                  "Event",
	"exam_assignment":        "Exam Assignment",
	"exam_evaluation":        "Exam Evaluation",
	"excursion":              "Excursion",
	"excused_lesson":         "Excused Lesson",
	"food_credit":            "Food Credit",
	"strava_vydaj":           "Food Served",
	"free_day":               "Free Day",
	"grade":                  "Grade",
	"grades_doc":             "Grades Document",
	"holiday":                "Holiday",
	"homework":               "Homework",
	"homework_student_state": "Homework Student State",
	"homework_test":          "Homework Test",
	"h_attendance":           "Attendance",
	"h_bee":                  "Bee",
	"h_clearcache":           "Clear Cache",
	"h_cleardbi":             "Clear DBI",
	"h_clearisicdata":        "Clear ISIC Data",
	"h_contest":              "Contest",
	"h_dailyplan":            "Daily Plan",
	"h_edusettings":          "Edu Settings",
	"h_financie":             "Finances",
	"h_grades":               "Grades",
	"h_homework":             "Homework",
	"h_igroups":              "Groups",
	"h_process":              "Process",
	"h_processtypes":         "Process Types",
	"h_settings":             "Settings",
	"h_substitution":         "Substitution",
	"h_timetable":            "Timetable",
	"h_userphoto":            "User Photo",
	"h_stravamenu":           "Strava Menu",
	"pipnutie":               "Check In/Out",
	"h_clearplany":           "Clear Plans",
	"sprava":                 "Message",
	"lesson":                 "Lesson",
	"message":                "Message",
	"news":                   "News",
	"new_menu":               "New Menu",
	"oral_exam":              "Oral Exam",
	"other":                  "Other",
	"paper":                  "Paper",
	"parents_evening":        "Parents' Evening",
	"poll":                   "Poll",
	"process":                "Process",
	"project":                "Project",
	"project_exam":           "Project Exam",
	"project_lesson":         "Project Lesson",
	"representation":         "Representation",
	"safety_instructioning":  "Safety Instructioning",
	"school_event":           "School Event",
	"school_trip":            "School Trip",
	"short_exam":             "Short Exam",
	"short_holiday":          "Short Holiday",
	"student_absent":         "Student Absent",
	"substitution":           "Substitution",
	"teacher_meeting":        "Teacher Meeting",
	"testing":                "Testing",
	"test_result":            "Test Result",
	"testpridelenie":         "Assigned Test",
	"timetable":              "Timetable",
	"tt_cancel":              "Timetable Cancel",
	"tutoring":               "Tutoring",
	"znamka":                 "New Grade",
}

// eventTypeIcons ports the Python reference's EVENT_TYPE_ICONS: a
// Font Awesome icon class for each EduPage timeline event type key.
var eventTypeIcons = map[string]string{
	"album":                  "fa-photo-video",
	"arrival_to_school":      "fa-school",
	"bee":                    "fa-bee",
	"big_exam":               "fa-file-alt",
	"booked_room":            "fa-door-closed",
	"change_room":            "fa-exchange-alt",
	"classification_meeting": "fa-users",
	"class_book":             "fa-book",
	"class_teacher_event":    "fa-chalkboard-teacher",
	"class_teacher_lesson":   "fa-chalkboard",
	"confirmation":           "fa-check-circle",
	"contest":                "fa-trophy",
	"culture":                "fa-theater-masks",
	"distant_learning":       "fa-laptop-house",
	"event":                  "fa-calendar-alt",
	"exam_assignment":        "fa-tasks",
	"exam_evaluation":        "fa-clipboard-check",
	"excursion":              "fa-bus",
	"excused_lesson":         "fa-user-check",
	"food_credit":            "fa-utensils",
	"strava_vydaj":           "fa-utensils",
	"free_day":               "fa-sun",
	"grade":                  "fa-graduation-cap",
	"grades_doc":             "fa-file-alt",
	"holiday":                "fa-umbrella-beach",
	"homework":               "fa-book-open",
	"homework_student_state": "fa-user-graduate",
	"homework_test":          "fa-file-alt",
	"h_attendance":           "fa-user-check",
	"h_bee":                  "fa-bee",
	"h_clearcache":           "fa-trash-alt",
	"h_cleardbi":             "fa-trash-alt",
	"h_clearisicdata":        "fa-trash-alt",
	"h_contest":              "fa-trophy",
	"h_dailyplan":            "fa-calendar-day",
	"h_edusettings":          "fa-cogs",
	"h_financie":             "fa-money-bill-wave",
	"h_grades":               "fa-graduation-cap",
	"h_homework":             "fa-book-open",
	"h_igroups":              "fa-users",
	"h_process":              "fa-cogs",
	"h_processtypes":         "fa-cogs",
	"h_settings":             "fa-cogs",
	"h_substitution":         "fa-exchange-alt",
	"h_timetable":            "fa-calendar-alt",
	"h_userphoto":            "fa-user",
	"h_stravamenu":           "fa-utensils",
	"pipnutie":               "fa-sign-in-alt",
	"h_clearplany":           "fa-trash-alt",
	"sprava":                 "fa-envelope",
	"lesson":                 "fa-chalkboard",
	"message":                "fa-envelope",
	"news":                   "fa-newspaper",
	"new_menu":               "fa-bars",
	"oral_exam":              "fa-microphone",
	"other":                  "fa-ellipsis-h",
	"paper":                  "fa-file-alt",
	"parents_evening":        "fa-users",
	"poll":                   "fa-poll",
	"process":                "fa-cogs",
	"project":                "fa-project-diagram",
	"project_exam":           "fa-file-alt",
	"project_lesson":         "fa-chalkboard",
	"representation":         "fa-users",
	"safety_instructioning":  "fa-shield-alt",
	"school_event":           "fa-school",
	"school_trip":            "fa-bus",
	"short_exam":             "fa-file-alt",
	"short_holiday":          "fa-umbrella-beach",
	"student_absent":         "fa-user-times",
	"substitution":           "fa-exchange-alt",
	"teacher_meeting":        "fa-users",
	"testing":                "fa-vial",
	"test_result":            "fa-clipboard-check",
	"testpridelenie":         "fa-tasks",
	"timetable":              "fa-calendar-alt",
	"tt_cancel":              "fa-calendar-times",
	"tutoring":               "fa-chalkboard-teacher",
	"znamka":                 "fa-graduation-cap",
}

// EventTypeName returns the human-readable label for a raw EduPage timeline
// event type key (as found in TimelineEvent.EventType). It tries an exact
// match first, then a case-insensitive one, and falls back to returning
// eventType unchanged when there is no match.
func EventTypeName(eventType string) string {
	if name, ok := eventTypeNames[eventType]; ok {
		return name
	}
	if name, ok := eventTypeNames[strings.ToLower(eventType)]; ok {
		return name
	}
	return eventType
}

// EventTypeIcon returns the Font Awesome icon class for a raw EduPage
// timeline event type key (as found in TimelineEvent.EventType). It tries an
// exact match first, then a case-insensitive one, and falls back to
// "fa-bell" when there is no match.
func EventTypeIcon(eventType string) string {
	if icon, ok := eventTypeIcons[eventType]; ok {
		return icon
	}
	if icon, ok := eventTypeIcons[strings.ToLower(eventType)]; ok {
		return icon
	}
	return "fa-bell"
}
