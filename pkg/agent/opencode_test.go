package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/failure"
)

// ---------------------------------------------------------------------------
// OpenCodeDetect tests
// ---------------------------------------------------------------------------

// TestOpenCodeDetect verifies detection of the opencode binary by command name.
func TestOpenCodeDetect(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{"opencode", true},
		{"./bin/opencode", true},
		{"/usr/local/bin/opencode", true},
		{"opencode.exe", true},
		{"C:\\Tools\\opencode.exe", true},
		{"trae", false},
		{"python3", false},
		{"", false},
		{"agent.sh", false},
		// Names that contain "opencode" as a substring but are not the binary.
		{"opencode-wrapper", false},
		{"myopencode", false},
	}
	for _, c := range cases {
		if got := OpenCodeDetect(c.command); got != c.want {
			t.Errorf("OpenCodeDetect(%q) = %v, want %v", c.command, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// OpenCodeInvoker.Name tests
// ---------------------------------------------------------------------------

// TestOpenCodeInvokerName verifies the Name method returns the expected format.
func TestOpenCodeInvokerName(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{"opencode", "opencode(opencode)"},
		{"", "opencode(opencode)"}, // empty defaults to "opencode"
		{"/usr/local/bin/opencode", "opencode(/usr/local/bin/opencode)"},
		{"./bin/opencode", "opencode(./bin/opencode)"},
	}
	for _, c := range cases {
		invoker := &OpenCodeInvoker{Command: c.command}
		if got := invoker.Name(); got != c.want {
			t.Errorf("Name() with command %q = %q, want %q", c.command, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// OpenCodeInvoker.AnalyzeAndFix tests
// ---------------------------------------------------------------------------

// TestOpenCodeInvokerNilFailureContext verifies that a nil failure context
// produces an error.
func TestOpenCodeInvokerNilFailureContext(t *testing.T) {
	invoker := &OpenCodeInvoker{Command: "opencode"}
	_, err := invoker.AnalyzeAndFix(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error for nil failure context, got nil")
	}
	if !strings.Contains(err.Error(), "nil failure context") {
		t.Errorf("error = %q, want it to contain 'nil failure context'", err.Error())
	}
}

// TestOpenCodeInvokerEmptyProjectRoot verifies that an empty project root
// (when WorkDir is also unset) produces an error.
func TestOpenCodeInvokerEmptyProjectRoot(t *testing.T) {
	fc := newFailureContext()
	fc.ProjectRoot = ""
	invoker := &OpenCodeInvoker{Command: "opencode", WorkDir: ""}
	_, err := invoker.AnalyzeAndFix(context.Background(), fc)
	if err == nil {
		t.Fatalf("expected error for empty project root, got nil")
	}
	if !strings.Contains(err.Error(), "project root is empty") {
		t.Errorf("error = %q, want it to contain 'project root is empty'", err.Error())
	}
}

// TestOpenCodeInvokerBinaryNotFound verifies that a non-existent binary
// produces a clear error.
func TestOpenCodeInvokerBinaryNotFound(t *testing.T) {
	dir := t.TempDir()
	fc := newFailureContext()
	fc.ProjectRoot = dir
	invoker := &OpenCodeInvoker{Command: "this-opencode-binary-does-not-exist-12345"}
	_, err := invoker.AnalyzeAndFix(context.Background(), fc)
	if err == nil {
		t.Fatalf("expected error for missing binary, got nil")
	}
	if !strings.Contains(err.Error(), "not found in PATH") {
		t.Errorf("error = %q, want it to contain 'not found in PATH'", err.Error())
	}
}

// TestOpenCodeInvokerRunFailure verifies that a failing opencode run produces
// an error. We simulate this with a fake binary that exits non-zero.
func TestOpenCodeInvokerRunFailure(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create a fake "opencode" script that exits with code 1.
	fakeBin := writeScript(t, `echo "fake opencode error" >&2
exit 1
`)
	fc := newFailureContext()
	fc.ProjectRoot = dir
	invoker := &OpenCodeInvoker{Command: fakeBin, Timeout: 10 * time.Second}
	_, err := invoker.AnalyzeAndFix(context.Background(), fc)
	if err == nil {
		t.Fatalf("expected error for failing opencode run, got nil")
	}
	if !strings.Contains(err.Error(), "opencode run failed") {
		t.Errorf("error = %q, want it to contain 'opencode run failed'", err.Error())
	}
}

// TestOpenCodeInvokerSuccessWithGit verifies that when opencode (simulated by
// a script that modifies a file) runs successfully, the modified files are
// detected via git status and returned in the FixResult.
func TestOpenCodeInvokerSuccessWithGit(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create an initial source file and commit it so git tracks it.
	targetPath := filepath.Join(dir, "main.go")
	writeFile(t, targetPath, "package main\n\nfunc main() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	// Create a fake "opencode" script that modifies the source file.
	// The script receives the prompt as the last arg but ignores it.
	fakeBin := writeScript(t, `cat > /dev/null 2>&1
# Simulate opencode modifying a source file.
cat > "`+targetPath+`" <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("fixed")
}
EOF
`)
	fc := newFailureContext()
	fc.ProjectRoot = dir
	invoker := &OpenCodeInvoker{Command: fakeBin, Timeout: 10 * time.Second}
	result, err := invoker.AnalyzeAndFix(context.Background(), fc)
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true; message=%q", result.Message)
	}
	if result.Mode != FixModeDirect {
		t.Errorf("Mode = %q, want %q", result.Mode, FixModeDirect)
	}
	if len(result.ModifiedFiles) == 0 {
		t.Fatalf("ModifiedFiles is empty, want at least one file")
	}
	// The modified file should be "main.go" (relative path).
	found := false
	for _, f := range result.ModifiedFiles {
		if f == "main.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ModifiedFiles = %v, want to contain 'main.go'", result.ModifiedFiles)
	}
	// Verify the file content was actually modified.
	data, _ := os.ReadFile(targetPath)
	if !strings.Contains(string(data), "fixed") {
		t.Errorf("file content = %q, want to contain 'fixed'", string(data))
	}
}

// TestOpenCodeInvokerNoModifications verifies that when opencode runs
// successfully but does not modify any files, the FixResult reports
// Fixed=false with a clear message.
func TestOpenCodeInvokerNoModifications(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create an initial source file and commit it so git tracks it.
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	// Create a fake "opencode" script that does nothing.
	fakeBin := writeScript(t, `cat > /dev/null 2>&1
`)
	fc := newFailureContext()
	fc.ProjectRoot = dir
	invoker := &OpenCodeInvoker{Command: fakeBin, Timeout: 10 * time.Second}
	result, err := invoker.AnalyzeAndFix(context.Background(), fc)
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if result.Fixed {
		t.Errorf("Fixed = true, want false; message=%q", result.Message)
	}
	if !strings.Contains(result.Message, "did not modify any files") {
		t.Errorf("Message = %q, want it to contain 'did not modify any files'", result.Message)
	}
}

// TestOpenCodeInvokerNonGitRepo verifies that the invoker works in a non-git
// directory by falling back to mtime-based file detection.
func TestOpenCodeInvokerNonGitRepo(t *testing.T) {
	dir := t.TempDir()
	// Intentionally do NOT init a git repo here.

	// Create an initial source file.
	targetPath := filepath.Join(dir, "main.go")
	writeFile(t, targetPath, "package main\n\nfunc main() {}\n")

	// Capture time before invocation to ensure mtime detection works.
	time.Sleep(1100 * time.Millisecond) // ensure fs mtime granularity
	snapshotTime := time.Now()

	// Create a fake "opencode" script that modifies the source file.
	fakeBin := writeScript(t, `cat > /dev/null 2>&1
cat > "`+targetPath+`" <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("fixed without git")
}
EOF
`)
	fc := newFailureContext()
	fc.ProjectRoot = dir
	invoker := &OpenCodeInvoker{Command: fakeBin, Timeout: 10 * time.Second}

	// We need to override the snapshotTime used internally. Since we can't
	// directly, we rely on the fact that the invoker captures its own
	// snapshotTime just before running the command. The file modification
	// happens during the run, so it will be after that timestamp.
	result, err := invoker.AnalyzeAndFix(context.Background(), fc)
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	_ = snapshotTime // snapshotTime is only for documentation here

	if !result.Fixed {
		t.Fatalf("Fixed = false, want true; message=%q", result.Message)
	}
	if len(result.ModifiedFiles) == 0 {
		t.Fatalf("ModifiedFiles is empty, want at least one file (non-git fallback)")
	}
}

// ---------------------------------------------------------------------------
// buildOpenCodePrompt tests
// ---------------------------------------------------------------------------

// TestBuildOpenCodePrompt verifies that the prompt contains the key failure
// context fields.
func TestBuildOpenCodePrompt(t *testing.T) {
	fc := newFailureContext()
	prompt := buildOpenCodePrompt(fc)

	// Should mention the project name.
	if !strings.Contains(prompt, "test-project") {
		t.Errorf("prompt missing project name: %s", prompt)
	}
	// Should mention the base URL.
	if !strings.Contains(prompt, "http://localhost:8080") {
		t.Errorf("prompt missing base URL: %s", prompt)
	}
	// Should mention the failed test name.
	if !strings.Contains(prompt, "test_login") {
		t.Errorf("prompt missing test name: %s", prompt)
	}
	// Should mention the failure message.
	if !strings.Contains(prompt, "expected 200, got 401") {
		t.Errorf("prompt missing failure message: %s", prompt)
	}
	// Should mention the error.
	if !strings.Contains(prompt, "status mismatch") {
		t.Errorf("prompt missing error: %s", prompt)
	}
	// Should mention the skill name.
	if !strings.Contains(prompt, "api") {
		t.Errorf("prompt missing skill name: %s", prompt)
	}
	// Should include instructions about not running tests.
	if !strings.Contains(prompt, "DO NOT run") {
		t.Errorf("prompt missing 'do not run tests' instruction: %s", prompt)
	}
}

// TestBuildOpenCodePromptTruncation verifies that when there are more than
// maxPromptFailedCases failures, the prompt truncates with a summary message.
func TestBuildOpenCodePromptTruncation(t *testing.T) {
	fc := &failure.Context{
		ProjectName: "trunc-test",
		TotalCases:  maxPromptFailedCases + 10,
		FailedCases: make([]failure.FailedCase, maxPromptFailedCases+5),
	}
	for i := range fc.FailedCases {
		fc.FailedCases[i] = failure.FailedCase{
			SkillName: "api",
			Name:      "test_" + string(rune('a'+i)),
			Message:   "fail",
		}
	}
	prompt := buildOpenCodePrompt(fc)
	if !strings.Contains(prompt, "more failure(s) omitted") {
		t.Errorf("prompt should contain truncation notice; got: %s", prompt[:200])
	}
}

// ---------------------------------------------------------------------------
// gitPorcelainFiles tests
// ---------------------------------------------------------------------------

// TestGitPorcelainFiles verifies that gitPorcelainFiles correctly parses
// `git status --porcelain` output.
func TestGitPorcelainFiles(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create and stage a file, then modify it to make it dirty.
	writeFile(t, filepath.Join(dir, "staged.go"), "package staged\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	// Now modify the file (unstaged change).
	writeFile(t, filepath.Join(dir, "staged.go"), "package staged // modified\n")
	// Also create an untracked file.
	writeFile(t, filepath.Join(dir, "untracked.go"), "package untracked\n")

	files, err := gitPorcelainFiles(dir)
	if err != nil {
		t.Fatalf("gitPorcelainFiles error: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected at least one dirty file, got 0")
	}
	// Should include both the modified file and the untracked file.
	if _, ok := files["staged.go"]; !ok {
		t.Errorf("expected 'staged.go' in dirty files, got: %v", files)
	}
	if _, ok := files["untracked.go"]; !ok {
		t.Errorf("expected 'untracked.go' in dirty files, got: %v", files)
	}
}

// TestGitPorcelainFilesNotARepo verifies that gitPorcelainFiles returns an
// error when the directory is not a git repository.
func TestGitPorcelainFilesNotARepo(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	// Do NOT init a git repo.
	_, err := gitPorcelainFiles(dir)
	if err == nil {
		t.Fatalf("expected error for non-git directory, got nil")
	}
}

// ---------------------------------------------------------------------------
// scanModifiedByMtime tests
// ---------------------------------------------------------------------------

// TestScanModifiedByMtime verifies that scanModifiedByMtime finds files
// modified after the given timestamp.
func TestScanModifiedByMtime(t *testing.T) {
	dir := t.TempDir()
	// Create an old file.
	oldPath := filepath.Join(dir, "old.go")
	writeFile(t, oldPath, "package old\n")
	// Set its mtime to the past.
	pastTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(oldPath, pastTime, pastTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// Wait a moment to ensure the new file's mtime is strictly after.
	time.Sleep(1100 * time.Millisecond)
	snapshotTime := time.Now()

	// Sleep briefly so the new file's mtime is strictly greater than
	// snapshotTime. Filesystem mtime granularity (and clock skew under -race)
	// can otherwise cause the write to land at or before snapshotTime.
	time.Sleep(50 * time.Millisecond)

	// Create a new file (modified after snapshot).
	newPath := filepath.Join(dir, "new.go")
	writeFile(t, newPath, "package new\n")

	modified := scanModifiedByMtime(dir, snapshotTime)
	found := false
	for _, f := range modified {
		if f == "new.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("scanModifiedByMtime = %v, want to contain 'new.go'", modified)
	}
	// old.go should NOT be in the list.
	for _, f := range modified {
		if f == "old.go" {
			t.Errorf("scanModifiedByMtime should not contain 'old.go' (old mtime), got: %v", modified)
			break
		}
	}
}

// TestScanModifiedByMtimeSkipDirs verifies that scanModifiedByMtime skips
// known VCS/build directories.
func TestScanModifiedByMtimeSkipDirs(t *testing.T) {
	dir := t.TempDir()
	snapshotTime := time.Now().Add(-1 * time.Hour)

	// Create a file in node_modules (should be skipped).
	nodeModules := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nodeModules, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(nodeModules, "dep.js"), "module.exports = {};\n")

	// Create a file in .git (should be skipped).
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(gitDir, "config"), "[core]\n")

	modified := scanModifiedByMtime(dir, snapshotTime)
	for _, f := range modified {
		if strings.HasPrefix(f, "node_modules/") {
			t.Errorf("scanModifiedByMtime should skip node_modules, got: %v", modified)
			break
		}
		if strings.HasPrefix(f, ".git/") {
			t.Errorf("scanModifiedByMtime should skip .git, got: %v", modified)
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// runGit runs a git command in the given directory, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

// ---------------------------------------------------------------------------
// Invoker interface conformance test
// ---------------------------------------------------------------------------

// TestOpenCodeInvokerImplementsInvoker verifies that OpenCodeInvoker
// implements the Invoker interface.
func TestOpenCodeInvokerImplementsInvoker(t *testing.T) {
	var _ Invoker = (*OpenCodeInvoker)(nil)
}

// ---------------------------------------------------------------------------
// Subdirectory path handling (the main bug fixed in this revision)
// ---------------------------------------------------------------------------

// TestGitPorcelainFilesInSubdirectory verifies that when the project root is a
// subdirectory of a git repo, gitPorcelainFiles returns paths relative to the
// project root (not the repo root), and only includes files under the project
// root.
//
// This is a regression test for the bug where opencode was reported as having
// modified files at the repo root (e.g. "cmd/taichi/copilot.go") when the
// project root was a subdirectory.
func TestGitPorcelainFilesInSubdirectory(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	// Create a git repo at repoRoot with a subdirectory projectRoot.
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	projectRoot := filepath.Join(repoRoot, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	// Create files at both the repo root and the project root.
	repoLevelFile := filepath.Join(repoRoot, "README.md")
	projectLevelFile := filepath.Join(projectRoot, "main.go")
	writeFile(t, repoLevelFile, "# repo\n")
	writeFile(t, projectLevelFile, "package main\n")

	// Commit them so git tracks them.
	runGit(t, repoRoot, "add", ".")
	runGit(t, repoRoot, "commit", "-m", "initial")

	// Now modify both files to make them dirty.
	writeFile(t, repoLevelFile, "# repo modified\n")
	writeFile(t, projectLevelFile, "package main // modified\n")

	// Run gitPorcelainFiles from the project root.
	files, err := gitPorcelainFiles(projectRoot)
	if err != nil {
		t.Fatalf("gitPorcelainFiles error: %v", err)
	}

	// The project-level file should be present with a project-root-relative path.
	if _, ok := files["main.go"]; !ok {
		t.Errorf("expected 'main.go' in files (project-root-relative), got: %v", files)
	}

	// The repo-level file should NOT be present (it's outside the project root).
	for path := range files {
		if strings.Contains(path, "README.md") {
			t.Errorf("repo-level file 'README.md' should not appear in project-root status; files: %v", files)
		}
		// No path should contain "../" (escape from project root).
		if strings.HasPrefix(path, "../") {
			t.Errorf("path %q escapes project root", path)
		}
	}
}

// TestDetectModifiedFilesExcludesPreDirty verifies that detectModifiedFiles
// only returns files that became dirty AFTER the pre-snapshot, not files that
// were already dirty before the Agent ran.
//
// This is a regression test for the bug where the user's own uncommitted work
// (e.g. the OpenCodeInvoker source code itself) was falsely attributed to the
// AI Agent.
func TestDetectModifiedFilesExcludesPreDirty(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create and commit an initial file.
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	// Modify main.go BEFORE the snapshot (pre-existing dirty).
	writeFile(t, filepath.Join(dir, "main.go"), "package main // pre-dirty\n")
	preDirty, _ := gitPorcelainFiles(dir)
	if len(preDirty) == 0 {
		t.Fatalf("expected at least one pre-dirty file, got 0")
	}

	// Simulate the Agent modifying a different file AFTER the snapshot.
	snapshotTime := time.Now()
	time.Sleep(1100 * time.Millisecond) // ensure mtime granularity
	writeFile(t, filepath.Join(dir, "other.go"), "package other // agent-modified\n")

	modified := detectModifiedFiles(dir, snapshotTime, preDirty)
	// main.go should NOT be in the result (it was pre-dirty).
	for _, f := range modified {
		if f == "main.go" {
			t.Errorf("detectModifiedFiles should exclude pre-dirty file 'main.go', got: %v", modified)
		}
	}
	// other.go SHOULD be in the result (it's new).
	found := false
	for _, f := range modified {
		if f == "other.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("detectModifiedFiles should include 'other.go' (newly modified), got: %v", modified)
	}
}
