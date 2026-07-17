package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestVersionCommandOutput verifies that the version command outputs version
// information including the binary name "taichi" and the Version string.
// The output format is "taichi <version> (go <runtime> <os>/<arch>)".
func TestVersionCommandOutput(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"version"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("version command returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "taichi") {
		t.Errorf("version output does not contain 'taichi':\n%s", output)
	}
	if !strings.Contains(output, Version) {
		t.Errorf("version output does not contain Version=%q:\n%s", Version, output)
	}
}

// TestVersionCommand_Help verifies that the version command help output is
// non-empty and contains the command name.
func TestVersionCommand_Help(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"version", "--help"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("version --help returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "version") {
		t.Errorf("help output does not contain 'version':\n%s", output)
	}
}
