package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tickraft/taichi/pkg/skill/builtin"
)

// TestBuiltinSkillsReturnsFive verifies that builtin.BuiltinSkills returns
// exactly 5 skills and that the "grpc" skill is present. This guards against
// a regression of the original bug where the MCP copy omitted the grpc skill,
// causing taichi_run to skip gRPC tests.
func TestBuiltinSkillsReturnsFive(t *testing.T) {
	skills := builtin.Skills()
	if len(skills) != 5 {
		t.Fatalf("BuiltinSkills count = %d, want 5", len(skills))
	}
	found := false
	for _, s := range skills {
		if s.Name() == "grpc" {
			found = true
			break
		}
	}
	if !found {
		t.Error("BuiltinSkills does not include the grpc skill (regression of the original MCP bug)")
	}
}

// writeMinimalRunnableConfig writes a minimal config that the orchestrator can
// execute end-to-end without starting a real service: a project with no env
// and a single api skill with zero test cases. The report output is redirected
// into a temp directory.
func writeMinimalRunnableConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := `projects:
  - name: unit-test
skills:
  - name: api
    kind: api
    enabled: true
    raw:
      timeout: 1s
report:
  output_dir: ` + filepath.Join(dir, "reports") + `
`
	path := filepath.Join(dir, "taichi.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

// TestRunCommand_MissingConfig verifies that the run command returns an error
// when the config path does not exist.
func TestRunCommand_MissingConfig(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"run", "-c", filepath.Join(t.TempDir(), "nonexistent.yaml")})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent config, got nil")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Errorf("error = %q, want it to contain 'load config'", err.Error())
	}
}

// TestRunCommand_ValidConfig verifies that the run command succeeds with a
// minimal valid config (no env, zero-case skill) and prints the project name
// in the run summary.
func TestRunCommand_ValidConfig(t *testing.T) {
	cfgPath := writeMinimalRunnableConfig(t)
	root := newRootCmd()
	root.SetArgs([]string{"run", "-c", cfgPath})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("run command returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "unit-test") {
		t.Errorf("output does not contain project name 'unit-test':\n%s", output)
	}
}

// TestRunCommand_Help verifies that the run command help output contains the
// command name and is non-empty.
func TestRunCommand_Help(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"run", "--help"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("run --help returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "run") {
		t.Errorf("help output does not contain 'run':\n%s", output)
	}
	if !strings.Contains(output, "--project") {
		t.Errorf("help output does not contain '--project' flag:\n%s", output)
	}
}
