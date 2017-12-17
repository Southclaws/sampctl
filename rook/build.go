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
func (pkg Package) Build(build string, ensure bool) (output string, err error) {
	config, err := pkg.GetBuildConfig(build)
	if err != nil {
		err = errors.Wrap(err, "failed to get build config")
		return
	}

	config.WorkingDir = filepath.Dir(util.FullPath(pkg.Entry))
	config.Input = filepath.Join(pkg.local, pkg.Entry)
	config.Output = filepath.Join(pkg.local, pkg.Output)

	if ensure {
		err = pkg.EnsureDependencies()
		if err != nil {
			err = errors.Wrap(err, "failed to ensure dependencies before build")
			return
		}
	}

	err = pkg.ResolveDependencies()
	if err != nil {
		err = errors.Wrap(err, "failed to resolve dependencies before build")
		return
	}

	for _, depMeta := range pkg.allDependencies {
		includePath := filepath.Join(pkg.local, "dependencies", depMeta.Repo, depMeta.Path)
		config.Includes = append(config.Includes, includePath)
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return
	}

	fmt.Println("building", pkg, "with", config.Version)

	err = compiler.CompileSource(pkg.local, cacheDir, config)
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
func (pkg Package) GetBuildConfig(name string) (config compiler.Config, err error) {
	if len(pkg.Builds) == 0 || name == "" {
		config = compiler.GetDefaultConfig()
		return
	}

	for _, cfg := range pkg.Builds {
		if cfg.Name == name {
			return cfg, nil
		}
	}

	err = errors.Errorf("build '%s' not found in config", name)

	return
}
