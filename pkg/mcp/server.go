// Package mcp implements the taichi MCP (Model Context Protocol) Server.
//
// taichi exposes its test orchestration capabilities to the AI Agent via the MCP Server, allowing the Agent to:
//   - run tests (taichi_run)
//   - inspect configuration (taichi_list)
//   - retrieve failing cases (taichi_failures)
//   - run regression tests (taichi_regression)
//
// The MCP protocol is based on JSON-RPC 2.0 over stdio transport (one JSON-RPC message per line).
// This implementation does not depend on a third-party MCP SDK; it uses only the standard library, per project conventions.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tickraft/taichi/pkg/config"
	"github.com/tickraft/taichi/pkg/failure"
	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/i18n"
	"github.com/tickraft/taichi/pkg/orchestrator"
	"github.com/tickraft/taichi/pkg/skill"
	"github.com/tickraft/taichi/pkg/skill/builtin"
)

// protocolVersion is the MCP protocol version.
const protocolVersion = "2024-11-05"

// Server is the taichi MCP Server.
type Server struct {
	configPath string
	version    string
}

// New creates an MCP Server.
// configPath is the default config file path; it can be overridden by tool call arguments.
func New(configPath, version string) *Server {
	return &Server{
		configPath: configPath,
		version:    version,
	}
}

// Serve starts the MCP Server, reading and writing JSON-RPC messages over stdio.
// It blocks until ctx is canceled or stdin is closed.
func (s *Server) Serve(ctx context.Context) error {
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read stdin: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		response := s.handleMessage(ctx, line)
		if response != "" {
			if _, err := writer.WriteString(response + "\n"); err != nil {
				return fmt.Errorf("write stdout: %w", err)
			}
			_ = writer.Flush()
		}
	}
}

// jsonRPCRequest is the JSON-RPC 2.0 request structure.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is the JSON-RPC 2.0 response structure.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCErr     `json:"error,omitempty"`
}

// jsonRPCErr is the JSON-RPC 2.0 error structure.
type jsonRPCErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// handleMessage processes a single JSON-RPC message and returns the response JSON (empty string when no response is needed).
func (s *Server) handleMessage(ctx context.Context, raw string) string {
	var req jsonRPCRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return s.errorResponse(nil, -32700, "Parse error: "+err.Error())
	}

	if req.JSONRPC != "2.0" {
		return s.errorResponse(req.ID, -32600, "Invalid Request: jsonrpc must be 2.0")
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		// Notification message; no response is needed.
		return ""
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return s.errorResponse(req.ID, -32601, "Method not found: "+req.Method)
	}
}

// handleInitialize handles the MCP initialize method.
func (s *Server) handleInitialize(req jsonRPCRequest) string {
	result := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "taichi",
			"version": s.version,
		},
	}
	return s.successResponse(req.ID, result)
}

// handleToolsList handles the MCP tools/list method and returns the available tool definitions.
func (s *Server) handleToolsList(req jsonRPCRequest) string {
	tools := []map[string]any{
		{
			"name":        "taichi_run",
			"description": i18n.T("mcp.tool.taichi_run.desc"),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"config_path": map[string]any{"type": "string", "description": i18n.T("mcp.tool.param.config_path")},
					"project":     map[string]any{"type": "string", "description": i18n.T("mcp.tool.param.project")},
					"skills":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": i18n.T("mcp.tool.param.skills")},
					"timeout":     map[string]any{"type": "string", "description": i18n.T("mcp.tool.param.timeout_seconds")},
				},
			},
		},
		{
			"name":        "taichi_list",
			"description": i18n.T("mcp.tool.taichi_list.desc"),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"config_path": map[string]any{"type": "string", "description": i18n.T("mcp.tool.param.config_path")},
				},
			},
		},
		{
			"name":        "taichi_failures",
			"description": i18n.T("mcp.tool.taichi_failures.desc"),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"config_path": map[string]any{"type": "string", "description": i18n.T("mcp.tool.param.config_path")},
					"reports_dir": map[string]any{"type": "string", "description": i18n.T("mcp.tool.param.reports_dir")},
				},
			},
		},
		{
			"name":        "taichi_regression",
			"description": i18n.T("mcp.tool.taichi_regression.desc"),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"config_path": map[string]any{"type": "string", "description": i18n.T("mcp.tool.param.config_path")},
					"project":     map[string]any{"type": "string", "description": i18n.T("mcp.tool.param.project")},
					"timeout":     map[string]any{"type": "string", "description": i18n.T("mcp.tool.param.timeout_seconds")},
				},
			},
		},
	}
	return s.successResponse(req.ID, map[string]any{"tools": tools})
}

// toolsCallParams holds the parameters of tools/call.
type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// handleToolsCall handles the MCP tools/call method and dispatches to the concrete tool implementation.
func (s *Server) handleToolsCall(ctx context.Context, req jsonRPCRequest) string {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.errorResponse(req.ID, -32602, "Invalid params: "+err.Error())
	}

	var result any
	var err error

	switch params.Name {
	case "taichi_run":
		result, err = s.toolRun(ctx, params.Arguments)
	case "taichi_list":
		result, err = s.toolList(params.Arguments)
	case "taichi_failures":
		result, err = s.toolFailures(params.Arguments)
	case "taichi_regression":
		result, err = s.toolRegression(ctx, params.Arguments)
	default:
		return s.errorResponse(req.ID, -32601, "Unknown tool: "+params.Name)
	}

	if err != nil {
		return s.errorResponse(req.ID, -32603, err.Error())
	}

	// MCP tools/call response format: a content array.
	return s.successResponse(req.ID, map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": toJSONString(result)},
		},
	})
}

// toolRun executes the taichi_run tool.
func (s *Server) toolRun(ctx context.Context, args map[string]any) (any, error) {
	configPath := getStringArg(args, "config_path", s.configPath)
	if configPath == "" {
		return nil, fmt.Errorf("config_path is required")
	}

	timeoutStr := getStringArg(args, "timeout", "")
	var timeout time.Duration
	if timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
		timeout = d
	}

	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	o := orchestrator.New()
	if err := o.RegisterBuiltinSkills(builtin.Skills()); err != nil {
		return nil, fmt.Errorf("register skills: %w", err)
	}

	result, err := o.Run(runCtx, orchestrator.Options{
		ConfigPath:  configPath,
		ProjectName: getStringArg(args, "project", ""),
		SkillFilter: getStringSliceArg(args, "skills"),
		Logger:      skill.NoOpLogger{},
	})
	if err != nil {
		return nil, err
	}

	return formatRunResult(result), nil
}

// toolList executes the taichi_list tool.
func (s *Server) toolList(args map[string]any) (any, error) {
	configPath := getStringArg(args, "config_path", s.configPath)
	if configPath == "" {
		return nil, fmt.Errorf("config_path is required")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return map[string]any{
		"projects": cfg.Projects,
		"envs":     cfg.Envs,
		"skills":   cfg.Skills,
		"report": map[string]any{
			"suite_name": cfg.Report.SuiteName,
			"output_dir": cfg.Report.OutputDir,
			"formats":    cfg.Report.Formats,
		},
		"autofix": map[string]any{
			"enabled":     cfg.Autofix.Enabled,
			"reports_dir": cfg.Autofix.ReportsDir,
		},
	}, nil
}

// toolFailures executes the taichi_failures tool.
func (s *Server) toolFailures(args map[string]any) (any, error) {
	configPath := getStringArg(args, "config_path", s.configPath)
	if configPath == "" {
		return nil, fmt.Errorf("config_path is required")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	reportsDir := getStringArg(args, "reports_dir", cfg.Report.OutputDir)
	if reportsDir == "" {
		reportsDir = "reports"
	}

	// Run a single test pass to obtain the failure context.
	o := orchestrator.New()
	if err := o.RegisterBuiltinSkills(builtin.Skills()); err != nil {
		return nil, fmt.Errorf("register skills: %w", err)
	}

	result, err := o.Run(context.Background(), orchestrator.Options{
		ConfigPath: configPath,
		Logger:     skill.NoOpLogger{},
	})
	if err != nil {
		return nil, err
	}

	fc := failure.FromResults(
		result.ProjectName,
		result.BaseURL,
		result.ProjectRoot,
		result.EnvLogPath,
		reportsDir,
		result.Results,
		nil,
	)

	if !fc.HasFailures() {
		return map[string]any{
			"has_failures": false,
			"summary":      formatSummary(result.Summary),
			"message":      "no failures detected",
		}, nil
	}

	return map[string]any{
		"has_failures":    true,
		"failure_context": fc,
		"summary":         formatSummary(result.Summary),
	}, nil
}

// toolRegression executes the taichi_regression tool.
func (s *Server) toolRegression(ctx context.Context, args map[string]any) (any, error) {
	configPath := getStringArg(args, "config_path", s.configPath)
	if configPath == "" {
		return nil, fmt.Errorf("config_path is required")
	}

	timeoutStr := getStringArg(args, "timeout", "")
	var timeout time.Duration
	if timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
		timeout = d
	}

	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	o := orchestrator.New()
	if err := o.RegisterBuiltinSkills(builtin.Skills()); err != nil {
		return nil, fmt.Errorf("register skills: %w", err)
	}

	result, err := o.Run(runCtx, orchestrator.Options{
		ConfigPath:  configPath,
		ProjectName: getStringArg(args, "project", ""),
		SkillFilter: []string{"regression"},
		Logger:      skill.NoOpLogger{},
	})
	if err != nil {
		return nil, err
	}

	return formatRunResult(result), nil
}

// formatRunResult formats an orchestrator.Result into an MCP response.
func formatRunResult(r orchestrator.Result) map[string]any {
	skillResults := make([]map[string]any, 0, len(r.SkillResults))
	for _, sr := range r.SkillResults {
		skillResults = append(skillResults, map[string]any{
			"skill":    sr.SkillName,
			"duration": sr.Duration.String(),
			"summary":  formatSummary(sr.Summary),
			"error":    skill.ErrorString(sr.Error),
		})
	}
	return map[string]any{
		"project":       r.ProjectName,
		"base_url":      r.BaseURL,
		"duration":      r.Duration.String(),
		"summary":       formatSummary(r.Summary),
		"env_log":       r.EnvLogPath,
		"skill_results": skillResults,
	}
}

// formatSummary formats a framework.TestSummary into a map.
func formatSummary(s framework.TestSummary) map[string]any {
	return map[string]any{
		"total":   s.Total,
		"passed":  s.Passed,
		"failed":  s.Failed,
		"skipped": s.Skipped,
	}
}

// successResponse builds a success response JSON.
func (s *Server) successResponse(id json.RawMessage, result any) string {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

// errorResponse builds an error response JSON.
func (s *Server) errorResponse(id json.RawMessage, code int, message string) string {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCErr{Code: code, Message: message},
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

// getStringArg retrieves a string value from the arguments map.
func getStringArg(args map[string]any, key, defaultVal string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

// getStringSliceArg retrieves a string slice value from the arguments map.
func getStringSliceArg(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// toJSONString serializes a value into a JSON string.
func toJSONString(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("error serializing result: %v", err)
	}
	return string(data)
}
