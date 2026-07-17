package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/failure"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// writeScript writes a /bin/sh script with the given body to a temp dir,
// makes it executable, and returns the absolute path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "agent.sh")
	content := "#!/bin/sh\n" + body
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatalf("chmod script: %v", err)
	}
	return p
}

// writeFile writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

// newFailureContext builds a sample FailureContext for testing.
func newFailureContext() *failure.Context {
	return &failure.Context{
		ProjectName: "test-project",
		BaseURL:     "http://localhost:8080",
		Timestamp:   "2024-01-01T00:00:00Z",
		ProjectRoot: "/tmp/test",
		TotalCases:  3,
		PassedCases: 2,
		FailedCases: []failure.FailedCase{
			{
				SkillName: "api",
				Name:      "test_login",
				Message:   "expected 200, got 401",
				Error:     "status mismatch",
				Duration:  "1.5s",
			},
		},
	}
}

// initGitRepo initializes a git repository in the given directory and sets a
// local identity so that git operations do not depend on global config.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}
}

// gitAvailable returns true if the git binary is found on PATH.
func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// patchAvailable returns true if the patch binary is found on PATH.
func patchAvailable() bool {
	_, err := exec.LookPath("patch")
	return err == nil
}

// ---------------------------------------------------------------------------
// Invoker interface conformance tests
// ---------------------------------------------------------------------------

// TestInvokerInterface verifies that CLIInvoker and HTTPInvoker implement the Invoker interface.
func TestInvokerInterface(t *testing.T) {
	var _ Invoker = (*CLIInvoker)(nil)
	var _ Invoker = (*HTTPInvoker)(nil)
}

// ---------------------------------------------------------------------------
// FixMode tests
// ---------------------------------------------------------------------------

// TestFixModeConstants verifies the string values of the FixMode constants.
func TestFixModeConstants(t *testing.T) {
	cases := []struct {
		mode FixMode
		want string
	}{
		{FixModePatch, "patch"},
		{FixModeDirect, "direct"},
	}
	for _, c := range cases {
		if string(c.mode) != c.want {
			t.Errorf("FixMode = %q, want %q", c.mode, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FixResult JSON tests
// ---------------------------------------------------------------------------

// TestFixResultJSONRoundTrip verifies that a fully populated FixResult can be
// marshaled and unmarshaled without loss.
func TestFixResultJSONRoundTrip(t *testing.T) {
	original := FixResult{
		Fixed:         true,
		Mode:          FixModePatch,
		Patch:         "--- a/file\n+++ b/file\n",
		ModifiedFiles: []string{"file1.go", "file2.go"},
		Message:       "fix applied",
		Analysis:      "root cause was nil pointer",
	}
	data, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded FixResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Fixed != original.Fixed {
		t.Errorf("Fixed = %v, want %v", decoded.Fixed, original.Fixed)
	}
	if decoded.Mode != original.Mode {
		t.Errorf("Mode = %q, want %q", decoded.Mode, original.Mode)
	}
	if decoded.Patch != original.Patch {
		t.Errorf("Patch = %q, want %q", decoded.Patch, original.Patch)
	}
	if len(decoded.ModifiedFiles) != len(original.ModifiedFiles) {
		t.Fatalf("ModifiedFiles len = %d, want %d", len(decoded.ModifiedFiles), len(original.ModifiedFiles))
	}
	for i, f := range original.ModifiedFiles {
		if decoded.ModifiedFiles[i] != f {
			t.Errorf("ModifiedFiles[%d] = %q, want %q", i, decoded.ModifiedFiles[i], f)
		}
	}
	if decoded.Message != original.Message {
		t.Errorf("Message = %q, want %q", decoded.Message, original.Message)
	}
	if decoded.Analysis != original.Analysis {
		t.Errorf("Analysis = %q, want %q", decoded.Analysis, original.Analysis)
	}
}

// TestFixResultJSONOmitEmpty verifies that omitempty fields are omitted when
// empty, and that non-omitempty fields are always present.
func TestFixResultJSONOmitEmpty(t *testing.T) {
	result := FixResult{
		Fixed:   false,
		Mode:    FixModeDirect,
		Message: "not fixed",
	}
	data, err := json.Marshal(&result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jsonStr := string(data)
	// Fields with omitempty should be absent when empty.
	if strings.Contains(jsonStr, "\"patch\"") {
		t.Errorf("empty Patch should be omitted; json=%s", jsonStr)
	}
	if strings.Contains(jsonStr, "\"modified_files\"") {
		t.Errorf("empty ModifiedFiles should be omitted; json=%s", jsonStr)
	}
	if strings.Contains(jsonStr, "\"analysis\"") {
		t.Errorf("empty Analysis should be omitted; json=%s", jsonStr)
	}
	// Fields without omitempty should always be present.
	if !strings.Contains(jsonStr, "\"fixed\"") {
		t.Errorf("Fixed should always be present; json=%s", jsonStr)
	}
	if !strings.Contains(jsonStr, "\"mode\"") {
		t.Errorf("Mode should always be present; json=%s", jsonStr)
	}
	if !strings.Contains(jsonStr, "\"message\"") {
		t.Errorf("Message should always be present; json=%s", jsonStr)
	}
}

// TestFixResultJSONUnmarshal verifies that a JSON string can be decoded into a FixResult.
func TestFixResultJSONUnmarshal(t *testing.T) {
	jsonStr := `{"fixed":true,"mode":"patch","patch":"--- a/f\n+++ b/f\n","modified_files":["f"],"message":"ok","analysis":"root cause"}`
	var result FixResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if result.Mode != FixModePatch {
		t.Errorf("Mode = %q, want %q", result.Mode, FixModePatch)
	}
	if result.Patch != "--- a/f\n+++ b/f\n" {
		t.Errorf("Patch = %q", result.Patch)
	}
	if len(result.ModifiedFiles) != 1 || result.ModifiedFiles[0] != "f" {
		t.Errorf("ModifiedFiles = %v", result.ModifiedFiles)
	}
	if result.Message != "ok" {
		t.Errorf("Message = %q, want %q", result.Message, "ok")
	}
	if result.Analysis != "root cause" {
		t.Errorf("Analysis = %q, want %q", result.Analysis, "root cause")
	}
}

// ---------------------------------------------------------------------------
// CLIInvoker tests
// ---------------------------------------------------------------------------

// TestCLIInvokerName verifies the Name method returns the expected format.
func TestCLIInvokerName(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{"trae", "cli(trae)"},
		{"python3", "cli(python3)"},
		{"/usr/local/bin/agent", "cli(/usr/local/bin/agent)"},
		{"", "cli()"},
	}
	for _, c := range cases {
		invoker := &CLIInvoker{Command: c.command}
		if got := invoker.Name(); got != c.want {
			t.Errorf("Name() with command %q = %q, want %q", c.command, got, c.want)
		}
	}
}

// TestCLIInvokerNilFailureContext verifies that a nil failure context produces an error.
func TestCLIInvokerNilFailureContext(t *testing.T) {
	invoker := &CLIInvoker{Command: "echo"}
	_, err := invoker.AnalyzeAndFix(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error for nil failure context, got nil")
	}
	if !strings.Contains(err.Error(), "nil failure context") {
		t.Errorf("error = %q, want it to contain 'nil failure context'", err.Error())
	}
}

// TestCLIInvokerSuccess verifies that a script returning valid FixResult JSON succeeds.
func TestCLIInvokerSuccess(t *testing.T) {
	// The script discards stdin and writes a FixResult JSON to stdout.
	script := writeScript(t, `cat > /dev/null
printf '%s' '{"fixed":true,"mode":"patch","patch":"--- a/f\n+++ b/f\n","message":"fixed"}'
`)
	invoker := &CLIInvoker{Command: script, Timeout: 10 * time.Second}
	result, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if result.Mode != FixModePatch {
		t.Errorf("Mode = %q, want %q", result.Mode, FixModePatch)
	}
	if result.Message != "fixed" {
		t.Errorf("Message = %q, want %q", result.Message, "fixed")
	}
}

// TestCLIInvokerInvalidJSON verifies that non-JSON stdout produces an error.
func TestCLIInvokerInvalidJSON(t *testing.T) {
	script := writeScript(t, `printf '%s' 'not json'
`)
	invoker := &CLIInvoker{Command: script, Timeout: 10 * time.Second}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err == nil {
		t.Fatalf("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse agent output") {
		t.Errorf("error = %q, want it to contain 'parse agent output'", err.Error())
	}
}

// TestCLIInvokerNonZeroExit verifies that a non-zero exit code produces an error
// that includes the stderr output.
func TestCLIInvokerNonZeroExit(t *testing.T) {
	script := writeScript(t, `echo "boom error" >&2
exit 1
`)
	invoker := &CLIInvoker{Command: script, Timeout: 10 * time.Second}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err == nil {
		t.Fatalf("expected error for non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "run agent command") {
		t.Errorf("error = %q, want it to contain 'run agent command'", err.Error())
	}
	// The stderr output should be included in the error message.
	if !strings.Contains(err.Error(), "boom error") {
		t.Errorf("error = %q, want it to contain stderr 'boom error'", err.Error())
	}
}

// TestCLIInvokerTimeout verifies that a script exceeding the timeout produces an error
// and the test completes quickly rather than waiting for the full sleep.
func TestCLIInvokerTimeout(t *testing.T) {
	script := writeScript(t, `sleep 30
`)
	invoker := &CLIInvoker{Command: script, Timeout: 100 * time.Millisecond}
	start := time.Now()
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	// The test should complete well under 30 seconds.
	if elapsed > 5*time.Second {
		t.Errorf("test took too long: %v, expected quick timeout", elapsed)
	}
}

// TestCLIInvokerDefaultTimeout verifies that Timeout=0 uses the default timeout
// and the invocation still succeeds for a fast script.
func TestCLIInvokerDefaultTimeout(t *testing.T) {
	script := writeScript(t, `printf '%s' '{"fixed":true,"mode":"direct","message":"ok"}'
`)
	invoker := &CLIInvoker{Command: script, Timeout: 0}
	result, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
}

// TestCLIInvokerWorkDir verifies that the WorkDir is used as the command's working directory.
func TestCLIInvokerWorkDir(t *testing.T) {
	dir := t.TempDir()
	// The script writes its current working directory to a marker file.
	// Note: the shell printf format uses %%s so that fmt.Sprintf leaves %s for the shell.
	marker := filepath.Join(dir, "cwd.txt")
	script := writeScript(t, fmt.Sprintf(`pwd > %q
printf '%%s' '{"fixed":true,"mode":"direct","message":"ok"}'
`, marker))
	invoker := &CLIInvoker{
		Command: script,
		Timeout: 10 * time.Second,
		WorkDir: dir,
	}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	// Verify the marker file contains the WorkDir.
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker file: %v", err)
	}
	cwd := strings.TrimSpace(string(data))
	if cwd != dir {
		t.Errorf("script CWD = %q, want %q", cwd, dir)
	}
}

// TestCLIInvokerStdinReceivesJSON verifies that the FailureContext is passed as
// JSON via stdin to the command.
func TestCLIInvokerStdinReceivesJSON(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "stdin.txt")
	// The script writes stdin to a file and then outputs a valid FixResult.
	// Note: the shell printf format uses %%s so that fmt.Sprintf leaves %s for the shell.
	script := writeScript(t, fmt.Sprintf(`cat > %q
printf '%%s' '{"fixed":true,"mode":"direct","message":"ok"}'
`, outFile))
	invoker := &CLIInvoker{Command: script, Timeout: 10 * time.Second}
	fc := newFailureContext()
	_, err := invoker.AnalyzeAndFix(context.Background(), fc)
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	// Read the captured stdin and verify it is the FailureContext JSON.
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read stdin capture: %v", err)
	}
	var decoded failure.Context
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal captured stdin: %v", err)
	}
	if decoded.ProjectName != fc.ProjectName {
		t.Errorf("stdin ProjectName = %q, want %q", decoded.ProjectName, fc.ProjectName)
	}
	if decoded.TotalCases != fc.TotalCases {
		t.Errorf("stdin TotalCases = %d, want %d", decoded.TotalCases, fc.TotalCases)
	}
}

// ---------------------------------------------------------------------------
// HTTPInvoker tests
// ---------------------------------------------------------------------------

// TestHTTPInvokerName verifies the Name method returns the expected format.
func TestHTTPInvokerName(t *testing.T) {
	cases := []struct {
		endpoint string
		want     string
	}{
		{"http://localhost:8080/fix", "http(http://localhost:8080/fix)"},
		{"https://api.example.com/v1/agent", "http(https://api.example.com/v1/agent)"},
		{"", "http()"},
	}
	for _, c := range cases {
		invoker := &HTTPInvoker{Endpoint: c.endpoint}
		if got := invoker.Name(); got != c.want {
			t.Errorf("Name() with endpoint %q = %q, want %q", c.endpoint, got, c.want)
		}
	}
}

// TestHTTPInvokerNilFailureContext verifies that a nil failure context produces an error.
func TestHTTPInvokerNilFailureContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called for nil failure context")
	}))
	defer server.Close()
	invoker := &HTTPInvoker{Endpoint: server.URL, Timeout: 10 * time.Second}
	_, err := invoker.AnalyzeAndFix(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error for nil failure context, got nil")
	}
	if !strings.Contains(err.Error(), "nil failure context") {
		t.Errorf("error = %q, want it to contain 'nil failure context'", err.Error())
	}
}

// TestHTTPInvokerSuccess verifies that a 200 response with valid JSON succeeds.
func TestHTTPInvokerSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixed":true,"mode":"patch","patch":"--- a/f\n+++ b/f\n","message":"fixed by agent"}`))
	}))
	defer server.Close()
	invoker := &HTTPInvoker{Endpoint: server.URL, Timeout: 10 * time.Second}
	result, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
	if result.Mode != FixModePatch {
		t.Errorf("Mode = %q, want %q", result.Mode, FixModePatch)
	}
	if result.Message != "fixed by agent" {
		t.Errorf("Message = %q, want %q", result.Message, "fixed by agent")
	}
}

// TestHTTPInvokerNon200Status verifies that a non-200 response produces an error
// that includes the status code and response body.
func TestHTTPInvokerNon200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()
	invoker := &HTTPInvoker{Endpoint: server.URL, Timeout: 10 * time.Second}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err == nil {
		t.Fatalf("expected error for non-200 status, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error = %q, want it to contain 'HTTP 500'", err.Error())
	}
	if !strings.Contains(err.Error(), "internal error") {
		t.Errorf("error = %q, want it to contain 'internal error'", err.Error())
	}
}

// TestHTTPInvokerInvalidJSON verifies that non-JSON response produces an error.
func TestHTTPInvokerInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer server.Close()
	invoker := &HTTPInvoker{Endpoint: server.URL, Timeout: 10 * time.Second}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err == nil {
		t.Fatalf("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse agent response") {
		t.Errorf("error = %q, want it to contain 'parse agent response'", err.Error())
	}
}

// TestHTTPInvokerBearerToken verifies that the Authorization header is set when Token is provided.
func TestHTTPInvokerBearerToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixed":true,"mode":"direct","message":"ok"}`))
	}))
	defer server.Close()
	invoker := &HTTPInvoker{
		Endpoint: server.URL,
		Token:    "secret-token",
		Timeout:  10 * time.Second,
	}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer secret-token")
	}
}

// TestHTTPInvokerNoToken verifies that the Authorization header is not set when Token is empty.
func TestHTTPInvokerNoToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixed":true,"mode":"direct","message":"ok"}`))
	}))
	defer server.Close()
	invoker := &HTTPInvoker{Endpoint: server.URL, Timeout: 10 * time.Second}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty", gotAuth)
	}
}

// TestHTTPInvokerContentType verifies that the Content-Type header is set to application/json.
func TestHTTPInvokerContentType(t *testing.T) {
	var gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixed":true,"mode":"direct","message":"ok"}`))
	}))
	defer server.Close()
	invoker := &HTTPInvoker{Endpoint: server.URL, Timeout: 10 * time.Second}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}
}

// TestHTTPInvokerRequestMethod verifies that the request method is POST.
func TestHTTPInvokerRequestMethod(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixed":true,"mode":"direct","message":"ok"}`))
	}))
	defer server.Close()
	invoker := &HTTPInvoker{Endpoint: server.URL, Timeout: 10 * time.Second}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("Method = %q, want %q", gotMethod, http.MethodPost)
	}
}

// TestHTTPInvokerRequestBody verifies that the request body is the FailureContext JSON.
func TestHTTPInvokerRequestBody(t *testing.T) {
	var bodyBytes []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixed":true,"mode":"direct","message":"ok"}`))
	}))
	defer server.Close()
	invoker := &HTTPInvoker{Endpoint: server.URL, Timeout: 10 * time.Second}
	fc := newFailureContext()
	_, err := invoker.AnalyzeAndFix(context.Background(), fc)
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	// Verify the request body decodes to the same FailureContext.
	var decoded failure.Context
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if decoded.ProjectName != fc.ProjectName {
		t.Errorf("request body ProjectName = %q, want %q", decoded.ProjectName, fc.ProjectName)
	}
	if decoded.TotalCases != fc.TotalCases {
		t.Errorf("request body TotalCases = %d, want %d", decoded.TotalCases, fc.TotalCases)
	}
	if len(decoded.FailedCases) != len(fc.FailedCases) {
		t.Fatalf("request body FailedCases len = %d, want %d", len(decoded.FailedCases), len(fc.FailedCases))
	}
}

// TestHTTPInvokerCustomClient verifies that a custom HTTP client is used.
func TestHTTPInvokerCustomClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixed":true,"mode":"direct","message":"ok"}`))
	}))
	defer server.Close()
	customClient := &http.Client{Timeout: 10 * time.Second}
	invoker := &HTTPInvoker{
		Endpoint: server.URL,
		Client:   customClient,
		Timeout:  10 * time.Second,
	}
	result, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err != nil {
		t.Fatalf("AnalyzeAndFix error: %v", err)
	}
	if !result.Fixed {
		t.Errorf("Fixed = false, want true")
	}
}

// TestHTTPInvokerTimeout verifies that a slow server with a short timeout produces
// an error and the test completes quickly rather than waiting for the server.
func TestHTTPInvokerTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until either the client disconnects or the test releases us.
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	// Use t.Cleanup so the release channel is closed before the server,
	// ensuring the handler unblocks and server.Close() does not hang.
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	invoker := &HTTPInvoker{
		Endpoint: server.URL,
		Timeout:  100 * time.Millisecond,
	}
	start := time.Now()
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("test took too long: %v, expected quick timeout", elapsed)
	}
}

// TestHTTPInvokerConnectionError verifies that an unreachable endpoint produces an error.
func TestHTTPInvokerConnectionError(t *testing.T) {
	// Use a port that should not be listening.
	invoker := &HTTPInvoker{
		Endpoint: "http://127.0.0.1:1/fix",
		Timeout:  500 * time.Millisecond,
	}
	_, err := invoker.AnalyzeAndFix(context.Background(), newFailureContext())
	if err == nil {
		t.Fatalf("expected connection error, got nil")
	}
	if !strings.Contains(err.Error(), "call agent endpoint") {
		t.Errorf("error = %q, want it to contain 'call agent endpoint'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// PatchApplier constructor tests
// ---------------------------------------------------------------------------

// TestNewPatchApplier verifies that NewPatchApplier sets the ProjectRoot field.
func TestNewPatchApplier(t *testing.T) {
	root := "/some/path"
	pa := NewPatchApplier(root)
	if pa.ProjectRoot != root {
		t.Errorf("ProjectRoot = %q, want %q", pa.ProjectRoot, root)
	}
}

// ---------------------------------------------------------------------------
// PatchApplier.Apply tests
// ---------------------------------------------------------------------------

// TestApplyEmptyPatch verifies that an empty patch produces an error.
func TestApplyEmptyPatch(t *testing.T) {
	pa := NewPatchApplier(t.TempDir())
	files, err := pa.Apply("")
	if err == nil {
		t.Fatalf("expected error for empty patch, got nil")
	}
	if !strings.Contains(err.Error(), "empty patch") {
		t.Errorf("error = %q, want it to contain 'empty patch'", err.Error())
	}
	if files != nil {
		t.Errorf("files = %v, want nil for empty patch", files)
	}
}

// TestApplyEmptyProjectRoot verifies that an empty project root produces an error.
func TestApplyEmptyProjectRoot(t *testing.T) {
	pa := NewPatchApplier("")
	files, err := pa.Apply("--- a/f\n+++ b/f\n")
	if err == nil {
		t.Fatalf("expected error for empty project root, got nil")
	}
	if !strings.Contains(err.Error(), "project root not set") {
		t.Errorf("error = %q, want it to contain 'project root not set'", err.Error())
	}
	if files != nil {
		t.Errorf("files = %v, want nil", files)
	}
}

// TestApplySuccess verifies that a valid patch is applied successfully via the
// patch command (in a non-git directory, git apply fails and patch is used).
func TestApplySuccess(t *testing.T) {
	if !patchAvailable() {
		t.Skip("patch command not available")
	}
	dir := t.TempDir()
	// Create the target file with content matching the patch context.
	targetPath := filepath.Join(dir, "sample.txt")
	writeFile(t, targetPath, "line1\nold line\nline3\n")
	patch := `--- a/sample.txt
+++ b/sample.txt
@@ -1,3 +1,3 @@
 line1
-old line
+new line
 line3
`
	pa := NewPatchApplier(dir)
	files, err := pa.Apply(patch)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	// Verify the returned file list.
	if len(files) != 1 || files[0] != "sample.txt" {
		t.Errorf("files = %v, want [sample.txt]", files)
	}
	// Verify the file content was modified.
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	want := "line1\nnew line\nline3\n"
	if string(data) != want {
		t.Errorf("file content = %q, want %q", string(data), want)
	}
}

// TestApplySuccessWithGitRepo verifies that git apply is used when the project
// root is a git repository.
func TestApplySuccessWithGitRepo(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	initGitRepo(t, dir)
	// Create the target file with content matching the patch context.
	targetPath := filepath.Join(dir, "sample.txt")
	writeFile(t, targetPath, "line1\nold line\nline3\n")
	patch := `--- a/sample.txt
+++ b/sample.txt
@@ -1,3 +1,3 @@
 line1
-old line
+new line
 line3
`
	pa := NewPatchApplier(dir)
	files, err := pa.Apply(patch)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if len(files) != 1 || files[0] != "sample.txt" {
		t.Errorf("files = %v, want [sample.txt]", files)
	}
	// Verify the file content was modified.
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	want := "line1\nnew line\nline3\n"
	if string(data) != want {
		t.Errorf("file content = %q, want %q", string(data), want)
	}
}

// TestApplyBothFail verifies that an invalid patch (referencing a non-existent
// file) causes both git apply and the patch command to fail, returning an error
// while still returning the parsed file list.
func TestApplyBothFail(t *testing.T) {
	dir := t.TempDir()
	// A valid diff format referencing a file that does not exist.
	patch := `--- a/nonexistent.txt
+++ b/nonexistent.txt
@@ -1,1 +1,1 @@
-old
+new
`
	pa := NewPatchApplier(dir)
	files, err := pa.Apply(patch)
	if err == nil {
		t.Fatalf("expected error for invalid patch, got nil")
	}
	if !strings.Contains(err.Error(), "both git apply and patch command failed") {
		t.Errorf("error = %q, want it to contain 'both git apply and patch command failed'", err.Error())
	}
	// Even on failure, the parsed files should be returned.
	if len(files) != 1 || files[0] != "nonexistent.txt" {
		t.Errorf("files = %v, want [nonexistent.txt]", files)
	}
}

// TestApplyMultipleFiles verifies that a patch modifying multiple files returns
// all file paths and modifies all files.
func TestApplyMultipleFiles(t *testing.T) {
	if !patchAvailable() {
		t.Skip("patch command not available")
	}
	dir := t.TempDir()
	// Create target files.
	writeFile(t, filepath.Join(dir, "a.txt"), "old a\n")
	writeFile(t, filepath.Join(dir, "b.txt"), "old b\n")
	patch := `--- a/a.txt
+++ b/a.txt
@@ -1,1 +1,1 @@
-old a
+new a
--- a/b.txt
+++ b/b.txt
@@ -1,1 +1,1 @@
-old b
+new b
`
	pa := NewPatchApplier(dir)
	files, err := pa.Apply(patch)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files len = %d, want 2; files=%v", len(files), files)
	}
	// Verify file contents were modified.
	dataA, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(dataA) != "new a\n" {
		t.Errorf("a.txt = %q, want %q", string(dataA), "new a\n")
	}
	dataB, _ := os.ReadFile(filepath.Join(dir, "b.txt"))
	if string(dataB) != "new b\n" {
		t.Errorf("b.txt = %q, want %q", string(dataB), "new b\n")
	}
}

// ---------------------------------------------------------------------------
// PatchApplier.VerifyDirectFix tests
// ---------------------------------------------------------------------------

// TestVerifyDirectFixEmpty verifies that an empty file list produces an error.
func TestVerifyDirectFixEmpty(t *testing.T) {
	pa := NewPatchApplier(t.TempDir())
	err := pa.VerifyDirectFix(nil)
	if err == nil {
		t.Fatalf("expected error for empty file list, got nil")
	}
	if !strings.Contains(err.Error(), "no modified files") {
		t.Errorf("error = %q, want it to contain 'no modified files'", err.Error())
	}
}

// TestVerifyDirectFixSuccess verifies that existing files pass verification.
func TestVerifyDirectFixSuccess(t *testing.T) {
	dir := t.TempDir()
	// Create files in the project root.
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	writeFile(t, filepath.Join(dir, "b.go"), "package b\n")
	pa := NewPatchApplier(dir)
	err := pa.VerifyDirectFix([]string{"a.go", "b.go"})
	if err != nil {
		t.Errorf("VerifyDirectFix error: %v", err)
	}
}

// TestVerifyDirectFixMissingFile verifies that a non-existent file fails verification.
func TestVerifyDirectFixMissingFile(t *testing.T) {
	pa := NewPatchApplier(t.TempDir())
	err := pa.VerifyDirectFix([]string{"nonexistent.go"})
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Errorf("error = %q, want it to contain 'not accessible'", err.Error())
	}
}

// TestVerifyDirectFixIsDirectory verifies that a directory path fails verification.
func TestVerifyDirectFixIsDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory.
	subDir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pa := NewPatchApplier(dir)
	err := pa.VerifyDirectFix([]string{"subdir"})
	if err == nil {
		t.Fatalf("expected error for directory path, got nil")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("error = %q, want it to contain 'is a directory'", err.Error())
	}
}

// TestVerifyDirectFixAbsolutePath verifies that absolute paths are handled correctly.
func TestVerifyDirectFixAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	absPath := filepath.Join(dir, "abs.go")
	writeFile(t, absPath, "package abs\n")
	pa := NewPatchApplier(dir)
	err := pa.VerifyDirectFix([]string{absPath})
	if err != nil {
		t.Errorf("VerifyDirectFix with absolute path error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// PatchApplier.ApplyResult tests
// ---------------------------------------------------------------------------

// TestApplyResultNil verifies that a nil FixResult produces an error.
func TestApplyResultNil(t *testing.T) {
	pa := NewPatchApplier(t.TempDir())
	err := pa.ApplyResult(nil)
	if err == nil {
		t.Fatalf("expected error for nil result, got nil")
	}
	if !strings.Contains(err.Error(), "nil fix result") {
		t.Errorf("error = %q, want it to contain 'nil fix result'", err.Error())
	}
}

// TestApplyResultNotFixed verifies that a not-fixed result produces an error
// that includes the result's Message.
func TestApplyResultNotFixed(t *testing.T) {
	pa := NewPatchApplier(t.TempDir())
	result := &FixResult{
		Fixed:   false,
		Mode:    FixModePatch,
		Message: "could not fix",
	}
	err := pa.ApplyResult(result)
	if err == nil {
		t.Fatalf("expected error for not-fixed result, got nil")
	}
	if !strings.Contains(err.Error(), "not fixed") {
		t.Errorf("error = %q, want it to contain 'not fixed'", err.Error())
	}
	if !strings.Contains(err.Error(), "could not fix") {
		t.Errorf("error = %q, want it to contain the result message", err.Error())
	}
}

// TestApplyResultPatchMode verifies that a patch-mode FixResult is applied to the workspace.
func TestApplyResultPatchMode(t *testing.T) {
	if !patchAvailable() {
		t.Skip("patch command not available")
	}
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "sample.txt")
	writeFile(t, targetPath, "line1\nold line\nline3\n")
	patch := `--- a/sample.txt
+++ b/sample.txt
@@ -1,3 +1,3 @@
 line1
-old line
+new line
 line3
`
	pa := NewPatchApplier(dir)
	result := &FixResult{
		Fixed: true,
		Mode:  FixModePatch,
		Patch: patch,
	}
	err := pa.ApplyResult(result)
	if err != nil {
		t.Fatalf("ApplyResult error: %v", err)
	}
	// Verify the file was modified.
	data, _ := os.ReadFile(targetPath)
	want := "line1\nnew line\nline3\n"
	if string(data) != want {
		t.Errorf("file content = %q, want %q", string(data), want)
	}
}

// TestApplyResultPatchModeEmptyPatch verifies that a patch-mode FixResult with
// an empty Patch field produces an error.
func TestApplyResultPatchModeEmptyPatch(t *testing.T) {
	pa := NewPatchApplier(t.TempDir())
	result := &FixResult{
		Fixed: true,
		Mode:  FixModePatch,
		Patch: "",
	}
	err := pa.ApplyResult(result)
	if err == nil {
		t.Fatalf("expected error for empty patch in patch mode, got nil")
	}
	if !strings.Contains(err.Error(), "patch mode but empty patch content") {
		t.Errorf("error = %q, want it to contain 'patch mode but empty patch content'", err.Error())
	}
}

// TestApplyResultDirectMode verifies that a direct-mode FixResult passes file verification.
func TestApplyResultDirectMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modified.go"), "package main\n")
	pa := NewPatchApplier(dir)
	result := &FixResult{
		Fixed:         true,
		Mode:          FixModeDirect,
		ModifiedFiles: []string{"modified.go"},
	}
	err := pa.ApplyResult(result)
	if err != nil {
		t.Errorf("ApplyResult error: %v", err)
	}
}

// TestApplyResultDirectModeMissingFile verifies that a direct-mode FixResult with
// a missing file fails verification.
func TestApplyResultDirectModeMissingFile(t *testing.T) {
	pa := NewPatchApplier(t.TempDir())
	result := &FixResult{
		Fixed:         true,
		Mode:          FixModeDirect,
		ModifiedFiles: []string{"nonexistent.go"},
	}
	err := pa.ApplyResult(result)
	if err == nil {
		t.Fatalf("expected error for missing file in direct mode, got nil")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Errorf("error = %q, want it to contain 'not accessible'", err.Error())
	}
}

// TestApplyResultUnknownMode verifies that an unknown fix mode produces an error.
func TestApplyResultUnknownMode(t *testing.T) {
	pa := NewPatchApplier(t.TempDir())
	result := &FixResult{
		Fixed:   true,
		Mode:    FixMode("unknown"),
		Message: "ok",
	}
	err := pa.ApplyResult(result)
	if err == nil {
		t.Fatalf("expected error for unknown mode, got nil")
	}
	if !strings.Contains(err.Error(), "unknown fix mode") {
		t.Errorf("error = %q, want it to contain 'unknown fix mode'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// parsePatchFiles tests
// ---------------------------------------------------------------------------

// TestParsePatchFiles verifies the parsing of file paths from unified diff patches.
func TestParsePatchFiles(t *testing.T) {
	cases := []struct {
		name  string
		patch string
		want  []string
	}{
		{
			name:  "empty patch",
			patch: "",
			want:  nil,
		},
		{
			name:  "single file with b/ prefix",
			patch: "--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n",
			want:  []string{"file.txt"},
		},
		{
			name:  "single file without prefix",
			patch: "--- file.txt\n+++ file.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n",
			want:  []string{"file.txt"},
		},
		{
			name: "multiple files",
			patch: "--- a/a.txt\n+++ b/a.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n" +
				"--- a/b.txt\n+++ b/b.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n",
			want: []string{"a.txt", "b.txt"},
		},
		{
			name:  "file deletion target is /dev/null",
			patch: "--- a/deleted.txt\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-old\n",
			want:  nil,
		},
		{
			name:  "file with trailing timestamp",
			patch: "--- a/file.txt\t2024-01-01 00:00:00\n+++ b/file.txt\t2024-01-01 00:00:00\n@@ -1,1 +1,1 @@\n-old\n+new\n",
			want:  []string{"file.txt"},
		},
		{
			name: "duplicate file paths are deduplicated",
			patch: "--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n" +
				"--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n",
			want: []string{"file.txt"},
		},
		{
			name:  "no +++ lines returns nil",
			patch: "--- a/file.txt\nsome context\n",
			want:  nil,
		},
		{
			name:  "new file creation from /dev/null",
			patch: "--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1,1 @@\n+new\n",
			want:  []string{"new.txt"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parsePatchFiles(c.patch)
			if len(got) != len(c.want) {
				t.Fatalf("parsePatchFiles() len = %d, want %d; got=%v", len(got), len(c.want), got)
			}
			for i, f := range c.want {
				if got[i] != f {
					t.Errorf("parsePatchFiles()[%d] = %q, want %q", i, got[i], f)
				}
			}
		})
	}
}
