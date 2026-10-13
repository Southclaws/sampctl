package pkgcontext

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func (pcx *PackageContext) validateDependencyDestinations() error {
	constraints := pcx.directDependencyConstraints()
	candidates := append([]versioning.DependencyMeta(nil), pcx.AllDependencies...)
	for _, declaration := range pcx.Package.GetAllDependencies() {
		meta, err := declaration.Explode()
		if err == nil {
			candidates = append(candidates, meta)
		}
	}
	seen := make(map[string]versioning.DependencyMeta)
	var failures []error
	for _, meta := range candidates {
		if meta.IsLocalScheme() {
			continue
		}
		if err := registerDependencyDestination(seen, inheritDirectDependencyConstraint(meta, constraints)); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// Unpinned references share the revision selected by the project's direct dependency.
func (pcx *PackageContext) directDependencyConstraints() map[versioning.DependencyMeta]versioning.DependencyMeta {
	constraints := make(map[versioning.DependencyMeta]versioning.DependencyMeta)
	for _, declaration := range pcx.Package.GetAllDependencies() {
		meta, err := declaration.Explode()
		if err == nil && !meta.IsLocalScheme() && hasExplicitDependencyConstraint(meta) {
			constraints[dependencyRepositoryIdentity(meta)] = meta
		}
	}
	return constraints
}

func inheritDirectDependencyConstraint(meta versioning.DependencyMeta, constraints map[versioning.DependencyMeta]versioning.DependencyMeta) versioning.DependencyMeta {
	if meta.IsLocalScheme() || hasExplicitDependencyConstraint(meta) {
		return meta
	}
	if direct, ok := constraints[dependencyRepositoryIdentity(meta)]; ok {
		meta.Tag, meta.Branch, meta.Commit = direct.Tag, direct.Branch, direct.Commit
	}
	return meta
}

func dependencyRepositoryIdentity(meta versioning.DependencyMeta) versioning.DependencyMeta {
	meta = managedDependencyIdentity(meta)
	meta.Tag, meta.Branch, meta.Commit = "", "", ""
	return meta
}

func registerDependencyDestination(seen map[string]versioning.DependencyMeta, meta versioning.DependencyMeta) error {
	key := strings.ToLower(meta.Repo)
	previous, exists := seen[key]
	if exists && managedDependencyIdentity(previous) != managedDependencyIdentity(meta) {
		return &DependencyError{Dependency: meta.String(), Operation: "ensure", Err: fmt.Errorf("dependencies %s and %s share the installation directory %s; use dependencies with distinct repository names", previous, meta, meta.Repo)}
	}
	seen[key] = meta
	return nil
}

func managedDependencyIdentity(meta versioning.DependencyMeta) versioning.DependencyMeta {
	meta.Site = normalizeDependencySite(meta.Site)
	meta.Path = ""
	meta.Scheme = ""
	meta.Local = ""
	meta.SSH = ""
	return meta
}
