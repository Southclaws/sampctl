package rook

import (
	"context"
	"fmt"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

// Uninstall removes a dependency from a package and attempts to delete the contents
func Uninstall(ctx context.Context, gh *github.Client, pkg types.Package, targets []versioning.DependencyString, development bool, auth transport.AuthMethod, platform, cacheDir string) (err error) {
	exists := false

	for _, target := range targets {
		_, err = versioning.DependencyString(target).Explode()
		if err != nil {
			return errors.Wrapf(err, "failed to parse %s as a dependency string", target)
		}

		if development {
			var (
				i   = 0
				dep versioning.DependencyString
			)
			for i, dep = range pkg.Development {
				if dep == target {
					exists = true
				}
			}

			if exists {
				pkg.Development = append(pkg.Development[:i], pkg.Development[i+1:]...)
			} else {
				print.Warn("target does not exist in dependencies")
				return
			}
		} else {
			var (
				i   = 0
				dep versioning.DependencyString
			)
			for i, dep = range pkg.Dependencies {
				fmt.Println(dep, target)
				if dep == target {
					exists = true
				}
			}

			if exists {
				pkg.Dependencies = append(pkg.Dependencies[:i], pkg.Dependencies[i+1:]...)
			} else {
				print.Warn("target does not exist in dependencies")
				return
			}
		}
	}

	err = pkg.WriteDefinition()

	return
}
