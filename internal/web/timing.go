package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// requestStartKey carries the wall-clock start of a request through its
// context, so render can report end-to-end latency in a Server-Timing header
// without every handler having to thread a timer through its signature.
type requestStartKey struct{}

// withRequestTiming stamps each request with its start time. It wraps the
// whole route table, so the reported total includes middleware and the
// upstream EduPage calls a handler makes, not just template rendering —
// which is exactly the part that used to dominate a page load.
func withRequestTiming(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), requestStartKey{}, time.Now())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// setServerTiming writes a Server-Timing header with the request's total and
// template-render durations, both in milliseconds. It must be called before
// the response headers are written. When the request was not run through
// withRequestTiming (e.g. a handler called directly by a test) it does
// nothing, so it is always safe to call.
//
// total - render is a close approximation of upstream EduPage time, which is
// the number this header exists to expose.
func setServerTiming(w http.ResponseWriter, r *http.Request, render time.Duration) {
	start, ok := r.Context().Value(requestStartKey{}).(time.Time)
	if !ok {
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "total;dur=%s, render;dur=%s", millisValue(time.Since(start)), millisValue(render))
	w.Header().Set("Server-Timing", b.String())
}

// millisValue formats a duration as a millisecond count with one decimal
// place, the shape the Server-Timing spec expects (e.g. "512.3").
func millisValue(d time.Duration) string {
	return strconv.FormatFloat(float64(d.Microseconds())/1000, 'f', 1, 64)
}
