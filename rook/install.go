package rook

import (
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

// Install adds a new dependency to an existing local parent package
func Install(pkg types.Package, targets []versioning.DependencyString, development bool) (err error) {
	// todo: version checks

	exists := false

	for _, target := range targets {
		for _, dep := range pkg.GetAllDependencies() {
			if dep == target {
				exists = true
			}
		}

		if !exists {
			if development {
				pkg.Development = append(pkg.Development, target)
			} else {
				pkg.Dependencies = append(pkg.Dependencies, target)
			}
		} else {
			print.Warn("target already exists in dependencies")
			return
		}
	}

	err = EnsureDependencies(&pkg)
	if err != nil {
		return
	}

	err = pkg.WriteDefinition()

	return
}
