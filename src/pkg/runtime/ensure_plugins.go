package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

// getPluginDirectory returns the directory used for plugins
func getPluginDirectory() string {
	return "plugins"
}

// EnsurePluginsRequest describes runtime plugin preparation.
type EnsurePluginsRequest struct {
	Context  context.Context
	GitHub   *github.Client
	Config   *run.Runtime
	CacheDir string
	NoCache  bool
}

// EnsureVersionedPluginRequest describes plugin acquisition and extraction.
type EnsureVersionedPluginRequest struct {
	Context            context.Context
	GitHub             *github.Client
	Meta               versioning.DependencyMeta
	Dir                string
	Platform           string
	Version            string
	CacheDir           string
	PluginDestDir      string
	Plugins            bool
	Includes           bool
	NoCache            bool
	IgnorePatterns     []string
	RestrictFilesToDir bool
}

// EnsurePlugins validates and downloads plugin binary files.
func EnsurePlugins(request EnsurePluginsRequest) (err error) {
	if err := request.Context.Err(); err != nil {
		return err
	}

	if _, err := runtimeFilePath(request.Config.WorkingDir, ".sampctl-layout-check"); err != nil {
		return err
	}

	if err := fs.EnsurePackageLayout(request.Config.WorkingDir, request.Config.IsOpenMP()); err != nil {
		return err
	}

	fileExt := pluginExtForFile(request.Config.Platform)

	var files []run.Plugin

	addedPlugins := make(map[run.Plugin]struct{})
	addedComponents := make(map[run.Plugin]struct{})

	for _, plugin := range request.Config.PluginDeps {
		if err := request.Context.Err(); err != nil {
			return err
		}

		// Local plugins and components already exist in the workspace.
		if plugin.IsLocalScheme() {
			name := run.Plugin(plugin.Repo)
			if plugin.Scheme == "component" {
				if _, ok := addedComponents[name]; ok {
					continue
				}
				print.Verb("adding local component", name)
				request.Config.Components = append(request.Config.Components, name)
				addedComponents[name] = struct{}{}
				continue
			}

			if _, ok := addedPlugins[name]; ok {
				continue
			}
			print.Verb("adding local plugin", name)
			request.Config.Plugins = append(request.Config.Plugins, name)
			addedPlugins[name] = struct{}{}
			continue
		}

		destDir := getPluginDirectory()
		if plugin.Scheme == "component" {
			destDir = "components"
		}

		files, err = EnsureVersionedPlugin(EnsureVersionedPluginRequest{
			Context:       request.Context,
			GitHub:        request.GitHub,
			Meta:          plugin,
			Dir:           request.Config.WorkingDir,
			Platform:      request.Config.Platform,
			Version:       request.Config.Version,
			CacheDir:      request.CacheDir,
			PluginDestDir: destDir,
			Plugins:       true,
			Includes:      false,
			NoCache:       request.NoCache,
		})
		if err != nil {
			return err
		}

		for _, file := range files {
			name := run.Plugin(strings.TrimSuffix(string(file), fileExt))

			if plugin.Scheme == "component" {
				if _, ok := addedComponents[name]; ok {
					continue
				}
				print.Verb("adding component by local filename", name)
				request.Config.Components = append(request.Config.Components, name)
				addedComponents[name] = struct{}{}
				continue
			}

			if _, ok := addedPlugins[name]; ok {
				continue
			}
			print.Verb("adding plugin by local filename", name)
			request.Config.Plugins = append(request.Config.Plugins, name)
			addedPlugins[name] = struct{}{}
		}
	}

	return nil
}

// EnsureVersionedPlugin automatically downloads a plugin binary from its github releases page
func EnsureVersionedPlugin(request EnsureVersionedPluginRequest) (files []run.Plugin, err error) {
	if err := request.Context.Err(); err != nil {
		return nil, err
	}

	filename, resource, err := EnsureVersionedPluginCached(EnsureVersionedPluginCachedRequest{
		Context:  request.Context,
		Meta:     request.Meta,
		Platform: request.Platform,
		Version:  request.Version,
		CacheDir: request.CacheDir,
		NoCache:  request.NoCache,
		GitHub:   request.GitHub,
	})
	if err != nil {
		return
	}

	print.Verb(request.Meta, "using plugin asset:", filename)

	if resource.Archive {
		print.Verb(request.Meta, "plugin resource is an archive")
		ext := filepath.Ext(filename)
		if ext == "" {
			ext = detectArchiveExt(filename)
		}

		paths := make(map[string]string)

		// get plugins
		if request.Plugins {
			if request.PluginDestDir == "" {
				return nil, errors.New("plugin destination directory is required")
			}
			for _, plugin := range resource.Plugins {
				pluginDir := request.PluginDestDir + "/"
				print.Verb(request.Meta, "marking plugin path", plugin, "for extraction to ./"+pluginDir)
				paths[plugin] = pluginDir
			}
		}

		// get include directories
		if request.Includes {
			for _, include := range resource.Includes {
				print.Verb(request.Meta, "marking include path", include, "for extraction")
				paths[include] = ""
			}
		}

		// get additional files
		for src, dest := range resource.Files {
			if _, ok := paths[src]; ok {
				// Don't override plugin/include destinations.
				continue
			}
			if request.RestrictFilesToDir && dest != "" && !filepath.IsLocal(dest) {
				return nil, errors.Wrapf(os.ErrInvalid, "resource file destination %q must stay inside the dependency directory", dest)
			}
			print.Verb(request.Meta, "marking misc file path", src, "for extraction to", dest)
			paths[src] = dest
		}

		if len(request.IgnorePatterns) > 0 {
			print.Verb(request.Meta, "using", len(request.IgnorePatterns), "ignore pattern(s) for extraction")
		}

		extractedFiles, extractErr := extractPluginArchive(request, filename, ext, paths)
		err = extractErr
		if err != nil {
			err = errors.Wrapf(err, "failed to extract plugin %s to %s", request.Meta, request.Dir)
			return
		}
		if len(extractedFiles) == 0 {
			//nolint:lll
			err = errors.Errorf("plugin %s archive produced no files. Check its resource paths and ignore patterns", request.Meta)
			return
		}

		print.Verb(request.Meta, "extracted", len(extractedFiles), "plugin files to", request.Dir)

		seen := make(map[run.Plugin]bool)
		for _, source := range resource.Plugins {
			for _, target := range extractedFiles[source] {
				plugin := run.Plugin(filepath.Base(target))
				if !seen[plugin] {
					files = append(files, plugin)
					seen[plugin] = true
				}
			}
		}
	} else {
		print.Verb(request.Meta, "plugin resource is a single file")
		base := filepath.Base(filename)
		if request.PluginDestDir == "" {
			return nil, errors.New("plugin destination directory is required")
		}

		finalDir := filepath.Join(request.Dir, request.PluginDestDir)
		destination, pathErr := runtimeFilePath(request.Dir, filepath.Join(request.PluginDestDir, base))
		if pathErr != nil {
			return nil, pathErr
		}

		err = fs.EnsureDir(finalDir, fs.PermDirShared)
		if err != nil {
			err = errors.Wrapf(err, "failed to create path for plugin resource %s to %s", filename, destination)
			return
		}

		info, statErr := os.Lstat(filename)
		if statErr != nil {
			return nil, statErr
		}
		if !info.Mode().IsRegular() {
			return nil, errors.Wrap(os.ErrInvalid, "plugin asset must be a regular file")
		}

		err = copyFileWithModeContext(request.Context, filename, destination, info.Mode())
		if err != nil {
			err = errors.Wrapf(err, "failed to copy non-archive file %s to %s", filename, destination)
			return
		}
		files = []run.Plugin{run.Plugin(base)}
	}

	return files, err
}
