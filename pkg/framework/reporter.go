package framework

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tickraft/taichi/pkg/i18n"
)

// TestSummary holds the aggregated statistics of a set of test results.
type TestSummary struct {
	// Total is the total number of recorded test results.
	Total int
	// Passed is the number of successful test results.
	Passed int
	// Failed is the number of test results that did not succeed and were not skipped.
	Failed int
	// Skipped is the number of test results that were skipped.
	Skipped int
	// Duration is the cumulative execution time of all recorded results.
	Duration time.Duration
}

// reportPayload is the on-disk representation written by WriteJSON, bundling the aggregated summary with each result.
type reportPayload struct {
	Summary TestSummary    `json:"summary"`
	Results []reportResult `json:"results"`
}

// reportResult is the JSON-serializable form of TestResult.
// The Error field is converted to a string because the error interface cannot be directly deserialized by encoding/json.
type reportResult struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Skipped  bool          `json:"skipped"`
	Message  string        `json:"message"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// toReportResult converts a TestResult to its JSON-serializable form.
func toReportResult(r TestResult) reportResult {
	out := reportResult{
		Name:     r.Name,
		Passed:   r.Passed,
		Skipped:  r.Skipped,
		Message:  r.Message,
		Duration: r.Duration,
	}
	if r.Error != nil {
		out.Error = r.Error.Error()
	}
	return out
}

// junitTestSuites is the root element of the JUnit XML report.
type junitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []junitTestSuite `xml:"testsuite"`
}

// junitTestSuite represents a single <testsuite> element.
type junitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      string          `xml:"time,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

// junitTestCase represents a single <testcase> element. Failure and Skipped are pointers so they can be omitted when nil.
type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

// junitFailure represents a <failure> element inside a test case.
type junitFailure struct {
	Message string `xml:"message,attr"`
	Content string `xml:",chardata"`
}

// junitSkipped represents a <skipped> element inside a test case.
type junitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

// TestReporter collects test results and generates multi-format reports (JSON, JUnit XML, human-readable summary).
//
// All public methods are concurrency-safe.
type TestReporter struct {
	mu        sync.Mutex
	results   []TestResult
	startTime time.Time
	// SuiteName is used for the <testsuite name> and testcase classname in JUnit XML.
	// When empty, it falls back to "taichi-tests".
	SuiteName string
}

// NewTestReporter returns a TestReporter with the start time set to the current moment.
func NewTestReporter() *TestReporter {
	return &TestReporter{
		startTime: time.Now(),
	}
}

// Record appends a TestResult to the reporter in a thread-safe manner.
func (r *TestReporter) Record(result TestResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result)
}

// Snapshot returns a copy of the recorded results, allowing external formatters to consume them without holding the lock.
func (r *TestReporter) Snapshot() []TestResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]TestResult, len(r.results))
	copy(out, r.results)
	return out
}

// Summary returns the aggregated statistics of the recorded results.
// The Duration field reflects the cumulative duration of all results, as well as the wall-clock time since the reporter was created (whichever is larger).
func (r *TestReporter) Summary() TestSummary {
	r.mu.Lock()
	defer r.mu.Unlock()

	summary := TestSummary{Total: len(r.results)}
	for _, res := range r.results {
		summary.Duration += res.Duration
		switch {
		case res.Skipped:
			summary.Skipped++
		case res.Passed:
			summary.Passed++
		default:
			summary.Failed++
		}
	}
	if elapsed := time.Since(r.startTime); elapsed > summary.Duration {
		summary.Duration = elapsed
	}
	return summary
}

// suiteNameLocked returns the valid suite name (falls back to the default when empty).
// The caller must already hold r.mu or pass in a copy.
func (r *TestReporter) suiteNameLocked() string {
	if r.SuiteName != "" {
		return r.SuiteName
	}
	return "taichi-tests"
}

// WriteJSON writes the recorded results and aggregated summary as formatted JSON to path.
// The parent directory is created automatically when it does not exist.
func (r *TestReporter) WriteJSON(path string) error {
	r.mu.Lock()
	results := make([]TestResult, len(r.results))
	copy(results, r.results)
	r.mu.Unlock()

	summary := r.computeSummaryLocked(results)
	reportResults := make([]reportResult, len(results))
	for i, res := range results {
		reportResults[i] = toReportResult(res)
	}
	payload := reportPayload{Summary: summary, Results: reportResults}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// WriteJUnitXML writes the recorded results in JUnit XML format to path.
// The parent directory is created automatically when it does not exist.
func (r *TestReporter) WriteJUnitXML(path string) error {
	r.mu.Lock()
	results := make([]TestResult, len(r.results))
	copy(results, r.results)
	suiteName := r.suiteNameLocked()
	r.mu.Unlock()

	summary := r.computeSummaryLocked(results)
	cases := make([]junitTestCase, 0, len(results))
	for _, res := range results {
		tc := junitTestCase{
			Name:      res.Name,
			Classname: suiteName,
			Time:      durationToSeconds(res.Duration),
		}
		if res.Skipped {
			tc.Skipped = &junitSkipped{Message: res.Message}
		} else if !res.Passed {
			msg := res.Message
			if msg == "" && res.Error != nil {
				msg = res.Error.Error()
			}
			if msg == "" {
				msg = i18n.T("reporter.test_failed")
			}
			content := msg
			if res.Error != nil {
				content = res.Error.Error()
			}
			tc.Failure = &junitFailure{Message: msg, Content: content}
		}
		cases = append(cases, tc)
	}

	suites := junitTestSuites{
		Suites: []junitTestSuite{
			{
				Name:      suiteName,
				Tests:     summary.Total,
				Failures:  summary.Failed,
				Skipped:   summary.Skipped,
				Time:      durationToSeconds(summary.Duration),
				TestCases: cases,
			},
		},
	}

	data, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal junit xml: %w", err)
	}
	output := append([]byte(xml.Header), data...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}
	if err := os.WriteFile(path, output, 0o644); err != nil {
		return fmt.Errorf("write junit xml: %w", err)
	}
	return nil
}

// PrintSummary writes a human-readable summary table to w.
func (r *TestReporter) PrintSummary(w io.Writer) {
	summary := r.Summary()
	r.mu.Lock()
	results := make([]TestResult, len(r.results))
	copy(results, r.results)
	r.mu.Unlock()

	_, _ = fmt.Fprintf(w, "\n%s\n", i18n.T("reporter.summary.title"))
	_, _ = fmt.Fprintf(w, "%s:   %d\n", i18n.T("reporter.summary.total"), summary.Total)
	_, _ = fmt.Fprintf(w, "%s:  %d\n", i18n.T("reporter.summary.passed"), summary.Passed)
	_, _ = fmt.Fprintf(w, "%s:  %d\n", i18n.T("reporter.summary.failed"), summary.Failed)
	_, _ = fmt.Fprintf(w, "%s: %d\n", i18n.T("reporter.summary.skipped"), summary.Skipped)
	_, _ = fmt.Fprintf(w, "%s: %s\n\n", i18n.T("reporter.summary.duration"), summary.Duration)

	if len(results) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "%-40s %-8s %-10s %s\n", i18n.T("reporter.table.test"), i18n.T("reporter.table.status"), i18n.T("reporter.table.duration"), i18n.T("reporter.table.message"))
	_, _ = fmt.Fprintf(w, "%s\n", repeat("-", 80))
	for _, res := range results {
		status := i18n.T("reporter.status.fail")
		switch {
		case res.Skipped:
			status = i18n.T("reporter.status.skip")
		case res.Passed:
			status = i18n.T("reporter.status.pass")
		}
		name := res.Name
		if len(name) > 38 {
			name = name[:35] + "..."
		}
		msg := res.Message
		if !res.Passed && res.Error != nil && msg == "" {
			msg = res.Error.Error()
		}
		if len(msg) > 30 {
			msg = msg[:27] + "..."
		}
		_, _ = fmt.Fprintf(w, "%-40s %-8s %-10s %s\n", name, status, res.Duration, msg)
	}
	_, _ = fmt.Fprintln(w)
}

// computeSummaryLocked computes the summary for the given results without acquiring the mutex.
// The caller must already hold the lock or pass in a copy.
func (r *TestReporter) computeSummaryLocked(results []TestResult) TestSummary {
	summary := TestSummary{Total: len(results)}
	for _, res := range results {
		summary.Duration += res.Duration
		switch {
		case res.Skipped:
			summary.Skipped++
		case res.Passed:
			summary.Passed++
		default:
			summary.Failed++
		}
	}
	if elapsed := time.Since(r.startTime); elapsed > summary.Duration {
		summary.Duration = elapsed
	}
	return summary
}

// durationToSeconds formats a duration as a decimal-seconds string suitable for the JUnit XML time attribute.
func durationToSeconds(d time.Duration) string {
	seconds := d.Seconds()
	return fmt.Sprintf("%.3f", seconds)
}

// repeat returns s repeated n times. A small helper that exists only to avoid importing strings just for the table separator.
func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
