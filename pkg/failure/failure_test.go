package failure

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
)

// ---------------------------------------------------------------------------
// errorString tests
// ---------------------------------------------------------------------------

// TestErrorString covers the unexported errorString helper for nil and non-nil errors.
func TestErrorString(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, ""},
		{"non-nil error", errors.New("boom"), "boom"},
		{"empty message error", errors.New(""), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := errorString(c.err); got != c.want {
				t.Errorf("errorString(%v) = %q, want %q", c.err, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FromResults tests
// ---------------------------------------------------------------------------

// TestFromResultsEmpty verifies that FromResults populates header fields and a
// parseable RFC3339 UTC timestamp when there are no results.
func TestFromResultsEmpty(t *testing.T) {
	fc := FromResults("proj", "http://localhost:8080", "/root", "/var/log/svc.log", "/reports", nil, nil)
	if fc == nil {
		t.Fatal("FromResults returned nil")
	}
	if fc.ProjectName != "proj" {
		t.Errorf("ProjectName = %q, want %q", fc.ProjectName, "proj")
	}
	if fc.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want %q", fc.BaseURL, "http://localhost:8080")
	}
	if fc.ProjectRoot != "/root" {
		t.Errorf("ProjectRoot = %q, want %q", fc.ProjectRoot, "/root")
	}
	if fc.EnvLogPath != "/var/log/svc.log" {
		t.Errorf("EnvLogPath = %q, want %q", fc.EnvLogPath, "/var/log/svc.log")
	}
	if fc.ReportsDir != "/reports" {
		t.Errorf("ReportsDir = %q, want %q", fc.ReportsDir, "/reports")
	}
	if fc.TotalCases != 0 {
		t.Errorf("TotalCases = %d, want 0", fc.TotalCases)
	}
	if fc.PassedCases != 0 {
		t.Errorf("PassedCases = %d, want 0", fc.PassedCases)
	}
	if fc.FailedCases != nil {
		t.Errorf("FailedCases = %v, want nil", fc.FailedCases)
	}
	if fc.Timestamp == "" {
		t.Fatal("Timestamp is empty, want RFC3339 timestamp")
	}
	// Timestamp must be parseable as RFC3339.
	if _, err := time.Parse(time.RFC3339, fc.Timestamp); err != nil {
		t.Errorf("Timestamp %q is not RFC3339: %v", fc.Timestamp, err)
	}
}

// TestFromResults covers counting logic and failed-case construction across
// mixed pass/fail/skip scenarios using table-driven subtests.
func TestFromResults(t *testing.T) {
	cases := []struct {
		name            string
		results         []framework.TestResult
		wantTotal       int
		wantPassed      int
		wantFailedCount int
		wantFailedCases []FailedCase
	}{
		{
			name: "all passed",
			results: []framework.TestResult{
				{Name: "t1", Passed: true, Message: "ok", Duration: 100 * time.Millisecond},
				{Name: "t2", Passed: true, Message: "ok", Duration: 200 * time.Millisecond},
			},
			wantTotal:       2,
			wantPassed:      2,
			wantFailedCount: 0,
		},
		{
			name: "all failed",
			results: []framework.TestResult{
				{Name: "t1", Passed: false, Message: "fail1", Error: errors.New("err1"), Duration: 50 * time.Millisecond},
				{Name: "t2", Passed: false, Message: "fail2", Error: nil, Duration: 60 * time.Millisecond},
			},
			wantTotal:       2,
			wantPassed:      0,
			wantFailedCount: 2,
			wantFailedCases: []FailedCase{
				{Name: "t1", Message: "fail1", Error: "err1", Duration: (50 * time.Millisecond).String()},
				{Name: "t2", Message: "fail2", Error: "", Duration: (60 * time.Millisecond).String()},
			},
		},
		{
			name: "all skipped",
			results: []framework.TestResult{
				{Name: "t1", Skipped: true},
				// Skipped takes precedence over Passed in the implementation.
				{Name: "t2", Skipped: true, Passed: true},
			},
			wantTotal:       2,
			wantPassed:      0,
			wantFailedCount: 0,
		},
		{
			name: "mixed pass skip fail",
			results: []framework.TestResult{
				{Name: "pass", Passed: true, Duration: 10 * time.Millisecond},
				{Name: "skip", Skipped: true, Duration: 0},
				{Name: "fail", Passed: false, Message: "broken", Error: errors.New("timeout"), Duration: 5 * time.Second},
			},
			wantTotal:       3,
			wantPassed:      1,
			wantFailedCount: 1,
			wantFailedCases: []FailedCase{
				{Name: "fail", Message: "broken", Error: "timeout", Duration: (5 * time.Second).String()},
			},
		},
		{
			name:            "nil results",
			results:         nil,
			wantTotal:       0,
			wantPassed:      0,
			wantFailedCount: 0,
		},
		{
			name: "zero duration failed case",
			results: []framework.TestResult{
				{Name: "t1", Passed: false, Message: "instant fail"},
			},
			wantTotal:       1,
			wantPassed:      0,
			wantFailedCount: 1,
			wantFailedCases: []FailedCase{
				{Name: "t1", Message: "instant fail", Error: "", Duration: time.Duration(0).String()},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fc := FromResults("proj", "", "", "", "", c.results, nil)
			if fc.TotalCases != c.wantTotal {
				t.Errorf("TotalCases = %d, want %d", fc.TotalCases, c.wantTotal)
			}
			if fc.PassedCases != c.wantPassed {
				t.Errorf("PassedCases = %d, want %d", fc.PassedCases, c.wantPassed)
			}
			if len(fc.FailedCases) != c.wantFailedCount {
				t.Fatalf("len(FailedCases) = %d, want %d", len(fc.FailedCases), c.wantFailedCount)
			}
			for i, want := range c.wantFailedCases {
				got := fc.FailedCases[i]
				if got.Name != want.Name {
					t.Errorf("FailedCases[%d].Name = %q, want %q", i, got.Name, want.Name)
				}
				if got.Message != want.Message {
					t.Errorf("FailedCases[%d].Message = %q, want %q", i, got.Message, want.Message)
				}
				if got.Error != want.Error {
					t.Errorf("FailedCases[%d].Error = %q, want %q", i, got.Error, want.Error)
				}
				if got.Duration != want.Duration {
					t.Errorf("FailedCases[%d].Duration = %q, want %q", i, got.Duration, want.Duration)
				}
				// SkillName is always empty: FromResults never populates it
				// (the skillNames parameter is currently unused by the implementation).
				if got.SkillName != "" {
					t.Errorf("FailedCases[%d].SkillName = %q, want empty", i, got.SkillName)
				}
			}
		})
	}
}

// TestFromResultsSkillNameAlwaysEmpty verifies that the skillNames parameter
// does not influence FailedCase.SkillName, matching the current implementation
// where skillNames is accepted but not used.
func TestFromResultsSkillNameAlwaysEmpty(t *testing.T) {
	results := []framework.TestResult{
		{Name: "t1", Passed: false, Message: "fail"},
	}
	skillNames := []string{"api", "ui"}
	fc := FromResults("proj", "", "", "", "", results, skillNames)
	if len(fc.FailedCases) != 1 {
		t.Fatalf("len(FailedCases) = %d, want 1", len(fc.FailedCases))
	}
	if fc.FailedCases[0].SkillName != "" {
		t.Errorf("SkillName = %q, want empty (skillNames is unused)", fc.FailedCases[0].SkillName)
	}
}

// TestFromResultsTimestampIsUTC verifies the generated timestamp is RFC3339
// and denotes UTC (Z suffix), since FromResults uses time.Now().UTC().
func TestFromResultsTimestampIsUTC(t *testing.T) {
	fc := FromResults("proj", "", "", "", "", nil, nil)
	if _, err := time.Parse(time.RFC3339, fc.Timestamp); err != nil {
		t.Fatalf("Timestamp %q not RFC3339: %v", fc.Timestamp, err)
	}
	if !strings.HasSuffix(fc.Timestamp, "Z") {
		t.Errorf("Timestamp = %q, want it to end with 'Z' (UTC)", fc.Timestamp)
	}
}

// ---------------------------------------------------------------------------
// HasFailures tests
// ---------------------------------------------------------------------------

// TestHasFailures covers the nil receiver, empty, nil-slice, and populated cases.
func TestHasFailures(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var fc *FailureContext
		if fc.HasFailures() {
			t.Error("nil receiver HasFailures() = true, want false")
		}
	})
	t.Run("empty failed cases", func(t *testing.T) {
		fc := &FailureContext{}
		if fc.HasFailures() {
			t.Error("empty FailedCases HasFailures() = true, want false")
		}
	})
	t.Run("nil failed cases slice", func(t *testing.T) {
		fc := &FailureContext{FailedCases: nil}
		if fc.HasFailures() {
			t.Error("nil FailedCases HasFailures() = true, want false")
		}
	})
	t.Run("with failures", func(t *testing.T) {
		fc := &FailureContext{
			FailedCases: []FailedCase{{Name: "t1", Message: "fail"}},
		}
		if !fc.HasFailures() {
			t.Error("non-empty FailedCases HasFailures() = false, want true")
		}
	})
}

// ---------------------------------------------------------------------------
// JSON marshal/unmarshal tests
// ---------------------------------------------------------------------------

// TestFailureContextJSONRoundTrip verifies that all fields survive a
// marshal/unmarshal cycle, including nested FailedCase entries.
func TestFailureContextJSONRoundTrip(t *testing.T) {
	original := &FailureContext{
		ProjectName: "tickraft",
		BaseURL:     "http://localhost:8080",
		Timestamp:   "2026-07-17T12:00:00Z",
		ProjectRoot: "/home/user/tickraft",
		EnvLogPath:  "/var/log/svc.log",
		ReportsDir:  "/reports",
		TotalCases:  10,
		PassedCases: 8,
		FailedCases: []FailedCase{
			{SkillName: "api", Name: "test-api-1", Message: "status 500", Error: "timeout", Duration: "2s"},
			{SkillName: "", Name: "test-ui-1", Message: "element not found", Error: "", Duration: "500ms"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var decoded FailureContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	if decoded.ProjectName != original.ProjectName {
		t.Errorf("ProjectName = %q, want %q", decoded.ProjectName, original.ProjectName)
	}
	if decoded.BaseURL != original.BaseURL {
		t.Errorf("BaseURL = %q, want %q", decoded.BaseURL, original.BaseURL)
	}
	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %q, want %q", decoded.Timestamp, original.Timestamp)
	}
	if decoded.ProjectRoot != original.ProjectRoot {
		t.Errorf("ProjectRoot = %q, want %q", decoded.ProjectRoot, original.ProjectRoot)
	}
	if decoded.EnvLogPath != original.EnvLogPath {
		t.Errorf("EnvLogPath = %q, want %q", decoded.EnvLogPath, original.EnvLogPath)
	}
	if decoded.ReportsDir != original.ReportsDir {
		t.Errorf("ReportsDir = %q, want %q", decoded.ReportsDir, original.ReportsDir)
	}
	if decoded.TotalCases != original.TotalCases {
		t.Errorf("TotalCases = %d, want %d", decoded.TotalCases, original.TotalCases)
	}
	if decoded.PassedCases != original.PassedCases {
		t.Errorf("PassedCases = %d, want %d", decoded.PassedCases, original.PassedCases)
	}
	if len(decoded.FailedCases) != len(original.FailedCases) {
		t.Fatalf("len(FailedCases) = %d, want %d", len(decoded.FailedCases), len(original.FailedCases))
	}
	for i, want := range original.FailedCases {
		got := decoded.FailedCases[i]
		if got != want {
			t.Errorf("FailedCases[%d] = %+v, want %+v", i, got, want)
		}
	}
}

// TestFailureContextJSONFieldNames verifies that the JSON output uses the
// snake_case field names defined by struct tags.
func TestFailureContextJSONFieldNames(t *testing.T) {
	fc := &FailureContext{
		ProjectName: "p",
		Timestamp:   "ts",
		TotalCases:  1,
		FailedCases: []FailedCase{{Name: "n", Message: "m", Duration: "d"}},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	s := string(data)

	requiredFields := []string{
		`"project_name"`,
		`"timestamp"`,
		`"total_cases"`,
		`"passed_cases"`,
		`"failed_cases"`,
		`"name"`,
		`"message"`,
		`"duration"`,
	}
	for _, f := range requiredFields {
		if !strings.Contains(s, f) {
			t.Errorf("marshaled JSON %q missing required field %s", s, f)
		}
	}
}

// TestFailureContextOMITEmpty verifies that optional FailureContext fields
// marked with omitempty are omitted when empty.
func TestFailureContextOMITEmpty(t *testing.T) {
	cases := []struct {
		name   string
		fc     *FailureContext
		expect string
	}{
		{"empty base_url", &FailureContext{ProjectName: "p", Timestamp: "t"}, `"base_url"`},
		{"empty project_root", &FailureContext{ProjectName: "p", Timestamp: "t"}, `"project_root"`},
		{"empty env_log_path", &FailureContext{ProjectName: "p", Timestamp: "t"}, `"env_log_path"`},
		{"empty reports_dir", &FailureContext{ProjectName: "p", Timestamp: "t"}, `"reports_dir"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := json.Marshal(c.fc)
			if err != nil {
				t.Fatalf("Marshal returned error: %v", err)
			}
			if strings.Contains(string(data), c.expect) {
				t.Errorf("marshaled JSON %q should not contain %q (omitempty)", string(data), c.expect)
			}
		})
	}
}

// TestFailedCaseOMITEmpty verifies that empty SkillName and Error fields are
// omitted from the serialized FailedCase JSON.
func TestFailedCaseOMITEmpty(t *testing.T) {
	fc := &FailureContext{
		ProjectName: "p",
		Timestamp:   "t",
		FailedCases: []FailedCase{
			{Name: "n", Message: "m", Duration: "d"}, // empty SkillName and Error
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	s := string(data)
	if strings.Contains(s, `"skill_name"`) {
		t.Errorf("marshaled JSON %q should not contain empty skill_name (omitempty)", s)
	}
	if strings.Contains(s, `"error"`) {
		t.Errorf("marshaled JSON %q should not contain empty error (omitempty)", s)
	}
}

// TestFailureContextMarshalNilFailedCases verifies that a nil FailedCases slice
// serializes to JSON null (standard Go encoding/json behavior).
func TestFailureContextMarshalNilFailedCases(t *testing.T) {
	fc := &FailureContext{ProjectName: "p", Timestamp: "t"} // FailedCases is nil
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if !strings.Contains(string(data), `"failed_cases":null`) {
		t.Errorf("marshaled JSON %q should contain \"failed_cases\":null for nil slice", string(data))
	}
}

// TestFailureContextEmptyStructJSON verifies that an empty FailureContext
// survives a marshal/unmarshal round-trip with all zero values preserved.
func TestFailureContextEmptyStructJSON(t *testing.T) {
	fc := &FailureContext{}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var decoded FailureContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if decoded.ProjectName != "" {
		t.Errorf("ProjectName = %q, want empty", decoded.ProjectName)
	}
	if decoded.TotalCases != 0 {
		t.Errorf("TotalCases = %d, want 0", decoded.TotalCases)
	}
	if decoded.PassedCases != 0 {
		t.Errorf("PassedCases = %d, want 0", decoded.PassedCases)
	}
	if decoded.FailedCases != nil {
		t.Errorf("FailedCases = %v, want nil", decoded.FailedCases)
	}
}

// TestFailureContextLargePayload verifies serialization integrity with a
// large number of failed cases.
func TestFailureContextLargePayload(t *testing.T) {
	fc := &FailureContext{
		ProjectName: "big-project",
		Timestamp:   "2026-07-17T00:00:00Z",
		TotalCases:  1000,
		PassedCases: 900,
	}
	for i := 0; i < 100; i++ {
		fc.FailedCases = append(fc.FailedCases, FailedCase{
			SkillName: "api",
			Name:      "test-case-" + strconv.Itoa(i),
			Message:   "failure message with some content",
			Error:     "error detail",
			Duration:  "1s",
		})
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var decoded FailureContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(decoded.FailedCases) != 100 {
		t.Errorf("len(FailedCases) = %d, want 100", len(decoded.FailedCases))
	}
	if decoded.TotalCases != 1000 {
		t.Errorf("TotalCases = %d, want 1000", decoded.TotalCases)
	}
	if decoded.FailedCases[0].Name != "test-case-0" {
		t.Errorf("FailedCases[0].Name = %q, want %q", decoded.FailedCases[0].Name, "test-case-0")
	}
	if decoded.FailedCases[99].Name != "test-case-99" {
		t.Errorf("FailedCases[99].Name = %q, want %q", decoded.FailedCases[99].Name, "test-case-99")
	}
}

// ---------------------------------------------------------------------------
// WriteToFile tests
// ---------------------------------------------------------------------------

// TestWriteToFileNilReceiver verifies that calling WriteToFile on a nil
// receiver returns an error mentioning the nil context.
func TestWriteToFileNilReceiver(t *testing.T) {
	var fc *FailureContext
	err := fc.WriteToFile(filepath.Join(t.TempDir(), "f.json"))
	if err == nil {
		t.Fatal("WriteToFile on nil receiver returned nil error")
	}
	if !strings.Contains(err.Error(), "nil failure context") {
		t.Errorf("error = %q, want it to contain 'nil failure context'", err.Error())
	}
}

// TestWriteToFileCreatesParentDir verifies that WriteToFile creates missing
// parent directories in the path.
func TestWriteToFileCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	// Use a nested path where the parent directory does not yet exist.
	nested := filepath.Join(dir, "nested", "deep", "failure.json")
	fc := &FailureContext{ProjectName: "p", Timestamp: "t"}
	if err := fc.WriteToFile(nested); err != nil {
		t.Fatalf("WriteToFile returned error: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("file was not created: %v", err)
	}
}

// TestWriteToFileContent verifies the written file contains indented JSON
// that decodes back to the original FailureContext.
func TestWriteToFileContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "failure.json")
	fc := &FailureContext{
		ProjectName: "tickraft",
		Timestamp:   "2026-07-17T12:00:00Z",
		TotalCases:  3,
		PassedCases: 2,
		FailedCases: []FailedCase{
			{Name: "t1", Message: "fail", Duration: "1s"},
		},
	}
	if err := fc.WriteToFile(path); err != nil {
		t.Fatalf("WriteToFile returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	var decoded FailureContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if decoded.ProjectName != "tickraft" {
		t.Errorf("ProjectName = %q, want %q", decoded.ProjectName, "tickraft")
	}
	if decoded.TotalCases != 3 {
		t.Errorf("TotalCases = %d, want 3", decoded.TotalCases)
	}
	if len(decoded.FailedCases) != 1 {
		t.Fatalf("len(FailedCases) = %d, want 1", len(decoded.FailedCases))
	}
	// Verify the file is indented (MarshalIndent uses two-space indent).
	if !strings.Contains(string(data), "\n  ") {
		t.Errorf("file content %q does not appear to be indented (MarshalIndent)", string(data))
	}
}

// TestWriteToFileOverwriteExisting verifies that WriteToFile overwrites a
// pre-existing file at the target path.
func TestWriteToFileOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "failure.json")
	// Pre-populate the file with stale content.
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale content: %v", err)
	}
	fc := &FailureContext{ProjectName: "fresh", Timestamp: "t"}
	if err := fc.WriteToFile(path); err != nil {
		t.Fatalf("WriteToFile returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	var decoded FailureContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if decoded.ProjectName != "fresh" {
		t.Errorf("ProjectName = %q, want %q (file not overwritten)", decoded.ProjectName, "fresh")
	}
}

// ---------------------------------------------------------------------------
// ReadFromFile tests
// ---------------------------------------------------------------------------

// TestReadFromFileNonExistent verifies that reading a missing file returns an
// error wrapping the read failure.
func TestReadFromFileNonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.json")
	_, err := ReadFromFile(path)
	if err == nil {
		t.Fatal("ReadFromFile on non-existent file returned nil error")
	}
	if !strings.Contains(err.Error(), "read failure context") {
		t.Errorf("error = %q, want it to contain 'read failure context'", err.Error())
	}
}

// TestReadFromFileInvalidJSON verifies that malformed JSON produces an
// unmarshal error.
func TestReadFromFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write bad content: %v", err)
	}
	_, err := ReadFromFile(path)
	if err == nil {
		t.Fatal("ReadFromFile on invalid JSON returned nil error")
	}
	if !strings.Contains(err.Error(), "unmarshal failure context") {
		t.Errorf("error = %q, want it to contain 'unmarshal failure context'", err.Error())
	}
}

// TestReadFromFileEmptyFile verifies that an empty file produces an error
// (empty input is not valid JSON).
func TestReadFromFileEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write empty content: %v", err)
	}
	_, err := ReadFromFile(path)
	if err == nil {
		t.Fatal("ReadFromFile on empty file returned nil error")
	}
}

// ---------------------------------------------------------------------------
// WriteToFile + ReadFromFile round-trip tests
// ---------------------------------------------------------------------------

// TestWriteReadRoundTrip verifies the full file I/O round-trip preserves all
// fields, including nested failed cases.
func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "failure.json")
	original := &FailureContext{
		ProjectName: "tickraft",
		BaseURL:     "http://localhost:9090",
		Timestamp:   "2026-07-17T08:30:00Z",
		ProjectRoot: "/srv/tickraft",
		EnvLogPath:  "/var/log/tickraft.log",
		ReportsDir:  "/srv/reports",
		TotalCases:  5,
		PassedCases: 3,
		FailedCases: []FailedCase{
			{SkillName: "api", Name: "case-1", Message: "bad status", Error: "500 internal", Duration: "200ms"},
			{SkillName: "ui", Name: "case-2", Message: "timeout", Error: "context deadline exceeded", Duration: "5s"},
		},
	}
	if err := original.WriteToFile(path); err != nil {
		t.Fatalf("WriteToFile returned error: %v", err)
	}
	loaded, err := ReadFromFile(path)
	if err != nil {
		t.Fatalf("ReadFromFile returned error: %v", err)
	}
	if loaded.ProjectName != original.ProjectName {
		t.Errorf("ProjectName = %q, want %q", loaded.ProjectName, original.ProjectName)
	}
	if loaded.BaseURL != original.BaseURL {
		t.Errorf("BaseURL = %q, want %q", loaded.BaseURL, original.BaseURL)
	}
	if loaded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %q, want %q", loaded.Timestamp, original.Timestamp)
	}
	if loaded.ProjectRoot != original.ProjectRoot {
		t.Errorf("ProjectRoot = %q, want %q", loaded.ProjectRoot, original.ProjectRoot)
	}
	if loaded.EnvLogPath != original.EnvLogPath {
		t.Errorf("EnvLogPath = %q, want %q", loaded.EnvLogPath, original.EnvLogPath)
	}
	if loaded.ReportsDir != original.ReportsDir {
		t.Errorf("ReportsDir = %q, want %q", loaded.ReportsDir, original.ReportsDir)
	}
	if loaded.TotalCases != original.TotalCases {
		t.Errorf("TotalCases = %d, want %d", loaded.TotalCases, original.TotalCases)
	}
	if loaded.PassedCases != original.PassedCases {
		t.Errorf("PassedCases = %d, want %d", loaded.PassedCases, original.PassedCases)
	}
	if len(loaded.FailedCases) != len(original.FailedCases) {
		t.Fatalf("len(FailedCases) = %d, want %d", len(loaded.FailedCases), len(original.FailedCases))
	}
	for i, want := range original.FailedCases {
		got := loaded.FailedCases[i]
		if got != want {
			t.Errorf("FailedCases[%d] = %+v, want %+v", i, got, want)
		}
	}
}

// TestWriteReadRoundTripEmpty verifies the round-trip behavior for an empty
// FailureContext (all zero values).
func TestWriteReadRoundTripEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-fc.json")
	original := &FailureContext{} // all zero values
	if err := original.WriteToFile(path); err != nil {
		t.Fatalf("WriteToFile returned error: %v", err)
	}
	loaded, err := ReadFromFile(path)
	if err != nil {
		t.Fatalf("ReadFromFile returned error: %v", err)
	}
	if loaded.ProjectName != "" {
		t.Errorf("ProjectName = %q, want empty", loaded.ProjectName)
	}
	if loaded.TotalCases != 0 {
		t.Errorf("TotalCases = %d, want 0", loaded.TotalCases)
	}
	if loaded.PassedCases != 0 {
		t.Errorf("PassedCases = %d, want 0", loaded.PassedCases)
	}
	if loaded.FailedCases != nil {
		t.Errorf("FailedCases = %v, want nil", loaded.FailedCases)
	}
}
