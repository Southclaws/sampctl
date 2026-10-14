package pkgcontext

import (
	"context"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func (pcx *PackageContext) buildPrepare(
	ctx context.Context,
	build string,
	ensure,
	forceUpdate bool,
) (config *build.Config, err error) {
	config = pcx.Package.GetBuildConfig(build)
	if config == nil {
		err = errors.Errorf("no build config named '%s'", build)
		return
	}

	if err = config.Compiler.Validate(); err != nil {
		err = errors.Wrap(err, "invalid compiler configuration")
		return
	}

	if err = pcx.resolveBuildPaths(config); err != nil {
		return nil, err
	}

	config.Includes = append(config.Includes, pcx.Package.LocalPath)

	if err = pcx.ensureBuildFile(config); err != nil {
		return
	}

	if ensure {
		err = pcx.EnsureDependencies(ctx, forceUpdate)
		if err != nil {
			err = errors.Wrap(err, "failed to ensure dependencies before build")
			return
		}
	}

	if err = pcx.appendBuildDependencyIncludes(config); err != nil {
		return
	}

	return config, err
}

func (pcx *PackageContext) resolveBuildPaths(config *build.Config) (err error) {
	if config.Input == "" && pcx.Package.Entry != "" {
		config.Input = packagePath(pcx.Package.LocalPath, pcx.Package.Entry)
	} else if config.Input != "" {
		config.Input = packagePath(pcx.Package.LocalPath, config.Input)
	}
	if config.Input != "" {
		config.Input, err = fs.Abs(config.Input)
		if err != nil {
			return errors.Wrap(err, "failed to resolve build input path")
		}
	}

	if config.WorkingDir == "" {
		switch {
		case config.Input != "":
			config.WorkingDir = filepath.Dir(config.Input)
		case pcx.Package.Entry != "":
			entryPath, absErr := fs.Abs(packagePath(pcx.Package.LocalPath, pcx.Package.Entry))
			if absErr != nil {
				return errors.Wrap(absErr, "failed to resolve package entry path")
			}
			config.WorkingDir = filepath.Dir(entryPath)
		}
	} else {
		config.WorkingDir, err = fs.Abs(packagePath(pcx.Package.LocalPath, config.WorkingDir))
		if err != nil {
			return errors.Wrap(err, "failed to resolve build working directory")
		}
	}

	if config.Output == "" && pcx.Package.Output != "" {
		config.Output = packagePath(pcx.Package.LocalPath, pcx.Package.Output)
	} else if config.Output != "" {
		config.Output = packagePath(pcx.Package.LocalPath, config.Output)
	}
	if config.Output != "" {
		config.Output, err = fs.Abs(config.Output)
		if err != nil {
			return errors.Wrap(err, "failed to resolve build output path")
		}
	}

	if config.Compiler.Path != "" {
		compilerPath := config.Compiler.Path
		if !filepath.IsAbs(compilerPath) {
			compilerPath = filepath.Join(pcx.Package.LocalPath, compilerPath)
		}
		config.Compiler.Path, err = fs.Abs(compilerPath)
		if err != nil {
			return errors.Wrap(err, "failed to resolve compiler path")
		}
	}

	return nil
}

func (pcx *PackageContext) appendBuildDependencyIncludes(config *build.Config) (err error) {
	for _, depMeta := range pcx.AllDependencies {

		// check if local package has a definition
		incPath := ""
		hasIncludeResources := false
		noPackage := false
		depDir := filepath.Join(pcx.Package.LocalPath, "dependencies", depMeta.Repo)
		pkgInner, errInner := pawnpackage.PackageFromDir(depDir)
		if errInner != nil {
			print.Verb(depMeta, "error while loading:", errInner, "using cached copy for include path checking")
			pkgInner, errInner = pawnpackage.GetCachedPackage(depMeta, pcx.CacheDir)
			if errInner != nil {
				noPackage = true
			}
		}

		if !noPackage {
			// check if package specifies an include path
			if pkgInner.IncludePath != "" {
				incPath = pkgInner.IncludePath
			}
			// check if the package specifies resources that contain includes
			for _, res := range pkgInner.Resources {
				if len(res.Includes) > 0 {
					hasIncludeResources = true
					break
				}
			}
		}

		if !hasIncludeResources {
			config.Includes = append(config.Includes, filepath.Join(depDir, incPath))
		}
	}

	for _, pluginMeta := range pcx.AllPlugins {
		if !pluginMeta.IsLocalScheme() {
			continue
		}
		if pluginMeta.Scheme != "plugin" && pluginMeta.Scheme != "component" {
			continue
		}

		localDepDir := filepath.Join(pcx.Package.LocalPath, pluginMeta.Local)
		if !fs.Exists(localDepDir) {
			err = errors.Errorf("local %s dependency path does not exist: %s", pluginMeta.Scheme, localDepDir)
			return
		}

		includeDir := localDepDir
		if pkgInner, errInner := pawnpackage.PackageFromDir(localDepDir); errInner == nil {
			if pkgInner.IncludePath != "" {
				includeDir = filepath.Join(localDepDir, pkgInner.IncludePath)
			}
		}

		config.Includes = append(config.Includes, includeDir)
	}

	config.Includes = append(config.Includes, pcx.AllIncludePaths...)

	return err
}
