package rook

import (
	"context"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/versioning"
)

// Uninstall removes a dependency from a package and attempts to delete the contents
func (pcx *PackageContext) Uninstall(ctx context.Context, targets []versioning.DependencyString, development bool) (err error) {
	for _, target := range targets {
		err = pcx.uninstall(ctx, target, development)
		if err != nil {
			return
		}
	}

	err = pcx.Package.WriteDefinition()

	return
}

func (pcx *PackageContext) uninstall(ctx context.Context, target versioning.DependencyString, development bool) (err error) {
	_, err = versioning.DependencyString(target).Explode()
	if err != nil {
		return errors.Wrapf(err, "failed to parse %s as a dependency string", target)
	}

	if development {
		pcx.uninstallDev(ctx, target)
	} else {
		pcx.uninstallDep(ctx, target)
	}
	return
}

// regular dependencies
func (pcx *PackageContext) uninstallDep(ctx context.Context, target versioning.DependencyString) (err error) {
	idx := -1
	for i, dep := range pcx.Package.Dependencies {
		if dep == target {
			idx = i
			break
		}
	}

	if idx == -1 {
		print.Warn("target does not exist in dependencies")
		return
	}

	pcx.Package.Dependencies = append(pcx.Package.Dependencies[:idx], pcx.Package.Dependencies[idx+1:]...)

	return
}

// development dependencies
func (pcx *PackageContext) uninstallDev(ctx context.Context, target versioning.DependencyString) (err error) {
	idx := -1
	for i, dep := range pcx.Package.Development {
		if dep == target {
			idx = i
			break
		}
	}

	if idx == -1 {
		print.Warn("target does not exist in dependencies")
		return
	}

	pcx.Package.Development = append(pcx.Package.Development[:idx], pcx.Package.Development[idx+1:]...)

	return
}
