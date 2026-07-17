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
		Short: i18n.T("cli.mcp.short"),
		Long:  i18n.T("cli.mcp.long"),
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
