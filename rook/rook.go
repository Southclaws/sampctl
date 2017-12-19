// Package rook defines an API for working with Pawn libraries as 'packages' similar to how Go
// handles packages. It uses GitHub as a backend and tries to infer as much as possible from a repo
// such as where source files are located. Tags are encouraged for versioning but if absent, the git
// SHA1 hash is used.
package rook

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

// Package documented in types package
type Package types.Package

func (pkg Package) String() string {
	return fmt.Sprintf("%s/%s:%s", pkg.User, pkg.Repo, pkg.Version)
}

// Validate checks a package for missing fields
func (pkg Package) Validate() (err error) {
	if pkg.Entry == "" {
		return errors.New("package does not define an entry point")
	}

	if pkg.Output == "" {
		return errors.New("package does not define an output file")
	}

	if pkg.Entry == pkg.Output {
		return errors.New("package entry and output point to the same file")
	}

	return
}

// PackageFromDep creates a Package object from a Dependency String
func PackageFromDep(depString versioning.DependencyString) (pkg Package, err error) {
	dep, err := depString.Explode()
	pkg.User, pkg.Repo, pkg.Path, pkg.Version = dep.User, dep.Repo, dep.Path, dep.Version
	return
}
