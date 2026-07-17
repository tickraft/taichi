package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/tickraft/taichi/pkg/i18n"
	"github.com/tickraft/taichi/pkg/mcp"
)

// newMCPCmd constructs the mcp subcommand, which starts the MCP Server.
func newMCPCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run as an MCP Server exposing taichi tools to AI Agents",
		Long: `Runs as an MCP (Model Context Protocol) Server communicating with
AI Agents via stdin/stdout.

AI Agents (e.g. Trae IDE) can invoke the following tools via MCP:
  - taichi_run        Run a test orchestration
  - taichi_list       List projects, environments, and skills in the config
  - taichi_failures   Get the failure context of the most recent run
  - taichi_regression Run regression tests

The protocol is JSON-RPC 2.0 over stdio with zero third-party deps.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPServer(cmd, gf)
		},
	}

	return cmd
}

// runMCPServer starts the MCP Server.
func runMCPServer(cmd *cobra.Command, gf *globalFlags) error {
	preloadLocale(cmd, gf)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for signals to shut down gracefully.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	server := mcp.New(gf.configPath, Version)
	fmt.Fprintln(os.Stderr, i18n.T("cli.mcp.log.serving", i18n.GetLocale()))
	if err := server.Serve(ctx); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("cli.mcp.error.serve"), err)
	}
	return nil
}
