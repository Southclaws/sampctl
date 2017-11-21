package rook

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/compiler"
	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/util"
)

// Build compiles a package, dependencies are ensured and a list of paths are sent to the compiler.
func (pkg Package) Build(version compiler.Version, ensure bool) (output string, err error) {
	if ensure {
		err = pkg.EnsureDependencies()
		if err != nil {
			err = errors.Wrap(err, "failed to ensure dependencies before build")
			return
		}
	}

	includes := make([]string, len(pkg.Dependencies))

	for _, depStr := range pkg.Dependencies {
		dep, err := PackageFromDep(depStr)
		if err != nil {
			return "", errors.Errorf("package dependency '%s' is invalid: %v", depStr, err)
		}

		includes = append(includes, filepath.Join(pkg.local, "dependencies", dep.Repo, dep.Path))
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return
	}

	fmt.Println("building", pkg, "with", version)

	err = compiler.CompileSource(
		filepath.Dir(util.FullPath(pkg.Entry)),
		filepath.Join(pkg.local, pkg.Entry),
		filepath.Join(pkg.local, pkg.Output),
		includes,
		cacheDir,
		version,
	)
	if err != nil {
		return
	}

	output = pkg.Output

	return
}
