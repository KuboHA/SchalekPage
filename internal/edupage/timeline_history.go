package edupage

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"time"
)

// NotificationsSince returns the logged-in user's timeline events created on
// or after `from`, fetched from EduPage's history endpoint. Unlike
// Notifications (which is read straight out of the login payload and is
// capped at roughly a month), this performs a real network request and can
// reach arbitrarily far into the past. It returns an empty slice and a nil
// error when EduPage has no events in range.
func (c *Client) NotificationsSince(from time.Time) ([]TimelineEvent, error) {
	values := url.Values{"datefrom": {from.Format(eduDateLayout)}}

	body, err := c.PostForm(
		"/timeline/?module=todo&filterTab=&akcia=getData&filterTab=messages",
		values,
	)
	if err != nil {
		return nil, fmt.Errorf("edupage: fetch notification history: %w", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("edupage: decode notification history: %w", err)
	}

	rawItems := pickSlice(payload["timelineItems"])
	if len(rawItems) == 0 {
		return []TimelineEvent{}, nil
	}

	// Per-user event state (starred/done) normally comes back alongside the
	// history itself; fall back to the state cached from login when a given
	// response happens to omit it.
	userProps := asMap(payload["timelineUserProps"])
	if userProps == nil {
		if data := c.Data(); data != nil {
			userProps = asMap(data["userProps"])
		}
	}

	events := make([]TimelineEvent, 0, len(rawItems))
	for _, raw := range rawItems {
		item := pickMap(raw)
		if item == nil {
			continue
		}

		event, ok := parseTimelineItem(item, userProps)
		if !ok {
			continue
		}

		events = append(events, event)
	}

	// Newest first, consistent with how the login-payload-derived
	// Notifications() feed is ordered.
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	return events, nil
}
