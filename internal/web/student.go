package web

import (
	"strings"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// matchStudentByUsername finds the roster entry whose name (with spaces
// removed) contains the given username, case-insensitively. It ports
// app.py's login-time student match: `username.lower() in
// student.name.lower().replace(' ', ”)`.
func matchStudentByUsername(students []edupage.Student, username string) (edupage.Student, bool) {
	needle := strings.ToLower(username)
	for _, s := range students {
		haystack := strings.ToLower(strings.ReplaceAll(s.Name, " ", ""))
		if strings.Contains(haystack, needle) {
			return s, true
		}
	}
	return edupage.Student{}, false
}

// resolveStudent finds the session's student in a freshly fetched roster:
// by id when one was recorded at login, otherwise by exact name match,
// porting app.py's per-request student lookup (`get_students()` is called
// fresh on every request there too).
func resolveStudent(students []edupage.Student, studentID int, studentName string) (edupage.Student, bool) {
	if studentID != 0 {
		for _, s := range students {
			if s.PersonID == studentID {
				return s, true
			}
		}
	}
	for _, s := range students {
		if s.Name == studentName {
			return s, true
		}
	}
	return edupage.Student{}, false
}
