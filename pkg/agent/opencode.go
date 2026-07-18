package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tickraft/taichi/pkg/failure"
)

// OpenCodeInvoker invokes the opencode CLI as the AI Agent.
//
// Unlike CLIInvoker, opencode does NOT use the stdin/stdout JSON protocol:
//   - It accepts a human-readable prompt as command-line args (opencode run "...")
//   - It directly modifies files in the working directory (direct mode)
//   - Its stdout is human-readable text (not FixResult JSON)
//   - Modified files are detected post-run via `git status --porcelain`
//
// Usage from taichi copilot:
//
//	taichi copilot --agent-cli opencode
//
// Extra opencode flags can be passed via --agent-args, e.g.:
//
//	taichi copilot --agent-cli opencode --agent-args "--model" --agent-args "anthropic/claude-3.5-sonnet"
type OpenCodeInvoker struct {
	// Command is the opencode binary path. Empty defaults to "opencode".
	Command string
	// Args are extra args passed to `opencode run` (e.g. ["--model", "anthropic/..."]).
	Args []string
	// Timeout is the timeout for a single invocation. 0 means use the default.
	Timeout time.Duration
	// WorkDir overrides the working directory. Empty means use fc.ProjectRoot.
	WorkDir string
}

// defaultOpenCodeTimeout is the default timeout for OpenCodeInvoker.
// opencode agent loops can take a while, so we default to 15 minutes.
const defaultOpenCodeTimeout = 15 * time.Minute

// maxPromptFailedCases limits how many failed cases are inlined into the
// prompt to avoid exceeding the model's context window on large failures.
const maxPromptFailedCases = 30

// Name implements Invoker.
func (o *OpenCodeInvoker) Name() string {
	cmd := o.Command
	if cmd == "" {
		cmd = "opencode"
	}
	return fmt.Sprintf("opencode(%s)", cmd)
}

// AnalyzeAndFix implements Invoker.
//
// It:
//  1. Builds a human-readable prompt from the failure context.
//  2. Snapshots the git status (if available) before invocation.
//  3. Runs `opencode run "<prompt>"` in the project root.
//  4. After completion, detects modified files via `git status --porcelain`.
//  5. Returns a FixResult in direct mode with the modified files list.
//
// If the project is not a git repository, falls back to a timestamp-based
// scan of files modified during the opencode invocation window.
func (o *OpenCodeInvoker) AnalyzeAndFix(ctx context.Context, fc *failure.Context) (*FixResult, error) {
	if fc == nil {
		return nil, fmt.Errorf("nil failure context")
	}

	timeout := o.Timeout
	if timeout <= 0 {
		timeout = defaultOpenCodeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	workDir := o.WorkDir
	if workDir == "" {
		workDir = fc.ProjectRoot
	}
	if workDir == "" {
		return nil, fmt.Errorf("opencode invoker: project root is empty; set WorkDir or fc.ProjectRoot")
	}

	// Resolve the opencode binary.
	binary := o.Command
	if binary == "" {
		binary = "opencode"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("opencode binary not found in PATH: %w", err)
	}

	// Snapshot git state before invocation (for non-git projects, snapshot mtimes).
	snapshotTime := time.Now()
	preDirty, _ := gitPorcelainFiles(workDir)

	// Build the prompt.
	prompt := buildOpenCodePrompt(fc)

	// Assemble the command: opencode run [extra args] "<prompt>"
	cmdArgs := make([]string, 0, 2+len(o.Args)+1)
	cmdArgs = append(cmdArgs, "run")
	cmdArgs = append(cmdArgs, o.Args...)
	cmdArgs = append(cmdArgs, prompt)

	cmd := exec.CommandContext(ctx, binary, cmdArgs...)
	cmd.Dir = workDir
	// Inherit stdout/stderr so the user can see opencode's progress in real time.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil // opencode does not read stdin in run mode
	// WaitDelay ensures cmd.Run returns promptly after the context kills the process.
	cmd.WaitDelay = 3 * time.Second

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("opencode run failed: %w", err)
	}

	// Detect modified files after invocation.
	modified := detectModifiedFiles(workDir, snapshotTime, preDirty)
	if len(modified) == 0 {
		return &FixResult{
			Fixed:   false,
			Mode:    FixModeDirect,
			Message: "opencode completed but did not modify any files",
		}, nil
	}

	return &FixResult{
		Fixed:         true,
		Mode:          FixModeDirect,
		ModifiedFiles: modified,
		Message:       fmt.Sprintf("opencode modified %d file(s)", len(modified)),
		Analysis:      summarizeModifiedFiles(workDir, modified),
	}, nil
}

// buildOpenCodePrompt builds a clear, actionable prompt for opencode from the
// failure context. The prompt is human-readable and includes all failure
// details so opencode can analyze and fix them without needing to read a
// separate JSON file.
func buildOpenCodePrompt(fc *failure.Context) string {
	var b strings.Builder
	b.WriteString("You are being invoked by the taichi test framework to fix failing tests in this project.\n\n")
	b.WriteString("DO NOT run any tests yourself. taichi will run regression tests after you finish.\n")
	b.WriteString("Make minimal, targeted fixes to the source code only.\n\n")

	fmt.Fprintf(&b, "Project: %s\n", fc.ProjectName)
	if fc.BaseURL != "" {
		fmt.Fprintf(&b, "Service Base URL: %s\n", fc.BaseURL)
	}
	if fc.ProjectRoot != "" {
		fmt.Fprintf(&b, "Project Root: %s\n", fc.ProjectRoot)
	}
	fmt.Fprintf(&b, "\nTest Results: %d passed, %d failed (out of %d total)\n",
		fc.PassedCases, len(fc.FailedCases), fc.TotalCases)

	if fc.EnvLogPath != "" {
		fmt.Fprintf(&b, "Env log: %s\n", fc.EnvLogPath)
	}
	if fc.ReportsDir != "" {
		fmt.Fprintf(&b, "Reports dir: %s\n", fc.ReportsDir)
	}

	b.WriteString("\n=== Failed Tests ===\n")
	limit := len(fc.FailedCases)
	if limit > maxPromptFailedCases {
		limit = maxPromptFailedCases
	}
	for i, f := range fc.FailedCases {
		if i >= limit {
			fmt.Fprintf(&b, "... and %d more failure(s) omitted (see reports dir for full details)\n",
				len(fc.FailedCases)-limit)
			break
		}
		fmt.Fprintf(&b, "\n%d. [%s] %s\n", i+1, f.SkillName, f.Name)
		if f.Message != "" {
			fmt.Fprintf(&b, "   Message: %s\n", f.Message)
		}
		if f.Error != "" {
			fmt.Fprintf(&b, "   Error: %s\n", f.Error)
		}
		if f.Duration != "" {
			fmt.Fprintf(&b, "   Duration: %s\n", f.Duration)
		}
	}

	b.WriteString("\n=== Instructions ===\n")
	b.WriteString("1. Read the failing test names and messages above to understand what is broken.\n")
	b.WriteString("2. Locate the corresponding source files in the project root.\n")
	b.WriteString("3. Fix the root cause of each failure with minimal changes.\n")
	b.WriteString("4. Do NOT modify test configuration or expectations to make tests pass artificially.\n")
	b.WriteString("5. Do NOT run any test commands - taichi handles regression testing.\n")
	return b.String()
}

// gitPorcelainFiles returns the set of file paths reported by
// `git status --porcelain` in the given directory. Returns an empty set if
// git is unavailable or the directory is not a git repository.
//
// Paths are normalized to be relative to the given directory (not the repo
// root), so that downstream consumers (e.g. VerifyDirectFix) can resolve them
// against the project root. This handles the case where the project root is a
// subdirectory of a larger git repository.
func gitPorcelainFiles(dir string) (map[string]struct{}, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git not found: %w", err)
	}

	// `-- .` limits output to changes under the current directory.
	cmd := exec.Command(gitPath, "status", "--porcelain", "--untracked-files=all", "--", ".")
	cmd.Dir = dir
	var stdout strings.Builder
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	// When run from a subdirectory of a git repo, `git status --porcelain`
	// still reports paths relative to the REPO root. We use `--show-prefix`
	// to get the path from repo root to our directory, then strip it so paths
	// become relative to dir (the project root).
	prefix := gitShowPrefix(dir)

	files := make(map[string]struct{})
	for _, line := range strings.Split(stdout.String(), "\n") {
		if len(line) < 3 {
			continue
		}
		// Format: "XY path" where XY is 2-char status and path follows.
		path := strings.TrimSpace(line[3:])
		// Handle rename: "R  old -> new"
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		path = strings.Trim(path, `"`)
		if path == "" {
			continue
		}
		// Strip the repo-root-relative prefix to make the path project-root-relative.
		if prefix != "" && strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
		}
		// Skip paths that escaped the project root (e.g., via "../").
		if path == "" || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
			continue
		}
		files[path] = struct{}{}
	}
	return files, nil
}

// gitShowPrefix returns the path from the git repo root to the given directory,
// with a trailing slash. Returns an empty string if dir is the repo root or git
// is unavailable. Example: for "/repo/sub/dir" where "/repo" is the git root,
// it returns "sub/dir/".
func gitShowPrefix(dir string) string {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	cmd := exec.Command(gitPath, "rev-parse", "--show-prefix")
	cmd.Dir = dir
	var stdout strings.Builder
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

// detectModifiedFiles returns the list of files modified between the
// pre-invocation git snapshot and the current state.
//
// In a git repository, it uses `git status --porcelain` and returns only files
// that are NEW or CHANGED compared to the pre-snapshot. This excludes
// pre-existing dirty files (e.g. the user's own uncommitted work) so they are
// not falsely attributed to the AI Agent.
//
// If git is not available, it falls back to scanning for files with mtime
// after snapshotTime.
func detectModifiedFiles(dir string, snapshotTime time.Time, preDirty map[string]struct{}) []string {
	// Try git-based detection first.
	current, err := gitPorcelainFiles(dir)
	if err == nil {
		// Diff against pre-snapshot: only return files that were NOT dirty
		// before the Agent ran. This prevents falsely attributing pre-existing
		// changes (e.g. the user's own work-in-progress) to the Agent.
		result := make([]string, 0, len(current))
		for path := range current {
			if _, wasDirty := preDirty[path]; !wasDirty {
				result = append(result, path)
			}
		}
		sort.Strings(result)
		if len(result) > 0 {
			return result
		}
		// Fall through to timestamp scan if git reports nothing new.
	}

	// Fallback: scan for files modified after snapshotTime.
	return scanModifiedByMtime(dir, snapshotTime)
}

// scanModifiedByMtime walks the directory tree and returns paths of files
// whose modification time is at or after the given timestamp.
//
// Common VCS/build directories are skipped to avoid noise.
func scanModifiedByMtime(dir string, after time.Time) []string {
	skipDirs := map[string]bool{
		".git":          true,
		"node_modules":  true,
		"bin":           true,
		"dist":          true,
		"build":         true,
		"reports":       true,
		".next":         true,
		".cache":        true,
		"__pycache__":   true,
		".pytest_cache": true,
		".venv":         true,
		"venv":          true,
	}

	var modified []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := filepath.Base(path)
			if skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		// Only consider source files.
		if !isSourceFile(path) {
			return nil
		}
		if info.ModTime().After(after) || info.ModTime().Equal(after) {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return nil
			}
			modified = append(modified, rel)
		}
		return nil
	})
	sort.Strings(modified)
	return modified
}

// isSourceFile returns true for common source file extensions.
func isSourceFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".java", ".kt",
		".rs", ".rb", ".php", ".c", ".cpp", ".h", ".hpp",
		".yaml", ".yml", ".json", ".toml", ".xml", ".html", ".css",
		".scss", ".sql", ".proto", ".sh", ".md":
		return true
	}
	return false
}

// summarizeModifiedFiles returns a brief JSON summary of the modified files
// for the FixResult.Analysis field. It includes file paths and a git diff
// stat if available.
func summarizeModifiedFiles(dir string, files []string) string {
	var b strings.Builder
	b.WriteString("Modified files:\n")
	for _, f := range files {
		fmt.Fprintf(&b, "  - %s\n", f)
	}

	// Try to include a git diff stat for context.
	if gitPath, err := exec.LookPath("git"); err == nil {
		cmd := exec.Command(gitPath, "diff", "--stat")
		cmd.Dir = dir
		var stdout strings.Builder
		cmd.Stdout = &stdout
		if cmd.Run() == nil && stdout.Len() > 0 {
			b.WriteString("\nGit diff stat:\n")
			b.WriteString(stdout.String())
		}
	}
	return b.String()
}

// OpenCodeDetect returns true if the given command name should be handled by
// OpenCodeInvoker instead of CLIInvoker. It checks the basename of the command
// path against known opencode binary names.
func OpenCodeDetect(command string) bool {
	if command == "" {
		return false
	}
	// Handle both Unix (/) and Windows (\) path separators, since the
	// command may have been typed on either platform.
	base := command
	if idx := strings.LastIndexAny(base, `/\`); idx >= 0 {
		base = base[idx+1:]
	}
	base = strings.TrimSuffix(base, ".exe")
	return base == "opencode"
}
