package registry

import (
	"fmt"
	"sync"
	"testing"

	"github.com/tickraft/taichi/pkg/skill"
)

// mockSkill is a configurable TestSkill stub used in registry tests.
// Configure/Setup/Teardown return nil; Run returns an empty SkillResult.
type mockSkill struct {
	name     string
	kind     skill.Kind
	priority skill.Priority
}

func (m *mockSkill) Name() string                 { return m.name }
func (m *mockSkill) Kind() skill.Kind             { return m.kind }
func (m *mockSkill) Priority() skill.Priority     { return m.priority }
func (m *mockSkill) Configure(skill.Config) error { return nil }
func (m *mockSkill) Setup(*skill.Context) error   { return nil }

func (m *mockSkill) Run(*skill.Context) skill.Result {
	return skill.Result{}
}

func (m *mockSkill) Teardown(*skill.Context) error { return nil }

// namesOfSkills is a small helper that extracts the Name() of each skill into a slice.
func namesOfSkills(in []skill.TestSkill) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, s.Name())
	}
	return out
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if got := r.Count(); got != 0 {
		t.Fatalf("Count() = %d, want 0", got)
	}
}

func TestRegister(t *testing.T) {
	r := NewRegistry()
	s := &mockSkill{name: "alpha", kind: skill.KindAPI, priority: skill.PriorityNormal}
	if err := r.Register(s, false); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	got, err := r.Get("alpha")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != s {
		t.Fatalf("Get returned %v, want %v", got, s)
	}
	if r.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", r.Count())
	}
}

func TestRegisterNil(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil, false); err == nil {
		t.Fatalf("Register(nil) expected error, got nil")
	}
	if r.Count() != 0 {
		t.Fatalf("Count() = %d, want 0 after failed Register(nil)", r.Count())
	}
}

func TestRegisterEmptyName(t *testing.T) {
	r := NewRegistry()
	s := &mockSkill{name: ""}
	if err := r.Register(s, false); err == nil {
		t.Fatalf("Register with empty name expected error, got nil")
	}
	if r.Count() != 0 {
		t.Fatalf("Count() = %d, want 0 after failed Register(empty name)", r.Count())
	}
}

func TestRegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	s1 := &mockSkill{name: "alpha", priority: skill.PriorityNormal}
	s2 := &mockSkill{name: "alpha", priority: skill.PriorityHigh}
	if err := r.Register(s1, false); err != nil {
		t.Fatalf("Register s1 returned error: %v", err)
	}
	if err := r.Register(s2, false); err == nil {
		t.Fatalf("Register s2 duplicate expected error, got nil")
	}
	// The original skill must remain registered.
	got, err := r.Get("alpha")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != s1 {
		t.Fatalf("After failed duplicate, Get returned %v, want %v", got, s1)
	}
	if r.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", r.Count())
	}
}

func TestRegisterOverwrite(t *testing.T) {
	r := NewRegistry()
	s1 := &mockSkill{name: "alpha", priority: skill.PriorityNormal}
	s2 := &mockSkill{name: "alpha", priority: skill.PriorityHigh}
	if err := r.Register(s1, false); err != nil {
		t.Fatalf("Register s1 returned error: %v", err)
	}
	if err := r.Register(s2, true); err != nil {
		t.Fatalf("Register s2 with overwrite returned error: %v", err)
	}
	got, err := r.Get("alpha")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got != s2 {
		t.Fatalf("After overwrite, Get returned %v, want %v", got, s2)
	}
	if r.Count() != 1 {
		t.Fatalf("Count() = %d, want 1 after overwrite", r.Count())
	}
}

func TestUnregister(t *testing.T) {
	r := NewRegistry()
	s := &mockSkill{name: "alpha"}
	if err := r.Register(s, false); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if err := r.Unregister("alpha"); err != nil {
		t.Fatalf("Unregister returned error: %v", err)
	}
	if r.Count() != 0 {
		t.Fatalf("Count() = %d, want 0 after Unregister", r.Count())
	}
	if _, err := r.Get("alpha"); err == nil {
		t.Fatalf("Get after Unregister expected error, got nil")
	}
}

func TestUnregisterMissing(t *testing.T) {
	r := NewRegistry()
	if err := r.Unregister("nope"); err == nil {
		t.Fatalf("Unregister non-existent expected error, got nil")
	}
}

func TestGet(t *testing.T) {
	r := NewRegistry()
	s := &mockSkill{name: "alpha"}
	if err := r.Register(s, false); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	got, err := r.Get("alpha")
	if err != nil {
		t.Fatalf("Get existing returned error: %v", err)
	}
	if got != s {
		t.Fatalf("Get returned %v, want %v", got, s)
	}
	if _, err := r.Get("missing"); err == nil {
		t.Fatalf("Get non-existing expected error, got nil")
	}
}

func TestList(t *testing.T) {
	r := NewRegistry()
	// Intentionally register in non-sorted order.
	a := &mockSkill{name: "c-skill"}
	b := &mockSkill{name: "a-skill"}
	c := &mockSkill{name: "b-skill"}
	for _, s := range []*mockSkill{a, b, c} {
		if err := r.Register(s, false); err != nil {
			t.Fatalf("Register returned error: %v", err)
		}
	}
	list := r.List()
	if len(list) != 3 {
		t.Fatalf("List len = %d, want 3", len(list))
	}
	want := []string{"a-skill", "b-skill", "c-skill"}
	for i, w := range want {
		if list[i].Name() != w {
			t.Fatalf("List[%d].Name() = %q, want %q", i, list[i].Name(), w)
		}
	}
}

func TestListEmpty(t *testing.T) {
	r := NewRegistry()
	list := r.List()
	if list == nil {
		t.Fatalf("List() = nil, want non-nil empty slice")
	}
	if len(list) != 0 {
		t.Fatalf("List() len = %d, want 0", len(list))
	}
}

func TestSelect(t *testing.T) {
	r := NewRegistry()
	// a runs first (lowest priority value), c runs last (highest).
	a := &mockSkill{name: "a", priority: skill.PriorityCritical} // 0
	b := &mockSkill{name: "b", priority: skill.PriorityNormal}   // 20
	c := &mockSkill{name: "c", priority: skill.PriorityLow}      // 30
	for _, s := range []*mockSkill{a, b, c} {
		if err := r.Register(s, false); err != nil {
			t.Fatalf("Register returned error: %v", err)
		}
	}

	t.Run("priority order ascending", func(t *testing.T) {
		// Configs intentionally in non-sorted order; selection must sort by priority.
		configs := []skill.Config{
			{Name: "c", Enabled: true},
			{Name: "a", Enabled: true},
			{Name: "b", Enabled: true},
		}
		selected, missing := r.Select(configs)
		if len(missing) != 0 {
			t.Fatalf("missing = %v, want empty", missing)
		}
		want := []string{"a", "b", "c"}
		got := namesOfSkills(selected)
		if len(got) != len(want) {
			t.Fatalf("selected len = %d, want %d", len(got), len(want))
		}
		for i, w := range want {
			if got[i] != w {
				t.Fatalf("selected[%d] = %q, want %q", i, got[i], w)
			}
		}
	})

	t.Run("config overrides priority", func(t *testing.T) {
		// Override b's priority to a value greater than c's so that b moves last.
		// A non-zero cfg.Priority takes precedence over the skill's own priority.
		configs := []skill.Config{
			{Name: "a", Enabled: true},
			{Name: "b", Enabled: true, Priority: skill.Priority(40)}, // override; default was 20
			{Name: "c", Enabled: true},
		}
		selected, missing := r.Select(configs)
		if len(missing) != 0 {
			t.Fatalf("missing = %v, want empty", missing)
		}
		// Without override the order would be [a, b, c]. With b overridden to 40
		// (greater than c's 30), the expected order is [a, c, b].
		want := []string{"a", "c", "b"}
		got := namesOfSkills(selected)
		if len(got) != len(want) {
			t.Fatalf("selected len = %d, want %d", len(got), len(want))
		}
		for i, w := range want {
			if got[i] != w {
				t.Fatalf("selected[%d] = %q, want %q (config priority wins)", i, got[i], w)
			}
		}
	})

	t.Run("disabled config not selected", func(t *testing.T) {
		configs := []skill.Config{
			{Name: "a", Enabled: true},
			{Name: "b", Enabled: false},
			{Name: "c", Enabled: true},
		}
		selected, missing := r.Select(configs)
		if len(missing) != 0 {
			t.Fatalf("missing = %v, want empty", missing)
		}
		if len(selected) != 2 {
			t.Fatalf("selected len = %d, want 2", len(selected))
		}
		for _, s := range selected {
			if s.Name() == "b" {
				t.Fatalf("disabled skill b was selected")
			}
		}
	})

	t.Run("unregistered skill in missing", func(t *testing.T) {
		configs := []skill.Config{
			{Name: "a", Enabled: true},
			{Name: "ghost", Enabled: true},
		}
		selected, missing := r.Select(configs)
		got := namesOfSkills(selected)
		if len(got) != 1 || got[0] != "a" {
			t.Fatalf("selected = %v, want [a]", got)
		}
		if len(missing) != 1 || missing[0] != "ghost" {
			t.Fatalf("missing = %v, want [ghost]", missing)
		}
	})

	t.Run("empty configs returns empty selected", func(t *testing.T) {
		selected, missing := r.Select(nil)
		if len(selected) != 0 {
			t.Fatalf("selected len = %d, want 0", len(selected))
		}
		if len(missing) != 0 {
			t.Fatalf("missing = %v, want empty", missing)
		}
	})

	t.Run("equal priorities stable", func(t *testing.T) {
		// Three skills with the same priority: selection must preserve the
		// insertion order of configs (sort.SliceStable).
		r2 := NewRegistry()
		x := &mockSkill{name: "x", priority: skill.PriorityNormal}
		y := &mockSkill{name: "y", priority: skill.PriorityNormal}
		z := &mockSkill{name: "z", priority: skill.PriorityNormal}
		for _, s := range []*mockSkill{x, y, z} {
			if err := r2.Register(s, false); err != nil {
				t.Fatalf("Register returned error: %v", err)
			}
		}
		// Pass configs in non-alphabetical order; expect selection to preserve it.
		configs := []skill.Config{
			{Name: "z", Enabled: true},
			{Name: "x", Enabled: true},
			{Name: "y", Enabled: true},
		}
		selected, _ := r2.Select(configs)
		got := namesOfSkills(selected)
		want := []string{"z", "x", "y"}
		if len(got) != len(want) {
			t.Fatalf("selected len = %d, want %d", len(got), len(want))
		}
		for i, w := range want {
			if got[i] != w {
				t.Fatalf("selected[%d] = %q, want %q (stable order with equal priority)", i, got[i], w)
			}
		}
	})
}

func TestClear(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"a", "b", "c"} {
		if err := r.Register(&mockSkill{name: n}, false); err != nil {
			t.Fatalf("Register(%q) returned error: %v", n, err)
		}
	}
	if r.Count() != 3 {
		t.Fatalf("Count() = %d, want 3 before Clear", r.Count())
	}
	r.Clear()
	if r.Count() != 0 {
		t.Fatalf("Count() = %d, want 0 after Clear", r.Count())
	}
	if list := r.List(); len(list) != 0 {
		t.Fatalf("List() len = %d after Clear, want 0", len(list))
	}
	// The registry must remain usable after Clear.
	if err := r.Register(&mockSkill{name: "again"}, false); err != nil {
		t.Fatalf("Register after Clear returned error: %v", err)
	}
	if r.Count() != 1 {
		t.Fatalf("Count() = %d, want 1 after re-register", r.Count())
	}
}

func TestCount(t *testing.T) {
	r := NewRegistry()
	if r.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", r.Count())
	}
	const n = 5
	for i := 0; i < n; i++ {
		if err := r.Register(&mockSkill{name: fmt.Sprintf("s-%d", i)}, false); err != nil {
			t.Fatalf("Register returned error: %v", err)
		}
		if got := r.Count(); got != i+1 {
			t.Fatalf("Count() after %d registers = %d, want %d", i+1, got, i+1)
		}
	}
	if r.Count() != n {
		t.Fatalf("Count() = %d, want %d", r.Count(), n)
	}
}

func TestConcurrency(t *testing.T) {
	r := NewRegistry()
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("skill-%03d", i)
			if err := r.Register(&mockSkill{name: name}, false); err != nil {
				t.Errorf("Register(%q) returned error: %v", name, err)
			}
		}(i)
	}
	wg.Wait()
	if got := r.Count(); got != n {
		t.Fatalf("Count() = %d, want %d", got, n)
	}
	// Each registered name must be retrievable.
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("skill-%03d", i)
		if _, err := r.Get(name); err != nil {
			t.Errorf("Get(%q) returned error: %v", name, err)
		}
	}
}
