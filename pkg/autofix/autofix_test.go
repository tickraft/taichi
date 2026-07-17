package autofix

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockServiceRestarter is a test double for ServiceRestarter that records restart calls.
type mockServiceRestarter struct {
	restartErr   error
	restartCalls int
	logPath      string
}

func (m *mockServiceRestarter) Restart() error {
	m.restartCalls++
	return m.restartErr
}

func (m *mockServiceRestarter) ServerLogPath() string {
	return m.logPath
}

// captureRule is a FixRule that matches every error type and records the FixContext it receives.
type captureRule struct {
	captured *FixContext
}

func (r *captureRule) Match(ErrorType) bool { return true }

func (r *captureRule) Apply(ctx *FixContext) FixResult {
	r.captured = ctx
	return FixResult{RuleName: "captureRule", Fixed: true}
}

func (r *captureRule) Name() string { return "captureRule" }

// newResp builds an *http.Response with the given status code.
func newResp(status int) *http.Response {
	return &http.Response{StatusCode: status, Body: http.NoBody}
}

// writeFile writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

// ---------------------------------------------------------------------------
// ErrorType and ErrorDetector tests
// ---------------------------------------------------------------------------

func TestErrorTypeString(t *testing.T) {
	cases := []struct {
		et   ErrorType
		want string
	}{
		{ErrorTypeNone, "none"},
		{ErrorTypeServiceDown, "service_down"},
		{ErrorTypeRateLimited, "rate_limited"},
		{ErrorTypeServerError, "server_error"},
		{ErrorTypeUnknown, "unknown"},
		{ErrorType(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.et.String(); got != c.want {
			t.Errorf("ErrorType(%d).String() = %q, want %q", int(c.et), got, c.want)
		}
	}
}

func TestNewErrorDetector(t *testing.T) {
	d := NewErrorDetector()
	if d.ConsecutiveFailures() != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", d.ConsecutiveFailures())
	}
}

func TestDetect(t *testing.T) {
	t.Run("NetworkErrorOnce", func(t *testing.T) {
		d := NewErrorDetector()
		got := d.Detect(nil, errors.New("network error"))
		if got != ErrorTypeNone {
			t.Errorf("Detect(nil, err) = %s, want %s", got, ErrorTypeNone)
		}
		if d.ConsecutiveFailures() != 1 {
			t.Errorf("ConsecutiveFailures = %d, want 1", d.ConsecutiveFailures())
		}
	})

	t.Run("NetworkErrorThreshold", func(t *testing.T) {
		d := NewErrorDetector()
		for i := 0; i < serviceDownThreshold; i++ {
			got := d.Detect(nil, errors.New("network error"))
			if i < serviceDownThreshold-1 {
				if got != ErrorTypeNone {
					t.Errorf("call %d: Detect = %s, want %s", i, got, ErrorTypeNone)
				}
			} else {
				if got != ErrorTypeServiceDown {
					t.Errorf("call %d: Detect = %s, want %s", i, got, ErrorTypeServiceDown)
				}
			}
		}
	})

	t.Run("NilResponseThreshold", func(t *testing.T) {
		d := NewErrorDetector()
		for i := 0; i < serviceDownThreshold; i++ {
			got := d.Detect(nil, nil)
			if i < serviceDownThreshold-1 {
				if got != ErrorTypeNone {
					t.Errorf("call %d: Detect(nil, nil) = %s, want %s", i, got, ErrorTypeNone)
				}
			} else {
				if got != ErrorTypeServiceDown {
					t.Errorf("call %d: Detect(nil, nil) = %s, want %s", i, got, ErrorTypeServiceDown)
				}
			}
		}
		if d.ConsecutiveFailures() != serviceDownThreshold {
			t.Errorf("ConsecutiveFailures = %d, want %d", d.ConsecutiveFailures(), serviceDownThreshold)
		}
	})

	t.Run("Response200", func(t *testing.T) {
		d := NewErrorDetector()
		d.Detect(nil, errors.New("err")) // counter = 1
		got := d.Detect(newResp(http.StatusOK), nil)
		if got != ErrorTypeNone {
			t.Errorf("Detect(200, nil) = %s, want %s", got, ErrorTypeNone)
		}
		if d.ConsecutiveFailures() != 0 {
			t.Errorf("ConsecutiveFailures = %d, want 0 (reset)", d.ConsecutiveFailures())
		}
	})

	t.Run("Response429", func(t *testing.T) {
		d := NewErrorDetector()
		d.Detect(nil, errors.New("err")) // counter = 1
		got := d.Detect(newResp(http.StatusTooManyRequests), nil)
		if got != ErrorTypeRateLimited {
			t.Errorf("Detect(429, nil) = %s, want %s", got, ErrorTypeRateLimited)
		}
		if d.ConsecutiveFailures() != 0 {
			t.Errorf("ConsecutiveFailures = %d, want 0 (reset)", d.ConsecutiveFailures())
		}
	})

	t.Run("Response500", func(t *testing.T) {
		d := NewErrorDetector()
		d.Detect(nil, errors.New("err")) // counter = 1
		got := d.Detect(newResp(http.StatusInternalServerError), nil)
		if got != ErrorTypeServerError {
			t.Errorf("Detect(500, nil) = %s, want %s", got, ErrorTypeServerError)
		}
		if d.ConsecutiveFailures() != 0 {
			t.Errorf("ConsecutiveFailures = %d, want 0 (reset)", d.ConsecutiveFailures())
		}
	})

	t.Run("Response404", func(t *testing.T) {
		d := NewErrorDetector()
		d.Detect(nil, errors.New("err")) // counter = 1
		got := d.Detect(newResp(http.StatusNotFound), nil)
		if got != ErrorTypeNone {
			t.Errorf("Detect(404, nil) = %s, want %s (4xx except 429)", got, ErrorTypeNone)
		}
		if d.ConsecutiveFailures() != 0 {
			t.Errorf("ConsecutiveFailures = %d, want 0 (reset)", d.ConsecutiveFailures())
		}
	})

	t.Run("ErrorsThenSuccess", func(t *testing.T) {
		d := NewErrorDetector()
		d.Detect(nil, errors.New("err1"))
		d.Detect(nil, errors.New("err2"))
		if d.ConsecutiveFailures() != 2 {
			t.Fatalf("ConsecutiveFailures = %d, want 2 before success", d.ConsecutiveFailures())
		}
		d.Detect(newResp(http.StatusOK), nil)
		if d.ConsecutiveFailures() != 0 {
			t.Errorf("ConsecutiveFailures = %d, want 0 (reset after 200)", d.ConsecutiveFailures())
		}
	})
}

func TestDetectFromHealth(t *testing.T) {
	t.Run("NetworkErrorUnderThreshold", func(t *testing.T) {
		d := NewErrorDetector()
		got := d.DetectFromHealth(nil, errors.New("net err"))
		if got != ErrorTypeNone {
			t.Errorf("DetectFromHealth(nil, err) = %s, want %s (under threshold)", got, ErrorTypeNone)
		}
		if d.ConsecutiveFailures() != 1 {
			t.Errorf("ConsecutiveFailures = %d, want 1", d.ConsecutiveFailures())
		}
	})

	t.Run("Response200Resets", func(t *testing.T) {
		d := NewErrorDetector()
		d.DetectFromHealth(nil, errors.New("err")) // counter = 1
		got := d.DetectFromHealth(newResp(http.StatusOK), nil)
		if got != ErrorTypeNone {
			t.Errorf("DetectFromHealth(200, nil) = %s, want %s", got, ErrorTypeNone)
		}
		if d.ConsecutiveFailures() != 0 {
			t.Errorf("ConsecutiveFailures = %d, want 0 (reset)", d.ConsecutiveFailures())
		}
	})

	t.Run("Response500UnderThreshold", func(t *testing.T) {
		d := NewErrorDetector()
		got := d.DetectFromHealth(newResp(http.StatusInternalServerError), nil)
		if got != ErrorTypeNone {
			t.Errorf("DetectFromHealth(500, nil) = %s, want %s (under threshold)", got, ErrorTypeNone)
		}
		if d.ConsecutiveFailures() != 1 {
			t.Errorf("ConsecutiveFailures = %d, want 1", d.ConsecutiveFailures())
		}
	})

	t.Run("Response500Threshold", func(t *testing.T) {
		d := NewErrorDetector()
		var got ErrorType
		for i := 0; i < serviceDownThreshold; i++ {
			got = d.DetectFromHealth(newResp(http.StatusInternalServerError), nil)
		}
		if got != ErrorTypeServiceDown {
			t.Errorf("after 3x 500: DetectFromHealth = %s, want %s", got, ErrorTypeServiceDown)
		}
		if d.ConsecutiveFailures() != serviceDownThreshold {
			t.Errorf("ConsecutiveFailures = %d, want %d", d.ConsecutiveFailures(), serviceDownThreshold)
		}
	})
}

func TestConsecutiveFailures(t *testing.T) {
	d := NewErrorDetector()
	if d.ConsecutiveFailures() != 0 {
		t.Fatalf("initial ConsecutiveFailures = %d, want 0", d.ConsecutiveFailures())
	}
	d.Detect(nil, errors.New("err"))
	if d.ConsecutiveFailures() != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", d.ConsecutiveFailures())
	}
	d.Detect(nil, errors.New("err"))
	if d.ConsecutiveFailures() != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", d.ConsecutiveFailures())
	}
}

func TestReset(t *testing.T) {
	d := NewErrorDetector()
	d.Detect(nil, errors.New("err"))
	d.Detect(nil, errors.New("err"))
	if d.ConsecutiveFailures() != 2 {
		t.Fatalf("ConsecutiveFailures = %d, want 2 before reset", d.ConsecutiveFailures())
	}
	d.Reset()
	if d.ConsecutiveFailures() != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0 after reset", d.ConsecutiveFailures())
	}
}

// ---------------------------------------------------------------------------
// FixRule tests
// ---------------------------------------------------------------------------

func TestServiceRestartRuleMatch(t *testing.T) {
	rule := ServiceRestartRule{}
	types := []ErrorType{ErrorTypeNone, ErrorTypeServiceDown, ErrorTypeRateLimited, ErrorTypeServerError, ErrorTypeUnknown}
	for _, et := range types {
		got := rule.Match(et)
		want := et == ErrorTypeServiceDown
		if got != want {
			t.Errorf("Match(%s) = %v, want %v", et, got, want)
		}
	}
}

func TestServiceRestartRuleApplyNilCtx(t *testing.T) {
	rule := ServiceRestartRule{}
	result := rule.Apply(nil)
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if result.Attempts != maxRestartAttempts {
		t.Errorf("Attempts = %d, want %d", result.Attempts, maxRestartAttempts)
	}
	if result.RuleName != "ServiceRestartRule" {
		t.Errorf("RuleName = %q, want ServiceRestartRule", result.RuleName)
	}
}

func TestServiceRestartRuleApplyNilLifecycle(t *testing.T) {
	rule := ServiceRestartRule{}
	result := rule.Apply(&FixContext{}) // Lifecycle is nil
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if result.Attempts != maxRestartAttempts {
		t.Errorf("Attempts = %d, want %d", result.Attempts, maxRestartAttempts)
	}
}

func TestServiceRestartRuleApplySuccess(t *testing.T) {
	mock := &mockServiceRestarter{} // Restart returns nil
	rule := ServiceRestartRule{}
	result := rule.Apply(&FixContext{Lifecycle: mock})
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if !result.Retry {
		t.Errorf("Retry = false, want true")
	}
	if result.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Attempts)
	}
	if mock.restartCalls != 1 {
		t.Errorf("restartCalls = %d, want 1", mock.restartCalls)
	}
}

func TestServiceRestartRuleApplyFail(t *testing.T) {
	mock := &mockServiceRestarter{restartErr: errors.New("restart failed")}
	rule := ServiceRestartRule{}
	result := rule.Apply(&FixContext{Lifecycle: mock})
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if result.Attempts != maxRestartAttempts {
		t.Errorf("Attempts = %d, want %d", result.Attempts, maxRestartAttempts)
	}
	if mock.restartCalls != maxRestartAttempts {
		t.Errorf("restartCalls = %d, want %d", mock.restartCalls, maxRestartAttempts)
	}
	if !strings.Contains(result.Message, "failed") {
		t.Errorf("Message = %q, want it to contain 'failed'", result.Message)
	}
}

func TestRateLimitRetryRuleMatch(t *testing.T) {
	rule := RateLimitRetryRule{}
	types := []ErrorType{ErrorTypeNone, ErrorTypeServiceDown, ErrorTypeRateLimited, ErrorTypeServerError, ErrorTypeUnknown}
	for _, et := range types {
		got := rule.Match(et)
		want := et == ErrorTypeRateLimited
		if got != want {
			t.Errorf("Match(%s) = %v, want %v", et, got, want)
		}
	}
}

func TestRateLimitRetryRuleApply(t *testing.T) {
	// Use a short delay to avoid the 2s default sleep in tests.
	rule := RateLimitRetryRule{retryDelay: 10 * time.Millisecond}
	result := rule.Apply(&FixContext{})
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if !result.Retry {
		t.Errorf("Retry = false, want true")
	}
	if result.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Attempts)
	}
	if result.RuleName != "RateLimitRetryRule" {
		t.Errorf("RuleName = %q, want RateLimitRetryRule", result.RuleName)
	}
}

func TestUnknownErrorReportRuleMatch(t *testing.T) {
	rule := UnknownErrorReportRule{}
	types := []ErrorType{ErrorTypeNone, ErrorTypeServiceDown, ErrorTypeRateLimited, ErrorTypeServerError, ErrorTypeUnknown}
	for _, et := range types {
		got := rule.Match(et)
		want := et == ErrorTypeServerError || et == ErrorTypeUnknown
		if got != want {
			t.Errorf("Match(%s) = %v, want %v", et, got, want)
		}
	}
}

func TestUnknownErrorReportRuleApply(t *testing.T) {
	rule := UnknownErrorReportRule{}
	ctx := &FixContext{
		Detected:   ErrorTypeServerError,
		ReportsDir: t.TempDir(),
		Body:       []byte("some body content"),
	}
	result := rule.Apply(ctx)
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if result.ErrorReport == "" {
		t.Fatalf("ErrorReport empty, want a path")
	}
	info, err := os.Stat(result.ErrorReport)
	if err != nil {
		t.Fatalf("error report file does not exist: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("error report file is empty")
	}

	// Verify the report content.
	data, err := os.ReadFile(result.ErrorReport)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report errorReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if report.ErrorType != ErrorTypeServerError.String() {
		t.Errorf("report.ErrorType = %q, want %q", report.ErrorType, ErrorTypeServerError.String())
	}
	if report.Body != "some body content" {
		t.Errorf("report.Body = %q, want 'some body content'", report.Body)
	}
}

func TestUnknownErrorReportRuleApplyNilCtx(t *testing.T) {
	rule := UnknownErrorReportRule{}
	result := rule.Apply(nil)
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if result.Message == "" {
		t.Errorf("Message empty, want an error message")
	}
	if !strings.Contains(result.Message, "nil fix context") {
		t.Errorf("Message = %q, want it to contain 'nil fix context'", result.Message)
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"hello", 10, "hello"},
		{"hello", 3, "hel"},
		{"hello", 5, "hello"},
		{"", 5, ""},
		// Multi-byte UTF-8: 4 Chinese characters, each 3 bytes.
		{"你好世界", 2, "你好"},
		{"你好世界", 4, "你好世界"},
		{"你好世界", 10, "你好世界"},
		{"你好世界", 0, ""},
	}
	for _, c := range cases {
		got := truncateRunes(c.s, c.max)
		if got != c.want {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
		}
	}
}

func TestReadLogTail(t *testing.T) {
	t.Run("EmptyPath", func(t *testing.T) {
		if got := readLogTail("", 10); got != "" {
			t.Errorf("readLogTail('', 10) = %q, want ''", got)
		}
	})

	t.Run("NonExistentPath", func(t *testing.T) {
		got := readLogTail(filepath.Join(t.TempDir(), "nope.log"), 10)
		if got != "" {
			t.Errorf("readLogTail(nonexistent) = %q, want ''", got)
		}
	})

	t.Run("FewerLinesThanN", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "log.txt")
		writeFile(t, path, "line1\nline2\nline3\nline4\nline5\n")
		got := readLogTail(path, 3)
		want := "line3\nline4\nline5"
		if got != want {
			t.Errorf("readLogTail(5 lines, 3) = %q, want %q", got, want)
		}
	})

	t.Run("MoreLinesThanN", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "log.txt")
		writeFile(t, path, "line1\nline2\nline3\n")
		got := readLogTail(path, 10)
		want := "line1\nline2\nline3"
		if got != want {
			t.Errorf("readLogTail(3 lines, 10) = %q, want %q", got, want)
		}
	})

	t.Run("EmptyFile", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "log.txt")
		writeFile(t, path, "")
		got := readLogTail(path, 10)
		if got != "" {
			t.Errorf("readLogTail(empty file) = %q, want ''", got)
		}
	})
}

// ---------------------------------------------------------------------------
// FixEngine tests
// ---------------------------------------------------------------------------

func TestNewFixEngine(t *testing.T) {
	engine := NewFixEngine(&mockServiceRestarter{}, t.TempDir())
	rules := engine.Rules()
	if len(rules) != 3 {
		t.Fatalf("Rules() len = %d, want 3", len(rules))
	}
	expected := []string{"ServiceRestartRule", "RateLimitRetryRule", "UnknownErrorReportRule"}
	for i, name := range expected {
		if rules[i] != name {
			t.Errorf("Rules()[%d] = %q, want %q", i, rules[i], name)
		}
	}
}

func TestRegister(t *testing.T) {
	engine := NewFixEngine(&mockServiceRestarter{}, t.TempDir())
	engine.Register(&captureRule{})
	rules := engine.Rules()
	if len(rules) != 4 {
		t.Fatalf("Rules() len = %d, want 4 after register", len(rules))
	}
	if rules[3] != "captureRule" {
		t.Errorf("Rules()[3] = %q, want captureRule", rules[3])
	}
}

func TestRegisterNil(t *testing.T) {
	engine := NewFixEngine(&mockServiceRestarter{}, t.TempDir())
	engine.Register(nil)
	rules := engine.Rules()
	if len(rules) != 3 {
		t.Errorf("Rules() len = %d, want 3 (nil rule should be ignored)", len(rules))
	}
}

func TestApplyServiceDown(t *testing.T) {
	mock := &mockServiceRestarter{} // Restart returns nil
	engine := NewFixEngine(mock, t.TempDir())
	result := engine.Apply(ErrorTypeServiceDown, &FixContext{})
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if !result.Retry {
		t.Errorf("Retry = false, want true")
	}
	if result.RuleName != "ServiceRestartRule" {
		t.Errorf("RuleName = %q, want ServiceRestartRule", result.RuleName)
	}
	if mock.restartCalls != 1 {
		t.Errorf("restartCalls = %d, want 1", mock.restartCalls)
	}
}

func TestApplyRateLimited(t *testing.T) {
	// Build the engine with a short-delay RateLimitRetryRule to avoid the 2s default sleep.
	engine := &FixEngine{
		lifecycle:  &mockServiceRestarter{},
		reportsDir: t.TempDir(),
		rules: []FixRule{
			ServiceRestartRule{},
			RateLimitRetryRule{retryDelay: 10 * time.Millisecond},
			UnknownErrorReportRule{},
		},
	}
	result := engine.Apply(ErrorTypeRateLimited, &FixContext{})
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if !result.Retry {
		t.Errorf("Retry = false, want true")
	}
	if result.RuleName != "RateLimitRetryRule" {
		t.Errorf("RuleName = %q, want RateLimitRetryRule", result.RuleName)
	}
}

func TestApplyServerError(t *testing.T) {
	engine := NewFixEngine(&mockServiceRestarter{}, t.TempDir())
	result := engine.Apply(ErrorTypeServerError, &FixContext{})
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if result.ErrorReport == "" {
		t.Fatalf("ErrorReport empty, want a path")
	}
	if _, err := os.Stat(result.ErrorReport); err != nil {
		t.Errorf("error report file does not exist: %v", err)
	}
	if result.RuleName != "UnknownErrorReportRule" {
		t.Errorf("RuleName = %q, want UnknownErrorReportRule", result.RuleName)
	}
}

func TestApplyUnknown(t *testing.T) {
	engine := NewFixEngine(&mockServiceRestarter{}, t.TempDir())
	result := engine.Apply(ErrorTypeUnknown, &FixContext{})
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if result.ErrorReport == "" {
		t.Fatalf("ErrorReport empty, want a path")
	}
	if _, err := os.Stat(result.ErrorReport); err != nil {
		t.Errorf("error report file does not exist: %v", err)
	}
}

func TestApplyNilCtx(t *testing.T) {
	engine := NewFixEngine(&mockServiceRestarter{}, t.TempDir())
	result := engine.Apply(ErrorTypeServiceDown, nil)
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if !strings.Contains(result.Message, "no matching fix rule") {
		t.Errorf("Message = %q, want it to contain 'no matching fix rule'", result.Message)
	}
}

func TestApplyNoMatch(t *testing.T) {
	// ErrorTypeNone is not matched by any default rule.
	engine := NewFixEngine(&mockServiceRestarter{}, t.TempDir())
	result := engine.Apply(ErrorTypeNone, &FixContext{})
	if result.Fixed {
		t.Errorf("Fixed = true, want false")
	}
	if !strings.Contains(result.Message, "no matching fix rule") {
		t.Errorf("Message = %q, want it to contain 'no matching fix rule'", result.Message)
	}
}

func TestApplyFillsDefaults(t *testing.T) {
	mock := &mockServiceRestarter{}
	reportsDir := t.TempDir()
	capture := &captureRule{}
	engine := &FixEngine{
		lifecycle:  mock,
		reportsDir: reportsDir,
		rules:      []FixRule{capture},
	}
	// ctx has nil Lifecycle and empty ReportsDir; the engine should fill them.
	ctx := &FixContext{}
	engine.Apply(ErrorTypeServerError, ctx)

	if capture.captured == nil {
		t.Fatalf("capture rule was not invoked")
	}
	if capture.captured.Lifecycle != mock {
		t.Errorf("Lifecycle was not filled from engine default")
	}
	if capture.captured.ReportsDir != reportsDir {
		t.Errorf("ReportsDir = %q, want %q", capture.captured.ReportsDir, reportsDir)
	}
	if capture.captured.Detected != ErrorTypeServerError {
		t.Errorf("Detected = %v, want %v", capture.captured.Detected, ErrorTypeServerError)
	}
}
