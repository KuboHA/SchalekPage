package edupage

import (
	"fmt"
	"strconv"
	"time"
)

// dateLayout is EduPage's date string format, e.g. in "datefrom"/"dateto".
const dateLayout = "2006-01-02"

// dbiGroup returns data["dbi"][name] as a map, or nil if data, "dbi", or the
// named group is missing or not an object. EduPage's dbi payload is keyed by
// numeric-string ids (e.g. dbi["students"]["1234"]).
func (c *Client) dbiGroup(name string) map[string]any {
	if c.data == nil {
		return nil
	}
	dbi := asMap(c.data["dbi"])
	if dbi == nil {
		return nil
	}
	return asMap(dbi[name])
}

// dbiItem returns the item with the given string id within the named dbi
// group, or nil if not found.
func (c *Client) dbiItem(groupName, id string) map[string]any {
	group := c.dbiGroup(groupName)
	if group == nil {
		return nil
	}
	return asMap(group[id])
}

// fullName joins firstname/lastname fields the way EduPage's dbi entries
// store them.
func fullName(item map[string]any) string {
	first := asString(item["firstname"])
	last := asString(item["lastname"])
	switch {
	case first == "":
		return last
	case last == "":
		return first
	default:
		return first + " " + last
	}
}

// parseEduDate parses an EduPage date string ("2006-01-02"), returning nil if
// it's empty or unparseable. It checks both the "datefrom" key used by the
// upstream Python reference and the "datumodkedy" key ("date from" in
// Slovak) some EduPage deployments send instead, in case one is present but
// not the other.
func personInSchoolSince(item map[string]any) *time.Time {
	for _, key := range []string{"datefrom", "datumodkedy"} {
		raw := asString(item[key])
		if raw == "" {
			continue
		}
		if t, err := time.Parse(dateLayout, raw); err == nil {
			return &t
		}
	}
	return nil
}

// classIDOf reads a person's class id, defensively trying both the
// "classid" key used by the upstream reference and the "triedaid" key
// ("class id" in Slovak) some EduPage deployments use instead.
func classIDOf(item map[string]any) int {
	if v, ok := item["classid"]; ok && v != nil {
		if n := asInt(v); n != 0 {
			return n
		}
	}
	return asInt(item["triedaid"])
}

// Students returns the students in the logged-in user's class, built from
// the login payload's dbi.students table.
func (c *Client) Students() ([]Student, error) {
	if c.data == nil {
		return nil, ErrNotLoggedIn
	}

	group := c.dbiGroup("students")
	if group == nil {
		return nil, nil
	}

	result := make([]Student, 0, len(group))
	for idStr, raw := range group {
		if idStr == "" {
			continue
		}
		item := asMap(raw)
		if item == nil {
			continue
		}
		personID, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}

		result = append(result, Student{
			PersonID:      personID,
			Name:          fullName(item),
			Gender:        asString(item["gender"]),
			InSchoolSince: personInSchoolSince(item),
			ClassID:       classIDOf(item),
			NumberInClass: asInt(item["numberinclass"]),
		})
	}
	return result, nil
}

// Teachers returns all teachers in the school, built from the login
// payload's dbi.teachers table.
func (c *Client) Teachers() ([]Teacher, error) {
	if c.data == nil {
		return nil, ErrNotLoggedIn
	}

	group := c.dbiGroup("teachers")
	if group == nil {
		return nil, nil
	}

	result := make([]Teacher, 0, len(group))
	for idStr, raw := range group {
		if idStr == "" {
			continue
		}
		item := asMap(raw)
		if item == nil {
			continue
		}
		personID, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}

		classroomID := asString(item["classroomid"])
		classroomShort := ""
		if classroomID != "" {
			if classroom := c.dbiItem("classrooms", classroomID); classroom != nil {
				classroomShort = asString(classroom["short"])
			}
		}

		result = append(result, Teacher{
			PersonID:      personID,
			Name:          fullName(item),
			Gender:        asString(item["gender"]),
			InSchoolSince: personInSchoolSince(item),
			ClassroomID:   classroomShort,
		})
	}
	return result, nil
}

// Classes returns all classes in the school, built from the login payload's
// dbi.classes table.
func (c *Client) Classes() ([]Class, error) {
	if c.data == nil {
		return nil, ErrNotLoggedIn
	}

	group := c.dbiGroup("classes")
	if group == nil {
		return nil, nil
	}

	result := make([]Class, 0, len(group))
	for idStr, raw := range group {
		if idStr == "" {
			continue
		}
		item := asMap(raw)
		if item == nil {
			continue
		}
		classID, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}

		result = append(result, Class{
			ClassID:     classID,
			Name:        asString(item["name"]),
			Short:       asString(item["short"]),
			TeacherID:   asInt(item["teacherid"]),
			Teacher2ID:  asInt(item["teacher2id"]),
			Grade:       asInt(item["grade"]),
			ClassroomID: asString(item["classroomid"]),
		})
	}
	return result, nil
}

// Subjects returns all subjects taught at the school, built from the login
// payload's dbi.subjects table.
func (c *Client) Subjects() ([]Subject, error) {
	if c.data == nil {
		return nil, ErrNotLoggedIn
	}

	group := c.dbiGroup("subjects")
	if group == nil {
		return nil, nil
	}

	result := make([]Subject, 0, len(group))
	for idStr, raw := range group {
		if idStr == "" {
			continue
		}
		item := asMap(raw)
		if item == nil {
			continue
		}
		subjectID, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}

		result = append(result, Subject{
			SubjectID: subjectID,
			Name:      asString(item["name"]),
			Short:     asString(item["short"]),
		})
	}
	return result, nil
}

// Classrooms returns all classrooms at the school, built from the login
// payload's dbi.classrooms table.
func (c *Client) Classrooms() ([]Classroom, error) {
	if c.data == nil {
		return nil, ErrNotLoggedIn
	}

	group := c.dbiGroup("classrooms")
	if group == nil {
		return nil, nil
	}

	result := make([]Classroom, 0, len(group))
	for idStr, raw := range group {
		if idStr == "" {
			continue
		}
		item := asMap(raw)
		if item == nil {
			continue
		}

		result = append(result, Classroom{
			ClassroomID: idStr,
			Name:        asString(item["name"]),
			Short:       asString(item["short"]),
		})
	}
	return result, nil
}

// SchoolYear returns the current school year as EduPage reports it, read
// from the login payload's dp.year field.
func (c *Client) SchoolYear() (int, error) {
	if c.data == nil {
		return 0, ErrNotLoggedIn
	}

	dp := asMap(c.data["dp"])
	if dp == nil {
		return 0, fmt.Errorf("edupage: %w: no \"dp\" in login data (try logging in again)", ErrMissingData)
	}
	return asInt(dp["year"]), nil
}
