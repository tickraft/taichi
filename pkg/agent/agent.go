// Package agent defines the fix integration interface and implementation between taichi and the AI Agent.
//
// When tests fail and the built-in autofix rules cannot handle them, taichi hands the failure
// context to the AI Agent via the Invoker interface for analysis and fix. The AI Agent can:
//   - Generate a unified diff patch (applied by PatchApplier, then rebuilt)
//   - Directly modify source files (taichi verifies the changes, then rebuilds)
//
// Two invocation styles are supported:
//   - CLIInvoker: invokes the AI Agent via command line (e.g. trae CLI, custom scripts)
//   - HTTPInvoker: invokes the AI Agent service via HTTP API
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/tickraft/taichi/pkg/failure"
)

// FixMode enumerates the fix execution modes of the AI Agent.
type FixMode string

const (
	// FixModePatch indicates the Agent generates a unified diff patch, which taichi applies.
	FixModePatch FixMode = "patch"
	// FixModeDirect indicates the Agent directly modifies source files; taichi only verifies.
	FixModeDirect FixMode = "direct"
)

// FixResult describes the fix output of the AI Agent.
type FixResult struct {
	// Fixed is true when the Agent considers the fix successful.
	Fixed bool `json:"fixed"`
	// Mode is the fix mode.
	Mode FixMode `json:"mode"`
	// Patch is the unified-diff patch (valid in FixModePatch mode).
	Patch string `json:"patch,omitempty"`
	// ModifiedFiles is the list of modified files (populated in both modes, used for verification and rollback).
	ModifiedFiles []string `json:"modified_files,omitempty"`
	// Message is a human-readable description returned by the Agent.
	Message string `json:"message"`
	// Analysis is the Agent's explanation of the failure cause.
	Analysis string `json:"analysis,omitempty"`
}

// Invoker defines the AI Agent invocation interface.
// Implementations are responsible for sending the failure context to the AI Agent, waiting for the Agent to analyze, and returning the fix result.
type Invoker interface {
	// AnalyzeAndFix analyzes the failure context and returns the fix result.
	AnalyzeAndFix(ctx context.Context, fc *failure.FailureContext) (*FixResult, error)
	// Name returns a human-readable name for the invoker.
	Name() string
}

// CLIInvoker invokes the AI Agent via the command line.
// The Agent script must read the FailureContext JSON from stdin and write the FixResult JSON to stdout.
type CLIInvoker struct {
	// Command is the command to execute (e.g. "trae", "python3").
	Command string
	// Args are the command arguments.
	Args []string
	// Timeout is the timeout for a single invocation. 0 means use the default 5 minutes.
	Timeout time.Duration
	// WorkDir is the working directory for command execution. Empty means the current directory.
	WorkDir string
}

// defaultCLITimeout is the default timeout for CLIInvoker.
const defaultCLITimeout = 5 * time.Minute

// Name implements Invoker.
func (c *CLIInvoker) Name() string {
	return fmt.Sprintf("cli(%s)", c.Command)
}

// AnalyzeAndFix implements Invoker.
// It passes the FailureContext as JSON via stdin to the command and reads the FixResult JSON from stdout.
func (c *CLIInvoker) AnalyzeAndFix(ctx context.Context, fc *failure.FailureContext) (*FixResult, error) {
	if fc == nil {
		return nil, fmt.Errorf("nil failure context")
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultCLITimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Command, c.Args...)
	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}

	input, err := json.Marshal(fc)
	if err != nil {
		return nil, fmt.Errorf("marshal failure context: %w", err)
	}
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run agent command %s: %w (stderr: %s)",
			c.Command, err, strings.TrimSpace(stderr.String()))
	}

	var result FixResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("parse agent output: %w (stdout: %s)",
			err, strings.TrimSpace(stdout.String()))
	}
	return &result, nil
}

// HTTPInvoker invokes the AI Agent service via HTTP API.
type HTTPInvoker struct {
	// Endpoint is the HTTP endpoint of the AI Agent (POST).
	Endpoint string
	// Token is the Bearer auth token (optional).
	Token string
	// Timeout is the HTTP request timeout. 0 means use the default 5 minutes.
	Timeout time.Duration
	// Client is a custom HTTP client. When nil, a default client is used.
	Client *http.Client
}

// defaultHTTPTimeout is the default timeout for HTTPInvoker.
const defaultHTTPTimeout = 5 * time.Minute

// Name implements Invoker.
func (h *HTTPInvoker) Name() string {
	return fmt.Sprintf("http(%s)", h.Endpoint)
}

// AnalyzeAndFix implements Invoker.
// It POSTs the FailureContext as JSON to the Endpoint and reads the FixResult JSON from the response body.
func (h *HTTPInvoker) AnalyzeAndFix(ctx context.Context, fc *failure.FailureContext) (*FixResult, error) {
	if fc == nil {
		return nil, fmt.Errorf("nil failure context")
	}

	timeout := h.Timeout
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(fc)
	if err != nil {
		return nil, fmt.Errorf("marshal failure context: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h.Token != "" {
		req.Header.Set("Authorization", "Bearer "+h.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call agent endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read agent response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result FixResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse agent response: %w (body: %s)",
			err, strings.TrimSpace(string(respBody)))
	}
	return &result, nil
}
