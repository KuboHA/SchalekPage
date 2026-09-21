// Package edupage is a Go client for the EduPage school system, ported from
// the Python reference implementation at https://github.com/EdupageAPI/edupage-api.
package edupage

import "time"

// Student is a student in the logged-in user's class.
type Student struct {
	PersonID      int
	Name          string
	Gender        string
	InSchoolSince *time.Time
	ClassID       int
	NumberInClass int
}

// Class is a school class.
type Class struct {
	ClassID     int
	Name        string
	Short       string
	TeacherID   int
	Teacher2ID  int
	Grade       int
	ClassroomID string
}

// Teacher is a teacher account.
type Teacher struct {
	PersonID      int
	Name          string
	Gender        string
	InSchoolSince *time.Time
	ClassroomID   string
}

// Subject is a school subject.
type Subject struct {
	SubjectID int
	Name      string
	Short     string
}

// Classroom is a physical classroom.
type Classroom struct {
	ClassroomID string
	Name        string
	Short       string
}

// TimelineEvent is one entry of the user's timeline (a notification).
type TimelineEvent struct {
	EventID        int
	Timestamp      time.Time
	Text           string
	AuthorName     string
	RecipientName  string
	EventType      string
	AdditionalData map[string]any

	// The fields below are derived from the timeline's per-user state map
	// (`userProps` in the login payload, `timelineUserProps` in a history
	// response). They are zero when no state has been recorded for the event.
	IsDone        bool
	DoneAt        *time.Time
	IsStarred     bool
	ReactionCount int
	CreatedAt     *time.Time
	IsRemoved     bool
}

// Lesson is a single lesson in a timetable.
type Lesson struct {
	Period       *int
	StartTime    time.Time
	EndTime      time.Time
	Subject      *Subject
	Classrooms   []Classroom
	Teachers     []Teacher
	OnlineLesson string
	Curriculum   string
	Groups       []string
	// IsCancelled marks a lesson EduPage has removed or flagged absent; such
	// lessons must not render as if they were taking place.
	IsCancelled bool
	// IsEvent marks a calendar entry (trip, meeting, ...) rather than a lesson.
	IsEvent bool
}

// Duration returns how long the lesson lasts.
func (l Lesson) Duration() time.Duration {
	if l.StartTime.IsZero() || l.EndTime.IsZero() {
		return 0
	}
	return l.EndTime.Sub(l.StartTime)
}

// Timetable is a day's worth of lessons.
type Timetable struct {
	Date    time.Time
	Lessons []Lesson
}

// Menu is one option within a meal.
type Menu struct {
	Name      string
	Allergens string
	Weight    string
	Number    string
	Rating    *MealRating
	// Label is EduPage's display name for the menu ("Menu A").
	Label string
	// Choosable is false for a menu the canteen lists but does not allow
	// this diner to pick.
	Choosable bool
	// OrderIndex is the key ("1", "2", ...) the ordering call expects.
	OrderIndex string
}

// MealRating is the user's rating of a served meal.
type MealRating struct {
	Date           string
	BoardsAverage  string
	QualityAverage string
	BoardsRatings  int
	QualityRatings int
}

// Meal is one servable meal (snack / lunch / afternoon snack) on a given day.
type Meal struct {
	Served       bool
	Amount       int
	Menus        []Menu
	CanBeChanged bool
	ChosenMenu   string
	OrderedMeal  string
	Title        string
	MenuID       string
	Date         time.Time

	// BoarderID ("stravnikid") identifies the diner whose order is being
	// changed; it is required by every canteen write call.
	BoarderID string
	// CanBeChangedUntil is the deadline after which the order is frozen. It is
	// nil when EduPage published no deadline for the meal.
	CanBeChangedUntil *time.Time
	// MealIndex is "1" (snack), "2" (lunch) or "3" (afternoon snack).
	MealIndex string
}

// Meals holds all meals for a single day.
type Meals struct {
	Snack          *Meal
	Lunch          *Meal
	AfternoonSnack *Meal
}

// Term is a school term ("P1" = first, "P2" = second).
type Term string

const (
	TermFirst  Term = "P1"
	TermSecond Term = "P2"
)

// Grade is a single grade received by the student.
type Grade struct {
	GradeN    string
	Comment   string
	Date      time.Time
	Subject   string
	SubjectID int
	Teacher   string
	TeacherID int
	Title     string
	MaxPoints float64
	Percent   float64
	Verbal    bool
	Weight    float64
	EventID   int
}

// ChangeAction is the kind of a timetable change.
type ChangeAction string

const (
	ActionChange   ChangeAction = "change"
	ActionAddition ChangeAction = "add"
	ActionDeletion ChangeAction = "remove"
)

// TimetableChange is one substitution entry for a day.
type TimetableChange struct {
	ChangeClass string
	LessonN     string // already formatted; "3" or "3-4"
	Title       string
	Action      ChangeAction
}
