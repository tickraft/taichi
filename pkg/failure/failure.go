// Package failure defines the structured failure context exchange contract between taichi and the AI Agent.
//
// When a test run has failing cases, taichi wraps the failure information into a Context
// and writes it to a JSON file for the AI Agent to consume. The AI Agent reads it, analyzes the
// failure cause, generates a fix, and then triggers a regression test, forming a fully automated
// test→fix→regression closed loop.
package failure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
)

// Context is the structured failure context, serving as the taichi ↔ AI Agent information exchange contract.
type Context struct {
	// ProjectName is the name of the project under test.
	ProjectName string `json:"project_name"`
	// BaseURL is the base URL of the service under test.
	BaseURL string `json:"base_url,omitempty"`
	// Timestamp is the time the failure context was generated (RFC3339).
	Timestamp string `json:"timestamp"`
	// ProjectRoot is the absolute path of the project under test's root directory.
	ProjectRoot string `json:"project_root,omitempty"`
	// EnvLogPath is the env (service) log file path.
	EnvLogPath string `json:"env_log_path,omitempty"`
	// ReportsDir is the report output directory.
	ReportsDir string `json:"reports_dir,omitempty"`
	// TotalCases is the total number of cases in this run.
	TotalCases int `json:"total_cases"`
	// PassedCases is the number of cases that passed.
	PassedCases int `json:"passed_cases"`
	// FailedCases is the list of failed case details.
	FailedCases []FailedCase `json:"failed_cases"`
}

// FailedCase describes the details of a single failed case.
type FailedCase struct {
	// SkillName is the name of the skill that produced this case (e.g. "api", "ui").
	SkillName string `json:"skill_name,omitempty"`
	// Name is the case identifier.
	Name string `json:"name"`
	// Message is a human-readable description of the failure.
	Message string `json:"message"`
	// Error is the string form of the underlying error (if any).
	Error string `json:"error,omitempty"`
	// Duration is the case execution duration.
	Duration string `json:"duration"`
}

// FromResults extracts the failure context from a snapshot of test results and skill results.
// skillResults is used to associate each result with its owning skill (matched by result record order against skill execution order).
// When an exact association cannot be made, SkillName is left empty.
func FromResults(
	projectName, baseURL, projectRoot, envLogPath, reportsDir string,
	results []framework.TestResult,
	skillNames []string,
) *Context {
	fc := &Context{
		ProjectName: projectName,
		BaseURL:     baseURL,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		ProjectRoot: projectRoot,
		EnvLogPath:  envLogPath,
		ReportsDir:  reportsDir,
	}

	for _, r := range results {
		fc.TotalCases++
		if r.Skipped {
			continue
		}
		if r.Passed {
			fc.PassedCases++
			continue
		}
		fc.FailedCases = append(fc.FailedCases, FailedCase{
			Name:     r.Name,
			Message:  r.Message,
			Error:    errorString(r.Error),
			Duration: r.Duration.String(),
		})
	}
	return fc
}

// HasFailures returns whether there are any failed cases.
func (fc *Context) HasFailures() bool {
	return fc != nil && len(fc.FailedCases) > 0
}

// WriteToFile writes the failure context as formatted JSON to path.
// The parent directory is created automatically when it does not exist.
func (fc *Context) WriteToFile(path string) error {
	if fc == nil {
		return fmt.Errorf("nil failure context")
	}
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal failure context: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create failure context dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write failure context: %w", err)
	}
	return nil
}

// ReadFromFile reads the JSON-formatted failure context from path.
func ReadFromFile(path string) (*Context, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read failure context: %w", err)
	}
	var fc Context
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("unmarshal failure context: %w", err)
	}
	return &fc, nil
}

// errorString safely converts an error to a string.
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
