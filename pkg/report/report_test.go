package report

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
)

// mockWriter is a stub Writer that records its invocation arguments and writes
// the configured content bytes to the target io.Writer.
type mockWriter struct {
	format     Format
	content    []byte
	called     bool
	gotSummary framework.TestSummary
	gotResults []framework.TestResult
	writeErr   error
}

func (m *mockWriter) Format() Format { return m.format }

func (m *mockWriter) Write(w io.Writer, summary framework.TestSummary, results []framework.TestResult) error {
	m.called = true
	m.gotSummary = summary
	m.gotResults = results
	if m.writeErr != nil {
		return m.writeErr
	}
	if len(m.content) > 0 {
		if _, err := w.Write(m.content); err != nil {
			return err
		}
	}
	return nil
}

// newPopulatedReporter returns a TestReporter preloaded with three results
// covering the pass / fail / skip outcomes.
func newPopulatedReporter() *framework.TestReporter {
	r := framework.NewTestReporter()
	r.Record(framework.TestResult{Name: "test-1", Passed: true, Duration: 10 * time.Millisecond})
	r.Record(framework.TestResult{Name: "test-2", Passed: false, Message: "boom", Duration: 5 * time.Millisecond})
	r.Record(framework.TestResult{Name: "test-3", Skipped: true, Message: "skip me", Duration: 1 * time.Millisecond})
	return r
}

func TestFormatConstants(t *testing.T) {
	if FormatJSON != "json" {
		t.Fatalf("FormatJSON = %q, want %q", FormatJSON, "json")
	}
	if FormatJUnit != "junit" {
		t.Fatalf("FormatJUnit = %q, want %q", FormatJUnit, "junit")
	}
	if FormatSummary != "summary" {
		t.Fatalf("FormatSummary = %q, want %q", FormatSummary, "summary")
	}
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	formats := r.Formats()
	if len(formats) != 0 {
		t.Fatalf("Formats() len = %d, want 0", len(formats))
	}
}

func TestRegister(t *testing.T) {
	r := NewRegistry()
	w := &mockWriter{format: "html", content: []byte("<html></html>")}
	r.Register(w)
	got, err := r.Get("html")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != w {
		t.Fatalf("Get returned %v, want %v", got, w)
	}
	formats := r.Formats()
	if len(formats) != 1 || formats[0] != "html" {
		t.Fatalf("Formats() = %v, want [html]", formats)
	}
}

func TestRegisterNil(t *testing.T) {
	r := NewRegistry()
	r.Register(nil)
	if len(r.Formats()) != 0 {
		t.Fatalf("Formats() len = %d, want 0 after Register(nil)", len(r.Formats()))
	}
}

func TestRegisterOverwrite(t *testing.T) {
	r := NewRegistry()
	w1 := &mockWriter{format: "html", content: []byte("v1")}
	w2 := &mockWriter{format: "html", content: []byte("v2")}
	r.Register(w1)
	r.Register(w2)
	got, err := r.Get("html")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != w2 {
		t.Fatalf("After overwrite, Get returned %v, want %v", got, w2)
	}
}

func TestGetMissing(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("html"); err == nil {
		t.Fatalf("Get unregistered format expected error, got nil")
	}
}

func TestFormats(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockWriter{format: "html"})
	r.Register(&mockWriter{format: "slack"})
	formats := r.Formats()
	if len(formats) != 2 {
		t.Fatalf("Formats() len = %d, want 2", len(formats))
	}
	got := map[Format]bool{}
	for _, f := range formats {
		got[f] = true
	}
	if !got["html"] || !got["slack"] {
		t.Fatalf("Formats() = %v, want both html and slack", formats)
	}
}

func TestGenerateDefaultFormats(t *testing.T) {
	reporter := newPopulatedReporter()
	dir := t.TempDir()
	paths := map[Format]string{
		FormatJSON:    filepath.Join(dir, "out.json"),
		FormatJUnit:   filepath.Join(dir, "out.xml"),
		FormatSummary: filepath.Join(dir, "out.txt"),
	}
	// Passing nil for formats triggers the default [json, junit, summary] set.
	if err := Generate(reporter, nil, nil, func(f Format) string {
		return paths[f]
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	for f, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("file for %s not created: %v", f, err)
		}
	}

	// Verify JSON content is parseable and contains the recorded results.
	data, err := os.ReadFile(paths[FormatJSON])
	if err != nil {
		t.Fatalf("ReadFile(json) error: %v", err)
	}
	var payload struct {
		Summary framework.TestSummary `json:"summary"`
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if payload.Summary.Total != 3 {
		t.Fatalf("summary.Total = %d, want 3", payload.Summary.Total)
	}
	if len(payload.Results) != 3 {
		t.Fatalf("results len = %d, want 3", len(payload.Results))
	}

	// Verify JUnit XML content contains the <testsuites> root element.
	xmlData, err := os.ReadFile(paths[FormatJUnit])
	if err != nil {
		t.Fatalf("ReadFile(junit) error: %v", err)
	}
	if !strings.Contains(string(xmlData), "<testsuites") {
		t.Fatalf("junit xml missing <testsuites>: %s", xmlData)
	}

	// Verify the summary file is non-empty.
	summaryData, err := os.ReadFile(paths[FormatSummary])
	if err != nil {
		t.Fatalf("ReadFile(summary) error: %v", err)
	}
	if len(summaryData) == 0 {
		t.Fatalf("summary file is empty")
	}
}

func TestGenerateJSON(t *testing.T) {
	reporter := newPopulatedReporter()
	path := filepath.Join(t.TempDir(), "result.json")
	if err := Generate(reporter, nil, []Format{FormatJSON}, func(f Format) string {
		if f == FormatJSON {
			return path
		}
		return ""
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("json file content is not valid JSON: %s", data)
	}
}

func TestGenerateJUnit(t *testing.T) {
	reporter := newPopulatedReporter()
	path := filepath.Join(t.TempDir(), "result.xml")
	if err := Generate(reporter, nil, []Format{FormatJUnit}, func(f Format) string {
		if f == FormatJUnit {
			return path
		}
		return ""
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !strings.Contains(string(data), "<testsuites") {
		t.Fatalf("junit xml missing <testsuites>: %s", data)
	}
	// Verify the content parses as XML.
	var suites struct {
		XMLName xml.Name `xml:"testsuites"`
	}
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("xml.Unmarshal error: %v", err)
	}
}

func TestGenerateSummaryToStdout(t *testing.T) {
	reporter := newPopulatedReporter()
	// pathFor returns "" for summary -> Generate writes to os.Stdout and reports no error.
	if err := Generate(reporter, nil, []Format{FormatSummary}, func(f Format) string {
		return ""
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
}

func TestGenerateSummaryToFile(t *testing.T) {
	reporter := newPopulatedReporter()
	path := filepath.Join(t.TempDir(), "summary.txt")
	if err := Generate(reporter, nil, []Format{FormatSummary}, func(f Format) string {
		if f == FormatSummary {
			return path
		}
		return ""
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("summary file not created: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("summary file is empty")
	}
}

func TestGenerateCustomFormat(t *testing.T) {
	reporter := newPopulatedReporter()
	registry := NewRegistry()
	w := &mockWriter{format: "html", content: []byte("<html>report</html>")}
	registry.Register(w)
	path := filepath.Join(t.TempDir(), "report.html")
	if err := Generate(reporter, registry, []Format{"html"}, func(f Format) string {
		if f == "html" {
			return path
		}
		return ""
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !w.called {
		t.Fatalf("custom Writer.Write was not called")
	}
	if w.gotSummary.Total != 3 {
		t.Fatalf("Writer received summary.Total = %d, want 3", w.gotSummary.Total)
	}
	if len(w.gotResults) != 3 {
		t.Fatalf("Writer received results len = %d, want 3", len(w.gotResults))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !bytes.Equal(data, w.content) {
		t.Fatalf("file content = %q, want %q", data, w.content)
	}
}

func TestGenerateCustomFormatToStdout(t *testing.T) {
	reporter := newPopulatedReporter()
	registry := NewRegistry()
	w := &mockWriter{format: "html", content: []byte("<html>report</html>")}
	registry.Register(w)
	// pathFor returns "" -> custom Writer.Write targets os.Stdout.
	if err := Generate(reporter, registry, []Format{"html"}, func(f Format) string {
		return ""
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !w.called {
		t.Fatalf("custom Writer.Write was not called")
	}
}

func TestGenerateUnknownFormatNoRegistry(t *testing.T) {
	reporter := newPopulatedReporter()
	// "html" is not a built-in format and registry is nil: Generate silently
	// continues without error.
	if err := Generate(reporter, nil, []Format{"html"}, func(f Format) string {
		return filepath.Join(t.TempDir(), "ignored.html")
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
}

func TestGenerateUnknownFormatMissingWriter(t *testing.T) {
	reporter := newPopulatedReporter()
	registry := NewRegistry()
	// registry is set but no writer is registered for "html" -> Generate must
	// surface the lookup error.
	err := Generate(reporter, registry, []Format{"html"}, func(f Format) string {
		return filepath.Join(t.TempDir(), "ignored.html")
	})
	if err == nil {
		t.Fatalf("Generate expected error for missing writer, got nil")
	}
	if !strings.Contains(err.Error(), "html") {
		t.Fatalf("error %q does not mention format html", err.Error())
	}
}
