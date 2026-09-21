package edupage

import (
	"reflect"
	"testing"
	"time"
)

func TestDecodeAdditionalData(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want map[string]any
	}{
		{
			name: "already an object",
			raw:  map[string]any{"nazov": "hi"},
			want: map[string]any{"nazov": "hi"},
		},
		{
			name: "JSON-encoded string",
			raw:  `{"nazov": "hi", "count": 3}`,
			want: map[string]any{"nazov": "hi", "count": float64(3)},
		},
		{
			name: "empty string",
			raw:  "",
			want: map[string]any{},
		},
		{
			name: "malformed JSON string",
			raw:  "{not json",
			want: map[string]any{},
		},
		{
			name: "nil",
			raw:  nil,
			want: map[string]any{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeAdditionalData(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("decodeAdditionalData(%#v) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParseTimelineItem(t *testing.T) {
	t.Run("plain message event", func(t *testing.T) {
		item := map[string]any{
			"timelineid":    "12345",
			"data":          `{"nazov": "fallback title"}`,
			"typ":           "sprava",
			"timestamp":     "2024-03-01 08:30:00",
			"text":          "Hello there",
			"user_meno":     "Jane Student",
			"vlastnik_meno": "John Teacher",
		}

		event, ok := parseTimelineItem(item)
		if !ok {
			t.Fatal("expected ok=true")
		}

		want := TimelineEvent{
			EventID:        12345,
			Timestamp:      time.Date(2024, 3, 1, 8, 30, 0, 0, time.UTC),
			Text:           "Hello there",
			AuthorName:     "John Teacher",
			RecipientName:  "Jane Student",
			EventType:      "sprava",
			AdditionalData: map[string]any{"nazov": "fallback title"},
		}
		if !reflect.DeepEqual(event, want) {
			t.Errorf("parseTimelineItem() = %#v, want %#v", event, want)
		}
	})

	t.Run("empty text falls back to additionalData.nazov", func(t *testing.T) {
		item := map[string]any{
			"timelineid": "1",
			"data":       map[string]any{"nazov": "Some grade title"},
			"typ":        "znamka",
			"timestamp":  "2024-01-01 00:00:00",
			"text":       "",
		}

		event, ok := parseTimelineItem(item)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if event.Text != "Some grade title" {
			t.Errorf("Text = %q, want %q", event.Text, "Some grade title")
		}
	})

	t.Run("important message uses messageContent", func(t *testing.T) {
		item := map[string]any{
			"timelineid": "2",
			"data":       map[string]any{"messageContent": "the real body"},
			"typ":        "sprava",
			"timestamp":  "2024-01-01 00:00:00",
			"text":       "Dôležitá správa: something",
		}

		event, ok := parseTimelineItem(item)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if event.Text != "the real body" {
			t.Errorf("Text = %q, want %q", event.Text, "the real body")
		}
	})

	t.Run("missing timelineid is skipped", func(t *testing.T) {
		_, ok := parseTimelineItem(map[string]any{"typ": "sprava"})
		if ok {
			t.Fatal("expected ok=false for missing timelineid")
		}
	})

	t.Run("non-numeric timelineid is skipped", func(t *testing.T) {
		_, ok := parseTimelineItem(map[string]any{"timelineid": "not-a-number"})
		if ok {
			t.Fatal("expected ok=false for non-numeric timelineid")
		}
	})
}

func TestPickHelpers(t *testing.T) {
	if got := pickString(float64(42)); got != "42" {
		t.Errorf("pickString(float64(42)) = %q, want %q", got, "42")
	}
	if got := pickString(nil); got != "" {
		t.Errorf("pickString(nil) = %q, want empty", got)
	}

	if got, ok := pickInt("17"); !ok || got != 17 {
		t.Errorf("pickInt(%q) = (%d, %v), want (17, true)", "17", got, ok)
	}
	if got, ok := pickInt("17.5"); !ok || got != 17 {
		t.Errorf("pickInt(%q) = (%d, %v), want (17, true)", "17.5", got, ok)
	}
	if _, ok := pickInt(""); ok {
		t.Error("pickInt(\"\") should not be ok")
	}

	if got, ok := pickFloat("3.5"); !ok || got != 3.5 {
		t.Errorf("pickFloat(\"3.5\") = (%v, %v), want (3.5, true)", got, ok)
	}

	// EduPage sometimes returns an empty object instead of an empty array.
	if got := pickSlice(map[string]any{}); len(got) != 0 {
		t.Errorf("pickSlice(empty map) = %#v, want empty slice", got)
	}
	if got := pickSlice(nil); got != nil {
		t.Errorf("pickSlice(nil) = %#v, want nil", got)
	}

	// Mixed digit groups collapse into one number, matching the Python
	// reference's filter(str.isdigit) behavior.
	if n, ok := parseIntDigits("3 - 4 lekcia"); !ok || n != 34 {
		t.Errorf("parseIntDigits(\"3 - 4 lekcia\") = (%d, %v), want (34, true)", n, ok)
	}
	if _, ok := parseIntDigits("no digits here"); ok {
		t.Error("parseIntDigits with no digits should not be ok")
	}
}
