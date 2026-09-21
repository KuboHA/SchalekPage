package edupage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TimetableChanges returns the substitutions (timetable changes) published
// for the given day. It returns an empty slice and a nil error when EduPage
// has no substitution page for that day.
func (c *Client) TimetableChanges(day time.Time) ([]TimetableChange, error) {
	html, err := c.substitutionHTML(day)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(html) == "" {
		return nil, nil
	}

	return parseSubstitutionHTML(html), nil
}

// substitutionHTML fetches the raw substitution-viewer HTML fragment for
// day, mirroring Substitution.__get_substitution_data from the Python
// reference. When EduPage reports its gsecHash as stale (a "reload" key in
// place of real data), it transparently refreshes the session and retries
// once — see Client.withSessionRecovery.
func (c *Client) substitutionHTML(day time.Time) (string, error) {
	fetch := func() ([]byte, error) {
		payload := map[string]any{
			"__args": []any{
				nil,
				map[string]string{
					"date": day.Format(eduDateLayout),
					"mode": "classes",
				},
			},
			// Read fresh on every call: a retry after Restore() must send
			// the just-refreshed hash, not the one captured before it.
			"__gsh": c.GsecHash(),
		}
		return c.PostJSON("/substitution/server/viewer.js?__func=getSubstViewerDayDataHtml", payload)
	}

	body, err := c.withSessionRecovery(fetch, hasReloadKey)
	if err != nil {
		return "", fmt.Errorf("edupage: fetch substitutions: %w", err)
	}

	var resp struct {
		R string `json:"r"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("edupage: decode substitutions response: %w", err)
	}

	return resp.R, nil
}

const (
	substClassDelim  = `</div><div class="section print-nobreak"><div class="header"><span class="print-font-resizable">`
	substRowsMarker  = `</span><div class="rows">`
	substRowDelim    = `<div class="row `
	substFooterDelim = `<div style="text-align:center;font-size:12px"><a href="https://www.asctimetables.com" target="_blank">www.asctimetables.com</a> -`
)

var substTagReplacer = strings.NewReplacer(
	"</div>", "",
	`<div class="period">`, "",
	`<span class="print-font-resizable">`, "",
	`<div class="info">`, "",
)

// parseSubstitutionHTML ports Substitution.get_timetable_changes's HTML
// scraping: the substitution viewer's response is not structured data, but a
// rendered HTML fragment with a fixed, class-delimited shape.
func parseSubstitutionHTML(html string) []TimetableChange {
	sections := strings.Split(html, substClassDelim)
	if len(sections) <= 1 {
		return nil
	}
	sections = sections[1:]

	if idx := strings.Index(sections[len(sections)-1], substFooterDelim); idx >= 0 {
		sections[len(sections)-1] = sections[len(sections)-1][:idx]
	}

	var changes []TimetableChange
	for _, section := range sections {
		section = substTagReplacer.Replace(section)

		parts := strings.SplitN(section, substRowsMarker, 2)
		if len(parts) != 2 {
			continue
		}
		changeClass := strings.TrimSpace(parts[0])

		rows := strings.Split(parts[1], substRowDelim)
		if len(rows) <= 1 {
			continue
		}
		rows = rows[1:]

		for _, row := range rows {
			row = strings.ReplaceAll(row, `">`, "</span>")

			fields := strings.SplitN(row, "</span>", 4)
			if len(fields) < 3 {
				continue
			}
			actionStr, lessonNRaw, title := fields[0], fields[1], fields[2]

			if strings.Contains(title, "<img src=") {
				if idx := strings.Index(title, ">"); idx >= 0 {
					title = title[idx+1:]
				}
			}

			changes = append(changes, TimetableChange{
				ChangeClass: changeClass,
				LessonN:     formatLessonN(lessonNRaw),
				Title:       strings.TrimSpace(title),
				Action:      parseChangeAction(actionStr),
			})
		}
	}

	return changes
}

// formatLessonN normalizes a raw lesson-number fragment ("3", "3 - 4", ...)
// into the contract's pre-formatted form ("3" or "3-4").
func formatLessonN(raw string) string {
	raw = strings.TrimSpace(raw)

	if strings.Contains(raw, "-") {
		var from, to string
		if parts := strings.SplitN(raw, " - ", 2); len(parts) == 2 {
			from, to = parts[0], parts[1]
		} else if parts := strings.SplitN(raw, "-", 2); len(parts) == 2 {
			from, to = parts[0], parts[1]
		}

		fromN, fromOK := parseIntDigits(from)
		toN, toOK := parseIntDigits(to)
		if fromOK && toOK {
			return strconv.Itoa(fromN) + "-" + strconv.Itoa(toN)
		}
	}

	if n, ok := parseIntDigits(raw); ok {
		return strconv.Itoa(n)
	}

	return raw
}

// parseChangeAction normalizes a raw action fragment scraped out of the
// substitution HTML into one of the ChangeAction constants, falling back to
// the trimmed raw value when it doesn't match a known action.
func parseChangeAction(raw string) ChangeAction {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ActionAddition):
		return ActionAddition
	case string(ActionChange):
		return ActionChange
	case string(ActionDeletion):
		return ActionDeletion
	default:
		return ChangeAction(strings.TrimSpace(raw))
	}
}
