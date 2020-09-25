package pkgcontext

import (
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/versioning"
)

// Uninstall removes a dependency from a package and attempts to delete the contents
func (pcx *PackageContext) Uninstall(
	targets []versioning.DependencyString,
	development bool,
) (err error) {
	for _, target := range targets {
		err = pcx.uninstall(target, development)
		if err != nil {
			return
		}
	}

	err = pcx.Package.WriteDefinition()

	return
}

func (pcx *PackageContext) uninstall(
	target versioning.DependencyString,
	development bool,
) (err error) {
	_, err = target.Explode()
	if err != nil {
		return errors.Wrapf(err, "failed to parse %s as a dependency string", target)
	}

	if development {
		err = pcx.uninstallDev(target)
	} else {
		err = pcx.uninstallDep(target)
	}
	return
}

// regular dependencies
func (pcx *PackageContext) uninstallDep(target versioning.DependencyString) (err error) {
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
func (pcx *PackageContext) uninstallDev(target versioning.DependencyString) (err error) {
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
