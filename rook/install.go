package rook

import (
	"fmt"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

// Install adds a new dependency to an existing local parent package
func Install(pkg types.Package, target versioning.DependencyString) (err error) {

	// todo: version checks
	exists := false
	for _, dep := range pkg.Dependencies {
		if dep == target {
			exists = true
		}
	}

	if !exists {
		pkg.Dependencies = append(pkg.Dependencies, target)
	} else {
		fmt.Println("target already exists in dependencies")
	}

	meta, err := target.Explode()
	if err != nil {
		return
	}

	err = EnsurePackage(pkg.Vendor, meta)
	if err != nil {
		return
	}

	// if pkg.format == "json" {
	// 	// generate pawn.json
	// } else {
	// 	// generate pawn.yaml
	// }

	return
}
