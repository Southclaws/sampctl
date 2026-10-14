package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/build"
	compilerpkg "github.com/Southclaws/sampctl/src/pkg/build/compiler"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/cache"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

type compilerRepairKind uint8

const (
	compilerRepairNone compilerRepairKind = iota
	compilerRepairPermissions
	compilerRepairInstallation
)

func checkCompilers(ctx context.Context, pkg pawnpackage.Package, opts checkOptions) ([]diagnostic, error) {
	if pkg.LocalPath != "" {
		opts.Dir = pkg.LocalPath
	}

	var results []diagnostic
	for _, cfg := range checkCompilerConfigs(pkg) {
		if err := ctx.Err(); err != nil {
			return results, err
		}

		result, repairKind := inspectCheckCompilerContext(ctx, cfg, opts)
		if opts.Fix && result.CanFix {
			var err error
			result, err = repairDiagnostic(ctx, result, func(ctx context.Context) error {
				resolved := cfg.Compiler.ResolveCompilerConfig()
				dir := filepath.Join(opts.CacheDir, "pawn", resolved.User, resolved.Repo, resolved.Version)

				switch repairKind {
				case compilerRepairInstallation:
					return compilerpkg.RepairManagedCompiler(ctx, checkCompilerRequest(resolved, opts))
				case compilerRepairPermissions:
					return repairCheckCompilerPermissionsAt(ctx, opts.CacheDir, result.Path, dir)
				default:
					return errors.New("compiler repair is unavailable")
				}
			}, func() diagnostic {
				verified, _ := inspectCheckCompilerContext(ctx, cfg, opts)
				return verified
			})
			if err != nil {
				return append(results, result), err
			}
		}

		results = append(results, result)
	}

	return results, nil
}

func checkCompilerConfigs(pkg pawnpackage.Package) []build.Config {
	if pkg.Build == nil && len(pkg.Builds) == 0 {
		return []build.Config{*pkg.GetBuildConfig("")}
	}

	var configs []build.Config
	if pkg.Build != nil {
		base := copyCheckCompilerConfig(*pkg.Build)
		copyPkg := pkg
		copyPkg.Build, copyPkg.Builds = &base, nil
		configs = append(configs, *copyPkg.GetBuildConfig(""))
	}

	for _, cfg := range pkg.Builds {
		if cfg == nil {
			continue
		}

		copyPkg, copyCfg := pkg, copyCheckCompilerConfig(*cfg)
		copyPkg.Builds = []*build.Config{&copyCfg}
		if pkg.Build != nil {
			base := copyCheckCompilerConfig(*pkg.Build)
			copyPkg.Build = &base
		}
		configs = append(configs, *copyPkg.GetBuildConfig(copyCfg.Name))
	}

	return configs
}

func copyCheckCompilerConfig(cfg build.Config) build.Config {
	return build.Config{Name: cfg.Name, Version: cfg.Version, Compiler: cfg.Compiler}
}

func inspectCheckCompilerContext(ctx context.Context, cfg build.Config, opts checkOptions) (diagnostic, compilerRepairKind) {
	result := diagnostic{Name: "compiler", Status: checkFailed}
	label := "Compiler"
	if cfg.Name != "" {
		label += fmt.Sprintf(" for build %q", cfg.Name)
	}

	if err := cfg.Compiler.Validate(); err != nil {
		result.Message, result.Err = label+" configuration is invalid", err
		result.Action = "Check the compiler settings in pawn.json or pawn.yaml."
		return result, compilerRepairNone
	}

	if cfg.Compiler.Path != "" {
		return inspectCheckCustomCompiler(cfg.Compiler.Path, label, opts), compilerRepairNone
	}

	resolved := cfg.Compiler.ResolveCompilerConfig()
	for _, component := range []string{resolved.User, resolved.Repo, resolved.Version} {
		if !checkCompilerPathComponent(component) {
			result.Message = label + " settings contain an invalid cache path"
			result.Action = "Check compiler.user, compiler.repo, and compiler.version in the project configuration."
			result.Err = fmt.Errorf("invalid compiler path component %q", component)
			return result, compilerRepairNone
		}
	}

	return inspectCheckManagedCompiler(ctx, resolved, label, opts)
}

func checkCompilerRequest(resolved build.CompilerConfig, opts checkOptions) compilerpkg.CompilerFetchRequest {
	return compilerpkg.CompilerFetchRequest{
		Meta:     versioning.DependencyMeta{Site: resolved.Site, User: resolved.User, Repo: resolved.Repo, Tag: resolved.Version},
		Dir:      filepath.Join(opts.CacheDir, "pawn", resolved.User, resolved.Repo, resolved.Version),
		Platform: opts.Platform, CacheDir: opts.CacheDir,
	}
}

func inspectCheckManagedCompiler(ctx context.Context, resolved build.CompilerConfig, label string, opts checkOptions) (diagnostic, compilerRepairKind) {
	dir := filepath.Join(opts.CacheDir, "pawn", resolved.User, resolved.Repo, resolved.Version)
	result := diagnostic{Name: "compiler", Status: checkFailed, Path: dir}

	state, stateErr := compilerpkg.InspectManagedCompiler(ctx, checkCompilerRequest(resolved, opts))
	if state.Active {
		result.Status, result.Message = checkWarning, label+" installation is in use"
		result.Action = "Wait for the current build or installation to finish, then retry."
		return result, compilerRepairNone
	}

	if state.Pending {
		result.Err, result.CanFix = stateErr, state.CanRepair && stateErr == nil
		result.Message = label + " installation was interrupted"
		if state.Committed {
			result.Message = label + " installation saved. Cleanup pending"
		}
		result.Action = "Run `sampctl check --verbose`. Keep the recovery files and backups for inspection."
		if result.CanFix {
			result.Action = "Run `sampctl check --fix` to finish compiler recovery."
			return result, compilerRepairInstallation
		}

		return result, compilerRepairNone
	}

	if err := checkCompilerDirectory(dir); err != nil {
		result.Message, result.Err = label+" "+resolved.Version+" installation is unavailable", err
		result.Action = "Run `sampctl build` to install the selected compiler."
		if state.CanRepair {
			result.CanFix = true
			result.Action = "Run `sampctl check --fix` to restore the compiler from its cached archive."
			return result, compilerRepairInstallation
		}

		return result, compilerRepairNone
	}

	metadata, err := readCheckCompilerMetadata(filepath.Join(opts.CacheDir, "compilers.json"))
	if err != nil {
		result.Status = checkSkipped
		result.Message, result.Err = label+" cannot be checked: cached compiler metadata is unavailable", err
		result.Action = "Run `sampctl build` to download compiler metadata."
		return result, compilerRepairNone
	}

	pkg, ok := metadata[opts.Platform]
	if !ok {
		result.Message = label + " metadata is unavailable for platform " + opts.Platform
		result.Action = "Check --platform or run `sampctl build` to refresh compiler metadata."
		return result, compilerRepairNone
	}

	if err := validateCheckCompilerPackage(pkg); err != nil {
		result.Status = checkSkipped
		result.Message, result.Err = label+" cannot be checked: invalid cached compiler metadata", err
		return result, compilerRepairNone
	}

	result = inspectCheckCompilerFiles(dir, pkg, label, opts)
	if result.CanFix && (!state.HasCompletion || state.PermissionsOnly) {
		return result, compilerRepairPermissions
	}

	if stateErr != nil {
		result.Status, result.Path, result.Err = checkFailed, dir, stateErr
		result.Message = label + " " + resolved.Version + " files failed verification"
		result.CanFix = state.CanRepair
		result.Action = "Inspect the compiler files manually. Use --verbose for details before retrying the build."
		if result.CanFix {
			result.Action = "Run `sampctl check --fix` to restore the compiler from its cached archive."
			return result, compilerRepairInstallation
		}
	} else if state.Valid && !state.Legacy {
		result.Status, result.Message = checkPassed, label+" is available"
		result.Err, result.Action, result.CanFix = nil, "", false
	}

	return result, compilerRepairNone
}

func readCheckCompilerMetadata(path string) (download.Compilers, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.Mode().IsRegular() {
		return nil, errors.New("compiler metadata is not a regular file")
	}

	return cache.ReadJSON[download.Compilers](path)
}

func inspectCheckCustomCompiler(path, label string, opts checkOptions) diagnostic {
	if !filepath.IsAbs(path) {
		path = filepath.Join(opts.Dir, path)
	}

	result := diagnostic{Name: "compiler", Status: checkFailed, Path: path}
	result.Action = "Check compiler.path in pawn.json or pawn.yaml and repair the custom compiler manually."
	if err := checkCompilerDirectory(path); err != nil {
		result.Message, result.Err = label+" directory is unavailable", err
		return result
	}

	binary := "pawncc"
	if runtime.GOOS == "windows" {
		binary = "pawncc.exe"
	}
	result.Path = filepath.Join(path, binary)
	info, err := checkCompilerReadableFile(result.Path)
	if err != nil {
		result.Message, result.Err = label+" executable is unavailable", err
		return result
	}

	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		result.Message = label + " executable does not have execute permission"
		return result
	}

	result.Status, result.Message, result.Action = checkPassed, label+" is available", ""
	return result
}

func inspectCheckCompilerFiles(dir string, pkg download.Compiler, label string, opts checkOptions) diagnostic {
	result := diagnostic{Name: "compiler", Status: checkFailed, Path: filepath.Join(dir, pkg.Binary)}
	result.Action = "Run `sampctl build` to install missing compiler files. Check existing custom files manually."
	binaryInfo, err := checkCompilerReadableFile(result.Path)
	if err != nil {
		result.Message, result.Err = label+" executable is unavailable", err
		return result
	}

	targets := make([]string, 0, len(pkg.Paths))
	for _, target := range pkg.Paths {
		if target == "" || strings.HasSuffix(target, "/") {
			result.Status = checkSkipped
			result.Message = label + " cannot be fully checked: metadata does not name every installed file"
			result.Action = "Check the installed compiler files manually."
			return result
		}
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		if _, err := checkCompilerReadableFile(filepath.Join(dir, target)); err != nil {
			result.Path = filepath.Join(dir, target)
			result.Message, result.Err = label+" installation is incomplete ("+target+")", err
			return result
		}
	}

	if runtime.GOOS != "windows" && fs.IsPosixPlatform(opts.Platform) && binaryInfo.Mode().Perm()&0o111 == 0 {
		result.Message = label + " executable does not have execute permission"
		if !checkCompilerSingleLink(binaryInfo) {
			result.Err = errors.New("automatic repair requires a compiler file with one hard link")
			result.Action = "Check the compiler executable and restore its execute permission manually."
			return result
		}
		if _, err := checkManagedRegularPath(opts.CacheDir, result.Path); err == nil {
			result.CanFix = true
			result.Action = "Run `sampctl check --fix` to restore the compiler's execute permission."
		} else {
			result.Err = err
			result.Action = "Check the compiler path and restore execute permission manually."
		}
		return result
	}

	result.Status, result.Message, result.Action = checkPassed, label+" is available", ""
	return result
}

func validateCheckCompilerPackage(pkg download.Compiler) error {
	if pkg.Binary == "" || !checkCompilerRelativePath(pkg.Binary) {
		return fmt.Errorf("invalid compiler binary path %q", pkg.Binary)
	}

	if download.ExtractFuncFromName(pkg.Method) == nil {
		return fmt.Errorf("unsupported compiler extraction method %q", pkg.Method)
	}

	if pkg.Match == "" {
		return fmt.Errorf("compiler asset match is empty")
	}
	if _, err := regexp.Compile(pkg.Match); err != nil {
		return fmt.Errorf("invalid compiler asset match: %w", err)
	}

	if len(pkg.Paths) == 0 {
		return fmt.Errorf("compiler extraction paths are empty")
	}

	sources := make([]string, 0, len(pkg.Paths))
	for source := range pkg.Paths {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		if target := pkg.Paths[source]; target != "" && !checkCompilerRelativePath(target) {
			return fmt.Errorf("invalid compiler extraction target %q", target)
		}
	}

	return nil
}

func checkCompilerPathComponent(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\:\x00")
}

func checkCompilerRelativePath(value string) bool {
	return filepath.IsLocal(value) && value != "." && !strings.ContainsAny(value, "\\:\x00")
}

func checkCompilerDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("compiler path %q is not a directory", path)
	}

	return nil
}

func checkCompilerReadableFile(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, fmt.Errorf("compiler file %q is not a nonempty regular file", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	return info, nil
}

func repairCheckCompilerPermissions(ctx context.Context, cacheDir, path string) (err error) {
	return repairCheckCompilerPermissionsAt(ctx, cacheDir, path, filepath.Dir(path))
}

func repairCheckCompilerPermissionsAt(ctx context.Context, cacheDir, path, installationDir string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	root, relative, info, err := openCheckManagedFileRoot(cacheDir, path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()

	file, err := root.Open(relative)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()

	current, err := file.Stat()
	if err != nil {
		return err
	}

	if !os.SameFile(info, current) {
		return fmt.Errorf("managed compiler changed during repair")
	}

	if !checkCompilerSingleLink(current) {
		return errors.New("automatic repair requires a compiler file with one hard link")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	binary, err := filepath.Rel(installationDir, path)
	if err != nil || !filepath.IsLocal(binary) {
		return errors.Join(errors.New("compiler executable is outside its installation"), err)
	}

	return compilerpkg.RepairManagedCompilerPermissions(ctx, installationDir, filepath.ToSlash(binary))
}
