package versioning

import (
	"fmt"
	"maps"
	"slices"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

type projectOverride struct {
	replacement string
	meta        DependencyMeta
}

// ProjectOverrides is the validated, exact-match policy from the root manifest.
type ProjectOverrides struct {
	rules  map[DependencyString]projectOverride
	source string
}

// ProjectOverrideError describes an invalid rule in the root manifest.
type ProjectOverrideError struct {
	Source      string
	Key         string
	Replacement string
	InvalidKey  bool
	Err         error
}

func (e *ProjectOverrideError) Error() string {
	if e.InvalidKey {
		return fmt.Sprintf("%s: invalid key %q: %v", e.Source, e.Key, e.Err)
	}
	return fmt.Sprintf("%s: invalid replacement %q for %q: %v", e.Source, e.Replacement, e.Key, e.Err)
}

func (e *ProjectOverrideError) Unwrap() error { return e.Err }

// NewProjectOverrides validates every rule, including rules unused by the current graph.
func NewProjectOverrides(overrides map[string]string, format string) (ProjectOverrides, error) {
	var policy ProjectOverrides
	if len(overrides) == 0 {
		return policy, nil
	}

	policy.source = "pawn." + format + " overrides"
	policy.rules = make(map[DependencyString]projectOverride, len(overrides))
	for _, original := range slices.Sorted(maps.Keys(overrides)) {
		replacement := overrides[original]
		if _, err := DependencyString(original).ExplodeWithOverrides(nil); err != nil {
			return ProjectOverrides{}, &ProjectOverrideError{Source: policy.source, Key: original, InvalidKey: true, Err: err}
		}
		meta, err := DependencyString(replacement).ExplodeWithOverrides(nil)
		if err != nil {
			return ProjectOverrides{}, &ProjectOverrideError{Source: policy.source, Key: original, Replacement: replacement, Err: err}
		}
		policy.rules[DependencyString(original)] = projectOverride{replacement: replacement, meta: meta}
	}
	return policy, nil
}

// Matches reports whether the declaration has an exact project rule.
func (p ProjectOverrides) Matches(declaration DependencyString) bool {
	_, ok := p.rules[declaration]
	return ok
}

// Replacement returns the parsed replacement for an exact declaration.
func (p ProjectOverrides) Replacement(declaration DependencyString) (DependencyMeta, bool) {
	rule, ok := p.rules[declaration]
	if ok && string(declaration) != rule.replacement {
		print.Verb(declaration, "->", rule.replacement, "\n  Source:", p.source)
	}
	return rule.meta, ok
}

// HasPinnedTags reports whether any replacement specifies a fixed tag.
func (p ProjectOverrides) HasPinnedTags() bool {
	for _, rule := range p.rules {
		if rule.meta.Tag != "" && rule.meta.Tag != "latest" {
			return true
		}
	}
	return false
}

// Resolve applies a single project substitution or falls back to the usual override loader.
func (p ProjectOverrides) Resolve(declaration DependencyString) (DependencyMeta, error) {
	if meta, ok := p.Replacement(declaration); ok {
		return meta, nil
	}
	return declaration.Explode()
}

// ResolveWithLegacy uses the supplied legacy overrides on a project rule miss.
func (p ProjectOverrides) ResolveWithLegacy(declaration DependencyString, legacy map[string]string) (DependencyMeta, error) {
	if meta, ok := p.Replacement(declaration); ok {
		return meta, nil
	}
	return declaration.ExplodeWithOverrides(legacy)
}
