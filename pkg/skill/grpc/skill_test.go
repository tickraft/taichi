package grpc

import (
	"testing"

	"github.com/tickraft/taichi/pkg/skill"
)

// TestSkill_Configure parses a representative raw config and verifies the resulting fields.
func TestSkill_Configure(t *testing.T) {
	s := &Skill{}
	raw := map[string]any{
		"target":   "127.0.0.1:9090",
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name":            "health-default",
				"type":            "health",
				"expected_status": "SERVING",
				"max_latency":     "2s",
			},
			map[string]any{
				"name":   "dial-explicit",
				"type":   "dial",
				"target": "10.0.0.1:9090",
			},
			map[string]any{
				"name":              "reflect-services",
				"type":              "reflect",
				"expected_services": []any{"myapp.UserService", "myapp.OrderService"},
			},
			map[string]any{
				// name missing → should be skipped.
				"type": "health",
			},
		},
	}
	if err := s.Configure(skill.Config{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if s.target != "127.0.0.1:9090" {
		t.Errorf("target = %q, want 127.0.0.1:9090", s.target)
	}
	if !s.insecure {
		t.Errorf("insecure = false, want true")
	}
	if s.timeout.Seconds() != 3 {
		t.Errorf("timeout = %v, want 3s", s.timeout)
	}
	// 3 valid cases (the 4th has no name and must be skipped).
	if len(s.cases) != 3 {
		t.Fatalf("cases count = %d, want 3", len(s.cases))
	}

	// Case 0: health with default target.
	c0 := s.cases[0]
	if c0.Name != "health-default" || c0.Type != CaseHealth {
		t.Errorf("case0: name=%q type=%q", c0.Name, c0.Type)
	}
	if c0.Target != "127.0.0.1:9090" {
		t.Errorf("case0 target = %q, want default 127.0.0.1:9090", c0.Target)
	}
	if c0.ExpectedStatus != "SERVING" {
		t.Errorf("case0 expected_status = %q, want SERVING", c0.ExpectedStatus)
	}
	if c0.MaxLatency != "2s" {
		t.Errorf("case0 max_latency = %q, want 2s", c0.MaxLatency)
	}

	// Case 1: dial with explicit target override.
	c1 := s.cases[1]
	if c1.Name != "dial-explicit" || c1.Type != CaseDial {
		t.Errorf("case1: name=%q type=%q", c1.Name, c1.Type)
	}
	if c1.Target != "10.0.0.1:9090" {
		t.Errorf("case1 target = %q, want 10.0.0.1:9090", c1.Target)
	}

	// Case 2: reflect with expected services.
	c2 := s.cases[2]
	if c2.Name != "reflect-services" || c2.Type != CaseReflect {
		t.Errorf("case2: name=%q type=%q", c2.Name, c2.Type)
	}
	if len(c2.ExpectedServices) != 2 {
		t.Fatalf("case2 expected_services len = %d, want 2", len(c2.ExpectedServices))
	}
	if c2.ExpectedServices[0] != "myapp.UserService" || c2.ExpectedServices[1] != "myapp.OrderService" {
		t.Errorf("case2 expected_services = %v", c2.ExpectedServices)
	}
}

// TestSkill_ConfigureEmpty ensures Configure tolerates a nil raw map.
func TestSkill_ConfigureEmpty(t *testing.T) {
	s := &Skill{}
	if err := s.Configure(skill.Config{}); err != nil {
		t.Fatalf("Configure with empty raw: %v", err)
	}
	if len(s.cases) != 0 {
		t.Errorf("expected 0 cases, got %d", len(s.cases))
	}
}

// TestSkill_NameKind verifies the identity methods.
func TestSkill_NameKind(t *testing.T) {
	s := &Skill{}
	if s.Name() != "grpc" {
		t.Errorf("Name = %q, want grpc", s.Name())
	}
	if s.Kind() != skill.KindGRPC {
		t.Errorf("Kind = %q, want %q", s.Kind(), skill.KindGRPC)
	}
}
