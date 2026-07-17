package skill

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
)

// DefaultHTTPTimeout is the default timeout for skill HTTP requests.
const DefaultHTTPTimeout = 5 * time.Second

// HTTPRequest performs an HTTP request and returns the response, body, and error.
// On failure it returns a non-empty error and a zero-value response. The response body is fully read and the Body is closed.
func HTTPRequest(client *http.Client, method, url string, headers map[string]string) (*http.Response, []byte, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build request %s %s: %w", method, url, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("perform request %s %s: %w", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("read response body %s %s: %w", method, url, err)
	}
	return resp, body, nil
}

// RecordResult records a result onto the reporter, automatically computing the duration since start.
func RecordResult(reporter *framework.TestReporter, name string, start time.Time, passed bool, message string, err error) {
	reporter.Record(framework.TestResult{
		Name:     name,
		Passed:   passed,
		Message:  message,
		Duration: time.Since(start),
		Error:    err,
	})
}

// AssertCommonEnvelope verifies that body carries the top-level code/msg/request_id fields,
// and that code equals expectedCode.
// This is the unified response contract of the tickraft ecosystem. Returns a field-existence assertion and a code-value assertion.
func AssertCommonEnvelope(asserts *framework.AssertionEngine, body []byte, expectedCode int) (framework.AssertResult, framework.AssertResult) {
	fields := asserts.AssertJSONFieldsExist(body, "code", "msg", "request_id")
	if !fields.Passed {
		return fields, framework.AssertResult{}
	}
	code := asserts.AssertJSONField(body, "code", expectedCode)
	return fields, code
}

// GetString reads a string field from the raw map; returns fallback when missing.
func GetString(raw map[string]any, key, fallback string) string {
	if v, ok := raw[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

// GetDuration reads a duration field from the raw map; returns fallback when missing.
func GetDuration(raw map[string]any, key string, fallback time.Duration) time.Duration {
	if v, ok := raw[key]; ok {
		switch d := v.(type) {
		case string:
			if parsed, err := time.ParseDuration(d); err == nil {
				return parsed
			}
		case time.Duration:
			return d
		}
	}
	return fallback
}

// GetInt reads an integer field from the raw map; returns fallback when missing.
func GetInt(raw map[string]any, key string, fallback int) int {
	if v, ok := raw[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return fallback
}

// GetBool reads a boolean field from the raw map; returns fallback when missing.
func GetBool(raw map[string]any, key string, fallback bool) bool {
	if v, ok := raw[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return fallback
}

// ToStringSlice converts an []any to []string, skipping non-string entries.
// Returns nil when v is nil or not a slice. Useful for skill configs that
// receive YAML lists as []any.
func ToStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ToStringMap converts a map[string]any to a map[string]string, formatting
// each value with fmt.Sprintf("%v"). Returns nil when v is nil or not a map.
func ToStringMap(v any) map[string]string {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		out[k] = fmt.Sprintf("%v", val)
	}
	return out
}

// ErrorString safely converts an error to its string representation,
// returning an empty string for a nil error.
func ErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
