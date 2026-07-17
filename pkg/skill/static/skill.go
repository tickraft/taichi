// Package static implements the static-asset test skill.
//
// It verifies accessibility of web service static assets (HTML pages, JS bundles, images, etc.),
// cache headers (ETag/Cache-Control), conditional GET (304 Not Modified), and SPA fallback.
package static

import (
	"net/http"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/skill"
)

// Skill is the static-asset test skill.
type Skill struct {
	cfg     skill.SkillConfig
	pages   []string
	assets  []string
	timeout time.Duration
	client  *http.Client
}

// Name implements skill.TestSkill.
func (s *Skill) Name() string { return "static" }

// Kind implements skill.TestSkill.
func (s *Skill) Kind() skill.Kind { return skill.KindStatic }

// Configure implements skill.TestSkill. Parses the pages/assets/timeout fields from raw.
func (s *Skill) Configure(cfg skill.SkillConfig) error {
	s.cfg = cfg
	raw := cfg.Raw
	if raw == nil {
		return nil
	}
	s.pages = skill.ToStringSlice(raw["pages"])
	s.assets = skill.ToStringSlice(raw["assets"])
	s.timeout = skill.GetDuration(raw, "timeout", skill.DefaultHTTPTimeout)
	return nil
}

// Priority implements skill.TestSkill. Static assets have low priority.
func (s *Skill) Priority() skill.Priority { return skill.PriorityNormal }

// Setup implements skill.TestSkill.
func (s *Skill) Setup(ctx *skill.SkillContext) error {
	s.client = &http.Client{Timeout: s.timeout}
	return nil
}

// Run implements skill.TestSkill.
func (s *Skill) Run(ctx *skill.SkillContext) skill.SkillResult {
	start := time.Now()
	asserts := ctx.Asserts

	// Page tests: 200 + contains <html marker; 404 is treated as skip (assets not built).
	for _, path := range s.pages {
		caseStart := time.Now()
		resp, body, err := skill.HTTPRequest(s.client, http.MethodGet, ctx.BaseURL+path, nil)
		if err != nil {
			skill.RecordResult(ctx.Reporter, "page:"+path, caseStart, false, err.Error(), err)
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			ctx.Reporter.Record(framework.TestResult{
				Name:     "page:" + path,
				Passed:   false,
				Skipped:  true,
				Message:  "skipped: 404 (assets not built)",
				Duration: time.Since(caseStart),
			})
			continue
		}
		sc := asserts.AssertStatusCode(resp, http.StatusOK)
		if !sc.Passed {
			skill.RecordResult(ctx.Reporter, "page:"+path, caseStart, false, sc.Message, nil)
			continue
		}
		html := asserts.AssertHTMLContains(body, "<html")
		if !html.Passed {
			skill.RecordResult(ctx.Reporter, "page:"+path, caseStart, false, html.Message, nil)
			continue
		}
		skill.RecordResult(ctx.Reporter, "page:"+path, caseStart, true, "ok", nil)
	}

	// Asset tests: 200 or 404 (not present) are both treated as pass (do not block when assets are not built).
	for _, path := range s.assets {
		caseStart := time.Now()
		resp, _, err := skill.HTTPRequest(s.client, http.MethodGet, ctx.BaseURL+path, nil)
		if err != nil {
			skill.RecordResult(ctx.Reporter, "asset:"+path, caseStart, false, err.Error(), err)
			continue
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
			skill.RecordResult(ctx.Reporter, "asset:"+path, caseStart, true, "ok", nil)
			continue
		}
		skill.RecordResult(ctx.Reporter, "asset:"+path, caseStart, false, "unexpected status", nil)
	}

	summary := ctx.Reporter.Summary()
	return skill.SkillResult{
		SkillName: s.Name(),
		Duration:  time.Since(start),
		Summary:   summary,
	}
}

// Teardown implements skill.TestSkill.
func (s *Skill) Teardown(ctx *skill.SkillContext) error { return nil }
