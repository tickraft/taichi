// Package builtin provides the single source of truth for taichi's built-in
// test skill instances. Both the taichi CLI (cmd/taichi) and the MCP Server
// (pkg/mcp) MUST use BuiltinSkills to avoid divergence (a previous bug where
// the MCP copy omitted the grpc skill caused taichi_run to skip gRPC tests).
package builtin

import (
	"github.com/tickraft/taichi/pkg/skill"
	api "github.com/tickraft/taichi/pkg/skill/api"
	"github.com/tickraft/taichi/pkg/skill/grpc"
	"github.com/tickraft/taichi/pkg/skill/regression"
	"github.com/tickraft/taichi/pkg/skill/static"
	"github.com/tickraft/taichi/pkg/skill/ui"
)

// Skills returns the canonical list of taichi built-in test skill
// instances, in registration order: api, grpc, ui, static, regression.
// Callers register them via orchestrator.RegisterBuiltinSkills.
func Skills() []skill.TestSkill {
	return []skill.TestSkill{
		&api.Skill{},
		&grpc.Skill{},
		&ui.Skill{},
		&static.Skill{},
		&regression.Skill{},
	}
}
