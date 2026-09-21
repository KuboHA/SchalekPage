package web

import (
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// attachmentView is a presentation-ready attachment extracted from a
// notification's AdditionalData payload.
type attachmentView struct {
	Name    string
	URL     string
	IsImage bool
}

// notificationView is a presentation-ready timeline notification, including
// resolved icon, relative timestamp, extracted attachments, confirmation
// count, and threaded replies. It is built from edupage.TimelineEvent by
// buildNotificationViews, porting the heuristics from app.py's dashboard().
type notificationView struct {
	EventID            int
	EventType          string
	IconClass          string
	FormattedTimestamp string
	Author             string
	Recipient          string
	Text               string
	Attachments        []attachmentView
	ConfirmationCount  int
	Replies            []*notificationView
	timestamp          time.Time
}

// timeSincePosted renders a coarse relative-time string, ported from
// app.py's time_since_posted.
func timeSincePosted(ts, now time.Time) string {
	diff := now.Sub(ts)
	if diff < 0 {
		diff = 0
	}
	days := int(diff.Hours()) / 24
	if days > 0 {
		return pluralize(days, "day") + " ago"
	}
	secs := int(diff.Seconds())
	if hours := secs / 3600; hours > 0 {
		return pluralize(hours, "hour") + " ago"
	}
	if minutes := secs / 60; minutes > 0 {
		return pluralize(minutes, "minute") + " ago"
	}
	return "Just now"
}

func pluralize(n int, unit string) string {
	s := unit
	if n != 1 {
		s += "s"
	}
	return strconv.Itoa(n) + " " + s
}

// extractConfirmCount recursively hunts additional_data for something that
// looks like a confirmation/like/thumbs-up count, porting app.py's
// _extract_confirm_count.
func extractConfirmCount(data any) int {
	switch v := data.(type) {
	case nil:
		return 0
	case float64:
		return int(v)
	case int:
		return v
	case []any:
		return len(v)
	case map[string]any:
		count := 0
		for k, sub := range v {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "confirm") || strings.Contains(lk, "like") || strings.Contains(lk, "thumb") {
				if sc := extractConfirmCount(sub); sc > count {
					count = sc
				}
			}
		}
		if c, ok := v["count"]; ok {
			if n, ok := toNumber(c); ok && int(n) > count {
				count = int(n)
			}
		}
		return count
	default:
		return 0
	}
}

func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

// resolveAttachmentURL prefixes a relative attachment path with baseURL,
// matching app.py's URL normalization.
func resolveAttachmentURL(rawURL, baseURL string) string {
	if rawURL == "" {
		return ""
	}
	lower := strings.ToLower(rawURL)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return rawURL
	}
	if baseURL == "" {
		return rawURL
	}
	local := rawURL
	if !strings.HasPrefix(local, "/") {
		local = "/" + local
	}
	return strings.TrimRight(baseURL, "/") + local
}

// attachmentCollector accumulates deduplicated attachments for one
// notification, porting app.py's _record_attachment closure.
type attachmentCollector struct {
	baseURL string
	seen    map[string]bool
	items   []attachmentView
}

func newAttachmentCollector(baseURL string) *attachmentCollector {
	return &attachmentCollector{baseURL: baseURL, seen: map[string]bool{}}
}

func (c *attachmentCollector) record(name, rawURL string) {
	if rawURL == "" {
		return
	}
	full := resolveAttachmentURL(rawURL, c.baseURL)
	if c.seen[full] {
		return
	}
	c.seen[full] = true
	if name == "" {
		name = "attachment"
	}
	c.items = append(c.items, attachmentView{Name: name, URL: full, IsImage: looksLikeJPEG(name, full)})
}

// looksLikeJPEG reports whether an attachment is a JPEG, checking the
// display name's extension first and, when that's inconclusive, falling
// back to the extension of the resolved URL's path.
//
// This is a deliberate divergence from app.py, which derived is_image
// purely from the attachment *name*: for an "/elearning/" scan hit whose
// display name comes from an arbitrary dict value (e.g. "Worksheet"), the
// Python behaviour silently treats an unambiguous
// "/elearning/materials/sheet.jpeg" as a non-image and drops it to a plain
// download link instead of the inline preview the templates support for
// .jpg/.jpeg. Falling back to the URL's path extension (via net/url +
// path.Ext, so a query string or fragment can't defeat the suffix check)
// fixes that without widening image support beyond the JPEG the preview
// markup is wired for.
func looksLikeJPEG(name, rawURL string) bool {
	if hasJPEGExt(name) {
		return true
	}
	if u, err := url.Parse(rawURL); err == nil {
		if hasJPEGExt(path.Ext(u.Path)) {
			return true
		}
	}
	return false
}

func hasJPEGExt(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg")
}

var attachmentKeys = []string{"attachments", "files", "prilohy", "docs"}

// extractAttachments ports app.py's attachment extraction: a first pass over
// well-known keys (list or dict shaped), followed by a recursive scan for
// anything that looks like an "/elearning/" path.
func extractAttachments(data map[string]any, baseURL string) []attachmentView {
	c := newAttachmentCollector(baseURL)

	for _, key := range attachmentKeys {
		raw, ok := data[key]
		if !ok {
			continue
		}
		switch list := raw.(type) {
		case []any:
			for _, item := range list {
				switch v := item.(type) {
				case map[string]any:
					name := firstString(v, "name", "filename", "title")
					url := firstString(v, "url", "link", "downloadUrl", "href")
					c.record(name, url)
				case string:
					name := v
					if idx := strings.LastIndex(v, "/"); idx >= 0 {
						name = v[idx+1:]
					}
					c.record(name, v)
				}
			}
		case map[string]any:
			for k2, v2 := range list {
				switch vv := v2.(type) {
				case map[string]any:
					name := firstString(vv, "name", "filename", "title", "file")
					url := firstString(vv, "url", "link", "downloadUrl", "href")
					if url == "" {
						url = k2
					}
					c.record(name, url)
				case string:
					c.record(vv, k2)
				default:
					name := k2
					if idx := strings.LastIndex(k2, "/"); idx >= 0 {
						name = k2[idx+1:]
					}
					c.record(name, k2)
				}
			}
		}
		break // python stops after the first matching key, via `break`
	}

	scanElearning(data, c)
	return c.items
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func isElearningPath(s string) bool {
	return strings.Contains(s, "/elearning/") || strings.HasPrefix(s, "/elearning") || strings.HasPrefix(s, "elearning/")
}

// scanElearning recursively walks obj looking for map keys or string values
// that look like "/elearning/..." paths, porting app.py's _scan.
func scanElearning(obj any, c *attachmentCollector) {
	switch v := obj.(type) {
	case map[string]any:
		for k, val := range v {
			if isElearningPath(k) {
				filename := k
				if idx := strings.LastIndex(k, "/"); idx >= 0 {
					filename = k[idx+1:]
				}
				if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
					filename = strings.TrimSpace(s)
				}
				c.record(filename, k)
			}
			if s, ok := val.(string); ok && isElearningPath(s) {
				name := k
				if idx := strings.LastIndex(k, "/"); idx >= 0 {
					name = k[idx+1:]
				}
				c.record(name, s)
			}
			scanElearning(val, c)
		}
	case []any:
		for _, item := range v {
			scanElearning(item, c)
		}
	}
}

var parentKeys = []string{"parent_id", "parentId", "reply_to", "replyTo", "parentTimelineId", "timeline_parent_id"}

// guessParentByPrefix ports app.py's _guess_parent_by_prefix: try the first
// 4 or 5 digits of the event id's string form as a candidate parent id.
func guessParentByPrefix(eid int, idMap map[int]*notificationView) (int, bool) {
	s := strconv.Itoa(eid)
	for _, plen := range []int{4, 5} {
		if len(s) > plen {
			prefix := s[:plen]
			pid, err := strconv.Atoi(prefix)
			if err != nil {
				continue
			}
			if pid != eid {
				if _, ok := idMap[pid]; ok {
					return pid, true
				}
			}
		}
	}
	return 0, false
}

// buildNotificationViews converts raw timeline events into threaded,
// enriched view models, porting the notification post-processing block from
// app.py's dashboard() route: attachment extraction, confirmation counts,
// and reply threading by explicit parent keys or an id-prefix heuristic.
func buildNotificationViews(events []edupage.TimelineEvent, baseURL string, now time.Time) []*notificationView {
	idMap := make(map[int]*notificationView, len(events))
	order := make([]int, 0, len(events))

	for i := range events {
		e := &events[i]
		nv := &notificationView{
			EventID:            e.EventID,
			EventType:          e.EventType,
			IconClass:          edupage.EventTypeIcon(strings.ToLower(e.EventType)),
			FormattedTimestamp: timeSincePosted(e.Timestamp, now),
			Author:             e.AuthorName,
			Recipient:          e.RecipientName,
			Text:               e.Text,
			timestamp:          e.Timestamp,
		}
		nv.ConfirmationCount = extractConfirmCount(e.AdditionalData)
		if e.AdditionalData != nil {
			nv.Attachments = extractAttachments(e.AdditionalData, baseURL)
		}

		eid := e.EventID
		if _, exists := idMap[eid]; exists {
			// Fall back to a synthetic, guaranteed-unique key when the
			// server sends a duplicate/zero event id.
			eid = -(len(idMap) + 1)
		}
		idMap[eid] = nv
		order = append(order, eid)
	}

	var mainNotifications []*notificationView
	for _, eid := range order {
		nv := idMap[eid]
		e := findEventByOrderIndex(events, eid, order)

		var parentID int
		var hasParent bool
		if e != nil && e.AdditionalData != nil {
			for _, pk := range parentKeys {
				raw, ok := e.AdditionalData[pk]
				if !ok || raw == nil {
					continue
				}
				switch v := raw.(type) {
				case float64:
					if v != 0 {
						parentID, hasParent = int(v), true
					}
				case string:
					if n, err := strconv.Atoi(v); err == nil {
						parentID, hasParent = n, true
					}
				}
				if hasParent {
					break
				}
			}
		}
		if !hasParent {
			if pid, ok := guessParentByPrefix(eid, idMap); ok {
				parentID, hasParent = pid, true
			}
		}

		if hasParent && parentID != eid {
			if parent, ok := idMap[parentID]; ok {
				parent.Replies = append(parent.Replies, nv)
				continue
			}
		}
		mainNotifications = append(mainNotifications, nv)
	}

	sort.SliceStable(mainNotifications, func(i, j int) bool {
		return mainNotifications[i].timestamp.After(mainNotifications[j].timestamp)
	})
	for _, nv := range mainNotifications {
		sort.SliceStable(nv.Replies, func(i, j int) bool {
			return nv.Replies[i].timestamp.Before(nv.Replies[j].timestamp)
		})
	}

	return mainNotifications
}

// findEventByOrderIndex maps a (possibly synthetic) id back to its source
// event, since idMap keys may have been rewritten to avoid collisions.
func findEventByOrderIndex(events []edupage.TimelineEvent, eid int, order []int) *edupage.TimelineEvent {
	for i, id := range order {
		if id == eid {
			return &events[i]
		}
	}
	return nil
}
