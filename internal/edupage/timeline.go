package edupage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// eduDateLayout is the layout EduPage uses for plain dates ("2006-01-02").
const eduDateLayout = "2006-01-02"

// eduTimeLayout is the layout EduPage uses for timestamps
// ("2006-01-02 15:04:05").
const eduTimeLayout = "2006-01-02 15:04:05"

// Notifications returns the logged-in user's timeline (the notification feed
// shown on the EduPage dashboard). It is built from the `items` list already
// present in the login payload (see Client.Data), so it performs no extra
// network request. It returns an empty slice and a nil error when the user
// simply has no timeline items.
func (c *Client) Notifications() ([]TimelineEvent, error) {
	data := c.Data()
	if data == nil {
		return nil, nil
	}

	rawItems := pickSlice(data["items"])
	if len(rawItems) == 0 {
		return nil, nil
	}

	events := make([]TimelineEvent, 0, len(rawItems))
	for _, raw := range rawItems {
		item := pickMap(raw)
		if item == nil {
			continue
		}

		event, ok := parseTimelineItem(item)
		if !ok {
			continue
		}

		events = append(events, event)
	}

	return events, nil
}

// parseTimelineItem parses a single raw timeline entry as returned by
// EduPage. It returns ok=false when the entry has no usable timeline id and
// should be skipped, mirroring the Python reference.
func parseTimelineItem(item map[string]any) (TimelineEvent, bool) {
	idStr := strings.TrimSpace(pickString(item["timelineid"]))
	if idStr == "" {
		return TimelineEvent{}, false
	}

	eventID, err := strconv.Atoi(idStr)
	if err != nil {
		return TimelineEvent{}, false
	}

	additionalData := decodeAdditionalData(item["data"])

	var timestamp time.Time
	if ts := pickString(item["timestamp"]); ts != "" {
		if parsed, err := time.Parse(eduTimeLayout, ts); err == nil {
			timestamp = parsed
		}
	}

	text := pickString(item["text"])
	// NOTE: "Dôležitá správa" ("Important message") is the EduPage UI's own
	// prefix for important messages; the real text lives in additionalData.
	if strings.HasPrefix(text, "Dôležitá správa") {
		if v, ok := additionalData["messageContent"]; ok {
			text = pickString(v)
		}
	}
	if text == "" {
		if v, ok := additionalData["nazov"]; ok {
			text = pickString(v)
		}
	}

	return TimelineEvent{
		EventID:        eventID,
		Timestamp:      timestamp,
		Text:           text,
		AuthorName:     pickString(item["vlastnik_meno"]),
		RecipientName:  pickString(item["user_meno"]),
		EventType:      pickString(item["typ"]),
		AdditionalData: additionalData,
	}, true
}

// decodeAdditionalData decodes a timeline entry's "data" field, which
// EduPage sometimes sends as a JSON-encoded string and sometimes as an
// already-decoded object. It never returns nil, so callers can index it
// directly.
func decodeAdditionalData(raw any) map[string]any {
	switch v := raw.(type) {
	case map[string]any:
		return v
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return map[string]any{}
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(v), &out); err == nil && out != nil {
			return out
		}
		return map[string]any{}
	default:
		return map[string]any{}
	}
}

// --- Defensive JSON helpers shared by this package's modules. ---
//
// EduPage's JSON responses are heterogeneous: numbers sometimes arrive as
// strings, empty PHP arrays serialize as JSON objects ({}) where a list is
// expected, and missing fields are simply absent rather than null. These
// helpers convert `any` values defensively instead of relying on type
// assertions that would panic.

// pickString coerces v to a string, formatting numbers and booleans
// reasonably and returning "" for nil or unrecognized types.
func pickString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// pickMap returns v as a map[string]any, or nil if v is not a JSON object.
func pickMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// pickSlice returns v as a []any. EduPage sometimes returns an object ({})
// instead of an empty array; in that case an empty (or, for a non-empty
// object, values-only) slice is returned instead of nil. Order is not
// guaranteed when v is an object.
func pickSlice(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case map[string]any:
		if len(t) == 0 {
			return []any{}
		}
		out := make([]any, 0, len(t))
		for _, val := range t {
			out = append(out, val)
		}
		return out
	default:
		return nil
	}
}

// pickInt coerces v to an int, accepting numbers and numeric strings
// (including ones with a fractional part, which are truncated).
func pickInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return int(f), true
		}
		return 0, false
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.Atoi(s); err == nil {
			return n, true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f), true
		}
		return 0, false
	default:
		return 0, false
	}
}

// pickFloat coerces v to a float64, accepting numbers and numeric strings.
func pickFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// pickBool coerces v to a bool, accepting JSON booleans and the common
// truthy string forms EduPage uses ("1", "true", "yes").
func pickBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "1", "true", "yes":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// parseIntDigits extracts the digits from s (discarding everything else,
// mirroring the Python reference's ModuleHelper.parse_int) and parses them
// as an int. It returns ok=false when s has no digits at all.
func parseIntDigits(s string) (int, bool) {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(b.String())
	if err != nil {
		return 0, false
	}
	return n, true
}

// isDigits reports whether s is non-empty and consists only of ASCII
// digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// extractBetween returns the substring of s that follows the first
// occurrence of after and precedes the next occurrence of before, mirroring
// the ad-hoc string splitting the Python reference uses to scrape values out
// of HTML/JS responses.
func extractBetween(s, after, before string) (string, bool) {
	idx := strings.Index(s, after)
	if idx < 0 {
		return "", false
	}
	rest := s[idx+len(after):]
	endIdx := strings.Index(rest, before)
	if endIdx < 0 {
		return "", false
	}
	return rest[:endIdx], true
}

// formatFloat formats f the way strconv.FormatFloat(f, 'f', -1, 64) would,
// i.e. without trailing zeros or an exponent.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
