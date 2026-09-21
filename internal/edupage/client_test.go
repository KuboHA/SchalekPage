package edupage

import (
	"errors"
	"testing"
)

func TestAsString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"already a string", "already a string"},
		{float64(42), "42"},
		{float64(3.5), "3.5"},
		{true, "1"},
		{false, "0"},
	}
	for _, tc := range cases {
		if got := asString(tc.in); got != tc.want {
			t.Errorf("asString(%#v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAsInt(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{nil, 0},
		{float64(7), 7},
		{"7", 7},
		{"  7  ", 7},
		{"not a number", 0},
		{"", 0},
		{"3.0", 3},
	}
	for _, tc := range cases {
		if got := asInt(tc.in); got != tc.want {
			t.Errorf("asInt(%#v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestAsMapRejectsNonObjects(t *testing.T) {
	if asMap(nil) != nil {
		t.Error("asMap(nil) should be nil")
	}
	if asMap([]any{}) != nil {
		t.Error("asMap of an array (EduPage's empty-object-as-array quirk) should be nil, not panic")
	}
	if asMap("a string") != nil {
		t.Error("asMap of a string should be nil")
	}
	m := map[string]any{"k": "v"}
	if got := asMap(m); got == nil || got["k"] != "v" {
		t.Error("asMap should pass through a real object")
	}
}

func TestAsBool(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{float64(1), true},
		{float64(0), false},
		{"1", true},
		{"0", false},
		{"true", true},
		{"garbage", false},
	}
	for _, tc := range cases {
		if got := asBool(tc.in); got != tc.want {
			t.Errorf("asBool(%#v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSessionIDNoSubdomainYet(t *testing.T) {
	c := New(0)
	if got := c.SessionID(); got != "" {
		t.Errorf("SessionID() before login = %q, want empty", got)
	}
}

func TestHasReloadKey(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"no reload key at all", `{"r":"<html>...</html>"}`, false},
		{"reload: true", `{"reload":true}`, true},
		{"reload: false", `{"reload":false}`, false},
		{"reload: null", `{"reload":null}`, false},
		{"reload as a non-bool truthy value", `{"reload":1}`, true},
		{"reload as a non-empty string", `{"reload":"1"}`, true},
		{"not JSON at all", `not json`, false},
		{"empty body", ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasReloadKey([]byte(tc.body)); got != tc.want {
				t.Errorf("hasReloadKey(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestRetryOnReload_NoReloadPassesThrough(t *testing.T) {
	fetchCalls, restoreCalls := 0, 0
	fetch := func() ([]byte, error) {
		fetchCalls++
		return []byte(`{"ok":true}`), nil
	}
	restore := func() error {
		restoreCalls++
		return nil
	}

	got, err := retryOnReload(fetch, hasReloadKey, restore)
	if err != nil {
		t.Fatalf("retryOnReload() error = %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Errorf("retryOnReload() = %q", got)
	}
	if fetchCalls != 1 {
		t.Errorf("fetch called %d times, want 1 (no reload -> no retry)", fetchCalls)
	}
	if restoreCalls != 0 {
		t.Errorf("restore called %d times, want 0", restoreCalls)
	}
}

func TestRetryOnReload_ReloadThenSuccessRetries(t *testing.T) {
	fetchCalls, restoreCalls := 0, 0
	fetch := func() ([]byte, error) {
		fetchCalls++
		if fetchCalls == 1 {
			return []byte(`{"reload":true}`), nil
		}
		return []byte(`{"ok":true}`), nil
	}
	restore := func() error {
		restoreCalls++
		return nil
	}

	got, err := retryOnReload(fetch, hasReloadKey, restore)
	if err != nil {
		t.Fatalf("retryOnReload() error = %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Errorf("retryOnReload() = %q, want the retried response", got)
	}
	if fetchCalls != 2 {
		t.Errorf("fetch called %d times, want exactly 2 (initial + one retry)", fetchCalls)
	}
	if restoreCalls != 1 {
		t.Errorf("restore called %d times, want exactly 1", restoreCalls)
	}
}

func TestRetryOnReload_StillReloadingGivesUp(t *testing.T) {
	fetchCalls, restoreCalls := 0, 0
	fetch := func() ([]byte, error) {
		fetchCalls++
		return []byte(`{"reload":true}`), nil
	}
	restore := func() error {
		restoreCalls++
		return nil
	}

	_, err := retryOnReload(fetch, hasReloadKey, restore)
	if !errors.Is(err, ErrSessionExpired) {
		t.Errorf("retryOnReload() error = %v, want it to wrap ErrSessionExpired", err)
	}
	// Never more than one retry, no matter how persistently the endpoint
	// keeps reporting "reload" — this is the infinite-recursion guard.
	if fetchCalls != 2 {
		t.Errorf("fetch called %d times, want exactly 2 (no more retries after the first)", fetchCalls)
	}
	if restoreCalls != 1 {
		t.Errorf("restore called %d times, want exactly 1", restoreCalls)
	}
}

func TestRetryOnReload_RestoreFailureWrapsSessionExpired(t *testing.T) {
	fetch := func() ([]byte, error) {
		return []byte(`{"reload":true}`), nil
	}
	restoreErr := errors.New("boom")
	restore := func() error {
		return restoreErr
	}

	_, err := retryOnReload(fetch, hasReloadKey, restore)
	if !errors.Is(err, ErrSessionExpired) {
		t.Errorf("retryOnReload() error = %v, want it to wrap ErrSessionExpired", err)
	}
}

func TestRetryOnReload_FetchErrorPassesThroughWithoutRestoring(t *testing.T) {
	fetchErr := errors.New("network down")
	restoreCalls := 0
	fetch := func() ([]byte, error) {
		return nil, fetchErr
	}
	restore := func() error {
		restoreCalls++
		return nil
	}

	_, err := retryOnReload(fetch, hasReloadKey, restore)
	if !errors.Is(err, fetchErr) {
		t.Errorf("retryOnReload() error = %v, want it to wrap the fetch error", err)
	}
	if restoreCalls != 0 {
		t.Errorf("restore called %d times, want 0 (a transport error is not a reload)", restoreCalls)
	}
}
