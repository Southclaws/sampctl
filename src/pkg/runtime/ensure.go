package runtime

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

// Ensure will make sure a Config's dir is representative of the held configuration.
// If any of the following are missing or mismatching, they will be automatically downloaded:
// - Server binaries (server, announce, npc)
// - Plugin binaries
// - Scripts: gamemodes and filterscripts
func Ensure(ctx context.Context, gh *github.Client, cfg *run.Runtime, noCache bool) (err error) {
	if err = cfg.Validate(); err != nil {
		return
	}

	cacheDir, err := fs.ConfigDir()
	if err != nil {
		return errors.Wrap(err, "failed to get config dir")
	}

	print.Verb("ensuring server binaries")
	_, err = EnsureBinariesContext(ctx, cacheDir, *cfg)
	if err != nil {
		return errors.Wrap(err, "failed to ensure runtime binaries")
	}

	print.Verb("ensuring all dependency and static plugins")
	err = EnsurePlugins(EnsurePluginsRequest{
		Context:  ctx,
		GitHub:   gh,
		Config:   cfg,
		CacheDir: cacheDir,
		NoCache:  noCache,
	})
	if err != nil {
		return errors.Wrap(err, "failed to ensure plugins")
	}

	print.Verb("ensuring all compiled scripts")
	err = EnsureScripts(*cfg)
	if err != nil {
		return errors.Wrap(err, "failed to ensure scripts")
	}

	return nil
}

// EnsureBinaries ensures the dir has all the necessary files to run a server
func EnsureBinaries(cacheDir string, cfg run.Runtime) (*RuntimeManifestInfo, error) {
	return EnsureBinariesContext(context.Background(), cacheDir, cfg)
}

// EnsureBinariesContext ensures the dir has all the necessary files to run a server.
func EnsureBinariesContext(ctx context.Context, cacheDir string, cfg run.Runtime) (*RuntimeManifestInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := runtimeFilePath(cfg.WorkingDir, installedRuntimeManifestFileName); err != nil {
		return nil, err
	}
	manifest, stageDir, err := ensureStagedRuntime(ctx, cacheDir, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to prepare runtime files")
	}
	manifestInfo := runtimeManifestToInfo(manifest)

	existingManifest, readErr := loadInstalledRuntimeManifest(cfg.WorkingDir)
	if readErr != nil {
		return nil, errors.Wrap(readErr, "failed to read installed runtime state")
	} else if existingManifest != nil {
		if existingManifest.matchesRuntime(cfg) && manifestsEqual(*existingManifest, manifest) {
			verifyErr := verifyRuntimeManifest(*existingManifest, cfg.WorkingDir)
			if verifyErr == nil {
				print.Verb("runtime binaries already up to date")
				if err := writeInstalledRuntimeManifest(ctx, cfg.WorkingDir, *existingManifest); err != nil {
					return manifestInfo, errors.Wrap(err, "runtime binaries verified; failed to save installed runtime state")
				}
				return manifestInfo, nil
			}
			print.Warn("installed runtime verification failed, reinstalling:", verifyErr)
		}
	} else if verifyErr := verifyRuntimeManifest(manifest, cfg.WorkingDir); verifyErr == nil {
		print.Verb("runtime binaries already up to date")
		if err := writeInstalledRuntimeManifest(ctx, cfg.WorkingDir, manifest); err != nil {
			return manifestInfo, errors.Wrap(err, "runtime binaries verified; failed to save installed runtime state")
		}
		return manifestInfo, nil
	}
	managed := make(map[string]struct{})
	if existingManifest != nil {
		for _, file := range existingManifest.Files {
			if _, err := runtimeFilePath(cfg.WorkingDir, file.Path); err != nil {
				return nil, err
			}
			managed[strings.ToLower(filepath.Clean(filepath.FromSlash(file.Path)))] = struct{}{}
		}
	}
	for _, file := range manifest.Files {
		target, err := runtimeFilePath(cfg.WorkingDir, file.Path)
		if err != nil {
			return nil, err
		}
		if _, err := os.Lstat(target); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		if _, owned := managed[strings.ToLower(filepath.Clean(filepath.FromSlash(file.Path)))]; owned {
			continue
		}
		if verifyRuntimeManifest(runtimeManifest{Files: []runtimeFileInfo{file}}, cfg.WorkingDir) != nil {
			return nil, errors.Wrapf(os.ErrExist, "runtime file %s already exists and is not tracked by the installed runtime", file.Path)
		}
	}

	if err = copyRuntimeFilesContext(ctx, manifest, stageDir, cfg.WorkingDir); err != nil {
		return nil, errors.Wrap(err, "failed to install runtime binaries")
	}
	if err := verifyRuntimeManifest(manifest, cfg.WorkingDir); err != nil {
		return nil, errors.Wrap(err, "failed to verify installed runtime binaries")
	}
	if existingManifest != nil {
		obsolete, err := obsoleteRuntimeFiles(*existingManifest, manifest, cfg.WorkingDir)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := removeRuntimeFiles(obsolete, cfg.WorkingDir); err != nil {
			return manifestInfo, errors.Wrap(err, "runtime binaries installed; failed to remove obsolete runtime files")
		}
	}
	if err := writeInstalledRuntimeManifest(ctx, cfg.WorkingDir, manifest); err != nil {
		return manifestInfo, errors.Wrap(err, "runtime binaries installed; failed to save installed runtime state")
	}

	return manifestInfo, nil
}

// EnsureScripts checks that all the declared scripts are present
func EnsureScripts(cfg run.Runtime) (err error) {
	errs := []string{}

	gamemodes := filepath.Join(cfg.WorkingDir, "gamemodes")
	if fs.Exists(gamemodes) {
		for _, gamemode := range cfg.Gamemodes {
			fullpath := filepath.Join(gamemodes, gamemode+".amx")
			if !fs.Exists(fullpath) {
				errs = append(errs, fmt.Sprintf(
					"gamemode '%s' is missing its .amx file from the gamemodes directory",
					gamemode,
				))
			}
		}
	}

	filterscripts := filepath.Join(cfg.WorkingDir, "filterscripts")
	if fs.Exists(filterscripts) {
		for _, filterscript := range cfg.Filterscripts {
			fullpath := filepath.Join(cfg.WorkingDir, "filterscripts", filterscript+".amx")
			if !fs.Exists(fullpath) {
				errs = append(errs, fmt.Sprintf(
					"filterscript '%s' is missing its .amx file from the filterscripts directory",
					filterscript,
				))
			}
		}
	}

	if len(errs) > 0 {
		err = errors.New(strings.Join(errs, ", "))
	}

	return err
}

func pluginExtForFile(os string) (ext string) {
	switch os {
	case "windows":
		ext = ".dll"
	case "linux", "darwin":
		ext = ".so"
	}
	return
}

func ensureStagedRuntime(ctx context.Context, cacheDir string, cfg run.Runtime) (_ runtimeManifest, _ string, err error) {
	if err := ctx.Err(); err != nil {
		return runtimeManifest{}, "", err
	}
	for _, part := range []string{cfg.Platform, cfg.Version} {
		if !filepath.IsLocal(part) || filepath.Base(part) != part || part == "." {
			return runtimeManifest{}, "", errors.Wrapf(os.ErrInvalid, "invalid runtime cache path component %q", part)
		}
	}
	stageDir := filepath.Join(cacheDir, runtimeStagingDir, cfg.Platform, cfg.Version)
	if _, err := runtimeFilePath(cacheDir, filepath.Join(runtimeStagingDir, cfg.Platform, cfg.Version, runtimeManifestFileName)); err != nil {
		return runtimeManifest{}, "", err
	}
	manifestPath := runtimeManifestPath(stageDir)

	if fs.Exists(manifestPath) {
		manifest, err := readRuntimeManifest(manifestPath)
		if err == nil && manifest.matchesRuntime(cfg) {
			if err = validateRuntimePackage(manifest, stageDir); err == nil {
				return manifest, stageDir, nil
			}
			print.Warn("staged runtime verification failed, rebuilding cache:", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(stageDir), 0o755); err != nil {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to create runtime staging directory")
	}
	prepared, err := os.MkdirTemp(filepath.Dir(stageDir), ".sampctl-runtime-")
	if err != nil {
		return runtimeManifest{}, "", err
	}
	cleanupPrepared := true
	defer func() {
		if cleanupPrepared {
			err = stderrors.Join(err, os.RemoveAll(prepared))
		}
	}()

	if err := getServerPackageContext(ctx, cacheDir, cfg.Version, prepared, cfg.Platform); err != nil {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to download runtime package")
	}

	manifest, err := buildRuntimeManifest(prepared, cfg)
	if err != nil {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to build runtime manifest")
	}

	if err := validateRuntimePackage(manifest, prepared); err != nil {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to validate runtime package")
	}
	if err := writeRuntimeManifest(runtimeManifestPath(prepared), manifest); err != nil {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to persist runtime manifest")
	}
	if err := ctx.Err(); err != nil {
		return runtimeManifest{}, "", err
	}
	var backup string
	if _, err := os.Lstat(stageDir); err == nil {
		backup, err = os.MkdirTemp(filepath.Dir(stageDir), ".sampctl-runtime-backup-")
		if err != nil {
			return runtimeManifest{}, "", err
		}
		if err := os.Remove(backup); err != nil {
			return runtimeManifest{}, "", err
		}
		if err := os.Rename(stageDir, backup); err != nil {
			return runtimeManifest{}, "", errors.Wrap(err, "failed to preserve previous runtime cache")
		}
	} else if !os.IsNotExist(err) {
		return runtimeManifest{}, "", err
	}
	if err := os.Rename(prepared, stageDir); err != nil {
		if backup != "" {
			if restoreErr := os.Rename(backup, stageDir); restoreErr != nil {
				cleanupPrepared = false
				err = stderrors.Join(err, errors.Wrapf(restoreErr, "previous runtime cache remains at %s", backup))
			}
		}
		return runtimeManifest{}, "", errors.Wrap(err, "failed to promote runtime cache")
	}
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			return runtimeManifest{}, "", errors.Wrapf(err, "runtime cache prepared; failed to remove previous cache at %s", backup)
		}
	}

	return manifest, stageDir, nil
}

func validateRuntimePackage(manifest runtimeManifest, root string) error {
	if err := verifyRuntimeManifest(manifest, root); err != nil {
		return err
	}
	binary := defaultServerBinary(manifest.Platform, run.DetectRuntimeType(manifest.Version))
	if binary == "" {
		return errors.Wrap(os.ErrInvalid, "unsupported runtime platform")
	}
	for _, file := range manifest.Files {
		if strings.Contains(filepath.Base(file.Path), binary) {
			if file.Size == 0 || (goruntime.GOOS != "windows" && fs.IsPosixPlatform(manifest.Platform) && os.FileMode(file.Mode)&0o111 == 0) {
				return errors.Wrapf(os.ErrInvalid, "runtime binary %s is empty or not executable", file.Path)
			}
			return nil
		}
	}
	return errors.Wrapf(os.ErrNotExist, "runtime package is missing %s", binary)
}

func obsoleteRuntimeFiles(previous, current runtimeManifest, root string) (runtimeManifest, error) {
	retained := make(map[string]struct{}, len(current.Files))
	for _, file := range current.Files {
		retained[strings.ToLower(filepath.Clean(filepath.FromSlash(file.Path)))] = struct{}{}
	}
	obsolete := previous
	obsolete.Files = nil
	for _, file := range previous.Files {
		if _, err := runtimeFilePath(root, file.Path); err != nil {
			return runtimeManifest{}, err
		}
		if _, exists := retained[strings.ToLower(filepath.Clean(filepath.FromSlash(file.Path)))]; exists {
			continue
		}
		// A changed file belongs to the user; only remove unchanged files from the old manifest.
		if verifyRuntimeManifest(runtimeManifest{Files: []runtimeFileInfo{file}}, root) == nil {
			obsolete.Files = append(obsolete.Files, file)
		}
	}
	return obsolete, nil
}
