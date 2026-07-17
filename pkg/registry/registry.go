// Package registry provides the test skill registry with dynamic load/unload.
//
// The registry is the core coordination component of the taichi orchestrator:
// skill implementations are registered via Register, and the orchestrator
// filters enabled skills by config, sorts them by priority, and runs
// Setup/Run/Teardown in order.
//
// The registry is concurrency-safe.
package registry

import (
	"fmt"
	"sort"
	"sync"

	"github.com/tickraft/taichi/pkg/skill"
)

// Registry is the skill registry. It maintains the mapping from skill name to
// instance, and supports register, lookup, and unregister.
//
// A zero-value Registry is not usable; use NewRegistry to create one.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]skill.TestSkill
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]skill.TestSkill)}
}

// Register registers s into the registry. If a skill with the same name already
// exists and overwrite=false, an error is returned.
// When overwrite=true, an already-registered skill with the same name is
// silently replaced.
func (r *Registry) Register(s skill.TestSkill, overwrite bool) error {
	if s == nil {
		return fmt.Errorf("register nil skill")
	}
	name := s.Name()
	if name == "" {
		return fmt.Errorf("skill name is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.skills[name]; exists && !overwrite {
		return fmt.Errorf("skill %q already registered", name)
	}
	r.skills[name] = s
	return nil
}

// Unregister unregisters a skill by name. Returns an error if the skill does not exist.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.skills[name]; !exists {
		return fmt.Errorf("skill %q not registered", name)
	}
	delete(r.skills, name)
	return nil
}

// Get returns the skill instance for the given name. Returns an error if not present.
func (r *Registry) Get(name string) (skill.TestSkill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	if !ok {
		return nil, fmt.Errorf("skill %q not registered", name)
	}
	return s, nil
}

// List returns a snapshot list of all registered skills (sorted by name).
func (r *Registry) List() []skill.TestSkill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]skill.TestSkill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}

// Select filters and returns enabled skill instances based on the SkillConfig list,
// sorted by Priority in ascending order.
// Skills not present in the config are not selected; skills present in the config
// but not registered are skipped and recorded in the missing return value.
func (r *Registry) Select(configs []skill.SkillConfig) (selected []skill.TestSkill, missing []string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type candidate struct {
		s        skill.TestSkill
		priority skill.Priority
	}
	cands := make([]candidate, 0, len(configs))
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		s, ok := r.skills[cfg.Name]
		if !ok {
			missing = append(missing, cfg.Name)
			continue
		}
		// When Priority is explicitly set in config (non-zero is treated as explicit), use the config value.
		p := s.Priority()
		if cfg.Priority != 0 {
			p = cfg.Priority
		}
		cands = append(cands, candidate{s: s, priority: p})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].priority < cands[j].priority
	})
	selected = make([]skill.TestSkill, 0, len(cands))
	for _, c := range cands {
		selected = append(selected, c.s)
	}
	return selected, missing
}

// Clear clears all registered skills.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills = make(map[string]skill.TestSkill)
}

// Count returns the number of registered skills.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skills)
}
