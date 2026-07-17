package framework

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/i18n"
)

func TestRecord(t *testing.T) {
	t.Run("single record", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Passed: true, Duration: 1 * time.Millisecond})
		snap := r.Snapshot()
		if len(snap) != 1 {
			t.Fatalf("expected 1 result, got %d", len(snap))
		}
		if snap[0].Name != "t1" {
			t.Fatalf("expected name t1, got %s", snap[0].Name)
		}
	})

	t.Run("multiple records preserve order", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Passed: true})
		r.Record(TestResult{Name: "t2", Passed: false})
		r.Record(TestResult{Name: "t3", Skipped: true})
		snap := r.Snapshot()
		if len(snap) != 3 {
			t.Fatalf("expected 3 results, got %d", len(snap))
		}
		wantNames := []string{"t1", "t2", "t3"}
		for i, want := range wantNames {
			if snap[i].Name != want {
				t.Fatalf("index %d: expected %s, got %s", i, want, snap[i].Name)
			}
		}
	})
}

func TestSnapshot(t *testing.T) {
	t.Run("returns a copy", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Passed: true})
		snap := r.Snapshot()
		// Mutating the returned slice must not affect the reporter.
		snap[0].Name = "mutated"
		snap = append(snap, TestResult{Name: "extra"})

		again := r.Snapshot()
		if len(again) != 1 {
			t.Fatalf("expected reporter to still hold 1 result, got %d", len(again))
		}
		if again[0].Name != "t1" {
			t.Fatalf("expected original name t1, got %s", again[0].Name)
		}
	})

	t.Run("empty reporter returns empty slice", func(t *testing.T) {
		r := NewTestReporter()
		snap := r.Snapshot()
		if snap == nil {
			t.Fatalf("expected non-nil slice")
		}
		if len(snap) != 0 {
			t.Fatalf("expected 0 results, got %d", len(snap))
		}
	})
}

func TestSummaryAggregation(t *testing.T) {
	t.Run("all passed", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Passed: true, Duration: 10 * time.Millisecond})
		r.Record(TestResult{Name: "t2", Passed: true, Duration: 20 * time.Millisecond})
		s := r.Summary()
		if s.Total != 2 {
			t.Fatalf("expected Total=2, got %d", s.Total)
		}
		if s.Passed != 2 {
			t.Fatalf("expected Passed=2, got %d", s.Passed)
		}
		if s.Failed != 0 {
			t.Fatalf("expected Failed=0, got %d", s.Failed)
		}
		if s.Skipped != 0 {
			t.Fatalf("expected Skipped=0, got %d", s.Skipped)
		}
	})

	t.Run("all failed", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Passed: false, Error: errors.New("boom")})
		r.Record(TestResult{Name: "t2", Passed: false})
		s := r.Summary()
		if s.Total != 2 || s.Failed != 2 || s.Passed != 0 || s.Skipped != 0 {
			t.Fatalf("unexpected summary: %+v", s)
		}
	})

	t.Run("mixed with skipped", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Passed: true})
		r.Record(TestResult{Name: "t2", Passed: false})
		r.Record(TestResult{Name: "t3", Skipped: true})
		s := r.Summary()
		if s.Total != 3 || s.Passed != 1 || s.Failed != 1 || s.Skipped != 1 {
			t.Fatalf("unexpected summary: %+v", s)
		}
	})

	t.Run("duration aggregation", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Duration: 1 * time.Second})
		r.Record(TestResult{Name: "t2", Duration: 2 * time.Second})
		r.Record(TestResult{Name: "t3", Duration: 3 * time.Second})
		s := r.Summary()
		// Duration is the max of cumulative (6s) and elapsed since start;
		// elapsed is tiny here so Duration should equal the cumulative sum.
		cumulative := 6 * time.Second
		if s.Duration < cumulative {
			t.Fatalf("expected Duration >= %s, got %s", cumulative, s.Duration)
		}
	})

	t.Run("empty reporter summary", func(t *testing.T) {
		r := NewTestReporter()
		s := r.Summary()
		if s.Total != 0 || s.Passed != 0 || s.Failed != 0 || s.Skipped != 0 {
			t.Fatalf("expected zero summary, got %+v", s)
		}
	})
}

func TestSummaryConcurrency(t *testing.T) {
	r := NewTestReporter()
	const goroutines = 100
	const perGoroutine = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				r.Record(TestResult{Name: "concurrent", Passed: true})
			}
		}()
	}
	wg.Wait()

	snap := r.Snapshot()
	expected := goroutines * perGoroutine
	if len(snap) != expected {
		t.Fatalf("expected %d results, got %d", expected, len(snap))
	}
	s := r.Summary()
	if s.Total != expected {
		t.Fatalf("expected Total=%d, got %d", expected, s.Total)
	}
}

func TestWriteJSON(t *testing.T) {
	t.Run("writes payload with summary and results", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Passed: true, Message: "ok", Duration: 1 * time.Millisecond})
		r.Record(TestResult{Name: "t2", Passed: false, Message: "bad", Duration: 2 * time.Millisecond, Error: errors.New("boom")})
		r.Record(TestResult{Name: "t3", Skipped: true, Message: "skip reason"})

		path := filepath.Join(t.TempDir(), "sub", "report.json")
		if err := r.WriteJSON(path); err != nil {
			t.Fatalf("WriteJSON failed: %v", err)
		}

		// Verify the parent directory was created.
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("report file not created: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read report failed: %v", err)
		}

		var payload reportPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if payload.Summary.Total != 3 {
			t.Fatalf("expected Total=3, got %d", payload.Summary.Total)
		}
		if payload.Summary.Passed != 1 {
			t.Fatalf("expected Passed=1, got %d", payload.Summary.Passed)
		}
		if payload.Summary.Failed != 1 {
			t.Fatalf("expected Failed=1, got %d", payload.Summary.Failed)
		}
		if payload.Summary.Skipped != 1 {
			t.Fatalf("expected Skipped=1, got %d", payload.Summary.Skipped)
		}

		if len(payload.Results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(payload.Results))
		}

		// Verify per-result fields.
		r0 := payload.Results[0]
		if r0.Name != "t1" || !r0.Passed || r0.Skipped || r0.Message != "ok" {
			t.Fatalf("unexpected r0: %+v", r0)
		}
		if r0.Duration != 1*time.Millisecond {
			t.Fatalf("expected r0 duration 1ms, got %v", r0.Duration)
		}
		if r0.Error != "" {
			t.Fatalf("expected empty error for passed result, got %q", r0.Error)
		}

		r1 := payload.Results[1]
		if r1.Name != "t2" || r1.Passed || r1.Skipped {
			t.Fatalf("unexpected r1: %+v", r1)
		}
		if r1.Error != "boom" {
			t.Fatalf("expected error 'boom', got %q", r1.Error)
		}

		r2 := payload.Results[2]
		if r2.Name != "t3" || !r2.Skipped {
			t.Fatalf("unexpected r2: %+v", r2)
		}
		if r2.Message != "skip reason" {
			t.Fatalf("expected skip reason message, got %q", r2.Message)
		}
	})

	t.Run("creates nested directory", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Passed: true})
		path := filepath.Join(t.TempDir(), "a", "b", "c", "report.json")
		if err := r.WriteJSON(path); err != nil {
			t.Fatalf("WriteJSON failed: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("report file not created: %v", err)
		}
	})
}

func TestWriteJUnitXML(t *testing.T) {
	t.Run("writes valid JUnit XML with counts and elements", func(t *testing.T) {
		r := NewTestReporter()
		r.SuiteName = "my-suite"
		r.Record(TestResult{Name: "t1", Passed: true, Duration: 1 * time.Millisecond})
		r.Record(TestResult{Name: "t2", Passed: false, Message: "failed msg", Duration: 2 * time.Millisecond, Error: errors.New("err detail")})
		r.Record(TestResult{Name: "t3", Skipped: true, Message: "skip msg"})

		path := filepath.Join(t.TempDir(), "sub", "report.xml")
		if err := r.WriteJUnitXML(path); err != nil {
			t.Fatalf("WriteJUnitXML failed: %v", err)
		}

		if _, err := os.Stat(path); err != nil {
			t.Fatalf("report file not created: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read report failed: %v", err)
		}

		// Verify the XML header is present.
		if !strings.HasPrefix(string(data), `<?xml`) {
			t.Fatalf("expected XML header prefix, got: %s", string(data[:min(len(data), 50)]))
		}

		var suites junitTestSuites
		if err := xml.Unmarshal(data, &suites); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if len(suites.Suites) != 1 {
			t.Fatalf("expected 1 testsuite, got %d", len(suites.Suites))
		}
		suite := suites.Suites[0]
		if suite.Name != "my-suite" {
			t.Fatalf("expected suite name my-suite, got %s", suite.Name)
		}
		if suite.Tests != 3 {
			t.Fatalf("expected Tests=3, got %d", suite.Tests)
		}
		if suite.Failures != 1 {
			t.Fatalf("expected Failures=1, got %d", suite.Failures)
		}
		if suite.Skipped != 1 {
			t.Fatalf("expected Skipped=1, got %d", suite.Skipped)
		}
		if len(suite.TestCases) != 3 {
			t.Fatalf("expected 3 testcases, got %d", len(suite.TestCases))
		}

		// Passed case: no failure, no skipped.
		tc0 := suite.TestCases[0]
		if tc0.Name != "t1" {
			t.Fatalf("expected tc0 name t1, got %s", tc0.Name)
		}
		if tc0.Classname != "my-suite" {
			t.Fatalf("expected tc0 classname my-suite, got %s", tc0.Classname)
		}
		if tc0.Failure != nil {
			t.Fatalf("expected no failure on passed case")
		}
		if tc0.Skipped != nil {
			t.Fatalf("expected no skipped on passed case")
		}

		// Failed case: failure element populated.
		tc1 := suite.TestCases[1]
		if tc1.Name != "t2" {
			t.Fatalf("expected tc1 name t2, got %s", tc1.Name)
		}
		if tc1.Failure == nil {
			t.Fatalf("expected failure element on failed case")
		}
		if tc1.Failure.Message != "failed msg" {
			t.Fatalf("expected failure message 'failed msg', got %q", tc1.Failure.Message)
		}
		if tc1.Failure.Content != "err detail" {
			t.Fatalf("expected failure content 'err detail', got %q", tc1.Failure.Content)
		}
		if tc1.Skipped != nil {
			t.Fatalf("expected no skipped on failed case")
		}

		// Skipped case: skipped element populated.
		tc2 := suite.TestCases[2]
		if tc2.Name != "t3" {
			t.Fatalf("expected tc2 name t3, got %s", tc2.Name)
		}
		if tc2.Skipped == nil {
			t.Fatalf("expected skipped element on skipped case")
		}
		if tc2.Skipped.Message != "skip msg" {
			t.Fatalf("expected skipped message 'skip msg', got %q", tc2.Skipped.Message)
		}
		if tc2.Failure != nil {
			t.Fatalf("expected no failure on skipped case")
		}
	})

	t.Run("creates nested directory", func(t *testing.T) {
		r := NewTestReporter()
		r.Record(TestResult{Name: "t1", Passed: true})
		path := filepath.Join(t.TempDir(), "x", "y", "report.xml")
		if err := r.WriteJUnitXML(path); err != nil {
			t.Fatalf("WriteJUnitXML failed: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("report file not created: %v", err)
		}
	})
}

func TestSuiteNameFallback(t *testing.T) {
	r := NewTestReporter()
	// SuiteName is empty by default.
	r.Record(TestResult{Name: "t1", Passed: true})

	path := filepath.Join(t.TempDir(), "report.xml")
	if err := r.WriteJUnitXML(path); err != nil {
		t.Fatalf("WriteJUnitXML failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report failed: %v", err)
	}

	var suites junitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(suites.Suites) != 1 {
		t.Fatalf("expected 1 testsuite, got %d", len(suites.Suites))
	}
	if suites.Suites[0].Name != "taichi-tests" {
		t.Fatalf("expected fallback name 'taichi-tests', got %q", suites.Suites[0].Name)
	}
	if len(suites.Suites[0].TestCases) != 1 {
		t.Fatalf("expected 1 testcase, got %d", len(suites.Suites[0].TestCases))
	}
	if suites.Suites[0].TestCases[0].Classname != "taichi-tests" {
		t.Fatalf("expected classname fallback 'taichi-tests', got %q", suites.Suites[0].TestCases[0].Classname)
	}
}

func TestPrintSummary(t *testing.T) {
	// Pin the locale to en-US so the expected strings are deterministic.
	prev := i18n.GetLocale()
	i18n.SetLocale(i18n.EnUS)
	defer i18n.SetLocale(prev)

	r := NewTestReporter()
	r.Record(TestResult{Name: "alpha-test-case", Passed: true, Message: "all good"})
	r.Record(TestResult{Name: "beta-test-case", Passed: false, Message: "something broke"})
	r.Record(TestResult{Name: "gamma-test-case", Skipped: true, Message: "skipped reason"})

	var buf bytes.Buffer
	r.PrintSummary(&buf)

	out := buf.String()

	// Verify summary section headers are present.
	for _, want := range []string{
		"=== Test Summary ===",
		"Total",
		"Passed",
		"Failed",
		"Skipped",
		"Duration",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q\noutput:\n%s", want, out)
		}
	}

	// Verify table headers and per-result rows.
	for _, want := range []string{"TEST", "STATUS", "DURATION", "MESSAGE"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain table header %q\noutput:\n%s", want, out)
		}
	}

	// Verify each test name appears.
	for _, want := range []string{"alpha-test-case", "beta-test-case", "gamma-test-case"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain test name %q\noutput:\n%s", want, out)
		}
	}

	// Verify status strings appear (en-US locale).
	for _, want := range []string{"PASS", "FAIL", "SKIP"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain status %q\noutput:\n%s", want, out)
		}
	}

	// Verify the separator line of 80 dashes is present.
	if !strings.Contains(out, repeat("-", 80)) {
		t.Errorf("expected output to contain separator line of 80 dashes")
	}
}

func TestDurationToSeconds(t *testing.T) {
	cases := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0.000"},
		{1 * time.Second, "1.000"},
		{1500 * time.Millisecond, "1.500"},
		{1 * time.Minute, "60.000"},
		{2500 * time.Microsecond, "0.003"}, // 0.0025 rounds to 0.003 with %.3f
		{-1 * time.Second, "-1.000"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.input.String(), func(t *testing.T) {
			got := durationToSeconds(c.input)
			if got != c.expected {
				t.Fatalf("durationToSeconds(%v) = %q, expected %q", c.input, got, c.expected)
			}
		})
	}
}

func TestRepeat(t *testing.T) {
	cases := []struct {
		name     string
		s        string
		n        int
		expected string
	}{
		{"positive n", "ab", 3, "ababab"},
		{"single repeat", "x", 1, "x"},
		{"zero n", "x", 0, ""},
		{"negative n", "x", -5, ""},
		{"empty string positive n", "", 5, ""},
		{"empty string zero n", "", 0, ""},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := repeat(c.s, c.n)
			if got != c.expected {
				t.Fatalf("repeat(%q, %d) = %q, expected %q", c.s, c.n, got, c.expected)
			}
		})
	}
}
