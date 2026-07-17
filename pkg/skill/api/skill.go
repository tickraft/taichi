// Package api implements the RESTful API test skill.
//
// It defines a set of HTTP endpoint cases via the config file, executes them one by one,
// and verifies status codes, response fields, business codes (code/msg/request_id unified
// response contract), and response latency.
package api

import (
	"net/http"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/skill"
)

// Skill is the API test skill.
type Skill struct {
	cfg     skill.Config
	cases   []apiCase
	client  *http.Client
	timeout time.Duration
}

// apiCase is the configuration of a single API case.
type apiCase struct {
	Name           string            `mapstructure:"name"`
	Method         string            `mapstructure:"method"`
	Path           string            `mapstructure:"path"`
	Headers        map[string]string `mapstructure:"headers"`
	ExpectedStatus int               `mapstructure:"expected_status"`
	ExpectedCode   int               `mapstructure:"expected_code"`
	ExpectedField  string            `mapstructure:"expected_field"`
	ExpectedValue  any               `mapstructure:"expected_value"`
	MaxLatency     string            `mapstructure:"max_latency"`
}

// Name implements skill.TestSkill.
func (s *Skill) Name() string { return "api" }

// Kind implements skill.TestSkill.
func (s *Skill) Kind() skill.Kind { return skill.KindAPI }

// Configure implements skill.TestSkill. Parses the cases and timeout fields from raw.
func (s *Skill) Configure(cfg skill.Config) error {
	s.cfg = cfg
	raw := cfg.Raw
	if raw == nil {
		return nil
	}
	if v, ok := raw["cases"]; ok {
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				c := apiCase{
					Name:           skill.GetString(m, "name", ""),
					Method:         skill.GetString(m, "method", http.MethodGet),
					Path:           skill.GetString(m, "path", ""),
					Headers:        skill.ToStringMap(m["headers"]),
					ExpectedStatus: skill.GetInt(m, "expected_status", http.StatusOK),
					ExpectedCode:   skill.GetInt(m, "expected_code", 0),
					ExpectedField:  skill.GetString(m, "expected_field", ""),
					ExpectedValue:  m["expected_value"],
					MaxLatency:     skill.GetString(m, "max_latency", ""),
				}
				if c.Name == "" || c.Path == "" {
					continue
				}
				s.cases = append(s.cases, c)
			}
		}
	}
	s.timeout = skill.GetDuration(raw, "timeout", skill.DefaultHTTPTimeout)
	return nil
}

// Priority implements skill.TestSkill. API has the highest priority.
func (s *Skill) Priority() skill.Priority { return skill.PriorityCritical }

// Setup implements skill.TestSkill. Creates the HTTP client.
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
		url := ctx.BaseURL + c.Path
		resp, body, err := skill.HTTPRequest(s.client, c.Method, url, c.Headers)
		if err != nil {
			skill.RecordResult(ctx.Reporter, c.Name, caseStart, false, err.Error(), err)
			continue
		}
		sc := asserts.AssertStatusCode(resp, c.ExpectedStatus)
		if !sc.Passed {
			skill.RecordResult(ctx.Reporter, c.Name, caseStart, false, sc.Message, nil)
			continue
		}
		// Verify the unified response contract.
		if c.ExpectedCode != 0 || hasEnvelope(body) {
			fields, code := skill.AssertCommonEnvelope(asserts, body, c.ExpectedCode)
			if !fields.Passed {
				skill.RecordResult(ctx.Reporter, c.Name, caseStart, false, fields.Message, nil)
				continue
			}
			if !code.Passed {
				skill.RecordResult(ctx.Reporter, c.Name, caseStart, false, code.Message, nil)
				continue
			}
		}
		// Verify the specified field value.
		if c.ExpectedField != "" {
			fp := asserts.AssertJSONPath(body, c.ExpectedField, c.ExpectedValue)
			if !fp.Passed {
				skill.RecordResult(ctx.Reporter, c.Name, caseStart, false, fp.Message, nil)
				continue
			}
		}
		// Verify the response latency.
		if c.MaxLatency != "" {
			max, perr := time.ParseDuration(c.MaxLatency)
			if perr == nil {
				rt := asserts.AssertResponseTime(time.Since(caseStart), max)
				if !rt.Passed {
					skill.RecordResult(ctx.Reporter, c.Name, caseStart, false, rt.Message, nil)
					continue
				}
			}
		}
		skill.RecordResult(ctx.Reporter, c.Name, caseStart, true, "ok", nil)
	}
	summary := ctx.Reporter.Summary()
	return skill.Result{
		SkillName: s.Name(),
		Duration:  time.Since(start),
		Summary:   summary,
	}
}

// Teardown implements skill.TestSkill.
func (s *Skill) Teardown(ctx *skill.Context) error {
	return nil
}

// hasEnvelope roughly checks whether body is in the unified response format (contains a code field).
func hasEnvelope(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	return framework.NewAssertionEngine().AssertJSONFieldsExist(body, "code").Passed
}
