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

	for _, depStr := range pkg.Dependencies {
		dep, err := PackageFromDep(depStr)
		if err != nil {
			return "", errors.Errorf("package dependency '%s' is invalid: %v", depStr, err)
		}

		includePath := filepath.Join(pkg.local, "dependencies", dep.Repo, dep.Path)

		config.Includes = append(config.Includes, includePath)
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return
	}

	fmt.Println("building", pkg, "with", config.Version)

	err = compiler.CompileSource(cacheDir, config)
	if err != nil {
		return
	}

	output = pkg.Output

	return
}

// GetBuildConfig returns a matching build by name from the package build list. If no name is
// specified, the first build is returned. If the package has no build definitions, a default
// configuration is returned.
func (pkg Package) GetBuildConfig(name string) (config compiler.Config, err error) {
	if len(pkg.Builds) == 0 {
		config = compiler.GetDefaultConfig()
		return
	}

	if name == "" {
		return compiler.MergeDefault(pkg.Builds[0]), nil
	}

	for _, cfg := range pkg.Builds {
		if cfg.Name == name {
			return compiler.MergeDefault(cfg), nil
		}
	}

	err = errors.Errorf("build '%s' not found in config", name)

	return
}
