package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/tickraft/taichi/pkg/i18n"
	"github.com/tickraft/taichi/pkg/orchestrator"
	"github.com/tickraft/taichi/pkg/skill"
	"github.com/tickraft/taichi/pkg/skill/builtin"
)

// runFlags holds the local options of the run subcommand.
type runFlags struct {
	// project specifies the project name for this run. Empty means the first project in config.
	project string
	// skills restricts this run to the specified skills (repeatable).
	skills []string
	// reportsDir overrides the report output directory in config.
	reportsDir string
	// timeout is the total timeout for this run. 0 means no limit.
	timeout time.Duration
}

// newRunCmd constructs the run subcommand.
func newRunCmd(gf *globalFlags) *cobra.Command {
	rf := &runFlags{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a test orchestration according to the config file",
		Long: `Loads the config file, starts the environment for the project under test,
runs enabled skills in priority order, collects results, and generates
JSON / JUnit XML / human-readable summary reports.

Exit code: 0 if all pass; 1 if any failure or runtime error occurs.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOrchestrator(cmd, gf, rf)
		},
	}

	cmd.Flags().StringVarP(&rf.project, "project", "p", "",
		"Project name for this run (defaults to the first project in config)")
	cmd.Flags().StringArrayVarP(&rf.skills, "skill", "s", nil,
		"Run only specified skills (repeatable, e.g. -s api -s ui)")
	cmd.Flags().StringVar(&rf.reportsDir, "reports-dir", "",
		"Override the report output directory in config")
	cmd.Flags().DurationVar(&rf.timeout, "timeout", 0,
		"Total timeout for this run (0 means no limit)")

	return cmd
}

// runOrchestrator executes the orchestration logic: builds the logger, registers
// builtin skills, runs the orchestration, and prints the summary.
func runOrchestrator(cmd *cobra.Command, gf *globalFlags, rf *runFlags) error {
	preloadLocale(cmd, gf)

	logger, logCleanup, err := newLogger(gf.logLevel)
	if err != nil {
		return err
	}
	defer logCleanup()

	ctx := context.Background()
	if rf.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rf.timeout)
		defer cancel()
	} else {
		// Listen for SIGINT / SIGTERM for graceful cancellation.
		ctx = withSignalCancel(ctx, logger)
	}

	o := orchestrator.New()
	if err := o.RegisterBuiltinSkills(builtin.Skills()); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("cli.run.error.register_builtin"), err)
	}

	result, err := o.Run(ctx, orchestrator.Options{
		ConfigPath:  gf.configPath,
		ProjectName: rf.project,
		SkillFilter: rf.skills,
		ReportsDir:  rf.reportsDir,
		Logger:      logger,
	})
	if err != nil {
		return err
	}

	printRunResult(cmd, result)
	// Exit with a non-zero code when there are failed cases.
	if result.Summary.Failed > 0 {
		return errors.New(i18n.T("cli.run.output.failed_count", result.Summary.Failed))
	}
	return nil
}

// printRunResult writes the run summary to stdout.
func printRunResult(cmd *cobra.Command, r orchestrator.Result) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "\n%s\n", i18n.T("cli.run.output.header"))
	_, _ = fmt.Fprintf(out, "%s:  %s\n", i18n.T("cli.run.output.project"), r.ProjectName)
	_, _ = fmt.Fprintf(out, "%s:  %s\n", i18n.T("cli.run.output.baseurl"), r.BaseURL)
	_, _ = fmt.Fprintf(out, "%s: %s\n", i18n.T("cli.run.output.duration"), r.Duration)
	_, _ = fmt.Fprintf(out, "%s:  %s\n", i18n.T("cli.run.output.summary"),
		i18n.T("cli.run.output.summary_format", r.Summary.Total, r.Summary.Passed, r.Summary.Failed, r.Summary.Skipped))
	if r.EnvLogPath != "" {
		_, _ = fmt.Fprintf(out, "%s:   %s\n", i18n.T("cli.run.output.envlog"), r.EnvLogPath)
	}
	if len(r.SkillResults) > 0 {
		_, _ = fmt.Fprintf(out, "\n%s:\n", i18n.T("cli.run.output.skills"))
		for _, sr := range r.SkillResults {
			status := "OK"
			if sr.Error != nil {
				status = "ERROR"
			} else if sr.Summary.Failed > 0 {
				status = "FAIL"
			}
			_, _ = fmt.Fprintf(out, "  - %-12s %-6s %s (total=%d passed=%d failed=%d skipped=%d)\n",
				sr.SkillName, status, sr.Duration,
				sr.Summary.Total, sr.Summary.Passed, sr.Summary.Failed, sr.Summary.Skipped)
		}
	}
	_, _ = fmt.Fprintln(out)
}

// withSignalCancel returns a context that is cancelled on SIGINT / SIGTERM.
func withSignalCancel(ctx context.Context, logger skill.Logger) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Warnf("received signal %s, cancelling...", sig)
		cancel()
	}()
	return ctx
}
