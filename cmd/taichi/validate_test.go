package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tickraft/taichi/pkg/i18n"
)

// validateWriteConfig writes content to a taichi.yaml file inside a temp dir
// and returns its path. The temp dir keeps the file scoped to the test lifetime.
func validateWriteConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "taichi.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

// runValidateCmd builds a validate command, captures its stdout/stderr, and
// executes it against the given config path. Returns the captured stdout and
// the execution error (nil on success).
func runValidateCmd(t *testing.T, configPath string) (string, error) {
	t.Helper()
	gf := &globalFlags{configPath: configPath}
	cmd := newValidateCmd(gf)

	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	// Force en-US locale so the success message is deterministic across hosts.
	i18n.SetLocale(i18n.EnUS)

	err := cmd.Execute()
	return out.String(), err
}

// TestValidateCmd_OK verifies that a valid config prints the success message
// and exits without error.
func TestValidateCmd_OK(t *testing.T) {
	const yamlContent = `
projects:
  - name: tickraft
    env: tickraft-backend
    skills: [api]
envs:
  tickraft-backend:
    kind: backend.go
    binary: bin/tickraft
    build: go build -o bin/tickraft ./cmd/tickraft
    config_path: configs/config.yaml
    config_flag: --config
    addr_flag: --addr
    health_path: /api/v1/health
skills:
  - name: api
    enabled: true
    priority: 0
    kind: api
    raw:
      timeout: 5s
report:
  suite_name: taichi-tests
  output_dir: reports
  formats: [json, junit, summary]
`
	path := validateWriteConfig(t, yamlContent)

	out, err := runValidateCmd(t, path)
	if err != nil {
		t.Fatalf("expected nil error, got: %v (stdout: %q)", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "valid") {
		t.Errorf("expected stdout to contain 'valid', got: %q", out)
	}
}

// TestValidateCmd_EmptyFile verifies that an empty config file (which loads as
// an empty Config and passes validation) still reports success.
func TestValidateCmd_EmptyFile(t *testing.T) {
	path := validateWriteConfig(t, "")

	out, err := runValidateCmd(t, path)
	if err != nil {
		t.Fatalf("expected nil error for empty config, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "valid") {
		t.Errorf("expected stdout to contain 'valid', got: %q", out)
	}
}

// TestValidateCmd_InvalidYAML verifies that malformed YAML returns an error.
func TestValidateCmd_InvalidYAML(t *testing.T) {
	const malformedYAML = `
projects:
  - name: tickraft
    env: backend
    broken: [unclosed bracket
`
	path := validateWriteConfig(t, malformedYAML)

	_, err := runValidateCmd(t, path)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

// TestValidateCmd_ProjectEnvNotFound verifies that a config referencing an
// undefined env returns a validation error.
func TestValidateCmd_ProjectEnvNotFound(t *testing.T) {
	const yamlContent = `
projects:
  - name: tickraft
    env: missing-env
`
	path := validateWriteConfig(t, yamlContent)

	_, err := runValidateCmd(t, path)
	if err == nil {
		t.Fatal("expected error for project referencing undefined env, got nil")
	}
}

// TestValidateCmd_NonExistentPath verifies that a non-existent config path
// returns an error.
func TestValidateCmd_NonExistentPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	_, err := runValidateCmd(t, path)
	if err == nil {
		t.Fatal("expected error for non-existent config path, got nil")
	}
}
