package skill

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
)

func TestDefaultHTTPTimeout(t *testing.T) {
	if DefaultHTTPTimeout != 5*time.Second {
		t.Errorf("DefaultHTTPTimeout = %v, want 5s", DefaultHTTPTimeout)
	}
}

func TestHTTPRequest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Test") != "val" {
				t.Errorf("header X-Test = %q, want 'val'", r.Header.Get("X-Test"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
		}))
		defer server.Close()

		resp, body, err := HTTPRequest(&http.Client{Timeout: DefaultHTTPTimeout}, "GET", server.URL, map[string]string{"X-Test": "val"})
		if err != nil {
			t.Fatalf("HTTPRequest err: %v", err)
		}
		if resp == nil {
			t.Fatalf("resp is nil")
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
		}
		if string(body) != "hello" {
			t.Errorf("body = %q, want 'hello'", string(body))
		}
	})

	t.Run("MalformedURL", func(t *testing.T) {
		// An unclosed IPv6 bracket makes url.Parse fail, exercising the NewRequest error path.
		_, _, err := HTTPRequest(&http.Client{}, "GET", "http://[::1", nil)
		if err == nil {
			t.Errorf("expected error for malformed URL, got nil")
		}
	})

	t.Run("ConnectionRefused", func(t *testing.T) {
		// Start a server, grab its address, then close it so the next request gets refused.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		addr := server.URL
		server.Close()

		_, _, err := HTTPRequest(&http.Client{Timeout: 2 * time.Second}, "GET", addr, nil)
		if err == nil {
			t.Errorf("expected error for connection refused, got nil")
		}
	})
}

func TestRecordResult(t *testing.T) {
	reporter := framework.NewTestReporter()

	// Record a passing result.
	start := time.Now()
	RecordResult(reporter, "test-pass", start, true, "all good", nil)

	// Record a failing result with an error.
	start2 := time.Now()
	testErr := errors.New("something broke")
	RecordResult(reporter, "test-fail", start2, false, "it failed", testErr)

	results := reporter.Snapshot()
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	r1 := results[0]
	if r1.Name != "test-pass" {
		t.Errorf("results[0].Name = %q, want 'test-pass'", r1.Name)
	}
	if !r1.Passed {
		t.Errorf("results[0].Passed = false, want true")
	}
	if r1.Message != "all good" {
		t.Errorf("results[0].Message = %q, want 'all good'", r1.Message)
	}
	if r1.Error != nil {
		t.Errorf("results[0].Error = %v, want nil", r1.Error)
	}
	if r1.Duration < 0 {
		t.Errorf("results[0].Duration = %v, want >= 0", r1.Duration)
	}

	r2 := results[1]
	if r2.Name != "test-fail" {
		t.Errorf("results[1].Name = %q, want 'test-fail'", r2.Name)
	}
	if r2.Passed {
		t.Errorf("results[1].Passed = true, want false")
	}
	if r2.Message != "it failed" {
		t.Errorf("results[1].Message = %q, want 'it failed'", r2.Message)
	}
	if r2.Error != testErr {
		t.Errorf("results[1].Error = %v, want %v", r2.Error, testErr)
	}
	if r2.Duration < 0 {
		t.Errorf("results[1].Duration = %v, want >= 0", r2.Duration)
	}
}

func TestAssertCommonEnvelope(t *testing.T) {
	asserts := framework.NewAssertionEngine()

	t.Run("AllFieldsPresentCodeMatches", func(t *testing.T) {
		body := []byte(`{"code":200,"msg":"ok","request_id":"abc"}`)
		fields, code := AssertCommonEnvelope(asserts, body, 200)
		if !fields.Passed {
			t.Errorf("fields.Passed = false, want true")
		}
		if !code.Passed {
			t.Errorf("code.Passed = false, want true")
		}
	})

	t.Run("MissingFields", func(t *testing.T) {
		body := []byte(`{"code":200}`)
		fields, _ := AssertCommonEnvelope(asserts, body, 200)
		if fields.Passed {
			t.Errorf("fields.Passed = true, want false (missing msg and request_id)")
		}
	})

	t.Run("CodeMismatch", func(t *testing.T) {
		body := []byte(`{"code":404,"msg":"not found","request_id":"xyz"}`)
		fields, code := AssertCommonEnvelope(asserts, body, 200)
		if !fields.Passed {
			t.Errorf("fields.Passed = false, want true (all fields present)")
		}
		if code.Passed {
			t.Errorf("code.Passed = true, want false (code mismatch)")
		}
	})
}

func TestGetString(t *testing.T) {
	// Existing string key.
	m := map[string]any{"name": "alice"}
	if got := GetString(m, "name", "fallback"); got != "alice" {
		t.Errorf("GetString(name) = %q, want 'alice'", got)
	}

	// Missing key returns fallback.
	if got := GetString(m, "missing", "fallback"); got != "fallback" {
		t.Errorf("GetString(missing) = %q, want 'fallback'", got)
	}

	// Existing non-string key returns fallback.
	m2 := map[string]any{"count": 42}
	if got := GetString(m2, "count", "fallback"); got != "fallback" {
		t.Errorf("GetString(count) = %q, want 'fallback' (non-string value)", got)
	}

	// Nil map returns fallback.
	if got := GetString(nil, "name", "fallback"); got != "fallback" {
		t.Errorf("GetString(nil map) = %q, want 'fallback'", got)
	}
}

func TestGetDuration(t *testing.T) {
	// String value parses to duration.
	m := map[string]any{"d": "5s"}
	if got := GetDuration(m, "d", time.Second); got != 5*time.Second {
		t.Errorf("GetDuration(string '5s') = %v, want 5s", got)
	}

	// time.Duration value returned as-is.
	m2 := map[string]any{"d": 3 * time.Second}
	if got := GetDuration(m2, "d", time.Second); got != 3*time.Second {
		t.Errorf("GetDuration(duration 3s) = %v, want 3s", got)
	}

	// Invalid string returns fallback.
	m3 := map[string]any{"d": "not-a-duration"}
	if got := GetDuration(m3, "d", time.Second); got != time.Second {
		t.Errorf("GetDuration(invalid string) = %v, want fallback 1s", got)
	}

	// Missing key returns fallback.
	m4 := map[string]any{}
	if got := GetDuration(m4, "d", time.Second); got != time.Second {
		t.Errorf("GetDuration(missing) = %v, want fallback 1s", got)
	}
}

func TestGetInt(t *testing.T) {
	// int value.
	m := map[string]any{"n": 42}
	if got := GetInt(m, "n", 0); got != 42 {
		t.Errorf("GetInt(int) = %d, want 42", got)
	}

	// int64 value.
	m2 := map[string]any{"n": int64(99)}
	if got := GetInt(m2, "n", 0); got != 99 {
		t.Errorf("GetInt(int64) = %d, want 99", got)
	}

	// float64 value.
	m3 := map[string]any{"n": float64(7)}
	if got := GetInt(m3, "n", 0); got != 7 {
		t.Errorf("GetInt(float64) = %d, want 7", got)
	}

	// Missing key returns fallback.
	m4 := map[string]any{}
	if got := GetInt(m4, "n", 5); got != 5 {
		t.Errorf("GetInt(missing) = %d, want fallback 5", got)
	}
}

func TestGetBool(t *testing.T) {
	// true value.
	m := map[string]any{"b": true}
	if got := GetBool(m, "b", false); got != true {
		t.Errorf("GetBool(true) = %v, want true", got)
	}

	// false value (fallback must not shadow an explicit false).
	m2 := map[string]any{"b": false}
	if got := GetBool(m2, "b", true); got != false {
		t.Errorf("GetBool(false) = %v, want false", got)
	}

	// Missing key returns fallback.
	m3 := map[string]any{}
	if got := GetBool(m3, "b", true); got != true {
		t.Errorf("GetBool(missing) = %v, want fallback true", got)
	}

	// Non-bool value returns fallback.
	m4 := map[string]any{"b": "not-a-bool"}
	if got := GetBool(m4, "b", true); got != true {
		t.Errorf("GetBool(non-bool) = %v, want fallback true", got)
	}
}

func TestNoOpLogger(t *testing.T) {
	// Verify the no-op methods do not panic.
	logger := NoOpLogger{}
	logger.Infof("info message: %s", "arg")
	logger.Warnf("warn message: %s", "arg")
	logger.Errorf("error message: %s", "arg")
}
