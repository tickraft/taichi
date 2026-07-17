package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolveSampleConfig resolves the absolute path to the sample config file
// shipped with the taichi repo.
func resolveSampleConfig(t *testing.T) string {
	t.Helper()
	rel := filepath.Join("..", "..", "configs", "taichi.yaml")
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatalf("resolve sample config path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("sample config not found at %s: %v", abs, err)
	}
	return abs
}

// TestListCommand_ValidConfig verifies that the list command with the sample
// config outputs the project name and skill names.
func TestListCommand_ValidConfig(t *testing.T) {
	cfgPath := resolveSampleConfig(t)
	root := newRootCmd()
	root.SetArgs([]string{"list", "-c", cfgPath})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("list command returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "tickraft") {
		t.Errorf("list output does not contain project name 'tickraft':\n%s", output)
	}
	if !strings.Contains(output, "api") {
		t.Errorf("list output does not contain skill name 'api':\n%s", output)
	}
}

// TestListCommand_MissingConfig verifies that the list command returns an
// error when the config path does not exist.
func TestListCommand_MissingConfig(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"list", "-c", filepath.Join(t.TempDir(), "nonexistent.yaml")})
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

// TestListCommand_Help verifies that the list command help output is non-empty
// and contains the command name.
func TestListCommand_Help(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"list", "--help"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("list --help returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "list") {
		t.Errorf("help output does not contain 'list':\n%s", output)
	}
}
