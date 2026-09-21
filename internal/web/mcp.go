// The Model Context Protocol endpoint: a single POST /mcp implementing the
// JSON-RPC 2.0 "Streamable HTTP" transport's non-streaming case (one request
// in, one JSON response out — no server-initiated SSE stream, which no tool
// here needs). Authentication is a bearer token minted by the OAuth flow in
// oauth.go, resolved back to the same *Session (and its live
// *edupage.Client) the browser dashboard uses.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

const mcpProtocolVersion = "2025-06-18"

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	jsonrpcParseError     = -32700
	jsonrpcInvalidRequest = -32600
	jsonrpcMethodNotFound = -32601
	jsonrpcInvalidParams  = -32602
	jsonrpcInternalError  = -32603
)

// mcpTool describes one tool's schema and how to run it against a session's
// live edupage.Client.
type mcpTool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Run         func(sess *Session, now time.Time, args json.RawMessage) (any, error)
}

// handleMCP serves POST /mcp: bearer-token authenticated JSON-RPC 2.0.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionFromBearer(r)
	if !ok {
		base := externalBaseURL(r)
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+base+`/.well-known/oauth-protected-resource"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req jsonrpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONRPC(w, jsonrpcResponse{JSONRPC: "2.0", Error: &jsonrpcError{Code: jsonrpcParseError, Message: "parse error"}})
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		writeJSONRPC(w, jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &jsonrpcError{Code: jsonrpcInvalidRequest, Message: "invalid request"}})
		return
	}

	// A JSON-RPC request with no "id" is a notification: the client neither
	// expects nor wants a response body.
	isNotification := len(req.ID) == 0

	result, rpcErr := s.dispatchMCP(sess, req)
	if isNotification {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := jsonrpcResponse{JSONRPC: "2.0", ID: req.ID}
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = result
	}
	writeJSONRPC(w, resp)
}

func writeJSONRPC(w http.ResponseWriter, resp jsonrpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// sessionFromBearer resolves the caller's session via "Authorization:
// Bearer <token>", the MCP counterpart to sessionFromRequest's cookie
// lookup.
func (s *Server) sessionFromBearer(r *http.Request) (*Session, bool) {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return nil, false
	}
	token := strings.TrimPrefix(auth, prefix)
	sessionID, ok := s.oauth.sessionIDForToken(token)
	if !ok {
		return nil, false
	}
	sess, ok := s.sessions.Get(sessionID)
	if !ok || !sess.Authenticated() {
		return nil, false
	}
	return sess, true
}

func (s *Server) dispatchMCP(sess *Session, req jsonrpcRequest) (any, *jsonrpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "schalekpage-edupage",
				"version": "0.1.0",
			},
		}, nil

	case "notifications/initialized", "ping":
		return map[string]any{}, nil

	case "tools/list":
		tools := make([]map[string]any, 0, len(mcpTools))
		for _, t := range mcpTools {
			tools = append(tools, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		return map[string]any{"tools": tools}, nil

	case "tools/call":
		return s.callMCPTool(sess, req.Params)

	default:
		return nil, &jsonrpcError{Code: jsonrpcMethodNotFound, Message: "method not found: " + req.Method}
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) callMCPTool(sess *Session, rawParams json.RawMessage) (any, *jsonrpcError) {
	var params toolCallParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, &jsonrpcError{Code: jsonrpcInvalidParams, Message: "invalid params"}
	}

	tool, ok := mcpToolsByName[params.Name]
	if !ok {
		return nil, &jsonrpcError{Code: jsonrpcInvalidParams, Message: "unknown tool: " + params.Name}
	}

	data, err := tool.Run(sess, s.now(), params.Arguments)
	if err != nil {
		// Tool failures are reported inside the JSON-RPC *result* as
		// isError, not as a JSON-RPC protocol error, per the MCP spec: the
		// call itself succeeded, the underlying EduPage operation didn't.
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		}, nil
	}

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, &jsonrpcError{Code: jsonrpcInternalError, Message: "failed to encode result"}
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(encoded)}},
	}, nil
}

// --- tool argument decoding helpers ---

type dateArg struct {
	Date string `json:"date"`
}

func decodeArgs[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	err := json.Unmarshal(raw, &v)
	return v, err
}

// --- tools ---

var mcpTools = []mcpTool{
	{
		Name:        "get_student_info",
		Description: "The logged-in student's name, class, school subdomain and username.",
		InputSchema: emptySchema(),
		Run: func(sess *Session, now time.Time, args json.RawMessage) (any, error) {
			students, err := sess.Client.Students()
			if err != nil {
				return nil, err
			}
			student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)

			var className string
			if studentOK {
				if classes, err := sess.Client.Classes(); err == nil {
					for _, c := range classes {
						if c.ClassID == student.ClassID {
							className = c.Short
							if className == "" {
								className = c.Name
							}
							break
						}
					}
				}
			}

			return map[string]any{
				"username":     sess.Username,
				"subdomain":    sess.Subdomain,
				"student_name": sess.StudentName,
				"class":        className,
				"resolved":     studentOK,
			}, nil
		},
	},
	{
		Name:        "get_grades",
		Description: "The student's grades for a school year and term (defaults to the current year, second term).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"year": map[string]any{"type": "integer", "description": "School year the term started in, e.g. 2024. Defaults to the current school year."},
				"term": map[string]any{"type": "string", "enum": []string{"P1", "P2"}, "description": "P1 (first term) or P2 (second term). Defaults to P2."},
			},
		},
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			args, err := decodeArgs[struct {
				Year int    `json:"year"`
				Term string `json:"term"`
			}](raw)
			if err != nil {
				return nil, err
			}
			year := args.Year
			if year == 0 {
				year, err = sess.Client.SchoolYear()
				if err != nil {
					return nil, err
				}
			}
			term := edupage.TermSecond
			if args.Term == string(edupage.TermFirst) {
				term = edupage.TermFirst
			}
			return sess.Client.GradesForTerm(year, term)
		},
	},
	{
		Name:        "get_timetable",
		Description: "The student's lesson timetable for a given day (defaults to today).",
		InputSchema: dateSchema("The day to fetch, as YYYY-MM-DD. Defaults to today."),
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			args, err := decodeArgs[dateArg](raw)
			if err != nil {
				return nil, err
			}
			day := parseDateOrToday(args.Date, now)
			return sess.Client.MyTimetable(day)
		},
	},
	{
		Name:        "get_lunches",
		Description: "Snack, lunch and afternoon-snack menus (and ordering status) for a given day (defaults to today).",
		InputSchema: dateSchema("The day to fetch, as YYYY-MM-DD. Defaults to today."),
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			args, err := decodeArgs[dateArg](raw)
			if err != nil {
				return nil, err
			}
			day := parseDateOrToday(args.Date, now)
			return sess.Client.MealsFor(day)
		},
	},
	{
		Name:        "get_substitutions",
		Description: "Timetable changes (substitutions, cancellations, room changes) for the student's class on a given day (defaults to today).",
		InputSchema: dateSchema("The day to fetch, as YYYY-MM-DD. Defaults to today."),
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			args, err := decodeArgs[dateArg](raw)
			if err != nil {
				return nil, err
			}
			day := parseDateOrToday(args.Date, now)
			return sess.Client.TimetableChanges(day)
		},
	},
	{
		Name:        "get_notifications",
		Description: "The student's recent EduPage timeline notifications (messages, confirmations, grades posted, etc.), newest first.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":     map[string]any{"type": "integer", "description": "Maximum number of notifications to return. Defaults to 20."},
				"date_from": map[string]any{"type": "string", "description": "Optional YYYY-MM-DD. When given, fetches timeline history back to this date instead of only the ~1 month cached at login."},
			},
		},
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			args, err := decodeArgs[struct {
				Limit    int    `json:"limit"`
				DateFrom string `json:"date_from"`
			}](raw)
			if err != nil {
				return nil, err
			}
			var events []edupage.TimelineEvent
			if args.DateFrom != "" {
				from, perr := time.Parse(dateLayout, args.DateFrom)
				if perr != nil {
					return nil, fmt.Errorf("date_from must be YYYY-MM-DD: %w", perr)
				}
				events, err = sess.Client.NotificationsSince(from)
			} else {
				events, err = sess.Client.Notifications()
			}
			if err != nil {
				return nil, err
			}
			limit := args.Limit
			if limit <= 0 {
				limit = 20
			}
			if limit > len(events) {
				limit = len(events)
			}
			return events[:limit], nil
		},
	},
	{
		Name:        "get_homework",
		Description: "Homework assignments, derived from the EduPage timeline (EduPage has no homework API). Completion state is read-only.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date_from": map[string]any{"type": "string", "description": "Optional YYYY-MM-DD to search further back than the ~1 month cached at login."},
			},
		},
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			events, err := timelineForTool(sess, raw)
			if err != nil {
				return nil, err
			}
			return buildHomeworkPageView(events, now), nil
		},
	},
	{
		Name:        "get_exams",
		Description: "Upcoming and past tests/exams, derived from the EduPage timeline.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date_from": map[string]any{"type": "string", "description": "Optional YYYY-MM-DD to search further back than the ~1 month cached at login."},
			},
		},
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			events, err := timelineForTool(sess, raw)
			if err != nil {
				return nil, err
			}
			return buildExamsPageView(events, now), nil
		},
	},
	{
		Name:        "get_ringing_times",
		Description: "The school's bell schedule for the day, plus the next bell after a given time. Answered from cached login data, so it costs no request to EduPage.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"after": map[string]any{"type": "string", "description": "Optional RFC3339 timestamp; the next bell is computed relative to this. Defaults to now."},
			},
		},
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			args, err := decodeArgs[struct {
				After string `json:"after"`
			}](raw)
			if err != nil {
				return nil, err
			}
			after := now
			if args.After != "" {
				parsed, perr := time.Parse(time.RFC3339, args.After)
				if perr != nil {
					return nil, fmt.Errorf("after must be an RFC3339 timestamp: %w", perr)
				}
				after = parsed
			}
			schedule, err := sess.Client.RingingTimes()
			if err != nil {
				return nil, err
			}
			next, err := sess.Client.NextRingingTime(after)
			if err != nil {
				return nil, err
			}
			return map[string]any{"schedule": schedule, "next": next}, nil
		},
	},
	{
		Name: "order_lunch",
		Description: "WRITE — places a REAL order in the school canteen, changing the student's actual meal booking. " +
			"Ordering closes at the canteen's deadline. Confirm with the user before calling.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date": map[string]any{"type": "string", "description": "Day to order for, YYYY-MM-DD. Defaults to today."},
				"meal": map[string]any{"type": "string", "enum": []string{"1", "2", "3"}, "description": "1 = snack, 2 = lunch, 3 = afternoon snack."},
				"menu": map[string]any{"type": "integer", "description": "Which menu option to order, 1-8 (1 = Menu I, 2 = Menu II, ...)."},
			},
			"required": []string{"meal", "menu"},
		},
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			args, err := decodeArgs[struct {
				Date string `json:"date"`
				Meal string `json:"meal"`
				Menu int    `json:"menu"`
			}](raw)
			if err != nil {
				return nil, err
			}
			day := parseDateOrToday(args.Date, now)
			meal, err := mealForWrite(sess, day, args.Meal, now)
			if err != nil {
				return nil, err
			}
			if err := sess.Client.ChooseMeal(meal, args.Menu); err != nil {
				return nil, err
			}
			return map[string]any{"ordered": true, "date": day.Format(dateLayout), "meal": args.Meal, "menu": args.Menu}, nil
		},
	},
	{
		Name:        "sign_off_lunch",
		Description: "WRITE — cancels a REAL canteen order for the given meal. Confirm with the user before calling.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date": map[string]any{"type": "string", "description": "Day to cancel, YYYY-MM-DD. Defaults to today."},
				"meal": map[string]any{"type": "string", "enum": []string{"1", "2", "3"}, "description": "1 = snack, 2 = lunch, 3 = afternoon snack."},
			},
			"required": []string{"meal"},
		},
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			args, err := decodeArgs[struct {
				Date string `json:"date"`
				Meal string `json:"meal"`
			}](raw)
			if err != nil {
				return nil, err
			}
			day := parseDateOrToday(args.Date, now)
			meal, err := mealForWrite(sess, day, args.Meal, now)
			if err != nil {
				return nil, err
			}
			if err := sess.Client.SignOffMeal(meal); err != nil {
				return nil, err
			}
			return map[string]any{"cancelled": true, "date": day.Format(dateLayout), "meal": args.Meal}, nil
		},
	},
	{
		Name: "send_message",
		Description: "WRITE — sends a REAL message to real people on the school's EduPage. " +
			"Recipients are ids like \"Student123\" or \"Teacher45\"; the literal \"*\" addresses the ENTIRE SCHOOL and must never be used without explicit user instruction. " +
			"Always confirm recipients and body with the user before calling.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recipients": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Recipient ids, e.g. [\"Teacher45\"]. Use get_student_info / the roster to resolve names to ids.",
				},
				"text": map[string]any{"type": "string", "description": "The message body."},
			},
			"required": []string{"recipients", "text"},
		},
		Run: func(sess *Session, now time.Time, raw json.RawMessage) (any, error) {
			args, err := decodeArgs[struct {
				Recipients []string `json:"recipients"`
				Text       string   `json:"text"`
			}](raw)
			if err != nil {
				return nil, err
			}
			if len(args.Recipients) == 0 {
				return nil, fmt.Errorf("recipients must not be empty")
			}
			if strings.TrimSpace(args.Text) == "" {
				return nil, fmt.Errorf("text must not be empty")
			}
			id, err := sess.Client.SendMessage(args.Recipients, args.Text)
			if err != nil {
				return nil, err
			}
			return map[string]any{"sent": true, "timeline_id": id, "recipients": args.Recipients}, nil
		},
	},
}

// timelineForTool resolves the shared optional `date_from` argument used by
// the timeline-derived tools, falling back to the cached login payload.
func timelineForTool(sess *Session, raw json.RawMessage) ([]edupage.TimelineEvent, error) {
	args, err := decodeArgs[struct {
		DateFrom string `json:"date_from"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if args.DateFrom == "" {
		return sess.Client.Notifications()
	}
	from, err := time.Parse(dateLayout, args.DateFrom)
	if err != nil {
		return nil, fmt.Errorf("date_from must be YYYY-MM-DD: %w", err)
	}
	return sess.Client.NotificationsSince(from)
}

// mealForWrite loads the meal a canteen write targets and refuses when the
// ordering deadline has passed, so a tool call fails with a clear reason
// rather than being rejected opaquely by EduPage.
func mealForWrite(sess *Session, day time.Time, index string, now time.Time) (*edupage.Meal, error) {
	meals, err := sess.Client.MealsFor(day)
	if err != nil {
		return nil, err
	}
	meal := mealByIndex(meals, index)
	if meal == nil {
		return nil, fmt.Errorf("no meal %q is served on %s", index, day.Format(dateLayout))
	}
	if !mealChangeable(meal, now) {
		return nil, fmt.Errorf("the ordering deadline for that meal has passed")
	}
	return meal, nil
}

var mcpToolsByName = func() map[string]mcpTool {
	m := make(map[string]mcpTool, len(mcpTools))
	for _, t := range mcpTools {
		m[t.Name] = t
	}
	return m
}()

func emptySchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func dateSchema(dateDescription string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"date": map[string]any{"type": "string", "description": dateDescription},
		},
	}
}
