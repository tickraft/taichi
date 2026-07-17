package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tickraft/taichi/pkg/config"
	"github.com/tickraft/taichi/pkg/i18n"
)

// newValidateCmd constructs the validate subcommand.
//
// The validate subcommand loads and validates a taichi configuration file
// without starting environments or executing skills. It is useful for
// verifying the config syntax and integrity before a run.
func newValidateCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a configuration file",
		Long: `Load and validate a taichi configuration file without starting environments or executing skills.

Useful for catching config syntax and integrity errors before a run.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateConfig(cmd, gf)
		},
	}
	return cmd
}

// validateConfig loads the config and reports whether it is valid.
//
// On success it prints a confirmation message to stdout and returns nil;
// on failure it returns the wrapped error so cobra exits with a non-zero code.
func validateConfig(cmd *cobra.Command, gf *globalFlags) error {
	cfg, err := config.Load(gf.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	applyConfigLocale(cmd, gf, cfg)

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), i18n.T("cli.validate.ok"))
	return nil
}
