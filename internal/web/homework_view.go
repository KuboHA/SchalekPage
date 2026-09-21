// Feature 5: a homework tracker. EduPage has no dedicated homework module —
// assignments arrive as timeline events, with the due date and title tucked
// away in AdditionalData["oldVals"]. This file filters the timeline for
// those events, digs the details out defensively, and groups them by due
// date for homework.html.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// homeworkEventTypes is the set of raw EduPage timeline event types that
// represent homework assignments, matched case-insensitively against
// edupage.TimelineEvent.EventType.
var homeworkEventTypes = map[string]bool{
	"homework":            true,
	"homeworkstudentstav": true,
	"etesthw":             true,
}

// isHomeworkEventType reports whether eventType is one of the raw EduPage
// timeline types that carry a homework assignment.
func isHomeworkEventType(eventType string) bool {
	return homeworkEventTypes[strings.ToLower(eventType)]
}

// extractOldVals digs AdditionalData["oldVals"] out of a timeline event's
// additional data defensively: EduPage sometimes sends it as an
// already-decoded object, sometimes as a JSON-encoded string, and often
// leaves it out entirely. It never panics on an unexpected shape; it just
// returns nil.
func extractOldVals(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	raw, ok := data["oldVals"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case map[string]any:
		return v
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(s), &out); err == nil {
			return out
		}
	}
	return nil
}

// stringFromAny coerces v to a string, accepting the handful of JSON shapes
// EduPage is known to use for a nominally-string field (plain strings,
// numbers, booleans), and returning "" for nil or an unrecognized shape
// (e.g. a nested object) rather than a confusing Go-syntax dump.
func stringFromAny(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case map[string]any, []any:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// homeworkDueDateLayouts are the date layouts oldVals.date has been observed
// in, tried in order.
var homeworkDueDateLayouts = []string{"2006-01-02", "2006-01-02 15:04:05"}

// homeworkDueDate parses oldVals["date"], reporting ok=false when the value
// is missing, empty, or doesn't match a known layout.
func homeworkDueDate(oldVals map[string]any) (time.Time, bool) {
	raw := strings.TrimSpace(stringFromAny(oldVals["date"]))
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range homeworkDueDateLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// homeworkTitle resolves a homework item's title: oldVals["title"] first,
// falling back to the event's own text (which parseTimelineItem has already
// resolved from additional_data.nazov / messageContent where needed) so a
// missing or malformed oldVals never leaves an item blank.
func homeworkTitle(oldVals map[string]any, fallback string) string {
	if title := strings.TrimSpace(stringFromAny(oldVals["title"])); title != "" {
		return title
	}
	return fallback
}

// homeworkItemView is a presentation-ready homework assignment.
//
// IsDone/HasDoneAt/DoneAt are read-only display state: EduPage exposes no
// write operation to mark homework done (there is no such call in the
// upstream API), so templates must render this as plain status text, never
// as an interactive checkbox implying the viewer can toggle it.
type homeworkItemView struct {
	EventID    int
	Title      string
	EventType  string
	TypeName   string
	IconClass  string
	AuthorName string
	IsDone     bool
	HasDoneAt  bool
	DoneAt     time.Time
}

// homeworkGroupView groups homework items sharing a due date. Items with no
// parseable due date land in the single HasDueDate=false group instead of
// being dropped.
type homeworkGroupView struct {
	HasDueDate   bool
	DueDate      time.Time
	IsPastDue    bool
	Items        []homeworkItemView
	OutstandingN int
	DoneN        int
}

// homeworkPageView is the fully precomputed view model for homework.html.
type homeworkPageView struct {
	Groups           []homeworkGroupView
	OutstandingTotal int
	DoneTotal        int
}

// homeworkBucketKey identifies one due-date group: either a specific
// (truncated to day) date, or the single "no due date" bucket.
type homeworkBucketKey struct {
	hasDate bool
	date    time.Time
}

// buildHomeworkPageView filters events down to homework assignments, groups
// them by due date (undated assignments form their own trailing group), and
// tallies done vs outstanding counts at both group and page level.
func buildHomeworkPageView(events []edupage.TimelineEvent, now time.Time) homeworkPageView {
	today := truncateToDate(now)

	buckets := map[homeworkBucketKey]*homeworkGroupView{}
	var order []homeworkBucketKey

	for _, e := range events {
		if !isHomeworkEventType(e.EventType) {
			continue
		}

		oldVals := extractOldVals(e.AdditionalData)
		due, hasDue := homeworkDueDate(oldVals)

		key := homeworkBucketKey{hasDate: hasDue}
		if hasDue {
			key.date = truncateToDate(due)
		}

		group, exists := buckets[key]
		if !exists {
			group = &homeworkGroupView{HasDueDate: hasDue, DueDate: key.date}
			if hasDue {
				group.IsPastDue = key.date.Before(today)
			}
			buckets[key] = group
			order = append(order, key)
		}

		item := homeworkItemView{
			EventID:    e.EventID,
			Title:      homeworkTitle(oldVals, e.Text),
			EventType:  e.EventType,
			TypeName:   edupage.EventTypeName(strings.ToLower(e.EventType)),
			IconClass:  edupage.EventTypeIcon(strings.ToLower(e.EventType)),
			AuthorName: e.AuthorName,
			IsDone:     e.IsDone,
		}
		if e.IsDone && e.DoneAt != nil {
			item.HasDoneAt = true
			item.DoneAt = *e.DoneAt
		}

		group.Items = append(group.Items, item)
		if item.IsDone {
			group.DoneN++
		} else {
			group.OutstandingN++
		}
	}

	// Dated groups sort soonest-first; the single undated bucket (if any)
	// always trails, since "no due date" isn't a date to sort by.
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.hasDate != b.hasDate {
			return a.hasDate
		}
		return a.date.Before(b.date)
	})

	var view homeworkPageView
	for _, key := range order {
		g := buckets[key]
		sort.SliceStable(g.Items, func(i, j int) bool {
			if g.Items[i].IsDone != g.Items[j].IsDone {
				return !g.Items[i].IsDone // outstanding first within a group
			}
			return g.Items[i].Title < g.Items[j].Title
		})
		view.Groups = append(view.Groups, *g)
		view.OutstandingTotal += g.OutstandingN
		view.DoneTotal += g.DoneN
	}

	return view
}

// homeworkPageData is the view model for homework.html.
type homeworkPageData struct {
	Student *studentSummary
	homeworkPageView
}

// handleHomework serves GET /homework: a due-date-grouped view of homework
// assignments sourced from the timeline feed. See the package doc comment
// above for why marking an item done isn't offered here.
func (s *Server) handleHomework(w http.ResponseWriter, r *http.Request, sess *Session) {
	now := s.now()

	students, err := sess.Client.Students()
	if err != nil {
		s.logger.Warn("homework: fetch students failed", "error", err)
	}
	student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)

	events, err := sess.Client.Notifications()
	if err != nil {
		s.logger.Warn("homework: fetch notifications failed", "error", err)
	}

	s.render(w, r, "homework.html", homeworkPageData{
		Student:          studentSummaryOrNil(student.Name, studentOK),
		homeworkPageView: buildHomeworkPageView(events, now),
	})
}
