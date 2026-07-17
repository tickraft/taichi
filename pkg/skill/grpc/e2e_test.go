package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/tickraft/taichi/pkg/framework"
	"github.com/tickraft/taichi/pkg/skill"
)

// runSkillE2E is a helper that builds a Skill from raw config, creates a fresh
// SkillContext, runs the skill, and returns the reporter snapshot + skill result.
//
// Each invocation gets its own reporter so cases from one test do not leak into another.
func runSkillE2E(t *testing.T, raw map[string]any) ([]framework.TestResult, skill.SkillResult) {
	t.Helper()

	s := &Skill{}
	if err := s.Configure(skill.SkillConfig{Raw: raw}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	reporter := framework.NewTestReporter()
	ctx := &skill.SkillContext{
		Ctx:         context.Background(),
		ProjectName: "grpc-e2e",
		Asserts:     framework.NewAssertionEngine(),
		Reporter:    reporter,
		Logger:      skill.NoOpLogger{},
		Extra:       make(map[string]any),
	}

	if err := s.Setup(ctx); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	result := s.Run(ctx)
	_ = s.Teardown(ctx)

	return reporter.Snapshot(), result
}

// TestE2E_HealthServing verifies that a health case expecting SERVING passes
// against the test server (which is set to SERVING).
func TestE2E_HealthServing(t *testing.T) {
	results, sr := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name":            "HealthServing",
				"type":            "health",
				"expected_status": "SERVING",
			},
		},
	})

	if sr.Summary.Total != 1 || sr.Summary.Passed != 1 {
		t.Fatalf("summary: total=%d passed=%d failed=%d, want 1/1/0",
			sr.Summary.Total, sr.Summary.Passed, sr.Summary.Failed)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("case should pass: %+v", results)
	}
}

// TestE2E_HealthWrongStatus verifies that a health case expecting NOT_SERVING
// fails when the server is actually SERVING.
func TestE2E_HealthWrongStatus(t *testing.T) {
	results, sr := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name":            "HealthNotServing",
				"type":            "health",
				"expected_status": "NOT_SERVING",
			},
		},
	})

	if sr.Summary.Failed != 1 {
		t.Fatalf("summary: total=%d passed=%d failed=%d, want failed=1",
			sr.Summary.Total, sr.Summary.Passed, sr.Summary.Failed)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("case should fail: %+v", results)
	}
}

// TestE2E_Dial verifies that a dial case passes against the reachable test server.
func TestE2E_Dial(t *testing.T) {
	results, sr := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name": "DialReachable",
				"type": "dial",
			},
		},
	})

	if sr.Summary.Passed != 1 {
		t.Fatalf("summary: total=%d passed=%d failed=%d, want passed=1",
			sr.Summary.Total, sr.Summary.Passed, sr.Summary.Failed)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("case should pass: %+v", results)
	}
}

// TestE2E_DialUnreachable verifies that a dial case fails against an unreachable target.
func TestE2E_DialUnreachable(t *testing.T) {
	// Port 1 is reserved and never listens; grpc.Dial with WithBlock will time out.
	results, sr := runSkillE2E(t, map[string]any{
		"target":   "127.0.0.1:1",
		"insecure": true,
		"timeout":  "1s",
		"cases": []any{
			map[string]any{
				"name": "DialUnreachable",
				"type": "dial",
			},
		},
	})

	if sr.Summary.Failed != 1 {
		t.Fatalf("summary: total=%d passed=%d failed=%d, want failed=1",
			sr.Summary.Total, sr.Summary.Passed, sr.Summary.Failed)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("case should fail: %+v", results)
	}
}

// TestE2E_ReflectHealthService verifies that a reflect case passes when querying
// for a service that the test server actually exposes (grpc.health.v1.Health).
func TestE2E_ReflectHealthService(t *testing.T) {
	results, sr := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name": "ReflectHealthService",
				"type": "reflect",
				"expected_services": []any{
					"grpc.health.v1.Health",
				},
			},
		},
	})

	if sr.Summary.Passed != 1 {
		t.Fatalf("summary: total=%d passed=%d failed=%d, want passed=1",
			sr.Summary.Total, sr.Summary.Passed, sr.Summary.Failed)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("case should pass: %+v", results)
	}
}

// TestE2E_ReflectMissingService verifies that a reflect case fails when the
// expected service is not exposed by the server.
func TestE2E_ReflectMissingService(t *testing.T) {
	results, sr := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name": "ReflectMissingService",
				"type": "reflect",
				"expected_services": []any{
					"nonexistent.FakeService",
				},
			},
		},
	})

	if sr.Summary.Failed != 1 {
		t.Fatalf("summary: total=%d passed=%d failed=%d, want failed=1",
			sr.Summary.Total, sr.Summary.Passed, sr.Summary.Failed)
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("case should fail: %+v", results)
	}
}

// TestE2E_MixedCases verifies multiple cases of different types in a single run,
// asserting that pass and fail results are correctly aggregated.
func TestE2E_MixedCases(t *testing.T) {
	results, sr := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name":            "HealthOK",
				"type":            "health",
				"expected_status": "SERVING",
			},
			map[string]any{
				"name": "DialOK",
				"type": "dial",
			},
			map[string]any{
				"name":            "HealthWrong",
				"type":            "health",
				"expected_status": "NOT_SERVING",
			},
			map[string]any{
				"name": "ReflectOK",
				"type": "reflect",
				"expected_services": []any{
					"grpc.health.v1.Health",
				},
			},
		},
	})

	// 3 pass (HealthOK, DialOK, ReflectOK), 1 fail (HealthWrong).
	if sr.Summary.Total != 4 || sr.Summary.Passed != 3 || sr.Summary.Failed != 1 {
		t.Fatalf("summary: total=%d passed=%d failed=%d, want 4/3/1",
			sr.Summary.Total, sr.Summary.Passed, sr.Summary.Failed)
	}
	if len(results) != 4 {
		t.Fatalf("results count = %d, want 4", len(results))
	}

	// Verify each case outcome by name.
	outcomes := make(map[string]bool, len(results))
	for _, r := range results {
		outcomes[r.Name] = r.Passed
	}
	if !outcomes["HealthOK"] {
		t.Error("HealthOK should pass")
	}
	if !outcomes["DialOK"] {
		t.Error("DialOK should pass")
	}
	if outcomes["HealthWrong"] {
		t.Error("HealthWrong should fail")
	}
	if !outcomes["ReflectOK"] {
		t.Error("ReflectOK should pass")
	}
}

// TestE2E_HealthDefaultStatus verifies that a health case without an explicit
// expected_status defaults to SERVING.
func TestE2E_HealthDefaultStatus(t *testing.T) {
	results, sr := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name": "HealthDefaultServing",
				"type": "health",
			},
		},
	})

	if sr.Summary.Passed != 1 {
		t.Fatalf("summary: total=%d passed=%d failed=%d, want passed=1 (default SERVING)",
			sr.Summary.Total, sr.Summary.Passed, sr.Summary.Failed)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("case should pass with default SERVING: %+v", results)
	}
}

// TestE2E_LatencyAssertion verifies that the max_latency assertion works: a
// generous limit passes, while an impossibly small limit fails.
func TestE2E_LatencyAssertion(t *testing.T) {
	// Generous limit: should pass.
	_, srPass := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name":        "DialWithGenerousLatency",
				"type":        "dial",
				"max_latency": "10s",
			},
		},
	})
	if srPass.Summary.Passed != 1 {
		t.Errorf("generous latency: expected pass, got total=%d passed=%d failed=%d",
			srPass.Summary.Total, srPass.Summary.Passed, srPass.Summary.Failed)
	}

	// Impossibly small limit: should fail on latency.
	_, srFail := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name":        "DialWithTightLatency",
				"type":        "dial",
				"max_latency": "1ns",
			},
		},
	})
	if srFail.Summary.Failed != 1 {
		t.Errorf("tight latency: expected fail, got total=%d passed=%d failed=%d",
			srFail.Summary.Total, srFail.Summary.Passed, srFail.Summary.Failed)
	}
}

// TestE2E_DurationRecorded verifies that the skill result duration is non-zero
// and individual case durations are recorded.
func TestE2E_DurationRecorded(t *testing.T) {
	results, sr := runSkillE2E(t, map[string]any{
		"target":   testServerAddr,
		"insecure": true,
		"timeout":  "3s",
		"cases": []any{
			map[string]any{
				"name": "WithDuration",
				"type": "dial",
			},
		},
	})

	if sr.Duration <= 0 {
		t.Errorf("skill duration = %v, want > 0", sr.Duration)
	}
	if len(results) != 1 || results[0].Duration <= 0 {
		t.Errorf("case duration should be recorded: %+v", results)
	}
	// Sanity: a local dial should complete well within 3 seconds.
	if results[0].Duration > 3*time.Second {
		t.Errorf("case duration = %v, want < 3s", results[0].Duration)
	}
}
