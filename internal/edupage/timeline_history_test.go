package edupage

import (
	"encoding/json"
	"testing"
)

// TestParseHistoryResponse exercises the parsing logic NotificationsSince
// applies to a decoded history response, without making a network request.
func TestParseHistoryResponse(t *testing.T) {
	t.Run("events with timelineUserProps state", func(t *testing.T) {
		raw := `{
			"timelineItems": [
				{
					"timelineid": "111",
					"typ": "homework",
					"text": "Read chapter 3",
					"timestamp": "2024-02-10 12:00:00",
					"pocet_reakcii": "2",
					"cas_pridania": "2024-02-10 11:00:00"
				},
				{
					"timelineid": "222",
					"typ": "sprava",
					"text": "Hi",
					"timestamp": "2024-02-11 09:00:00"
				}
			],
			"timelineUserProps": {
				"111": {"starred": "1", "doneMaxCas": "2024-02-12 08:00:00"}
			}
		}`

		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("unmarshal fixture: %v", err)
		}

		rawItems := pickSlice(payload["timelineItems"])
		if len(rawItems) != 2 {
			t.Fatalf("rawItems = %d, want 2", len(rawItems))
		}
		userProps := asMap(payload["timelineUserProps"])

		events := make([]TimelineEvent, 0, len(rawItems))
		for _, r := range rawItems {
			item := pickMap(r)
			if item == nil {
				continue
			}
			event, ok := parseTimelineItem(item, userProps)
			if !ok {
				continue
			}
			events = append(events, event)
		}

		if len(events) != 2 {
			t.Fatalf("events = %d, want 2", len(events))
		}

		var homework, message TimelineEvent
		for _, e := range events {
			switch e.EventID {
			case 111:
				homework = e
			case 222:
				message = e
			}
		}

		if !homework.IsStarred {
			t.Error("homework event should be starred")
		}
		if !homework.IsDone || homework.DoneAt == nil {
			t.Error("homework event should be done, with DoneAt set")
		}
		if homework.ReactionCount != 2 {
			t.Errorf("homework ReactionCount = %d, want 2", homework.ReactionCount)
		}
		if homework.CreatedAt == nil {
			t.Error("homework CreatedAt should be set")
		}

		if message.IsStarred || message.IsDone {
			t.Error("message event should have no userProps state")
		}
	})

	t.Run("missing timelineUserProps falls back to nil state", func(t *testing.T) {
		raw := `{
			"timelineItems": [
				{"timelineid": "1", "typ": "sprava", "text": "hi", "timestamp": "2024-01-01 00:00:00"}
			]
		}`

		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("unmarshal fixture: %v", err)
		}

		userProps := asMap(payload["timelineUserProps"])
		if userProps != nil {
			t.Fatalf("expected nil userProps, got %#v", userProps)
		}

		rawItems := pickSlice(payload["timelineItems"])
		item := pickMap(rawItems[0])
		event, ok := parseTimelineItem(item, userProps)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if event.IsStarred || event.IsDone {
			t.Errorf("expected zero-value state, got %#v", event)
		}
	})

	t.Run("empty timelineItems yields no events", func(t *testing.T) {
		raw := `{"timelineItems": [], "timelineUserProps": {}}`

		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("unmarshal fixture: %v", err)
		}

		rawItems := pickSlice(payload["timelineItems"])
		if len(rawItems) != 0 {
			t.Fatalf("rawItems = %d, want 0", len(rawItems))
		}
	})

	t.Run("timelineItems as empty object (EduPage's empty-array quirk)", func(t *testing.T) {
		raw := `{"timelineItems": {}, "timelineUserProps": {}}`

		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("unmarshal fixture: %v", err)
		}

		rawItems := pickSlice(payload["timelineItems"])
		if len(rawItems) != 0 {
			t.Fatalf("rawItems = %d, want 0", len(rawItems))
		}
	})
}
