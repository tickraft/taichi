// Package regression implements the regression test skill.
//
// After auto-fix or other skills run, it re-probes critical-path endpoints to verify the fix did not introduce regressions.
// The case set is defined via config; unlike the API skill, the regression case set is smaller and more stable, and any failure means the overall regression failed.
package regression

import (
	"net/http"
	"time"

	"github.com/tickraft/taichi/pkg/skill"
)

// Skill is the regression test skill.
type Skill struct {
	cfg     skill.Config
	cases   []regressionCase
	timeout time.Duration
	client  *http.Client
}

// regressionCase is the configuration of a single regression case.
type regressionCase struct {
	Name           string `mapstructure:"name"`
	Path           string `mapstructure:"path"`
	ExpectedStatus int    `mapstructure:"expected_status"`
	ExpectedCode   int    `mapstructure:"expected_code"`
	SkipOn404      bool   `mapstructure:"skip_on_404"`
}

// Name implements skill.TestSkill.
func (s *Skill) Name() string { return "regression" }

// Kind implements skill.TestSkill.
func (s *Skill) Kind() skill.Kind { return skill.KindRegression }

// Configure implements skill.TestSkill.
func (s *Skill) Configure(cfg skill.Config) error {
	s.cfg = cfg
	raw := cfg.Raw
	if raw == nil {
		return nil
	}
	s.timeout = skill.GetDuration(raw, "timeout", skill.DefaultHTTPTimeout)
	if v, ok := raw["cases"]; ok {
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				name := skill.GetString(m, "name", "")
				path := skill.GetString(m, "path", "")
				if name == "" || path == "" {
					continue
				}
				c := regressionCase{
					Name:           name,
					Path:           path,
					ExpectedStatus: skill.GetInt(m, "expected_status", http.StatusOK),
					ExpectedCode:   skill.GetInt(m, "expected_code", 0),
					SkipOn404:      skill.GetBool(m, "skip_on_404", false),
				}
				s.cases = append(s.cases, c)
			}
		}
	}
	return nil
}

// Priority implements skill.TestSkill. Regression has the lowest priority (runs last).
func (s *Skill) Priority() skill.Priority { return skill.PriorityLow }

// Setup implements skill.TestSkill.
func (s *Skill) Setup(ctx *skill.Context) error {
	s.client = &http.Client{Timeout: s.timeout}
	return nil
}

// Run implements skill.TestSkill.
func (s *Skill) Run(ctx *skill.Context) skill.Result {
	start := time.Now()
	asserts := ctx.Asserts
	for _, c := range s.cases {
		caseStart := time.Now()
		resp, body, err := skill.HTTPRequest(s.client, http.MethodGet, ctx.BaseURL+c.Path, nil)
		if err != nil {
			skill.RecordResult(ctx.Reporter, "regression:"+c.Name, caseStart, false, err.Error(), err)
			continue
		}
		if c.SkipOn404 && resp.StatusCode == http.StatusNotFound {
			skill.RecordResult(ctx.Reporter, "regression:"+c.Name, caseStart, true, "skipped: 404 (resource not built)", nil)
			continue
		}
		sc := asserts.AssertStatusCode(resp, c.ExpectedStatus)
		if !sc.Passed {
			skill.RecordResult(ctx.Reporter, "regression:"+c.Name, caseStart, false, sc.Message, nil)
			continue
		}
		// Verify the business code (if configured).
		if c.ExpectedCode != 0 {
			_, code := skill.AssertCommonEnvelope(asserts, body, c.ExpectedCode)
			if !code.Passed {
				skill.RecordResult(ctx.Reporter, "regression:"+c.Name, caseStart, false, code.Message, nil)
				continue
			}
		}
		skill.RecordResult(ctx.Reporter, "regression:"+c.Name, caseStart, true, "ok", nil)
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
