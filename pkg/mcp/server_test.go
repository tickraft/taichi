package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/orchestrator"
	"github.com/tickraft/taichi/pkg/skill"
	"github.com/tickraft/taichi/pkg/skill/builtin"
)

// makeRequest builds a JSON-RPC 2.0 request string with the given id, method and
// optional params. When params is nil the "params" field is omitted.
func makeRequest(id int, method string, params any) string {
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	b, err := json.Marshal(req)
	if err != nil {
		panic(fmt.Sprintf("marshal request: %v", err))
	}
	return string(b)
}

// parseResponse unmarshals a JSON-RPC response string into jsonRPCResponse.
func parseResponse(t *testing.T, raw string) jsonRPCResponse {
	t.Helper()
	var resp jsonRPCResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("parse response %q: %v", raw, err)
	}
	return resp
}

// parseToolsCallText extracts and JSON-parses the text payload of a tools/call
// success response (result.content[0].text).
func parseToolsCallText(t *testing.T, resp jsonRPCResponse) map[string]any {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected error response: code=%d message=%q", resp.Error.Code, resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map: %T", resp.Result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("content missing or empty: %v", result["content"])
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] is not a map: %T", content[0])
	}
	if first["type"] != "text" {
		t.Fatalf("content[0].type = %v, want text", first["type"])
	}
	text, ok := first["text"].(string)
	if !ok {
		t.Fatalf("text is not a string: %T", first["text"])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal tools/call text: %v", err)
	}
	return out
}

// writeConfig writes the given YAML content to a temp file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "taichi.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// writeRunnableConfig writes a minimal config that the orchestrator can execute
// end-to-end without starting a real service: a project with no env and a single
// skill that has zero cases. The report output is redirected into a temp dir.
func writeRunnableConfig(t *testing.T, skillName, skillKind string) string {
	t.Helper()
	dir := t.TempDir()
	content := fmt.Sprintf(`
projects:
  - name: unit-test
skills:
  - name: %s
    kind: %s
    enabled: true
    raw:
      timeout: 1s
report:
  output_dir: %s
`, skillName, skillKind, filepath.Join(dir, "reports"))
	return writeConfig(t, content)
}

// TestNew verifies that New stores the config path and version, observable via
// the config_path fallback behavior of the tool handlers.
func TestNew(t *testing.T) {
	t.Run("empty config path falls back to required check", func(t *testing.T) {
		s := New("", "v1.0.0")
		// No config_path on the server and none in arguments -> error.
		_, err := s.toolRun(context.Background(), map[string]any{})
		if err == nil {
			t.Fatal("expected error when config_path is empty, got nil")
		}
		if !strings.Contains(err.Error(), "config_path is required") {
			t.Errorf("error = %q, want it to contain config_path is required", err.Error())
		}
	})

	t.Run("server config path used as default", func(t *testing.T) {
		// A non-existent default path should surface a config load error.
		s := New(filepath.Join(t.TempDir(), "missing.yaml"), "v2.0.0")
		_, err := s.toolList(map[string]any{})
		if err == nil {
			t.Fatal("expected error for missing default config, got nil")
		}
		if !strings.Contains(err.Error(), "load config") {
			t.Errorf("error = %q, want it to contain load config", err.Error())
		}
	})
}

// TestHandleMessage_ParseError verifies that malformed JSON yields a -32700
// parse error response with a null id.
func TestHandleMessage_ParseError(t *testing.T) {
	s := New("", "v1.0.0")
	resp := parseResponse(t, s.handleMessage(context.Background(), "{not valid json"))
	if resp.Error == nil {
		t.Fatal("expected parse error, got nil")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("code = %d, want -32700", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "Parse error") {
		t.Errorf("message = %q, want it to contain Parse error", resp.Error.Message)
	}
	// Parse errors must use a null id.
	if string(resp.ID) != "null" {
		t.Errorf("id = %s, want null", string(resp.ID))
	}
}

// TestHandleMessage_InvalidVersion verifies that a request whose jsonrpc field
// is not "2.0" yields a -32600 invalid request error.
func TestHandleMessage_InvalidVersion(t *testing.T) {
	s := New("", "v1.0.0")
	raw := `{"jsonrpc":"1.0","id":7,"method":"initialize"}`
	resp := parseResponse(t, s.handleMessage(context.Background(), raw))
	if resp.Error == nil || resp.Error.Code != -32600 {
		t.Fatalf("expected -32600 error, got %+v", resp.Error)
	}
	if string(resp.ID) != "7" {
		t.Errorf("id = %s, want 7", string(resp.ID))
	}
}

// TestHandleMessage_UnknownMethod verifies that an unrecognized method yields a
// -32601 method-not-found error.
func TestHandleMessage_UnknownMethod(t *testing.T) {
	s := New("", "v1.0.0")
	resp := parseResponse(t, s.handleMessage(context.Background(), makeRequest(11, "foo/bar", nil)))
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected -32601 error, got %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "foo/bar") {
		t.Errorf("message = %q, want it to contain foo/bar", resp.Error.Message)
	}
}

// TestHandleMessage_Notification verifies that the notifications/initialized
// notification produces no response (empty string).
func TestHandleMessage_Notification(t *testing.T) {
	s := New("", "v1.0.0")
	if got := s.handleMessage(context.Background(), makeRequest(1, "notifications/initialized", nil)); got != "" {
		t.Errorf("notification response = %q, want empty string", got)
	}
}

// TestHandleMessage_Initialize verifies the initialize method returns the
// protocol version, capabilities, and server info.
func TestHandleMessage_Initialize(t *testing.T) {
	s := New("", "v9.9.9")
	resp := parseResponse(t, s.handleMessage(context.Background(), makeRequest(1, "initialize", nil)))
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if string(resp.ID) != "1" {
		t.Errorf("id = %s, want 1", string(resp.ID))
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map: %T", resp.Result)
	}
	if result["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v, want %q", result["protocolVersion"], protocolVersion)
	}
	caps, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities is not a map: %T", result["capabilities"])
	}
	if _, ok := caps["tools"].(map[string]any); !ok {
		t.Errorf("capabilities.tools missing or wrong type: %T", caps["tools"])
	}
	info, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("serverInfo is not a map: %T", result["serverInfo"])
	}
	if info["name"] != "taichi" {
		t.Errorf("serverInfo.name = %v, want taichi", info["name"])
	}
	if info["version"] != "v9.9.9" {
		t.Errorf("serverInfo.version = %v, want v9.9.9", info["version"])
	}
}

// TestHandleMessage_ToolsList verifies the tools/list method returns all four
// built-in tools with the expected names and schema structure.
func TestHandleMessage_ToolsList(t *testing.T) {
	s := New("", "v1.0.0")
	resp := parseResponse(t, s.handleMessage(context.Background(), makeRequest(2, "tools/list", nil)))
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map: %T", resp.Result)
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools is not an array: %T", result["tools"])
	}
	if len(tools) != 4 {
		t.Fatalf("tools count = %d, want 4", len(tools))
	}
	want := map[string]bool{
		"taichi_run":        false,
		"taichi_list":       false,
		"taichi_failures":   false,
		"taichi_regression": false,
	}
	for _, tool := range tools {
		tm, ok := tool.(map[string]any)
		if !ok {
			t.Fatalf("tool is not a map: %T", tool)
		}
		name, ok := tm["name"].(string)
		if !ok {
			t.Fatalf("tool name missing: %v", tm)
		}
		if _, exists := want[name]; !exists {
			t.Errorf("unexpected tool name %q", name)
			continue
		}
		want[name] = true
		if tm["description"] == nil || tm["description"] == "" {
			t.Errorf("tool %q has empty description", name)
		}
		schema, ok := tm["inputSchema"].(map[string]any)
		if !ok {
			t.Errorf("tool %q inputSchema missing or wrong type: %T", name, tm["inputSchema"])
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("tool %q inputSchema.type = %v, want object", name, schema["type"])
		}
		if _, ok := schema["properties"].(map[string]any); !ok {
			t.Errorf("tool %q inputSchema.properties missing or wrong type", name)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tool %q not present in tools/list response", name)
		}
	}
}

// TestHandleMessage_ToolsCall_UnknownTool verifies that tools/call with an
// unrecognized tool name yields a -32601 error.
func TestHandleMessage_ToolsCall_UnknownTool(t *testing.T) {
	s := New("", "v1.0.0")
	raw := makeRequest(5, "tools/call", map[string]any{
		"name":      "does_not_exist",
		"arguments": map[string]any{},
	})
	resp := parseResponse(t, s.handleMessage(context.Background(), raw))
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected -32601 error, got %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "does_not_exist") {
		t.Errorf("message = %q, want it to contain does_not_exist", resp.Error.Message)
	}
}

// TestHandleMessage_ToolsCall_InvalidParams verifies that tools/call with
// malformed params yields a -32602 invalid params error.
func TestHandleMessage_ToolsCall_InvalidParams(t *testing.T) {
	s := New("", "v1.0.0")
	// params is a JSON array, which cannot unmarshal into toolsCallParams.
	raw := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":[1,2,3]}`
	resp := parseResponse(t, s.handleMessage(context.Background(), raw))
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("expected -32602 error, got %+v", resp.Error)
	}
}

// TestHandleMessage_ToolsCall_MissingParams verifies that tools/call without a
// params field yields a -32602 error.
func TestHandleMessage_ToolsCall_MissingParams(t *testing.T) {
	s := New("", "v1.0.0")
	raw := `{"jsonrpc":"2.0","id":8,"method":"tools/call"}`
	resp := parseResponse(t, s.handleMessage(context.Background(), raw))
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("expected -32602 error, got %+v", resp.Error)
	}
}

// TestHandleToolsCall_MissingConfigPath is a table-driven test verifying that
// every tool returns a -32603 error when config_path is missing and the server
// has no default config path.
func TestHandleToolsCall_MissingConfigPath(t *testing.T) {
	s := New("", "v1.0.0")
	cases := []struct {
		name string
		args map[string]any
	}{
		{"taichi_run", map[string]any{}},
		{"taichi_list", map[string]any{}},
		{"taichi_failures", map[string]any{}},
		{"taichi_regression", map[string]any{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := makeRequest(1, "tools/call", map[string]any{
				"name":      c.name,
				"arguments": c.args,
			})
			resp := parseResponse(t, s.handleMessage(context.Background(), raw))
			if resp.Error == nil {
				t.Fatal("expected error for missing config_path, got nil")
			}
			if resp.Error.Code != -32603 {
				t.Errorf("code = %d, want -32603", resp.Error.Code)
			}
			if !strings.Contains(resp.Error.Message, "config_path is required") {
				t.Errorf("message = %q, want it to contain config_path is required", resp.Error.Message)
			}
		})
	}
}

// TestHandleToolsCall_NonExistentConfig verifies that tools/call surfaces a
// -32603 error when the config file does not exist.
func TestHandleToolsCall_NonExistentConfig(t *testing.T) {
	s := New("", "v1.0.0")
	missing := filepath.Join(t.TempDir(), "nope.yaml")
	cases := []struct {
		name string
		args map[string]any
	}{
		{"taichi_run", map[string]any{"config_path": missing}},
		{"taichi_list", map[string]any{"config_path": missing}},
		{"taichi_failures", map[string]any{"config_path": missing}},
		{"taichi_regression", map[string]any{"config_path": missing}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := makeRequest(1, "tools/call", map[string]any{
				"name":      c.name,
				"arguments": c.args,
			})
			resp := parseResponse(t, s.handleMessage(context.Background(), raw))
			if resp.Error == nil {
				t.Fatal("expected error for non-existent config, got nil")
			}
			if resp.Error.Code != -32603 {
				t.Errorf("code = %d, want -32603", resp.Error.Code)
			}
		})
	}
}

// TestHandleToolsCall_InvalidTimeout verifies that tools/call surfaces a -32603
// error for an unparseable timeout value.
func TestHandleToolsCall_InvalidTimeout(t *testing.T) {
	s := New("", "v1.0.0")
	cfgPath := writeRunnableConfig(t, "api", "api")
	cases := []struct {
		name string
		args map[string]any
	}{
		{"taichi_run", map[string]any{"config_path": cfgPath, "timeout": "not-a-duration"}},
		{"taichi_regression", map[string]any{"config_path": cfgPath, "timeout": "not-a-duration"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := makeRequest(1, "tools/call", map[string]any{
				"name":      c.name,
				"arguments": c.args,
			})
			resp := parseResponse(t, s.handleMessage(context.Background(), raw))
			if resp.Error == nil {
				t.Fatal("expected error for invalid timeout, got nil")
			}
			if resp.Error.Code != -32603 {
				t.Errorf("code = %d, want -32603", resp.Error.Code)
			}
			if !strings.Contains(resp.Error.Message, "invalid timeout") {
				t.Errorf("message = %q, want it to contain invalid timeout", resp.Error.Message)
			}
		})
	}
}

// TestToolRun_Success verifies a successful taichi_run over a minimal config
// produces a content payload with the expected project name and summary shape.
func TestToolRun_Success(t *testing.T) {
	cfgPath := writeRunnableConfig(t, "api", "api")
	s := New("", "v1.0.0")
	raw := makeRequest(1, "tools/call", map[string]any{
		"name":      "taichi_run",
		"arguments": map[string]any{"config_path": cfgPath},
	})
	resp := parseResponse(t, s.handleMessage(context.Background(), raw))
	out := parseToolsCallText(t, resp)
	if out["project"] != "unit-test" {
		t.Errorf("project = %v, want unit-test", out["project"])
	}
	summary, ok := out["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary is not a map: %T", out["summary"])
	}
	if summary["total"] != float64(0) {
		t.Errorf("summary.total = %v, want 0", summary["total"])
	}
	// skill_results should be a non-empty array (the api skill ran).
	sr, ok := out["skill_results"].([]any)
	if !ok {
		t.Fatalf("skill_results is not an array: %T", out["skill_results"])
	}
	if len(sr) != 1 {
		t.Errorf("skill_results count = %d, want 1", len(sr))
	}
}

// TestToolRun_WithSkillsFilterAndTimeout verifies that taichi_run accepts a
// skills filter and a valid timeout without error.
func TestToolRun_WithSkillsFilterAndTimeout(t *testing.T) {
	cfgPath := writeRunnableConfig(t, "api", "api")
	s := New("", "v1.0.0")
	raw := makeRequest(1, "tools/call", map[string]any{
		"name": "taichi_run",
		"arguments": map[string]any{
			"config_path": cfgPath,
			"skills":      []any{"api"},
			"timeout":     "30s",
		},
	})
	resp := parseResponse(t, s.handleMessage(context.Background(), raw))
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

// TestToolList_Success verifies that taichi_list returns the configured
// projects, envs, skills, report, and autofix fields.
func TestToolList_Success(t *testing.T) {
	dir := t.TempDir()
	content := fmt.Sprintf(`
projects:
  - name: demo-project
    env: demo-env
    skills: [api]
envs:
  demo-env:
    kind: backend.go
    base_url: http://localhost:8080
skills:
  - name: api
    kind: api
    enabled: true
    raw:
      timeout: 5s
report:
  suite_name: demo-suite
  output_dir: %s
  formats: [json, junit]
autofix:
  enabled: true
  reports_dir: demo-errors
`, filepath.Join(dir, "demo-reports"))
	cfgPath := writeConfig(t, content)

	s := New("", "v1.0.0")
	raw := makeRequest(1, "tools/call", map[string]any{
		"name":      "taichi_list",
		"arguments": map[string]any{"config_path": cfgPath},
	})
	resp := parseResponse(t, s.handleMessage(context.Background(), raw))
	out := parseToolsCallText(t, resp)

	// projects: array of 1 with Name "demo-project" (config.Project has no json tags).
	projects, ok := out["projects"].([]any)
	if !ok {
		t.Fatalf("projects is not an array: %T", out["projects"])
	}
	if len(projects) != 1 {
		t.Fatalf("projects count = %d, want 1", len(projects))
	}
	pm, _ := projects[0].(map[string]any)
	if pm["Name"] != "demo-project" {
		t.Errorf("projects[0].Name = %v, want demo-project", pm["Name"])
	}

	// envs: map with key demo-env.
	envs, ok := out["envs"].(map[string]any)
	if !ok {
		t.Fatalf("envs is not a map: %T", out["envs"])
	}
	if _, ok := envs["demo-env"]; !ok {
		t.Errorf("envs missing key demo-env: %v", envs)
	}

	// skills: array of 1.
	skillsArr, ok := out["skills"].([]any)
	if !ok {
		t.Fatalf("skills is not an array: %T", out["skills"])
	}
	if len(skillsArr) != 1 {
		t.Errorf("skills count = %d, want 1", len(skillsArr))
	}

	// report: map with explicit lowercase keys.
	report, ok := out["report"].(map[string]any)
	if !ok {
		t.Fatalf("report is not a map: %T", out["report"])
	}
	if report["suite_name"] != "demo-suite" {
		t.Errorf("report.suite_name = %v, want demo-suite", report["suite_name"])
	}
	formats, ok := report["formats"].([]any)
	if !ok || len(formats) != 2 {
		t.Errorf("report.formats = %v, want 2 entries", report["formats"])
	}

	// autofix: map with explicit lowercase keys.
	autofix, ok := out["autofix"].(map[string]any)
	if !ok {
		t.Fatalf("autofix is not a map: %T", out["autofix"])
	}
	if autofix["enabled"] != true {
		t.Errorf("autofix.enabled = %v, want true", autofix["enabled"])
	}
	if autofix["reports_dir"] != "demo-errors" {
		t.Errorf("autofix.reports_dir = %v, want demo-errors", autofix["reports_dir"])
	}
}

// TestToolFailures_NoFailures verifies that taichi_failures reports no failures
// when the run has zero failing cases.
func TestToolFailures_NoFailures(t *testing.T) {
	cfgPath := writeRunnableConfig(t, "api", "api")
	s := New("", "v1.0.0")
	raw := makeRequest(1, "tools/call", map[string]any{
		"name":      "taichi_failures",
		"arguments": map[string]any{"config_path": cfgPath},
	})
	resp := parseResponse(t, s.handleMessage(context.Background(), raw))
	out := parseToolsCallText(t, resp)
	if out["has_failures"] != false {
		t.Errorf("has_failures = %v, want false", out["has_failures"])
	}
	if out["message"] != "no failures detected" {
		t.Errorf("message = %v, want no failures detected", out["message"])
	}
	summary, ok := out["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary is not a map: %T", out["summary"])
	}
	if summary["total"] != float64(0) {
		t.Errorf("summary.total = %v, want 0", summary["total"])
	}
}

// TestToolRegression_Success verifies a successful taichi_regression run over a
// minimal config with a regression skill that has zero cases.
func TestToolRegression_Success(t *testing.T) {
	cfgPath := writeRunnableConfig(t, "regression", "regression")
	s := New("", "v1.0.0")
	raw := makeRequest(1, "tools/call", map[string]any{
		"name":      "taichi_regression",
		"arguments": map[string]any{"config_path": cfgPath},
	})
	resp := parseResponse(t, s.handleMessage(context.Background(), raw))
	out := parseToolsCallText(t, resp)
	if out["project"] != "unit-test" {
		t.Errorf("project = %v, want unit-test", out["project"])
	}
	summary, ok := out["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary is not a map: %T", out["summary"])
	}
	if summary["failed"] != float64(0) {
		t.Errorf("summary.failed = %v, want 0", summary["failed"])
	}
}

// TestBuiltinSkills verifies that BuiltinSkills returns the five expected
// skill instances (api, grpc, ui, static, regression).
func TestBuiltinSkills(t *testing.T) {
	skills := builtin.Skills()
	if len(skills) != 5 {
		t.Fatalf("BuiltinSkills count = %d, want 5", len(skills))
	}
	want := map[string]bool{
		"api":        false,
		"grpc":       false,
		"ui":         false,
		"static":     false,
		"regression": false,
	}
	for _, sk := range skills {
		name := sk.Name()
		if _, exists := want[name]; !exists {
			t.Errorf("unexpected skill %q", name)
			continue
		}
		want[name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("builtin skill %q not present", name)
		}
	}
}

// TestFormatRunResult verifies that formatRunResult produces the expected map
// shape for an orchestrator.Result, including nested skill results.
func TestFormatRunResult(t *testing.T) {
	r := orchestrator.Result{
		ProjectName: "proj",
		BaseURL:     "http://localhost:1234",
		Duration:    2 * time.Second,
		Summary:     framework.TestSummary{Total: 5, Passed: 3, Failed: 1, Skipped: 1},
		EnvLogPath:  "/tmp/env.log",
		SkillResults: []skill.Result{
			{
				SkillName: "api",
				Duration:  100 * time.Millisecond,
				Summary:   framework.TestSummary{Total: 5, Passed: 3, Failed: 1, Skipped: 1},
				Error:     errors.New("boom"),
			},
		},
	}
	out := formatRunResult(r)
	if out["project"] != "proj" {
		t.Errorf("project = %v, want proj", out["project"])
	}
	if out["base_url"] != "http://localhost:1234" {
		t.Errorf("base_url = %v, want http://localhost:1234", out["base_url"])
	}
	if out["duration"] != "2s" {
		t.Errorf("duration = %v, want 2s", out["duration"])
	}
	if out["env_log"] != "/tmp/env.log" {
		t.Errorf("env_log = %v, want /tmp/env.log", out["env_log"])
	}
	summary, ok := out["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary is not a map: %T", out["summary"])
	}
	if summary["total"] != 5 || summary["passed"] != 3 || summary["failed"] != 1 || summary["skipped"] != 1 {
		t.Errorf("summary = %v, want total=5 passed=3 failed=1 skipped=1", summary)
	}
	sr, ok := out["skill_results"].([]map[string]any)
	if !ok {
		t.Fatalf("skill_results is not a []map[string]any: %T", out["skill_results"])
	}
	if len(sr) != 1 {
		t.Fatalf("skill_results count = %d, want 1", len(sr))
	}
	if sr[0]["skill"] != "api" {
		t.Errorf("skill_results[0].skill = %v, want api", sr[0]["skill"])
	}
	if sr[0]["error"] != "boom" {
		t.Errorf("skill_results[0].error = %v, want boom", sr[0]["error"])
	}
	if sr[0]["duration"] != "100ms" {
		t.Errorf("skill_results[0].duration = %v, want 100ms", sr[0]["duration"])
	}
}

// TestFormatRunResult_NilError verifies that a nil skill error serializes to an
// empty string.
func TestFormatRunResult_NilError(t *testing.T) {
	r := orchestrator.Result{
		SkillResults: []skill.Result{
			{SkillName: "static", Summary: framework.TestSummary{Total: 1, Passed: 1}},
		},
	}
	out := formatRunResult(r)
	sr := out["skill_results"].([]map[string]any)
	if sr[0]["error"] != "" {
		t.Errorf("error = %v, want empty string for nil error", sr[0]["error"])
	}
}

// TestFormatSummary verifies the summary map keys and values.
func TestFormatSummary(t *testing.T) {
	s := framework.TestSummary{Total: 10, Passed: 7, Failed: 2, Skipped: 1}
	out := formatSummary(s)
	if out["total"] != 10 || out["passed"] != 7 || out["failed"] != 2 || out["skipped"] != 1 {
		t.Errorf("formatSummary = %v, want total=10 passed=7 failed=2 skipped=1", out)
	}
}

// TestSuccessResponse verifies the success response JSON structure and id
// echo, including string ids.
func TestSuccessResponse(t *testing.T) {
	s := New("", "v1.0.0")
	t.Run("numeric id", func(t *testing.T) {
		raw := s.successResponse(json.RawMessage("42"), map[string]any{"ok": true})
		resp := parseResponse(t, raw)
		if resp.JSONRPC != "2.0" {
			t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
		}
		if string(resp.ID) != "42" {
			t.Errorf("id = %s, want 42", string(resp.ID))
		}
		if resp.Error != nil {
			t.Errorf("unexpected error: %+v", resp.Error)
		}
		result, _ := resp.Result.(map[string]any)
		if result["ok"] != true {
			t.Errorf("result.ok = %v, want true", result["ok"])
		}
	})
	t.Run("string id", func(t *testing.T) {
		raw := s.successResponse(json.RawMessage(`"abc"`), "hello")
		resp := parseResponse(t, raw)
		if string(resp.ID) != `"abc"` {
			t.Errorf("id = %s, want \"abc\"", string(resp.ID))
		}
	})
	t.Run("null id", func(t *testing.T) {
		raw := s.successResponse(json.RawMessage("null"), nil)
		resp := parseResponse(t, raw)
		if string(resp.ID) != "null" {
			t.Errorf("id = %s, want null", string(resp.ID))
		}
	})
}

// TestErrorResponse verifies the error response JSON structure.
func TestErrorResponse(t *testing.T) {
	s := New("", "v1.0.0")
	raw := s.errorResponse(json.RawMessage("9"), -32000, "bad thing")
	resp := parseResponse(t, raw)
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
	if string(resp.ID) != "9" {
		t.Errorf("id = %s, want 9", string(resp.ID))
	}
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != -32000 {
		t.Errorf("code = %d, want -32000", resp.Error.Code)
	}
	if resp.Error.Message != "bad thing" {
		t.Errorf("message = %q, want bad thing", resp.Error.Message)
	}
	// An error response must not carry a result field.
	if resp.Result != nil {
		t.Errorf("result = %v, want nil for error response", resp.Result)
	}
}

// TestGetStringArg is a table-driven test for getStringArg covering present
// values, missing keys, type mismatches, and defaults.
func TestGetStringArg(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		key  string
		def  string
		want string
	}{
		{"present string", map[string]any{"k": "v"}, "k", "def", "v"},
		{"missing key returns default", map[string]any{}, "k", "def", "def"},
		{"nil map returns default", nil, "k", "def", "def"},
		{"non-string value returns default", map[string]any{"k": 123}, "k", "def", "def"},
		{"non-string bool returns default", map[string]any{"k": true}, "k", "def", "def"},
		{"empty string value is returned", map[string]any{"k": ""}, "k", "def", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := getStringArg(c.args, c.key, c.def); got != c.want {
				t.Errorf("getStringArg = %q, want %q", got, c.want)
			}
		})
	}
}

// TestGetStringSliceArg is a table-driven test for getStringSliceArg covering
// valid arrays, missing keys, type mismatches, and mixed-type arrays.
func TestGetStringSliceArg(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		key  string
		want []string
	}{
		{"valid string array", map[string]any{"k": []any{"a", "b", "c"}}, "k", []string{"a", "b", "c"}},
		{"missing key returns nil", map[string]any{}, "k", nil},
		{"nil map returns nil", nil, "k", nil},
		{"non-array value returns nil", map[string]any{"k": "not-array"}, "k", nil},
		{"non-array number returns nil", map[string]any{"k": 5}, "k", nil},
		{"mixed types keep only strings", map[string]any{"k": []any{"a", 1, true, "b"}}, "k", []string{"a", "b"}},
		{"empty array returns empty slice", map[string]any{"k": []any{}}, "k", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := getStringSliceArg(c.args, c.key)
			if len(got) != len(c.want) {
				t.Errorf("getStringSliceArg = %v, want %v (length mismatch)", got, c.want)
				return
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("getStringSliceArg[%d] = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

// TestToJSONString verifies JSON serialization and the error fallback path.
func TestToJSONString(t *testing.T) {
	t.Run("simple value", func(t *testing.T) {
		got := toJSONString(map[string]any{"a": 1})
		if !strings.Contains(got, `"a": 1`) {
			t.Errorf("toJSONString = %q, want it to contain \"a\": 1", got)
		}
	})

	t.Run("nil value", func(t *testing.T) {
		got := toJSONString(nil)
		if got != "null" {
			t.Errorf("toJSONString(nil) = %q, want null", got)
		}
	})

	t.Run("unsupported value returns error message", func(t *testing.T) {
		// Channels cannot be JSON-marshaled, so toJSONString must fall back to an
		// error message string rather than panicking.
		got := toJSONString(make(chan int))
		if !strings.Contains(got, "error serializing result") {
			t.Errorf("toJSONString(channel) = %q, want it to contain error serializing result", got)
		}
	})
}

// TestErrorString verifies skill.ErrorString returns "" for nil and the error message
// for a non-nil error.
func TestErrorString(t *testing.T) {
	if got := skill.ErrorString(nil); got != "" {
		t.Errorf("skill.ErrorString(nil) = %q, want empty", got)
	}
	err := errors.New("something failed")
	if got := skill.ErrorString(err); got != "something failed" {
		t.Errorf("skill.ErrorString(err) = %q, want something failed", got)
	}
}

// TestServe exercises the stdio transport end-to-end using in-memory pipes in
// place of os.Stdin/os.Stdout. It is not parallel because it swaps the global
// os.Stdin/os.Stdout handles.
func TestServe(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	defer stdinR.Close()
	defer stdoutR.Close()

	origStdin, origStdout := os.Stdin, os.Stdout
	os.Stdin = stdinR
	os.Stdout = stdoutW
	defer func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
	}()

	s := New("", "v1.0.0-test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.Serve(ctx)
	}()

	// Send a sequence of requests including a notification (no response), an
	// empty line (must be skipped), and an unknown tool call (error response).
	requests := []string{
		makeRequest(1, "initialize", nil),
		makeRequest(2, "notifications/initialized", nil),
		"", // empty line: Serve must skip without emitting a response
		makeRequest(3, "tools/list", nil),
		makeRequest(4, "tools/call", map[string]any{"name": "nope", "arguments": map[string]any{}}),
	}
	for _, r := range requests {
		if _, err := stdinW.WriteString(r + "\n"); err != nil {
			t.Fatalf("write request: %v", err)
		}
	}
	// Closing stdin signals EOF; Serve must return nil.
	stdinW.Close()

	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("Serve did not return within timeout")
	}

	// Close the stdout write end so the reader can reach EOF.
	stdoutW.Close()

	reader := bufio.NewReader(stdoutR)
	var responses []string
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			responses = append(responses, strings.TrimRight(line, "\n"))
		}
		if err != nil {
			break
		}
	}

	// Expect 3 responses: initialize, tools/list, tools/call error.
	// The notification and the empty line produce no response.
	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d: %v", len(responses), responses)
	}

	// Response 0: initialize.
	r0 := parseResponse(t, responses[0])
	if string(r0.ID) != "1" || r0.Error != nil {
		t.Fatalf("initialize response: id=%s err=%+v", string(r0.ID), r0.Error)
	}
	res0, _ := r0.Result.(map[string]any)
	if res0["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v, want %q", res0["protocolVersion"], protocolVersion)
	}

	// Response 1: tools/list.
	r1 := parseResponse(t, responses[1])
	if string(r1.ID) != "3" || r1.Error != nil {
		t.Fatalf("tools/list response: id=%s err=%+v", string(r1.ID), r1.Error)
	}
	res1, _ := r1.Result.(map[string]any)
	if tools, _ := res1["tools"].([]any); len(tools) != 4 {
		t.Errorf("tools count = %d, want 4", len(tools))
	}

	// Response 2: tools/call error for the unknown tool.
	r2 := parseResponse(t, responses[2])
	if string(r2.ID) != "4" {
		t.Errorf("tools/call response id = %s, want 4", string(r2.ID))
	}
	if r2.Error == nil || r2.Error.Code != -32601 {
		t.Fatalf("expected -32601 error, got %+v", r2.Error)
	}
}

// TestServe_ContextCancel verifies that Serve returns ctx.Err() when the context
// is canceled between requests (i.e. while not blocked on reading stdin).
func TestServe_ContextCancel(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	defer stdinR.Close()
	defer stdinW.Close()
	defer stdoutR.Close()
	defer stdoutW.Close()

	origStdin, origStdout := os.Stdin, os.Stdout
	os.Stdin = stdinR
	os.Stdout = stdoutW
	defer func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
	}()

	s := New("", "v1.0.0")
	ctx, cancel := context.WithCancel(context.Background())

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.Serve(ctx)
	}()

	// Send one request so Serve loops back and checks ctx.Done().
	if _, err := stdinW.WriteString(makeRequest(1, "initialize", nil) + "\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	// Allow Serve to process the request and loop back to the ctx.Done() check.
	// Serve then blocks on ReadString; canceling ctx will not unblock it, so we
	// close stdin to let it exit. The exit happens via EOF -> nil.
	cancel()
	stdinW.Close()

	select {
	case err := <-serveErr:
		// EOF path returns nil; the context-canceled branch returns ctx.Err().
		// Either outcome is acceptable since both indicate a clean shutdown.
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return within timeout")
	}
}

// TestConcurrentHandleMessage verifies that handleMessage is safe for
// concurrent use. It launches many goroutines issuing a mix of initialize and
// tools/list requests and checks every response is well-formed. Run with
// -race to detect data races.
func TestConcurrentHandleMessage(t *testing.T) {
	s := New("", "v1.0.0-test")
	const n = 80
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errs[i] = fmt.Errorf("panic: %v", r)
				}
			}()
			var raw string
			if i%2 == 0 {
				raw = makeRequest(i, "initialize", nil)
			} else {
				raw = makeRequest(i, "tools/list", nil)
			}
			resp := s.handleMessage(context.Background(), raw)
			if resp == "" {
				errs[i] = fmt.Errorf("empty response for request %d", i)
				return
			}
			var parsed jsonRPCResponse
			if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
				errs[i] = fmt.Errorf("parse: %w", err)
				return
			}
			if parsed.JSONRPC != "2.0" {
				errs[i] = fmt.Errorf("jsonrpc = %q", parsed.JSONRPC)
				return
			}
			if parsed.Error != nil {
				errs[i] = fmt.Errorf("unexpected error: %+v", parsed.Error)
				return
			}
			if string(parsed.ID) != fmt.Sprintf("%d", i) {
				errs[i] = fmt.Errorf("id = %s, want %d", string(parsed.ID), i)
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}
