package pkgcontext

import (
	"context"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/versioning"
)

// Install adds a new dependency to an existing local parent package
func (pcx *PackageContext) Install(
	ctx context.Context,
	targets []versioning.DependencyString,
	development bool,
) (err error) {
	exists := false

	for _, target := range targets {
		_, err = target.Explode()
		if err != nil {
			return errors.Wrapf(err, "failed to parse %s as a dependency string", target)
		}

		for _, dep := range pcx.Package.GetAllDependencies() {
			if dep == target {
				exists = true
			}
		}

		if !exists {
			if development {
				pcx.Package.Development = append(pcx.Package.Development, target)
			} else {
				pcx.Package.Dependencies = append(pcx.Package.Dependencies, target)
			}
		} else {
			print.Warn("target already exists in dependencies")
			return
		}
	}

	print.Verb(pcx.Package, "ensuring dependencies are cached for package context")
	err = pcx.EnsureDependenciesCached()
	if err != nil {
		return
	}

	print.Verb(pcx.Package, "ensuring dependencies are installed for package context")
	err = pcx.EnsureDependencies(ctx, true)
	if err != nil {
		return
	}

	err = pcx.Package.WriteDefinition()
	if err != nil {
		return
	}

	return nil
}
