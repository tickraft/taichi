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
		Short: "AI Agent collaborative test → fix → regression fully automated loop",
		Long: `AI Agent collaborative test → fix → regression fully automated loop:

1. Run tests (orchestrator.Run)
2. If all pass, return immediately
3. If there are failures:
   a. Build a failure context (JSON)
   b. Invoke the AI Agent to analyze and fix
   c. Apply the fix (patch or direct mode)
   d. Re-run tests (regression)
   e. If regression passes, return success; otherwise increment the
      round and go back to step 3
4. If failures remain after MaxRounds, return the final failure result

Agent invocation (one of):
  --agent-cli trae --agent-args "agent fix"
  --agent-endpoint http://localhost:8080/fix

Agent script protocol:
  stdin:  FailureContext JSON
  stdout: FixResult JSON {"fixed":true,"mode":"patch","patch":"...","message":"..."}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCopilot(cmd, gf, af)
		},
	}

	cmd.Flags().StringVarP(&af.project, "project", "p", "",
		"Project name for this run")
	cmd.Flags().StringArrayVarP(&af.skills, "skill", "s", nil,
		"Run only specified skills (repeatable)")
	cmd.Flags().StringVar(&af.reportsDir, "reports-dir", "",
		"Override the report output directory in config")
	cmd.Flags().DurationVar(&af.timeout, "timeout", 0,
		"Total timeout for this run (0 means no limit)")
	cmd.Flags().IntVar(&af.maxRounds, "max-rounds", 3,
		"Maximum fix rounds (default 3)")
	cmd.Flags().StringVar(&af.agentCLI, "agent-cli", "",
		"AI Agent command line (e.g. trae), exchanges JSON via stdin/stdout")
	cmd.Flags().StringArrayVar(&af.agentArgs, "agent-args", nil,
		"AI Agent command args (repeatable)")
	cmd.Flags().StringVar(&af.agentEndpoint, "agent-endpoint", "",
		"AI Agent HTTP endpoint (mutually exclusive with --agent-cli)")
	cmd.Flags().StringVar(&af.agentToken, "agent-token", "",
		"Bearer token for HTTP mode")
	cmd.Flags().DurationVar(&af.agentTimeout, "agent-timeout", 5*time.Minute,
		"Single Agent invocation timeout")

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
	_, _ = fmt.Fprintf(out, "\n%s\n", i18n.T("cli.copilot.output.header"))
	_, _ = fmt.Fprintf(out, "%s: %s\n", i18n.T("cli.copilot.output.total_duration"), r.TotalDuration)
	_, _ = fmt.Fprintf(out, "%s:         %d\n", i18n.T("cli.copilot.output.rounds"), len(r.Rounds))
	_, _ = fmt.Fprintf(out, "%s:          %v\n", i18n.T("cli.copilot.output.fixed"), r.Fixed)

	_, _ = fmt.Fprintf(out, "\n%s:\n", i18n.T("cli.copilot.output.final_result"))
	_, _ = fmt.Fprintf(out, "  %s:  %s\n", i18n.T("cli.copilot.output.project"), r.Final.ProjectName)
	_, _ = fmt.Fprintf(out, "  %s:  %s\n", i18n.T("cli.copilot.output.baseurl"), r.Final.BaseURL)
	_, _ = fmt.Fprintf(out, "  %s: %s\n", i18n.T("cli.copilot.output.duration"), r.Final.Duration)
	_, _ = fmt.Fprintf(out, "  %s:  %s\n", i18n.T("cli.copilot.output.summary"),
		i18n.T("cli.copilot.output.summary_format",
			r.Final.Summary.Total, r.Final.Summary.Passed,
			r.Final.Summary.Failed, r.Final.Summary.Skipped))

	if len(r.Rounds) > 0 {
		_, _ = fmt.Fprintf(out, "\n%s:\n", i18n.T("cli.copilot.output.fix_rounds"))
		for _, round := range r.Rounds {
			_, _ = fmt.Fprintf(out, "  %s %d:\n", i18n.T("cli.copilot.output.round"), round.Round)
			_, _ = fmt.Fprintf(out, "    %s:     %d\n", i18n.T("cli.copilot.output.failures"), len(round.FailureContext.FailedCases))
			if round.AgentError != nil {
				_, _ = fmt.Fprintf(out, "    %s:  %v\n", i18n.T("cli.copilot.output.agent_error"), round.AgentError)
				continue
			}
			if round.FixResult != nil {
				_, _ = fmt.Fprintf(out, "    %s:        %v\n", i18n.T("cli.copilot.output.fixed_label"), round.FixResult.Fixed)
				_, _ = fmt.Fprintf(out, "    %s:         %s\n", i18n.T("cli.copilot.output.mode"), round.FixResult.Mode)
				_, _ = fmt.Fprintf(out, "    %s:      %s\n", i18n.T("cli.copilot.output.message"), round.FixResult.Message)
				if round.FixResult.Analysis != "" {
					_, _ = fmt.Fprintf(out, "    %s:     %s\n", i18n.T("cli.copilot.output.analysis"), round.FixResult.Analysis)
				}
			}
			if round.ApplyError != nil {
				_, _ = fmt.Fprintf(out, "    %s:  %v\n", i18n.T("cli.copilot.output.apply_error"), round.ApplyError)
			}
		}
	}
	_, _ = fmt.Fprintln(out)
}
