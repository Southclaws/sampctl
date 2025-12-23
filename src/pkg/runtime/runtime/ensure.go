package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
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
	err = EnsureBinaries(cacheDir, *cfg)
	if err != nil {
		return errors.Wrap(err, "failed to ensure runtime binaries")
	}

	print.Verb("ensuring all dependency and static plugins")
	err = EnsurePlugins(ctx, gh, cfg, cacheDir, noCache)
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
func EnsureBinaries(cacheDir string, cfg run.Runtime) error {
	manifest, stageDir, err := ensureStagedRuntime(cacheDir, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to prepare runtime files")
	}

	installManifestPath := runtimeManifestPath(cfg.WorkingDir)
	if fs.Exists(installManifestPath) {
		existingManifest, readErr := readRuntimeManifest(installManifestPath)
		if readErr != nil {
			print.Warn("failed to read installed runtime manifest:", readErr)
		} else {
			if existingManifest.matchesRuntime(cfg) {
				verifyErr := verifyRuntimeManifest(existingManifest, cfg.WorkingDir)
				if verifyErr == nil {
					print.Verb("runtime binaries already up to date")
					return nil
				}
				print.Warn("installed runtime verification failed, reinstalling:", verifyErr)
			}

			if removeErr := removeRuntimeFiles(existingManifest, cfg.WorkingDir); removeErr != nil {
				print.Warn("failed to remove previous runtime binaries:", removeErr)
			}
		}
	}

	if err = copyRuntimeFiles(manifest, stageDir, cfg.WorkingDir); err != nil {
		return errors.Wrap(err, "failed to install runtime binaries")
	}

	if err = writeRuntimeManifest(installManifestPath, manifest); err != nil {
		print.Warn("failed to write runtime manifest:", err)
	}

	return nil
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

func ensureStagedRuntime(cacheDir string, cfg run.Runtime) (runtimeManifest, string, error) {
	stageDir := filepath.Join(cacheDir, runtimeStagingDir, cfg.Platform, cfg.Version)
	manifestPath := runtimeManifestPath(stageDir)

	if fs.Exists(manifestPath) {
		manifest, err := readRuntimeManifest(manifestPath)
		if err == nil && manifest.matchesRuntime(cfg) {
			if err = verifyRuntimeManifest(manifest, stageDir); err == nil {
				return manifest, stageDir, nil
			}
			print.Warn("staged runtime verification failed, rebuilding cache:", err)
		}
	}

	if err := os.RemoveAll(stageDir); err != nil && !os.IsNotExist(err) {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to clean runtime staging directory")
	}
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to create runtime staging directory")
	}

	if err := GetServerPackage(cfg.Version, stageDir, cfg.Platform); err != nil {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to download runtime package")
	}

	manifest, err := buildRuntimeManifest(stageDir, cfg)
	if err != nil {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to build runtime manifest")
	}

	if err := writeRuntimeManifest(manifestPath, manifest); err != nil {
		return runtimeManifest{}, "", errors.Wrap(err, "failed to persist runtime manifest")
	}

	return manifest, stageDir, nil
}
