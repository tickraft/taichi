// Package report provides the test report extension point and multi-format output.
//
// The report extension point allows third parties to register custom report
// formats (e.g. HTML, Slack webhook, Feishu card).
// Three formats are built in: JSON, JUnit XML, and Summary (human-readable table),
// implemented by framework.TestReporter.
package report

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/tickraft/taichi/pkg/framework"
)

// Format is the report format identifier.
type Format string

const (
	// FormatJSON outputs formatted JSON.
	FormatJSON Format = "json"
	// FormatJUnit outputs JUnit XML.
	FormatJUnit Format = "junit"
	// FormatSummary outputs a human-readable table.
	FormatSummary Format = "summary"
)

// Writer is the output interface for custom report formats.
// Implementations receive a snapshot from TestReporter (via Summary and Snapshot)
// and format the output to w themselves.
type Writer interface {
	// Format returns the format identifier of this Writer.
	Format() Format
	// Write writes the report to w.
	Write(w io.Writer, summary framework.TestSummary, results []framework.TestResult) error
}

// Registry maintains the registered report Writers.
// The three built-in formats are implemented natively by framework.TestReporter
// and do not go through the Writer interface; custom formats are registered into
// the Registry via the Writer interface.
type Registry struct {
	mu      sync.RWMutex
	writers map[Format]Writer
}

// NewRegistry returns an empty Registry (without built-in formats; built-in
// formats go through a special branch in Generate).
func NewRegistry() *Registry {
	return &Registry{writers: make(map[Format]Writer)}
}

// Register registers or overwrites a Writer. A nil Writer is ignored.
func (r *Registry) Register(w Writer) {
	if w == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writers[w.Format()] = w
}

// Get returns the Writer for the given format. Returns an error if not registered.
func (r *Registry) Get(f Format) (Writer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.writers[f]
	if !ok {
		return nil, fmt.Errorf("report writer %q not registered", f)
	}
	return w, nil
}

// Formats returns all registered custom formats (excluding built-in formats).
func (r *Registry) Formats() []Format {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Format, 0, len(r.writers))
	for f := range r.writers {
		out = append(out, f)
	}
	return out
}

// Generate generates reports for the given formats and paths.
// When formats is empty, [FormatJSON, FormatJUnit, FormatSummary] is used.
// pathFor receives a format and returns the output path (e.g. "reports/result.json");
// returning an empty string skips that format.
//
// Built-in formats (json/junit/summary) are implemented natively by
// framework.TestReporter; other formats go through the custom Writer registered
// in the Registry.
func Generate(reporter *framework.TestReporter, registry *Registry, formats []Format, pathFor func(Format) string) error {
	if len(formats) == 0 {
		formats = []Format{FormatJSON, FormatJUnit, FormatSummary}
	}
	summary := reporter.Summary()
	results := reporter.Snapshot()
	for _, f := range formats {
		path := pathFor(f)
		switch f {
		case FormatJSON:
			if path == "" {
				continue
			}
			if err := reporter.WriteJSON(path); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
		case FormatJUnit:
			if path == "" {
				continue
			}
			if err := reporter.WriteJUnitXML(path); err != nil {
				return fmt.Errorf("write junit: %w", err)
			}
		case FormatSummary:
			if path == "" {
				// Default output to stdout.
				reporter.PrintSummary(os.Stdout)
				continue
			}
			file, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("create summary file: %w", err)
			}
			reporter.PrintSummary(file)
			_ = file.Close()
		default:
			if registry == nil {
				continue
			}
			w, err := registry.Get(f)
			if err != nil {
				return err
			}
			if path == "" {
				if err := w.Write(os.Stdout, summary, results); err != nil {
					return fmt.Errorf("write %s: %w", f, err)
				}
				continue
			}
			file, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("create %s file: %w", f, err)
			}
			if err := w.Write(file, summary, results); err != nil {
				_ = file.Close()
				return fmt.Errorf("write %s: %w", f, err)
			}
			_ = file.Close()
		}
	}
	return nil
}
