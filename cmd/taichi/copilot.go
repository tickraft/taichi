package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/tickraft/taichi/pkg/agent"
	"github.com/tickraft/taichi/pkg/i18n"
	"github.com/tickraft/taichi/pkg/orchestrator"
	"github.com/tickraft/taichi/pkg/skill/builtin"
)

// copilotFlags holds the options of the copilot subcommand.
type copilotFlags struct {
	// project specifies the project name for this run.
	project string
	// skills restricts this run to the specified skills.
	skills []string
	// reportsDir overrides the report output directory in config.
	reportsDir string
	// timeout is the total timeout for this run.
	timeout time.Duration
	// maxRounds is the maximum number of fix rounds.
	maxRounds int
	// agentCLI is the AI Agent command line (e.g. "trae").
	agentCLI string
	// agentArgs is the args passed to the AI Agent command.
	agentArgs []string
	// agentEndpoint is the AI Agent HTTP endpoint (mutually exclusive with agentCLI).
	agentEndpoint string
	// agentToken is the Bearer token used in HTTP mode.
	agentToken string
	// agentTimeout is the timeout for a single Agent invocation.
	agentTimeout time.Duration
}

// newCopilotCmd constructs the copilot subcommand (AI Agent collaborative
// test → fix → regression loop).
func newCopilotCmd(gf *globalFlags) *cobra.Command {
	af := &copilotFlags{}

	cmd := &cobra.Command{
		Use:   "copilot",
		Short: i18n.T("cli.copilot.short"),
		Long:  i18n.T("cli.copilot.long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCopilot(cmd, gf, af)
		},
	}

	cmd.Flags().StringVarP(&af.project, "project", "p", "",
		i18n.T("cli.copilot.flag.project"))
	cmd.Flags().StringArrayVarP(&af.skills, "skill", "s", nil,
		i18n.T("cli.copilot.flag.skill"))
	cmd.Flags().StringVar(&af.reportsDir, "reports-dir", "",
		i18n.T("cli.copilot.flag.reports_dir"))
	cmd.Flags().DurationVar(&af.timeout, "timeout", 0,
		i18n.T("cli.copilot.flag.timeout"))
	cmd.Flags().IntVar(&af.maxRounds, "max-rounds", 3,
		i18n.T("cli.copilot.flag.max_rounds"))
	cmd.Flags().StringVar(&af.agentCLI, "agent-cli", "",
		i18n.T("cli.copilot.flag.agent_cli"))
	cmd.Flags().StringArrayVar(&af.agentArgs, "agent-args", nil,
		i18n.T("cli.copilot.flag.agent_args"))
	cmd.Flags().StringVar(&af.agentEndpoint, "agent-endpoint", "",
		i18n.T("cli.copilot.flag.agent_endpoint"))
	cmd.Flags().StringVar(&af.agentToken, "agent-token", "",
		i18n.T("cli.copilot.flag.agent_token"))
	cmd.Flags().DurationVar(&af.agentTimeout, "agent-timeout", 5*time.Minute,
		i18n.T("cli.copilot.flag.agent_timeout"))

	return cmd
}

// runCopilot executes the copilot orchestration (test → fix → regression loop).
func runCopilot(cmd *cobra.Command, gf *globalFlags, af *copilotFlags) error {
	preloadLocale(cmd, gf)

	logger, logCleanup, err := newLogger(gf.logLevel)
	if err != nil {
		return err
	}
	defer logCleanup()

	// Build the Agent Invoker.
	invoker, err := buildInvoker(af)
	if err != nil {
		return err
	}
	if invoker == nil {
		return errors.New(i18n.T("cli.copilot.error.no_invoker"))
	}

	ctx := context.Background()
	if af.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, af.timeout)
		defer cancel()
	} else {
		ctx = withSignalCancel(ctx, logger)
	}

	o := orchestrator.New()
	if err := o.RegisterBuiltinSkills(builtin.Skills()); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("cli.copilot.error.register"), err)
	}

	result, err := o.RunCopilot(ctx, orchestrator.CopilotOptions{
		Options: orchestrator.Options{
			ConfigPath:  gf.configPath,
			ProjectName: af.project,
			SkillFilter: af.skills,
			ReportsDir:  af.reportsDir,
			Logger:      logger,
		},
		MaxRounds: af.maxRounds,
		Invoker:   invoker,
	})
	if err != nil {
		return err
	}

	printCopilotResult(cmd, result)
	if result.Final.Summary.Failed > 0 {
		return errors.New(i18n.T("cli.copilot.error.failed_after", result.Final.Summary.Failed, len(result.Rounds)))
	}
	return nil
}

// buildInvoker builds the Agent Invoker based on command-line flags.
func buildInvoker(af *copilotFlags) (agent.Invoker, error) {
	if af.agentCLI != "" && af.agentEndpoint != "" {
		return nil, errors.New(i18n.T("cli.copilot.error.mutual_exclusive"))
	}

	if af.agentCLI != "" {
		return &agent.CLIInvoker{
			Command: af.agentCLI,
			Args:    af.agentArgs,
			Timeout: af.agentTimeout,
		}, nil
	}

	if af.agentEndpoint != "" {
		return &agent.HTTPInvoker{
			Endpoint: af.agentEndpoint,
			Token:    af.agentToken,
			Timeout:  af.agentTimeout,
		}, nil
	}

	return nil, nil
}

// printCopilotResult writes the copilot result to stdout.
func printCopilotResult(cmd *cobra.Command, r *orchestrator.CopilotResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\n%s\n", i18n.T("cli.copilot.output.header"))
	fmt.Fprintf(out, "%s: %s\n", i18n.T("cli.copilot.output.total_duration"), r.TotalDuration)
	fmt.Fprintf(out, "%s:         %d\n", i18n.T("cli.copilot.output.rounds"), len(r.Rounds))
	fmt.Fprintf(out, "%s:          %v\n", i18n.T("cli.copilot.output.fixed"), r.Fixed)

	fmt.Fprintf(out, "\n%s:\n", i18n.T("cli.copilot.output.final_result"))
	fmt.Fprintf(out, "  %s:  %s\n", i18n.T("cli.copilot.output.project"), r.Final.ProjectName)
	fmt.Fprintf(out, "  %s:  %s\n", i18n.T("cli.copilot.output.baseurl"), r.Final.BaseURL)
	fmt.Fprintf(out, "  %s: %s\n", i18n.T("cli.copilot.output.duration"), r.Final.Duration)
	fmt.Fprintf(out, "  %s:  %s\n", i18n.T("cli.copilot.output.summary"),
		i18n.T("cli.copilot.output.summary_format",
			r.Final.Summary.Total, r.Final.Summary.Passed,
			r.Final.Summary.Failed, r.Final.Summary.Skipped))

	if len(r.Rounds) > 0 {
		fmt.Fprintf(out, "\n%s:\n", i18n.T("cli.copilot.output.fix_rounds"))
		for _, round := range r.Rounds {
			fmt.Fprintf(out, "  %s %d:\n", i18n.T("cli.copilot.output.round"), round.Round)
			fmt.Fprintf(out, "    %s:     %d\n", i18n.T("cli.copilot.output.failures"), len(round.FailureContext.FailedCases))
			if round.AgentError != nil {
				fmt.Fprintf(out, "    %s:  %v\n", i18n.T("cli.copilot.output.agent_error"), round.AgentError)
				continue
			}
			if round.FixResult != nil {
				fmt.Fprintf(out, "    %s:        %v\n", i18n.T("cli.copilot.output.fixed_label"), round.FixResult.Fixed)
				fmt.Fprintf(out, "    %s:         %s\n", i18n.T("cli.copilot.output.mode"), round.FixResult.Mode)
				fmt.Fprintf(out, "    %s:      %s\n", i18n.T("cli.copilot.output.message"), round.FixResult.Message)
				if round.FixResult.Analysis != "" {
					fmt.Fprintf(out, "    %s:     %s\n", i18n.T("cli.copilot.output.analysis"), round.FixResult.Analysis)
				}
			}
			if round.ApplyError != nil {
				fmt.Fprintf(out, "    %s:  %v\n", i18n.T("cli.copilot.output.apply_error"), round.ApplyError)
			}
		}
	}
	fmt.Fprintln(out)
}
