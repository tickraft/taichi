package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/skill"
)

// recordingLogger is a skill.Logger implementation that records formatted log messages.
type recordingLogger struct {
	mu       sync.Mutex
	messages []string
}

// Infof implements skill.Logger.
func (l *recordingLogger) Infof(format string, args ...any) {
	l.append(format, args...)
}

// Warnf implements skill.Logger.
func (l *recordingLogger) Warnf(format string, args ...any) {
	l.append(format, args...)
}

// Errorf implements skill.Logger.
func (l *recordingLogger) Errorf(format string, args ...any) {
	l.append(format, args...)
}

func (l *recordingLogger) append(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, fmt.Sprintf(format, args...))
}

// Messages returns a copy of the recorded log messages.
func (l *recordingLogger) Messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.messages))
	copy(out, l.messages)
	return out
}

// newTestContext builds a SkillContext backed by a fresh reporter and assertion engine.
func newTestContext(t *testing.T, baseURL string, logger skill.Logger) *skill.SkillContext {
	t.Helper()
	if logger == nil {
		logger = skill.NoOpLogger{}
	}
	return &skill.SkillContext{
		Ctx:         context.Background(),
		ProjectName: "test-project",
		BaseURL:     baseURL,
		Asserts:     framework.NewAssertionEngine(),
		Reporter:    framework.NewTestReporter(),
		ReportsDir:  t.TempDir(),
		Logger:      logger,
		Extra:       make(map[string]any),
	}
}

// writeScript writes a /bin/sh script with the given body to a temp dir,
// makes it executable, and returns the absolute path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "plugin.sh")
	content := "#!/bin/sh\n" + body
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatalf("chmod script: %v", err)
	}
	return p
}

// configureAndSetup wires the skill with the given raw config and invokes Setup.
func configureAndSetup(t *testing.T, s *Skill, raw map[string]any) {
	t.Helper()
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := s.Setup(nil); err != nil {
		t.Fatalf("Setup: %v", err)
	}
}

// runWithScript configures the skill to invoke the given script path and runs it
// against a fresh context.
func runWithScript(t *testing.T, scriptPath string, logger skill.Logger, extraRaw ...map[string]any) skill.SkillResult {
	t.Helper()
	raw := map[string]any{
		"command": scriptPath,
		"timeout": "10s",
	}
	for _, r := range extraRaw {
		for k, v := range r {
			raw[k] = v
		}
	}
	s := New("myplugin")
	configureAndSetup(t, s, raw)
	ctx := newTestContext(t, "http://example.test", logger)
	return s.Run(ctx)
}

// TestName verifies the skill identifier is the name supplied to New.
func TestName(t *testing.T) {
	s := New("myplugin")
	if got := s.Name(); got != "myplugin" {
		t.Errorf("Name() = %q, want \"myplugin\"", got)
	}
}

// TestKind verifies the skill category.
func TestKind(t *testing.T) {
	s := New("myplugin")
	if got := s.Kind(); got != skill.KindPlugin {
		t.Errorf("Kind() = %q, want %q", got, skill.KindPlugin)
	}
}

// TestPriority verifies the configured priority.
func TestPriority(t *testing.T) {
	s := New("myplugin")
	if got := s.Priority(); got != skill.PriorityNormal {
		t.Errorf("Priority() = %v, want %v", got, skill.PriorityNormal)
	}
}

// TestConfigureNil verifies that a nil raw map produces an error.
func TestConfigureNil(t *testing.T) {
	s := New("myplugin")
	if err := s.Configure(skill.SkillConfig{}); err == nil {
		t.Errorf("Configure with nil raw: expected error, got nil")
	}
}

// TestConfigureNoCommand verifies that a raw map without "command" produces an error.
func TestConfigureNoCommand(t *testing.T) {
	s := New("myplugin")
	raw := map[string]any{"args": []any{"-x"}}
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err == nil {
		t.Errorf("Configure without command: expected error, got nil")
	}
}

// TestConfigureEmptyCommand verifies that an empty command produces an error.
func TestConfigureEmptyCommand(t *testing.T) {
	s := New("myplugin")
	raw := map[string]any{"command": ""}
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err == nil {
		t.Errorf("Configure with empty command: expected error, got nil")
	}
}

// TestConfigureValid verifies all control fields are parsed and that the remaining
// business config excludes the control fields.
func TestConfigureValid(t *testing.T) {
	s := New("myplugin")
	raw := map[string]any{
		"command":   "./script",
		"args":      []any{"-v"},
		"env":       []any{"K=V"},
		"workdir":   ".",
		"timeout":   "10s",
		"endpoints": []any{"/api/v1/custom"},
	}
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if s.command != "./script" {
		t.Errorf("command = %q, want ./script", s.command)
	}
	if len(s.args) != 1 || s.args[0] != "-v" {
		t.Errorf("args = %v, want [-v]", s.args)
	}
	if len(s.env) != 1 || s.env[0] != "K=V" {
		t.Errorf("env = %v, want [K=V]", s.env)
	}
	if s.workdir != "." {
		t.Errorf("workdir = %q, want .", s.workdir)
	}
	if s.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", s.timeout)
	}
	// Control fields must not appear in the business config.
	for _, key := range []string{"command", "args", "env", "workdir", "timeout"} {
		if _, ok := s.config[key]; ok {
			t.Errorf("config should not contain control field %q", key)
		}
	}
	// Custom business fields must be preserved.
	if v, ok := s.config["endpoints"]; !ok {
		t.Errorf("config should contain custom field \"endpoints\"")
	} else if _, ok := v.([]any); !ok {
		t.Errorf("endpoints has unexpected type %T", v)
	}
}

// TestConfigureExtractsConfig verifies that custom fields are extracted into s.config
// while control fields are removed.
func TestConfigureExtractsConfig(t *testing.T) {
	s := New("myplugin")
	raw := map[string]any{
		"command":   "./script",
		"endpoints": []any{"/a", "/b"},
		"retries":   3,
	}
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, ok := s.config["command"]; ok {
		t.Errorf("config must not contain \"command\"")
	}
	if v, ok := s.config["endpoints"]; !ok {
		t.Errorf("config should contain \"endpoints\"")
	} else if arr, ok := v.([]any); !ok || len(arr) != 2 {
		t.Errorf("endpoints = %v, want 2-element slice", v)
	}
	if v, ok := s.config["retries"]; !ok {
		t.Errorf("config should contain \"retries\"")
	} else if v != 3 {
		t.Errorf("retries = %v, want 3", v)
	}
}

// TestSetup verifies Setup returns nil.
func TestSetup(t *testing.T) {
	s := New("myplugin")
	if err := s.Configure(skill.SkillConfig{Raw: map[string]any{"command": "./x"}}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := s.Setup(nil); err != nil {
		t.Errorf("Setup returned non-nil error: %v", err)
	}
}

// TestTeardown verifies Teardown returns nil.
func TestTeardown(t *testing.T) {
	s := New("myplugin")
	if err := s.Teardown(nil); err != nil {
		t.Errorf("Teardown returned non-nil error: %v", err)
	}
}

// TestRunSuccess verifies that a plugin returning one passing case produces Passed=1.
func TestRunSuccess(t *testing.T) {
	script := writeScript(t, `printf '%s' '{"cases":[{"name":"ok","passed":true,"message":"ok","duration_ms":1}]}'
`)
	res := runWithScript(t, script, nil)
	if res.Error != nil {
		t.Fatalf("Run error: %v", res.Error)
	}
	if res.Summary.Total != 1 || res.Summary.Passed != 1 {
		t.Errorf("summary = %+v, want Total=1 Passed=1", res.Summary)
	}
}

// TestRunPluginOutputError verifies that an output with a non-empty Error field surfaces as a skill error.
func TestRunPluginOutputError(t *testing.T) {
	script := writeScript(t, `printf '%s' '{"error":"boom"}'
`)
	res := runWithScript(t, script, nil)
	if res.Error == nil {
		t.Errorf("expected error, got nil; summary=%+v", res.Summary)
	}
	if res.Error != nil && !strings.Contains(res.Error.Error(), "boom") {
		t.Errorf("error = %v, want substring \"boom\"", res.Error)
	}
}

// TestRunPluginFailureCase verifies that a failed plugin case counts as Failed=1.
func TestRunPluginFailureCase(t *testing.T) {
	script := writeScript(t, `printf '%s' '{"cases":[{"name":"fail","passed":false,"message":"nope","error":"detail"}]}'
`)
	res := runWithScript(t, script, nil)
	if res.Error != nil {
		t.Fatalf("Run error: %v", res.Error)
	}
	if res.Summary.Total != 1 || res.Summary.Failed != 1 {
		t.Errorf("summary = %+v, want Total=1 Failed=1", res.Summary)
	}
}

// TestRunPluginSkippedCase verifies that a skipped plugin case counts as Skipped=1.
func TestRunPluginSkippedCase(t *testing.T) {
	script := writeScript(t, `printf '%s' '{"cases":[{"name":"skip","passed":false,"skipped":true,"message":"n/a"}]}'
`)
	res := runWithScript(t, script, nil)
	if res.Error != nil {
		t.Fatalf("Run error: %v", res.Error)
	}
	if res.Summary.Total != 1 || res.Summary.Skipped != 1 {
		t.Errorf("summary = %+v, want Total=1 Skipped=1", res.Summary)
	}
}

// TestRunPluginInvalidJSON verifies that non-JSON stdout surfaces as a skill error.
func TestRunPluginInvalidJSON(t *testing.T) {
	script := writeScript(t, `printf '%s' 'not json'
`)
	res := runWithScript(t, script, nil)
	if res.Error == nil {
		t.Errorf("expected error for invalid JSON, got nil")
	}
}

// TestRunPluginExitNonZero verifies that a non-zero exit surfaces as a skill error.
func TestRunPluginExitNonZero(t *testing.T) {
	script := writeScript(t, `printf '%s' '{"cases":[]}'
exit 1
`)
	res := runWithScript(t, script, nil)
	if res.Error == nil {
		t.Errorf("expected error for non-zero exit, got nil")
	}
}

// TestRunPluginTimeout verifies that a plugin exceeding the configured timeout surfaces
// an error whose message contains "timeout".
func TestRunPluginTimeout(t *testing.T) {
	script := writeScript(t, `sleep 30
`)
	s := New("myplugin")
	configureAndSetup(t, s, map[string]any{
		"command": script,
		"timeout": "100ms",
	})
	ctx := newTestContext(t, "http://example.test", nil)
	res := s.Run(ctx)
	if res.Error == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if !strings.Contains(res.Error.Error(), "timeout") {
		t.Errorf("error = %v, want substring \"timeout\"", res.Error)
	}
}

// TestRunPluginStderrForwarded verifies that stderr output is forwarded to the logger.
func TestRunPluginStderrForwarded(t *testing.T) {
	script := writeScript(t, `echo "stderr line 1" >&2
echo "stderr line 2" >&2
printf '%s' '{"cases":[]}'
`)
	logger := &recordingLogger{}
	res := runWithScript(t, script, logger)
	if res.Error != nil {
		t.Fatalf("Run error: %v", res.Error)
	}
	msgs := logger.Messages()
	// Expect at least one message containing the stderr content.
	var foundLine bool
	for _, m := range msgs {
		if strings.Contains(m, "stderr line 1") || strings.Contains(m, "stderr line 2") {
			foundLine = true
			break
		}
	}
	if !foundLine {
		t.Errorf("stderr not forwarded to logger; messages=%v", msgs)
	}
	// Each forwarded line should be tagged with the plugin name.
	var foundTag bool
	for _, m := range msgs {
		if strings.Contains(m, "[plugin:myplugin]") {
			foundTag = true
			break
		}
	}
	if !foundTag {
		t.Errorf("stderr lines should be tagged with [plugin:myplugin]; messages=%v", msgs)
	}
}

// TestRunPluginMultipleCases verifies a mixed case set produces the expected summary.
func TestRunPluginMultipleCases(t *testing.T) {
	script := writeScript(t, `printf '%s' '{"cases":[{"name":"ok","passed":true,"message":"ok"},{"name":"fail","passed":false,"message":"nope"},{"name":"skip","passed":false,"skipped":true,"message":"n/a"}]}'
`)
	res := runWithScript(t, script, nil)
	if res.Error != nil {
		t.Fatalf("Run error: %v", res.Error)
	}
	if res.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3; summary=%+v", res.Summary.Total, res.Summary)
	}
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1; summary=%+v", res.Summary.Passed, res.Summary)
	}
	if res.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1; summary=%+v", res.Summary.Failed, res.Summary)
	}
	if res.Summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1; summary=%+v", res.Summary.Skipped, res.Summary)
	}
}

// TestRunPluginEmptyCaseName verifies that a case with an empty name falls back to the skill name.
func TestRunPluginEmptyCaseName(t *testing.T) {
	script := writeScript(t, `printf '%s' '{"cases":[{"name":"","passed":true,"message":"ok"}]}'
`)
	s := New("myplugin")
	configureAndSetup(t, s, map[string]any{
		"command": script,
		"timeout": "10s",
	})
	ctx := newTestContext(t, "http://example.test", nil)
	res := s.Run(ctx)
	if res.Error != nil {
		t.Fatalf("Run error: %v", res.Error)
	}
	if res.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1; summary=%+v", res.Summary.Passed, res.Summary)
	}
	results := ctx.Reporter.Snapshot()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "myplugin" {
		t.Errorf("case name = %q, want \"myplugin\" (skill name fallback)", results[0].Name)
	}
}
