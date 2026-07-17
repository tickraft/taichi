package framework

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
	"time"
)

// newResp builds a minimal *http.Response with only StatusCode populated, which is
// all AssertStatusCode inspects.
func newResp(status int) *http.Response {
	return &http.Response{StatusCode: status}
}

func TestAssertStatusCode(t *testing.T) {
	engine := NewAssertionEngine()

	t.Run("nil response returns failure", func(t *testing.T) {
		r := engine.AssertStatusCode(nil, 200)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
		if r.Message != "response is nil" {
			t.Fatalf("expected message %q, got %q", "response is nil", r.Message)
		}
		if r.Expected != 200 {
			t.Fatalf("expected Expected=200, got %v", r.Expected)
		}
		if r.Actual != nil {
			t.Fatalf("expected Actual=nil, got %v", r.Actual)
		}
	})

	t.Run("matching status code", func(t *testing.T) {
		r := engine.AssertStatusCode(newResp(200), 200)
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
		if r.Actual != 200 {
			t.Fatalf("expected Actual=200, got %v", r.Actual)
		}
	})

	t.Run("non-matching status code", func(t *testing.T) {
		r := engine.AssertStatusCode(newResp(404), 200)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
		if r.Actual != 404 {
			t.Fatalf("expected Actual=404, got %v", r.Actual)
		}
	})

	t.Run("table-driven status codes", func(t *testing.T) {
		cases := []struct {
			name     string
			actual   int
			expected int
			passed   bool
		}{
			{"200 matches 200", 200, 200, true},
			{"500 matches 500", 500, 500, true},
			{"404 vs 200", 404, 200, false},
			{"301 vs 200", 301, 200, false},
			{"0 vs 0", 0, 0, true},
		}
		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				r := engine.AssertStatusCode(newResp(c.actual), c.expected)
				if r.Passed != c.passed {
					t.Fatalf("expected Passed=%v, got %v (msg=%s)", c.passed, r.Passed, r.Message)
				}
			})
		}
	})
}

func TestAssertJSONField(t *testing.T) {
	engine := NewAssertionEngine()

	t.Run("valid JSON int field matches int expected", func(t *testing.T) {
		body := []byte(`{"count": 42, "name": "taichi"}`)
		r := engine.AssertJSONField(body, "count", 42)
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("valid JSON int field matches float64 expected", func(t *testing.T) {
		// JSON 42 decodes to float64(42), coerced to int(42); valuesEqual(int, float64) is true.
		body := []byte(`{"count": 42}`)
		r := engine.AssertJSONField(body, "count", float64(42))
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("fractional float field matches float64 expected", func(t *testing.T) {
		body := []byte(`{"ratio": 3.14}`)
		r := engine.AssertJSONField(body, "ratio", 3.14)
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("fractional float field mismatches int expected", func(t *testing.T) {
		body := []byte(`{"ratio": 3.14}`)
		r := engine.AssertJSONField(body, "ratio", 3)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})

	t.Run("string field matches", func(t *testing.T) {
		body := []byte(`{"name": "taichi"}`)
		r := engine.AssertJSONField(body, "name", "taichi")
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("string field mismatches", func(t *testing.T) {
		body := []byte(`{"name": "taichi"}`)
		r := engine.AssertJSONField(body, "name", "other")
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})

	t.Run("bool field matches", func(t *testing.T) {
		body := []byte(`{"ok": true}`)
		r := engine.AssertJSONField(body, "ok", true)
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("bool field mismatches", func(t *testing.T) {
		body := []byte(`{"ok": true}`)
		r := engine.AssertJSONField(body, "ok", false)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})

	t.Run("missing field returns failure", func(t *testing.T) {
		body := []byte(`{"a": 1}`)
		r := engine.AssertJSONField(body, "missing", 1)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
		if r.Message == "" {
			t.Fatalf("expected non-empty message")
		}
	})

	t.Run("invalid JSON returns failure", func(t *testing.T) {
		body := []byte(`{not json`)
		r := engine.AssertJSONField(body, "a", 1)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
		if r.Message == "" {
			t.Fatalf("expected non-empty message")
		}
	})
}

func TestAssertJSONFieldsExist(t *testing.T) {
	engine := NewAssertionEngine()

	t.Run("all fields present", func(t *testing.T) {
		body := []byte(`{"a": 1, "b": 2, "c": 3}`)
		r := engine.AssertJSONFieldsExist(body, "a", "b", "c")
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("some fields missing", func(t *testing.T) {
		body := []byte(`{"a": 1}`)
		r := engine.AssertJSONFieldsExist(body, "a", "b", "c")
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
		if r.Message == "" {
			t.Fatalf("expected non-empty message")
		}
	})

	t.Run("no fields requested", func(t *testing.T) {
		body := []byte(`{"a": 1}`)
		r := engine.AssertJSONFieldsExist(body)
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("invalid JSON returns failure", func(t *testing.T) {
		body := []byte(`{broken`)
		r := engine.AssertJSONFieldsExist(body, "a")
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})
}

func TestAssertHTMLContains(t *testing.T) {
	engine := NewAssertionEngine()

	t.Run("all substrings present", func(t *testing.T) {
		body := []byte(`<html><body><h1>Hello</h1><p>World</p></body></html>`)
		r := engine.AssertHTMLContains(body, "<h1>Hello</h1>", "World")
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("some substrings missing", func(t *testing.T) {
		body := []byte(`<html><body>Hello</body></html>`)
		r := engine.AssertHTMLContains(body, "Hello", "Missing")
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})

	t.Run("empty substring list returns success", func(t *testing.T) {
		body := []byte(`anything`)
		r := engine.AssertHTMLContains(body)
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
		if r.Message != "no substrings requested" {
			t.Fatalf("expected message %q, got %q", "no substrings requested", r.Message)
		}
	})
}

func TestAssertResponseTime(t *testing.T) {
	engine := NewAssertionEngine()

	t.Run("within limit", func(t *testing.T) {
		r := engine.AssertResponseTime(50*time.Millisecond, 100*time.Millisecond)
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		r := engine.AssertResponseTime(100*time.Millisecond, 100*time.Millisecond)
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("exceeds limit", func(t *testing.T) {
		r := engine.AssertResponseTime(200*time.Millisecond, 100*time.Millisecond)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})
}

func TestAssertJSONPath(t *testing.T) {
	engine := NewAssertionEngine()

	t.Run("simple top-level path", func(t *testing.T) {
		body := []byte(`{"name": "taichi"}`)
		r := engine.AssertJSONPath(body, "name", "taichi")
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("nested object path", func(t *testing.T) {
		body := []byte(`{"data": {"user": {"id": 7}}}`)
		r := engine.AssertJSONPath(body, "data.user.id", 7)
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("array index path", func(t *testing.T) {
		body := []byte(`{"data": {"items": [{"name": "first"}, {"name": "second"}]}}`)
		r := engine.AssertJSONPath(body, "data.items.0.name", "first")
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("array index second element", func(t *testing.T) {
		body := []byte(`{"data": {"items": [{"name": "first"}, {"name": "second"}]}}`)
		r := engine.AssertJSONPath(body, "data.items.1.name", "second")
		if !r.Passed {
			t.Fatalf("expected Passed=true, got false: %s", r.Message)
		}
	})

	t.Run("array index out of bounds fails", func(t *testing.T) {
		body := []byte(`{"data": {"items": [{"name": "first"}]}}`)
		r := engine.AssertJSONPath(body, "data.items.5.name", "first")
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})

	t.Run("missing key fails", func(t *testing.T) {
		body := []byte(`{"data": {"user": {}}}`)
		r := engine.AssertJSONPath(body, "data.user.id", 7)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})

	t.Run("empty segment fails", func(t *testing.T) {
		body := []byte(`{"data": 1}`)
		// The leading dot produces an empty first segment after Split.
		r := engine.AssertJSONPath(body, ".data", 1)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})

	t.Run("invalid JSON fails", func(t *testing.T) {
		body := []byte(`{not json`)
		r := engine.AssertJSONPath(body, "data", 1)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})

	t.Run("value mismatch fails", func(t *testing.T) {
		body := []byte(`{"a": 1}`)
		r := engine.AssertJSONPath(body, "a", 2)
		if r.Passed {
			t.Fatalf("expected Passed=false, got true")
		}
	})
}

func TestCoerceJSONNumber(t *testing.T) {
	cases := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{"json.Number int", json.Number("42"), int(42)},
		{"json.Number negative int", json.Number("-7"), int(-7)},
		{"json.Number float", json.Number("3.14"), float64(3.14)},
		// A number too large for int64 but valid as float64 is coerced to float64 (with precision loss).
		{"json.Number large beyond int64 parsed as float", json.Number("1e20"), float64(1e20)},
		// A json.Number that cannot be parsed as int or float falls through to its string form.
		{"json.Number unparseable falls back to string", json.Number("invalid"), "invalid"},
		{"float64 whole", float64(42), int(42)},
		{"float64 fractional", float64(3.14), float64(3.14)},
		{"float64 negative whole", float64(-5), int(-5)},
		{"string passthrough", "hello", "hello"},
		{"bool passthrough", true, true},
		{"nil passthrough", nil, nil},
		{"int passthrough", int(5), int(5)},
		{"int64 passthrough", int64(99), int64(99)},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := coerceJSONNumber(c.input)
			if got != c.expected {
				t.Fatalf("coerceJSONNumber(%T %v) = %T %v, expected %T %v",
					c.input, c.input, got, got, c.expected, c.expected)
			}
		})
	}
}

func TestIsWholeFloat(t *testing.T) {
	cases := []struct {
		name     string
		input    float64
		expected bool
	}{
		{"positive whole", 42.0, true},
		{"negative whole", -7.0, true},
		{"zero", 0.0, true},
		{"fractional", 3.14, false},
		{"negative fractional", -1.5, false},
		{"NaN", math.NaN(), false},
		// 2^64 is well beyond int64 range.
		// Note: 1<<63-1 cannot be used here because it rounds to 2^63 in float64,
		// and the int64(2^63) overflow conversion is implementation-defined per
		// the Go spec (saturates to MaxInt64 on some platforms, wraps to MinInt64
		// on others), making the result non-portable. 1<<62 is the largest power
		// of two safely inside int64 range and exactly representable as float64.
		{"large beyond int64", math.Pow(2, 64), false},
		{"large negative beyond int64", -math.Pow(2, 64), false},
		{"large int64 representable", float64(1 << 62), true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := isWholeFloat(c.input)
			if got != c.expected {
				t.Fatalf("isWholeFloat(%v) = %v, expected %v", c.input, got, c.expected)
			}
		})
	}
}

func TestValuesEqual(t *testing.T) {
	cases := []struct {
		name     string
		actual   interface{}
		expected interface{}
		want     bool
	}{
		{"int vs int equal", int(1), int(1), true},
		{"int vs int unequal", int(1), int(2), false},
		{"int vs float64 equal", int(1), float64(1), true},
		{"int vs float64 unequal", int(1), float64(2), false},
		{"int vs int64 equal", int(1), int64(1), true},
		{"int vs int32 equal", int(1), int32(1), true},
		{"int vs float32 equal", int(1), float32(1), true},
		{"int64 vs int equal", int64(1), int(1), true},
		{"int64 vs int64 equal", int64(1), int64(1), true},
		{"int64 vs float64 equal", int64(1), float64(1), true},
		{"float64 vs int equal", float64(1), int(1), true},
		{"float64 vs int64 equal", float64(1), int64(1), true},
		{"float64 vs float64 equal", float64(3.14), float64(3.14), true},
		{"float64 vs float32 equal", float64(1), float32(1), true},
		{"string vs string equal", "a", "a", true},
		{"string vs string unequal", "a", "b", false},
		{"bool vs bool equal", true, true, true},
		{"bool vs bool unequal", true, false, false},
		{"nil vs nil", nil, nil, true},
		{"nil vs int", nil, 1, false},
		{"int vs nil", 1, nil, false},
		{"string vs int mismatched types", "a", 1, false},
		{"bool vs string mismatched types", true, "true", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := valuesEqual(c.actual, c.expected)
			if got != c.want {
				t.Fatalf("valuesEqual(%T %v, %T %v) = %v, want %v",
					c.actual, c.actual, c.expected, c.expected, got, c.want)
			}
		})
	}
}

func TestNavigateSegment(t *testing.T) {
	t.Run("object key", func(t *testing.T) {
		obj := map[string]interface{}{"foo": "bar"}
		got := navigateSegment(obj, "foo")
		if got != "bar" {
			t.Fatalf("expected bar, got %v", got)
		}
	})

	t.Run("object missing key returns nil", func(t *testing.T) {
		obj := map[string]interface{}{"foo": "bar"}
		got := navigateSegment(obj, "missing")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("array index", func(t *testing.T) {
		arr := []interface{}{"a", "b", "c"}
		got := navigateSegment(arr, "1")
		if got != "b" {
			t.Fatalf("expected b, got %v", got)
		}
	})

	t.Run("array index zero", func(t *testing.T) {
		arr := []interface{}{"a", "b", "c"}
		got := navigateSegment(arr, "0")
		if got != "a" {
			t.Fatalf("expected a, got %v", got)
		}
	})

	t.Run("array out of bounds returns nil", func(t *testing.T) {
		arr := []interface{}{"a"}
		got := navigateSegment(arr, "5")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("negative array index returns nil", func(t *testing.T) {
		arr := []interface{}{"a"}
		got := navigateSegment(arr, "-1")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("numeric segment on non-array returns nil", func(t *testing.T) {
		obj := map[string]interface{}{"foo": "bar"}
		got := navigateSegment(obj, "0")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("string segment on non-object returns nil", func(t *testing.T) {
		arr := []interface{}{"a"}
		got := navigateSegment(arr, "foo")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("segment on scalar returns nil", func(t *testing.T) {
		got := navigateSegment("scalar", "foo")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("segment on nil returns nil", func(t *testing.T) {
		got := navigateSegment(nil, "foo")
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}
