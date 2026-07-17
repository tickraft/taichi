package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/tickraft/taichi/pkg/i18n"
)

// Version is the version of the taichi binary; it can be injected at build time via -ldflags:
//
//	go build -ldflags "-X main.Version=1.0.0" -o bin/taichi ./cmd/taichi
//
// Defaults when not injected.
var Version = "0.1.0-dev"

// newVersionCmd returns the version subcommand.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: i18n.T("cli.version.short"),
		Long:  i18n.T("cli.version.long"),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n",
				i18n.T("cli.version.format", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH))
		},
	}
}
