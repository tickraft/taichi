package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tickraft/taichi/pkg/config"
	"github.com/tickraft/taichi/pkg/i18n"
	"github.com/tickraft/taichi/pkg/registry"
	"github.com/tickraft/taichi/pkg/skill/builtin"
)

// newListCmd constructs the list subcommand.
func newListCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects, environments, and registered skills in the config",
		Long: `Loads the config file and shows:
  - Projects under test with their environment and skills
  - Defined environments
  - taichi built-in and registered custom skills

Useful for verifying the config before a run.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listConfig(cmd, gf)
		},
	}
	return cmd
}

// listConfig loads the config and prints the projects, environments, and skills.
func listConfig(cmd *cobra.Command, gf *globalFlags) error {
	cfg, err := config.Load(gf.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	applyConfigLocale(cmd, gf, cfg)

	out := cmd.OutOrStdout()

	// 1. Projects.
	_, _ = fmt.Fprintf(out, "%s\n", i18n.T("cli.list.section.projects", len(cfg.Projects)))
	for _, p := range cfg.Projects {
		_, _ = fmt.Fprintf(out, "  - %s\n", p.Name)
		if p.Root != "" {
			_, _ = fmt.Fprintf(out, "      %s:   %s\n", i18n.T("cli.list.label.root"), p.Root)
		}
		if p.Env != "" {
			_, _ = fmt.Fprintf(out, "      %s:    %s\n", i18n.T("cli.list.label.env"), p.Env)
		}
		if len(p.Skills) > 0 {
			_, _ = fmt.Fprintf(out, "      %s: %v\n", i18n.T("cli.list.label.skills"), p.Skills)
		} else {
			_, _ = fmt.Fprintf(out, "      %s: %s\n", i18n.T("cli.list.label.skills"), i18n.T("cli.list.label.skills_all"))
		}
	}

	// 2. Environments.
	_, _ = fmt.Fprintf(out, "\n%s\n", i18n.T("cli.list.section.envs", len(cfg.Envs)))
	for name, e := range cfg.Envs {
		_, _ = fmt.Fprintf(out, "  - %s (kind=%s)\n", name, e.Kind)
		if e.Port != 0 {
			_, _ = fmt.Fprintf(out, "      %s:     %d\n", i18n.T("cli.list.label.port"), e.Port)
		}
		if e.BaseURL != "" {
			_, _ = fmt.Fprintf(out, "      %s: %s\n", i18n.T("cli.list.label.base_url"), e.BaseURL)
		}
		if e.BinaryPath != "" {
			_, _ = fmt.Fprintf(out, "      %s:   %s\n", i18n.T("cli.list.label.binary"), e.BinaryPath)
		}
		if e.BuildTarget != "" {
			_, _ = fmt.Fprintf(out, "      %s:    %s\n", i18n.T("cli.list.label.build"), e.BuildTarget)
		}
		if e.HealthPath != "" {
			_, _ = fmt.Fprintf(out, "      %s:   %s\n", i18n.T("cli.list.label.health"), e.HealthPath)
		}
		if e.Command != "" {
			_, _ = fmt.Fprintf(out, "      %s:  %s\n", i18n.T("cli.list.label.command"), e.Command)
		}
		if e.ReadyURL != "" {
			_, _ = fmt.Fprintf(out, "      %s:    %s\n", i18n.T("cli.list.label.ready"), e.ReadyURL)
		}
	}

	// 3. Skill configs.
	_, _ = fmt.Fprintf(out, "\n%s\n", i18n.T("cli.list.section.skill_cfgs", len(cfg.Skills)))
	for _, sc := range cfg.Skills {
		state := i18n.T("cli.list.state.disabled")
		if sc.Enabled {
			state = i18n.T("cli.list.state.enabled")
		}
		_, _ = fmt.Fprintf(out, "  - %-12s %s=%-10s %s=%-3d %s\n",
			sc.Name, i18n.T("cli.list.label.kind"), sc.Kind, i18n.T("cli.list.label.priority"), sc.Priority, state)
	}

	// 4. Registered builtin skills.
	reg := registry.NewRegistry()
	for _, s := range builtin.Skills() {
		_ = reg.Register(s, true)
	}
	registered := reg.List()
	_, _ = fmt.Fprintf(out, "\n%s\n", i18n.T("cli.list.section.registered", len(registered)))
	for _, s := range registered {
		_, _ = fmt.Fprintf(out, "  - %-12s %s=%-10s %s=%-3d\n",
			s.Name(), i18n.T("cli.list.label.kind"), s.Kind(), i18n.T("cli.list.label.priority"), s.Priority())
	}

	// 5. Report and autofix config.
	_, _ = fmt.Fprintf(out, "\n%s\n", i18n.T("cli.list.section.report"))
	_, _ = fmt.Fprintf(out, "  %s: %s\n", i18n.T("cli.list.label.suite_name"), cfg.Report.SuiteName)
	_, _ = fmt.Fprintf(out, "  %s: %s\n", i18n.T("cli.list.label.output_dir"), cfg.Report.OutputDir)
	_, _ = fmt.Fprintf(out, "  %s:    %v\n", i18n.T("cli.list.label.formats"), cfg.Report.Formats)
	_, _ = fmt.Fprintf(out, "\n%s\n", i18n.T("cli.list.section.autofix"))
	_, _ = fmt.Fprintf(out, "  %s:     %v\n", i18n.T("cli.list.label.enabled"), cfg.Autofix.Enabled)
	_, _ = fmt.Fprintf(out, "  %s: %s\n", i18n.T("cli.list.label.reports_dir"), cfg.Autofix.ReportsDir)

	return nil
}
