package rook

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/compiler"
	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// Build compiles a package, dependencies are ensured and a list of paths are sent to the compiler.
func Build(pkg *types.Package, build, platform string, ensure bool) (output string, err error) {
	config := GetBuildConfig(*pkg, build)
	if config == nil {
		err = errors.Errorf("no build config named '%s'", build)
		return
	}

	config.WorkingDir = filepath.Dir(util.FullPath(pkg.Entry))
	config.Input = filepath.Join(pkg.Local, pkg.Entry)
	config.Output = filepath.Join(pkg.Local, pkg.Output)

	if ensure {
		err = EnsureDependencies(pkg)
		if err != nil {
			err = errors.Wrap(err, "failed to ensure dependencies before build")
			return
		}
	}

	err = ResolveDependencies(pkg)
	if err != nil {
		err = errors.Wrap(err, "failed to resolve dependencies before build")
		return
	}

	for _, depMeta := range pkg.AllDependencies {
		includePath := filepath.Join(pkg.Local, "dependencies", depMeta.Repo, depMeta.Path)
		config.Includes = append(config.Includes, includePath)
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return
	}

	fmt.Println("building", pkg, "with", config.Version)

	err = compiler.CompileSource(pkg.Local, cacheDir, platform, *config)
	if err != nil {
		err = errors.Wrap(err, "failed to compile package entry")
		return
	}

	output = pkg.Output

	return
}

// GetBuildConfig returns a matching build by name from the package build list. If no name is
// specified, the first build is returned. If the package has no build definitions, a default
// configuration is returned.
func GetBuildConfig(pkg types.Package, name string) (config *types.BuildConfig) {
	if len(pkg.Builds) == 0 || name == "" {
		config = types.GetBuildConfigDefault()
	} else {
		for _, cfg := range pkg.Builds {
			if cfg.Name == name {
				config = &cfg
			}
		}
	}

	return
}
