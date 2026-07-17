package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/tickraft/taichi/pkg/config"
	"github.com/tickraft/taichi/pkg/i18n"
	"github.com/tickraft/taichi/pkg/skill"
)

// defaultEncoderConfig is the encoder config for taichi CLI console logs (colored, ISO time).
var defaultEncoderConfig = zapcore.EncoderConfig{
	TimeKey:        "T",
	LevelKey:       "L",
	NameKey:        "N",
	MessageKey:     "M",
	EncodeLevel:    zapcore.CapitalColorLevelEncoder,
	EncodeTime:     zapcore.ISO8601TimeEncoder,
	EncodeDuration: zapcore.StringDurationEncoder,
}

// globalFlags holds the persistent options shared by all subcommands.
type globalFlags struct {
	// configPath is the config file path.
	configPath string
	// logLevel controls log verbosity: debug / info / warn / error.
	logLevel string
	// locale is the UI language: auto / zh-CN / en-US.
	locale string
}

// newRootCmd constructs the taichi root command and its subcommand tree.
func newRootCmd() *cobra.Command {
	gf := &globalFlags{}

	root := &cobra.Command{
		Use:   "taichi",
		Short: "taichi test orchestration framework",
		Long: `taichi is a general-purpose automated test orchestration framework.

It provides a Skill extension mechanism, multi-environment lifecycle
management, auto-fix, and multi-format report output.

Describe the project under test, environments, and skills via a config
file to orchestrate a complete test run. Built-in skills: API / gRPC /
UI / Static / Regression. Register custom skills by implementing the
pkg/skill.TestSkill interface.

taichi supports bidirectional integration with AI Agents (e.g. Trae IDE):
  - Acts as an MCP Server exposing taichi tools to AI Agents
  - In copilot mode, invokes an AI Agent for code fixes, completing a
    test → fix → regression loop

Quick start:
  taichi run --config configs/taichi.yaml
  taichi list --config configs/taichi.yaml
  taichi mcp --config configs/taichi.yaml
  taichi copilot --config configs/taichi.yaml --agent-cli trae
  taichi version
`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Apply the --locale flag (default auto triggers system environment detection).
			// Note: the config file is not loaded yet; if --locale is auto and the config
			// file specifies a locale, the subcommand will call applyConfigLocale to
			// override it after loading the config.
			i18n.SetLocale(i18n.ParseLocale(gf.locale))
			return nil
		},
	}

	root.PersistentFlags().StringVarP(&gf.configPath, "config", "c", "configs/taichi.yaml",
		"Config file path (YAML)")
	root.PersistentFlags().StringVar(&gf.logLevel, "log-level", "info",
		"Log level: debug / info / warn / error")
	root.PersistentFlags().StringVar(&gf.locale, "locale", "auto",
		"UI language: auto / zh-CN / en-US (auto detects from system environment)")

	root.AddCommand(
		newRunCmd(gf),
		newListCmd(gf),
		newValidateCmd(gf),
		newVersionCmd(),
		newMCPCmd(gf),
		newCopilotCmd(gf),
	)
	return root
}

// applyConfigLocale is called after a subcommand loads the config.
//
// Locale resolution priority:
//  1. --locale flag explicitly set (highest)
//  2. config file locale field
//  3. system environment variable auto-detection (default)
//
// If --locale is explicitly set, this function does nothing; otherwise it
// applies the locale from the config.
func applyConfigLocale(cmd *cobra.Command, gf *globalFlags, cfg *config.Config) {
	if cmd.Flags().Changed("locale") {
		return
	}
	if cfg.Locale != "" {
		i18n.SetLocale(i18n.ParseLocale(cfg.Locale))
	}
}

// preloadLocale preloads the locale for subcommands that do not load the config
// directly (e.g. run / copilot).
//
// These subcommands delegate config loading to orchestrator.Run(), but the
// locale must take effect before orchestration starts. This function loads the
// config with minimal overhead to read only the locale field.
// Load failures are silently ignored (orchestrator will load again and report errors).
func preloadLocale(cmd *cobra.Command, gf *globalFlags) {
	if cmd.Flags().Changed("locale") {
		return
	}
	cfg, err := config.Load(gf.configPath)
	if err != nil {
		return
	}
	if cfg.Locale != "" {
		i18n.SetLocale(i18n.ParseLocale(cfg.Locale))
	}
}

// newLogger builds a skill.Logger implementation based on logLevel (backed by zap).
// Falls back to info level on parse failure. Logs are written to stderr.
func newLogger(level string) (skill.Logger, func(), error) {
	var zapLevel zap.AtomicLevel
	switch level {
	case "debug":
		zapLevel = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapLevel = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapLevel = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	logger, err := zap.Config{
		Level:            zapLevel,
		Development:      false,
		Encoding:         "console",
		EncoderConfig:    defaultEncoderConfig,
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}.Build()
	if err != nil {
		return nil, nil, fmt.Errorf("build logger: %w", err)
	}
	sugared := logger.Sugar()
	cleanup := func() { _ = sugared.Sync() }
	return zapLogger{sugared: sugared}, cleanup, nil
}

// zapLogger adapts *zap.SugaredLogger to the skill.Logger interface.
type zapLogger struct {
	sugared *zap.SugaredLogger
}

// Infof implements skill.Logger.
func (z zapLogger) Infof(format string, args ...any) { z.sugared.Infof(format, args...) }

// Warnf implements skill.Logger.
func (z zapLogger) Warnf(format string, args ...any) { z.sugared.Warnf(format, args...) }

// Errorf implements skill.Logger.
func (z zapLogger) Errorf(format string, args ...any) { z.sugared.Errorf(format, args...) }
