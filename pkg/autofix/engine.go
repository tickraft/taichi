package autofix

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxRestartAttempts is the maximum number of restart attempts ServiceRestartRule makes before giving up.
const maxRestartAttempts = 2

// defaultRateLimitRetryDelay is the default delay RateLimitRetryRule waits before signaling the caller to retry the original request.
const defaultRateLimitRetryDelay = 2 * time.Second

// errorReportBodyLimit is the maximum number of runes of the response body written to the error report.
const errorReportBodyLimit = 1000

// errorReportLogTail is the number of trailing service log lines included in the error report.
const errorReportLogTail = 100

// ServiceRestarter abstracts the subset of ServiceLifecycle needed by FixRule:
// restarting the service and locating its log file. *framework.ServiceLifecycle implements this interface.
type ServiceRestarter interface {
	Restart() error
	ServerLogPath() string
}

// FixContext carries the information FixRule needs to attempt a fix: the service lifecycle (for restart),
// the original request and response that triggered the error, the detected error type, and the error report output directory.
type FixContext struct {
	// Lifecycle is used for restart operations. nil when restart is not available; rules that require it should handle this case themselves.
	Lifecycle ServiceRestarter
	// Detected is the ErrorDetector classification result that triggered this fix attempt.
	Detected ErrorType
	// Request is the original HTTP request that failed. May be nil.
	Request *http.Request
	// Response is the received HTTP response (if any). May be nil.
	Response *http.Response
	// Body is the response body captured for inspection. May be nil.
	Body []byte
	// ReportsDir is the error report output directory.
	ReportsDir string
}

// FixResult describes the outcome of a FixRule.Apply attempt.
type FixResult struct {
	// RuleName is the name of the rule that produced this result.
	RuleName string
	// Fixed is true when the error was successfully fixed.
	Fixed bool
	// Message is a human-readable description of the result.
	Message string
	// Retry is true when the caller should resend the original request after the fix.
	Retry bool
	// Attempts is the number of fix attempts performed by this rule.
	Attempts int
	// ErrorReport is the path of the error report file written to disk (if any). Empty when no report was produced.
	ErrorReport string
}

// FixRule defines an auto-fix strategy for a class of errors detected during test execution.
type FixRule interface {
	// Match returns true when the rule can handle the given error type.
	Match(detected ErrorType) bool
	// Apply attempts to fix the error, returning a FixResult describing the outcome.
	Apply(ctx *FixContext) FixResult
	// Name returns a human-readable name for the rule.
	Name() string
}

// ServiceRestartRule restarts the service when ErrorTypeServiceDown is detected.
// It retries up to maxRestartAttempts times before giving up.
type ServiceRestartRule struct{}

// Match returns true when the detected error is ErrorTypeServiceDown.
func (ServiceRestartRule) Match(detected ErrorType) bool {
	return detected == ErrorTypeServiceDown
}

// Name returns a human-readable name for the rule.
func (ServiceRestartRule) Name() string {
	return "ServiceRestartRule"
}

// Apply attempts up to maxRestartAttempts restarts. Any success returns a FixResult
// signaling the caller to retry the original request. All failures (or no available lifecycle) return Fixed=false.
func (r ServiceRestartRule) Apply(ctx *FixContext) FixResult {
	ruleName := r.Name()
	if ctx == nil || ctx.Lifecycle == nil {
		return FixResult{
			RuleName: ruleName,
			Fixed:    false,
			Attempts: maxRestartAttempts,
			Message:  "service restart failed after 2 attempts",
		}
	}
	for attempt := 1; attempt <= maxRestartAttempts; attempt++ {
		if err := ctx.Lifecycle.Restart(); err != nil {
			continue
		}
		return FixResult{
			RuleName: ruleName,
			Fixed:    true,
			Retry:    true,
			Attempts: attempt,
			Message:  fmt.Sprintf("service restarted successfully after %d attempt(s)", attempt),
		}
	}
	return FixResult{
		RuleName: ruleName,
		Fixed:    false,
		Attempts: maxRestartAttempts,
		Message:  "service restart failed after 2 attempts",
	}
}

// RateLimitRetryRule handles ErrorTypeRateLimited: it waits for a configurable delay and then signals the caller to retry the original request.
// The rule itself does not perform the retry; the caller is responsible for resending when Retry=true.
type RateLimitRetryRule struct {
	retryDelay time.Duration
}

// NewRateLimitRetryRule returns a RateLimitRetryRule using the default retry delay.
func NewRateLimitRetryRule() RateLimitRetryRule {
	return RateLimitRetryRule{retryDelay: defaultRateLimitRetryDelay}
}

// Match returns true when the detected error is ErrorTypeRateLimited.
func (RateLimitRetryRule) Match(detected ErrorType) bool {
	return detected == ErrorTypeRateLimited
}

// Name returns a human-readable name for the rule.
func (RateLimitRetryRule) Name() string {
	return "RateLimitRetryRule"
}

// Apply waits for the configured retry delay and then signals the caller to retry the original request. A non-positive retryDelay falls back to the default delay.
func (r RateLimitRetryRule) Apply(ctx *FixContext) FixResult {
	delay := r.retryDelay
	if delay <= 0 {
		delay = defaultRateLimitRetryDelay
	}
	time.Sleep(delay)
	return FixResult{
		RuleName: r.Name(),
		Fixed:    true,
		Retry:    true,
		Attempts: 1,
		Message:  fmt.Sprintf("waited %s after rate limit", delay),
	}
}

// UnknownErrorReportRule handles ErrorTypeServerError and ErrorTypeUnknown:
// it writes a JSON error report to disk for later manual analysis. It does not attempt an auto-fix, as these errors typically require human intervention.
type UnknownErrorReportRule struct{}

// Match returns true when the detected error is ErrorTypeServerError or ErrorTypeUnknown.
func (UnknownErrorReportRule) Match(detected ErrorType) bool {
	return detected == ErrorTypeServerError || detected == ErrorTypeUnknown
}

// Name returns a human-readable name for the rule.
func (UnknownErrorReportRule) Name() string {
	return "UnknownErrorReportRule"
}

// Apply writes an error report JSON to ctx.ReportsDir, containing the timestamp, error type, request details,
// response status and body (truncated to errorReportBodyLimit runes), and the trailing errorReportLogTail lines of the service log.
// It returns Fixed=false, as manual intervention is required.
func (r UnknownErrorReportRule) Apply(ctx *FixContext) FixResult {
	path, err := r.writeReport(ctx)
	if err != nil {
		return FixResult{
			RuleName: r.Name(),
			Fixed:    false,
			Message:  fmt.Sprintf("failed to write error report: %v", err),
		}
	}
	return FixResult{
		RuleName:    r.Name(),
		Fixed:       false,
		ErrorReport: path,
		Message:     "error report written, manual intervention required",
	}
}

// errorReport is the JSON structure UnknownErrorReportRule writes to disk.
type errorReport struct {
	Timestamp      string `json:"timestamp"`
	ErrorType      string `json:"error_type"`
	RequestMethod  string `json:"request_method"`
	RequestURL     string `json:"request_url"`
	ResponseStatus int    `json:"response_status"`
	Body           string `json:"body"`
	ServerLogTail  string `json:"server_log_tail"`
}

// writeReport creates the report directory (if needed), builds the report payload from the fix context,
// and writes it as a timestamp-named JSON file. It returns the path of the written file.
func (r UnknownErrorReportRule) writeReport(ctx *FixContext) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("nil fix context")
	}
	if err := os.MkdirAll(ctx.ReportsDir, 0o755); err != nil {
		return "", fmt.Errorf("create reports dir: %w", err)
	}
	report := r.buildReport(ctx)
	name := fmt.Sprintf("errors-%s.json", time.Now().Format("20060102-150405"))
	path := filepath.Join(ctx.ReportsDir, name)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return path, nil
}

// buildReport assembles the errorReport payload from the fix context, reading request/response details and the service log tail.
func (r UnknownErrorReportRule) buildReport(ctx *FixContext) errorReport {
	report := errorReport{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		ErrorType: ctx.Detected.String(),
	}
	if ctx.Request != nil {
		report.RequestMethod = ctx.Request.Method
		if ctx.Request.URL != nil {
			report.RequestURL = ctx.Request.URL.String()
		}
	}
	if ctx.Response != nil {
		report.ResponseStatus = ctx.Response.StatusCode
	}
	report.Body = truncateRunes(string(ctx.Body), errorReportBodyLimit)
	if ctx.Lifecycle != nil {
		report.ServerLogTail = readLogTail(ctx.Lifecycle.ServerLogPath(), errorReportLogTail)
	}
	return report
}

// FixEngine applies registered FixRule instances to fix detected errors.
// Rules are evaluated in registration order; the first matching rule wins.
type FixEngine struct {
	lifecycle  ServiceRestarter
	reportsDir string
	rules      []FixRule
}

// NewFixEngine creates a FixEngine pre-registered with the default rule set
// (ServiceRestartRule, RateLimitRetryRule, UnknownErrorReportRule).
// lifecycle and reportsDir serve as engine-level defaults used when the caller does not set the corresponding FixContext fields.
func NewFixEngine(lifecycle ServiceRestarter, reportsDir string) *FixEngine {
	return &FixEngine{
		lifecycle:  lifecycle,
		reportsDir: reportsDir,
		rules: []FixRule{
			ServiceRestartRule{},
			NewRateLimitRetryRule(),
			UnknownErrorReportRule{},
		},
	}
}

// Register appends a FixRule to the engine. Subsequent Apply calls will match in registration order.
func (e *FixEngine) Register(rule FixRule) {
	if rule == nil {
		return
	}
	e.rules = append(e.rules, rule)
}

// Rules returns a snapshot of registered rule names, useful for debugging and listing.
func (e *FixEngine) Rules() []string {
	out := make([]string, 0, len(e.rules))
	for _, r := range e.rules {
		out = append(out, r.Name())
	}
	return out
}

// Apply iterates registered rules in order and returns the Apply result of the first rule whose Match returns true for the detected error type.
// Before invoking the rule, the engine fills in unset FixContext fields (Lifecycle, ReportsDir, Detected) with its own defaults.
// When no rule matches, it returns Fixed=false with a descriptive message.
func (e *FixEngine) Apply(detected ErrorType, ctx *FixContext) FixResult {
	if ctx == nil {
		return FixResult{Fixed: false, Message: "no matching fix rule"}
	}
	ctx.Detected = detected
	if ctx.Lifecycle == nil && e.lifecycle != nil {
		ctx.Lifecycle = e.lifecycle
	}
	if ctx.ReportsDir == "" {
		ctx.ReportsDir = e.reportsDir
	}
	for _, rule := range e.rules {
		if rule.Match(detected) {
			return rule.Apply(ctx)
		}
	}
	return FixResult{Fixed: false, Message: "no matching fix rule"}
}

// truncateRunes returns the first max runes of s. When s contains fewer than max runes, it is returned as-is. A non-positive max returns an empty string.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// readLogTail reads the file at path and returns the last n lines (joined by newlines).
// It returns an empty string when the file cannot be read or path is empty.
func readLogTail(path string, n int) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
