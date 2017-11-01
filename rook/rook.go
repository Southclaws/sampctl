// Package rook defines an API for working with Pawn libraries as 'packages' similar to how Go
// handles packages. It uses GitHub as a backend and tries to infer as much as possible from a repo
// such as where source files are located. Tags are encouraged for versioning but if absent, the git
// SHA1 hash is used.
package rook

import (
	"fmt"
)

// Package represents a definition for a Pawn package and can either be used to define a build or
// as a description of a package in a repository. This is akin to npm's package.json and combines
// a project's dependencies with a description of that project.
//
// For example, a gamemode that includes a library does not need to define the User, Repo, Version,
// Contributors and Include fields at all, it can just define the Dependencies list in order to
// build correctly.
//
// On the flip side, a library written in pure Pawn should define some contributors and a web URL
// but, being written in pure Pawn, has no dependencies.
//
// Finally, if a repository stores its package source files in a subdirectory, that directory should
// be specified in the Include field. This is common practice for plugins that store the plugin
// source code in the root and the Pawn source in a subdirectory called 'include'.
type Package struct {
	// Inferred metadata
	user    string // github username
	repo    string // github repository
	version string // either git sha1 or git tag

	// Metadata
	Contributors []string `json:"contributors"` // list of contributors
	Website      string   `json:"website"`      // website or forum topic associated with the package

	// Functional
	Include      []string     `json:"incude"`       // list of paths that contain the package library source files if they do not exist in the repository's root
	Dependencies []Dependency `json:"dependencies"` // list of packages that the package depends on
}

// GetURL generates a GitHub URL for a package - it does not test the validity of the URL
func (pkg Package) GetURL() string {
	return fmt.Sprintf("https://github.com/%s/%s", pkg.user, pkg.repo)
}
