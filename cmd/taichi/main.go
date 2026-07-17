// Package main is the CLI entry point of the taichi test orchestration framework.
//
// The taichi binary provides the following subcommands:
//   - run: execute a test orchestration according to the config file
//   - list: list projects, environments, and registered skills in the config
//   - mcp: run as an MCP Server
//   - copilot: AI Agent collaborative test → fix → regression loop
//   - version: print version information
package main

import (
	"os"
	"strings"

	"github.com/tickraft/taichi/pkg/i18n"
)

func main() {
	// Initialize the locale based on the --locale flag or system environment before
	// constructing the command tree. os.Args is pre-scanned because cobra's --help
	// does not trigger PersistentPreRunE, but the help text (Short/Long) needs the
	// correct locale when newRootCmd is constructed.
	locale := detectLocaleFromArgs(os.Args)
	i18n.SetLocale(locale)

	if err := newRootCmd().Execute(); err != nil {
		// cobra has already printed the error to stderr; just exit with a non-zero code.
		os.Exit(1)
	}
}

// detectLocaleFromArgs extracts the --locale value from command-line arguments.
// Falls back to system environment detection (i18n.DetectLocale) when not found.
//
// Supports two formats:
//
//	--locale=zh-CN
//	--locale zh-CN
func detectLocaleFromArgs(args []string) i18n.Locale {
	for i, arg := range args {
		if arg == "--locale" && i+1 < len(args) {
			return i18n.ParseLocale(args[i+1])
		}
		if strings.HasPrefix(arg, "--locale=") {
			return i18n.ParseLocale(strings.TrimPrefix(arg, "--locale="))
		}
	}
	return i18n.DetectLocale()
}
