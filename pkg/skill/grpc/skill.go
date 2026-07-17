// Package grpc implements the gRPC test skill.
//
// It performs config-driven gRPC smoke checks against a target service, covering
// the three most common readiness scenarios:
//   - health:    calls grpc.health.v1.Health/Check and asserts the serving status.
//   - dial:      establishes a connection and asserts the underlying connectivity state.
//   - reflect:   queries grpc.reflection.v1.ServerReflection and asserts that the
//     expected service names are exposed.
//
// For full unary/streaming RPC testing with compiled protobuf stubs, use the
// third-party plugin skill (kind: plugin) and write a small Go helper that imports
// the generated client package. This skill intentionally avoids dynamic protobuf
// message construction to keep the dependency surface minimal.
package grpc

import (
	"time"

	"github.com/tickraft/taichi/pkg/skill"
)

// CaseType enumerates the supported gRPC case kinds.
type CaseType string

const (
	// CaseHealth performs a grpc.health.v1 Health/Check call.
	CaseHealth CaseType = "health"
	// CaseDial verifies that a connection can be established and reaches the expected state.
	CaseDial CaseType = "dial"
	// CaseReflect queries server reflection and verifies exposed services.
	CaseReflect CaseType = "reflect"
)

// Skill is the gRPC test skill.
type Skill struct {
	cfg     skill.SkillConfig
	cases   []grpcCase
	timeout time.Duration
	// target is the default host:port derived from the first case or the skill config.
	// Per-case target overrides take precedence.
	target string
	// insecure controls TLS. When true, plaintext (h2c) is used. Default true.
	insecure bool
}

// grpcCase is the configuration of a single gRPC case.
type grpcCase struct {
	Name     string   `mapstructure:"name"`
	Type     CaseType `mapstructure:"type"`
	Target   string   `mapstructure:"target"`
	Insecure *bool    `mapstructure:"insecure"`
	// Health-specific: expected serving status string. One of SERVING, NOT_SERVING,
	// UNKNOWN, SERVICE_UNKNOWN. Empty defaults to SERVING.
	ExpectedStatus string `mapstructure:"expected_status"`
	// Reflect-specific: list of service names (fully-qualified) that must be present.
	ExpectedServices []string `mapstructure:"expected_services"`
	// MaxLatency caps the case duration; empty means no latency assertion.
	MaxLatency string `mapstructure:"max_latency"`
}

// Name implements skill.TestSkill.
func (s *Skill) Name() string { return "grpc" }

// Kind implements skill.TestSkill.
func (s *Skill) Kind() skill.Kind { return skill.KindGRPC }

// Configure implements skill.TestSkill. Parses cases, target, insecure, timeout from raw.
func (s *Skill) Configure(cfg skill.SkillConfig) error {
	s.cfg = cfg
	raw := cfg.Raw
	if raw == nil {
		return nil
	}
	s.target = skill.GetString(raw, "target", "")
	s.insecure = skill.GetBool(raw, "insecure", true)
	s.timeout = skill.GetDuration(raw, "timeout", skill.DefaultHTTPTimeout)

	if v, ok := raw["cases"]; ok {
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				c := grpcCase{
					Name:             skill.GetString(m, "name", ""),
					Type:             CaseType(skill.GetString(m, "type", string(CaseHealth))),
					Target:           skill.GetString(m, "target", ""),
					ExpectedStatus:   skill.GetString(m, "expected_status", ""),
					ExpectedServices: skill.ToStringSlice(m["expected_services"]),
					MaxLatency:       skill.GetString(m, "max_latency", ""),
				}
				if c.Name == "" {
					continue
				}
				if c.Target == "" {
					c.Target = s.target
				}
				s.cases = append(s.cases, c)
			}
		}
	}
	return nil
}

// Priority implements skill.TestSkill. gRPC checks are critical-path smoke tests.
func (s *Skill) Priority() skill.Priority { return skill.PriorityCritical }

// Setup implements skill.TestSkill. No persistent resources are needed.
func (s *Skill) Setup(ctx *skill.SkillContext) error {
	return nil
}

// Run implements skill.TestSkill.
func (s *Skill) Run(ctx *skill.SkillContext) skill.SkillResult {
	start := time.Now()
	for _, c := range s.cases {
		caseStart := time.Now()
		msg, err := s.runCase(ctx, c, caseStart)
		passed := err == nil
		skill.RecordResult(ctx.Reporter, c.Name, caseStart, passed, msg, err)
	}
	summary := ctx.Reporter.Summary()
	return skill.SkillResult{
		SkillName: s.Name(),
		Duration:  time.Since(start),
		Summary:   summary,
	}
}

// Teardown implements skill.TestSkill.
func (s *Skill) Teardown(ctx *skill.SkillContext) error {
	return nil
}
