package framework

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AssertionEngine provides a collection of assertion helpers for HTTP responses, JSON payloads, HTML bodies, and timing expectations.
//
// All methods return an AssertResult describing the outcome and are concurrency-safe: the engine itself holds no mutable state.
type AssertionEngine struct{}

// NewAssertionEngine returns an AssertionEngine ready to use.
func NewAssertionEngine() *AssertionEngine {
	return &AssertionEngine{}
}

// AssertStatusCode compares the HTTP status code of resp with the expected value, returning an AssertResult describing the outcome.
func (a *AssertionEngine) AssertStatusCode(resp *http.Response, expected int) AssertResult {
	if resp == nil {
		return AssertResult{
			Passed:   false,
			Message:  "response is nil",
			Expected: expected,
			Actual:   nil,
		}
	}
	actual := resp.StatusCode
	passed := actual == expected
	msg := fmt.Sprintf("expected status %d, got %d", expected, actual)
	if passed {
		msg = fmt.Sprintf("status code %d matches expected", expected)
	}
	return AssertResult{
		Passed:   passed,
		Message:  msg,
		Expected: expected,
		Actual:   actual,
	}
}

// AssertJSONField parses body as a JSON object, extracts the top-level field, and compares it with expected.
// JSON numbers are coerced to int or float64 so callers can pass any numeric type.
func (a *AssertionEngine) AssertJSONField(body []byte, field string, expected interface{}) AssertResult {
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		return AssertResult{
			Passed:   false,
			Message:  fmt.Sprintf("failed to parse JSON: %v", err),
			Expected: expected,
			Actual:   nil,
		}
	}
	raw, ok := obj[field]
	if !ok {
		return AssertResult{
			Passed:   false,
			Message:  fmt.Sprintf("field %q not found", field),
			Expected: expected,
			Actual:   nil,
		}
	}
	actual := coerceJSONNumber(raw)
	if valuesEqual(actual, expected) {
		return AssertResult{
			Passed:   true,
			Message:  fmt.Sprintf("field %q matches expected value", field),
			Expected: expected,
			Actual:   actual,
		}
	}
	return AssertResult{
		Passed:   false,
		Message:  fmt.Sprintf("field %q expected %v (%T), got %v (%T)", field, expected, expected, actual, actual),
		Expected: expected,
		Actual:   actual,
	}
}

// AssertJSONFieldsExist parses body as a JSON object and verifies that all listed top-level fields exist.
func (a *AssertionEngine) AssertJSONFieldsExist(body []byte, fields ...string) AssertResult {
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		return AssertResult{
			Passed:   false,
			Message:  fmt.Sprintf("failed to parse JSON: %v", err),
			Expected: fields,
			Actual:   nil,
		}
	}
	missing := make([]string, 0, len(fields))
	for _, f := range fields {
		if _, ok := obj[f]; !ok {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return AssertResult{
			Passed:   true,
			Message:  fmt.Sprintf("all %d fields present", len(fields)),
			Expected: fields,
			Actual:   fields,
		}
	}
	return AssertResult{
		Passed:   false,
		Message:  fmt.Sprintf("missing fields: %s", strings.Join(missing, ", ")),
		Expected: fields,
		Actual:   missing,
	}
}

// AssertHTMLContains converts body to a string and verifies that all substrings are present.
func (a *AssertionEngine) AssertHTMLContains(body []byte, substrings ...string) AssertResult {
	if len(substrings) == 0 {
		return AssertResult{
			Passed:   true,
			Message:  "no substrings requested",
			Expected: substrings,
			Actual:   substrings,
		}
	}
	content := string(body)
	missing := make([]string, 0, len(substrings))
	for _, s := range substrings {
		if !strings.Contains(content, s) {
			missing = append(missing, s)
		}
	}
	if len(missing) == 0 {
		return AssertResult{
			Passed:   true,
			Message:  fmt.Sprintf("all %d substrings found", len(substrings)),
			Expected: substrings,
			Actual:   substrings,
		}
	}
	return AssertResult{
		Passed:   false,
		Message:  fmt.Sprintf("missing substrings: %s", strings.Join(missing, ", ")),
		Expected: substrings,
		Actual:   missing,
	}
}

// AssertResponseTime compares the observed duration with the maximum allowed duration.
func (a *AssertionEngine) AssertResponseTime(duration time.Duration, max time.Duration) AssertResult {
	passed := duration <= max
	msg := fmt.Sprintf("duration %s within limit %s", duration, max)
	if !passed {
		msg = fmt.Sprintf("duration %s exceeds limit %s", duration, max)
	}
	return AssertResult{
		Passed:   passed,
		Message:  msg,
		Expected: max.String(),
		Actual:   duration.String(),
	}
}

// AssertJSONPath uses dotted-path notation (numeric segments index arrays, e.g. "data.items.0.name")
// to navigate the JSON tree of body; the resolved value is compared with expected using the same coercion rules.
func (a *AssertionEngine) AssertJSONPath(body []byte, path string, expected interface{}) AssertResult {
	var root interface{}
	if err := json.Unmarshal(body, &root); err != nil {
		return AssertResult{
			Passed:   false,
			Message:  fmt.Sprintf("failed to parse JSON: %v", err),
			Expected: expected,
			Actual:   nil,
		}
	}
	current := root
	segments := strings.Split(path, ".")
	for _, seg := range segments {
		if seg == "" {
			return AssertResult{
				Passed:   false,
				Message:  fmt.Sprintf("invalid empty segment in path %q", path),
				Expected: expected,
				Actual:   nil,
			}
		}
		current = navigateSegment(current, seg)
		if current == nil {
			return AssertResult{
				Passed:   false,
				Message:  fmt.Sprintf("path %q could not be resolved at segment %q", path, seg),
				Expected: expected,
				Actual:   nil,
			}
		}
	}
	actual := coerceJSONNumber(current)
	if valuesEqual(actual, expected) {
		return AssertResult{
			Passed:   true,
			Message:  fmt.Sprintf("path %q matches expected value", path),
			Expected: expected,
			Actual:   actual,
		}
	}
	return AssertResult{
		Passed:   false,
		Message:  fmt.Sprintf("path %q expected %v (%T), got %v (%T)", path, expected, expected, actual, actual),
		Expected: expected,
		Actual:   actual,
	}
}

// navigateSegment advances the cursor by a single path segment in the JSON tree.
// Numeric segments index arrays/slices; other segments index objects. Returns nil when not applicable.
func navigateSegment(current interface{}, seg string) interface{} {
	if index, err := strconv.Atoi(seg); err == nil {
		arr, ok := current.([]interface{})
		if !ok {
			return nil
		}
		if index < 0 || index >= len(arr) {
			return nil
		}
		return arr[index]
	}
	obj, ok := current.(map[string]interface{})
	if !ok {
		return nil
	}
	return obj[seg]
}

// coerceJSONNumber converts a json.Number to int or float64 when possible,
// so callers can compare against any Go numeric type. Other values are returned as-is.
func coerceJSONNumber(v interface{}) interface{} {
	switch n := v.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
		if f, err := n.Float64(); err == nil {
			return f
		}
		return n.String()
	case float64:
		// encoding/json produces float64 when UseNumber is not enabled; coerce integer values to int
		// so callers can compare against int literals.
		if isWholeFloat(n) {
			return int(n)
		}
		return n
	default:
		return v
	}
}

// isWholeFloat reports whether f can represent an integer without loss.
func isWholeFloat(f float64) bool {
	if f != f || f > 1<<63-1 || f < -(1<<63-1) {
		return false
	}
	return float64(int64(f)) == f
}

// valuesEqual compares two values using the same coercion rules, so an int expected value can match a float64 actual value, etc.
func valuesEqual(actual, expected interface{}) bool {
	if actual == nil || expected == nil {
		return actual == expected
	}
	switch a := actual.(type) {
	case int:
		switch e := expected.(type) {
		case int:
			return a == e
		case int32:
			return int64(a) == int64(e)
		case int64:
			return int64(a) == e
		case float32:
			return float64(a) == float64(e)
		case float64:
			return float64(a) == e
		}
	case int64:
		switch e := expected.(type) {
		case int:
			return a == int64(e)
		case int64:
			return a == e
		case float64:
			return float64(a) == e
		}
	case float64:
		switch e := expected.(type) {
		case int:
			return a == float64(e)
		case int64:
			return a == float64(e)
		case float64:
			return a == e
		case float32:
			return a == float64(e)
		}
	case string:
		if e, ok := expected.(string); ok {
			return a == e
		}
	case bool:
		if e, ok := expected.(bool); ok {
			return a == e
		}
	}
	return actual == expected
}
