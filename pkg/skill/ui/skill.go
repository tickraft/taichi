// Package ui implements the UI / page test skill.
//
// It probes frontend page load performance and basic rendering via HTTP.
// Browser-level E2E tests (Playwright) require extension via external skills;
// this skill only provides HTTP-layer page accessibility, time-to-first-byte, and keyword-existence verification.
package ui

import (
	"net/http"
	"time"

	"github.com/tickraft/taichi/pkg/skill"
)

// Skill is the UI test skill.
type Skill struct {
	cfg     skill.Config
	pages   []pageCase
	timeout time.Duration
	client  *http.Client
}

// pageCase is the configuration of a single page case.
type pageCase struct {
	Path       string   `mapstructure:"path"`
	Contains   []string `mapstructure:"contains"`
	MaxLatency string   `mapstructure:"max_latency"`
}

// Name implements skill.TestSkill.
func (s *Skill) Name() string { return "ui" }

// Kind implements skill.TestSkill.
func (s *Skill) Kind() skill.Kind { return skill.KindUI }

// Configure implements skill.TestSkill.
func (s *Skill) Configure(cfg skill.Config) error {
	s.cfg = cfg
	raw := cfg.Raw
	if raw == nil {
		return nil
	}
	s.timeout = skill.GetDuration(raw, "timeout", skill.DefaultHTTPTimeout)
	if v, ok := raw["pages"]; ok {
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				path := skill.GetString(m, "path", "")
				if path == "" {
					continue
				}
				c := pageCase{
					Path:       path,
					Contains:   skill.ToStringSlice(m["contains"]),
					MaxLatency: skill.GetString(m, "max_latency", ""),
				}
				s.pages = append(s.pages, c)
			}
		}
	}
	return nil
}

// Priority implements skill.TestSkill. UI has lower priority than API.
func (s *Skill) Priority() skill.Priority { return skill.PriorityHigh }

// Setup implements skill.TestSkill.
func (s *Skill) Setup(ctx *skill.Context) error {
	s.client = &http.Client{Timeout: s.timeout}
	return nil
}

// Run implements skill.TestSkill.
func (s *Skill) Run(ctx *skill.Context) skill.Result {
	start := time.Now()
	asserts := ctx.Asserts
	for _, p := range s.pages {
		caseStart := time.Now()
		resp, body, err := skill.HTTPRequest(s.client, http.MethodGet, ctx.BaseURL+p.Path, nil)
		if err != nil {
			skill.RecordResult(ctx.Reporter, "ui:"+p.Path, caseStart, false, err.Error(), err)
			continue
		}
		sc := asserts.AssertStatusCode(resp, http.StatusOK)
		if !sc.Passed {
			skill.RecordResult(ctx.Reporter, "ui:"+p.Path, caseStart, false, sc.Message, nil)
			continue
		}
		if len(p.Contains) > 0 {
			html := asserts.AssertHTMLContains(body, p.Contains...)
			if !html.Passed {
				skill.RecordResult(ctx.Reporter, "ui:"+p.Path, caseStart, false, html.Message, nil)
				continue
			}
		}
		if p.MaxLatency != "" {
			if max, perr := time.ParseDuration(p.MaxLatency); perr == nil {
				rt := asserts.AssertResponseTime(time.Since(caseStart), max)
				if !rt.Passed {
					skill.RecordResult(ctx.Reporter, "ui:"+p.Path, caseStart, false, rt.Message, nil)
					continue
				}
			}
		}
		skill.RecordResult(ctx.Reporter, "ui:"+p.Path, caseStart, true, "ok", nil)
	}
	summary := ctx.Reporter.Summary()
	return skill.Result{
		SkillName: s.Name(),
		Duration:  time.Since(start),
		Summary:   summary,
	}
}

// Teardown implements skill.TestSkill.
func (s *Skill) Teardown(ctx *skill.Context) error { return nil }
