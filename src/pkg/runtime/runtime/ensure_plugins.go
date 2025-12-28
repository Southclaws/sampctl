package runtime

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
	pkgresource "github.com/Southclaws/sampctl/src/resource"
)

// getPluginDirectory returns the directory used for plugins
func getPluginDirectory() string {
	return "plugins"
}

// EnsurePlugins validates and downloads plugin binary files
func EnsurePlugins(
	ctx context.Context,
	gh *github.Client,
	cfg *run.Runtime,
	cacheDir string,
	noCache bool,
) (err error) {
	if err := fs.EnsurePackageLayout(cfg.WorkingDir, cfg.IsOpenMP()); err != nil {
		return err
	}

	fileExt := pluginExtForFile(cfg.Platform)

	var files []run.Plugin

	addedPlugins := make(map[run.Plugin]struct{})
	addedComponents := make(map[run.Plugin]struct{})

	for _, plugin := range cfg.PluginDeps {
		// Local scheme dependencies (plugin://local/... / component://local/...) are already present
		// in the workspace; they should not be downloaded from the network.
		if plugin.IsLocalScheme() {
			name := run.Plugin(plugin.Repo)
			if plugin.Scheme == "component" {
				if _, ok := addedComponents[name]; ok {
					continue
				}
				print.Verb("adding local component", name)
				cfg.Components = append(cfg.Components, name)
				addedComponents[name] = struct{}{}
				continue
			}

			if _, ok := addedPlugins[name]; ok {
				continue
			}
			print.Verb("adding local plugin", name)
			cfg.Plugins = append(cfg.Plugins, name)
			addedPlugins[name] = struct{}{}
			continue
		}

		destDir := getPluginDirectory()
		if plugin.Scheme == "component" {
			destDir = "components"
		}

		files, err = EnsureVersionedPlugin(ctx, gh, plugin, cfg.WorkingDir, cfg.Platform, cfg.Version, cacheDir, destDir, true, false, noCache, nil)
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
				cfg.Components = append(cfg.Components, name)
				addedComponents[name] = struct{}{}
				continue
			}

			if _, ok := addedPlugins[name]; ok {
				continue
			}
			print.Verb("adding plugin by local filename", name)
			cfg.Plugins = append(cfg.Plugins, name)
			addedPlugins[name] = struct{}{}
		}
	}

	return nil
}

// EnsureVersionedPlugin automatically downloads a plugin binary from its github releases page
func EnsureVersionedPlugin(
	ctx context.Context,
	gh *github.Client,
	meta versioning.DependencyMeta,
	dir string,
	platform string,
	version string,
	cacheDir string,
	pluginDestDir string,
	plugins bool,
	includes bool,
	noCache bool,
	ignorePatterns []string,
) (files []run.Plugin, err error) {
	filename, resource, err := EnsureVersionedPluginCached(ctx, meta, platform, version, cacheDir, noCache, gh)
	if err != nil {
		return
	}

	print.Verb(meta, "retrieved package to file:", filename)

	if resource.Archive {
		print.Verb(meta, "plugin resource is an archive")
		ext := filepath.Ext(filename)
		if ext == "" {
			ext = detectArchiveExt(filename)
		}

		paths := make(map[string]string)

		// get plugins
		if plugins {
			if pluginDestDir == "" {
				return nil, errors.New("pluginDestDir is required when plugins=true")
			}
			for _, plugin := range resource.Plugins {
				pluginDir := pluginDestDir + "/"
				print.Verb(meta, "marking plugin path", plugin, "for extraction to ./"+pluginDir)
				paths[plugin] = pluginDir
			}
		}

		// get include directories
		if includes {
			for _, include := range resource.Includes {
				print.Verb(meta, "marking include path", include, "for extraction")
				paths[include] = ""
			}
		}

		// get additional files
		for src, dest := range resource.Files {
			if _, ok := paths[src]; ok {
				// Don't override plugin/include destinations.
				continue
			}
			print.Verb(meta, "marking misc file path", src, "for extraction to", dest)
			paths[src] = dest
		}

		if len(ignorePatterns) > 0 {
			print.Verb(meta, "using", len(ignorePatterns), "ignore pattern(s) for extraction")
		}

		var extractedFiles map[string]string
		switch ext {
		case ".zip":
			extractedFiles, err = download.UnzipWithIgnore(filename, dir, paths, ignorePatterns)
		case ".gz":
			extractedFiles, err = download.UntarWithIgnore(filename, dir, paths, ignorePatterns)
		default:
			err = errors.Errorf("unsupported archive format: %s", filename)
			return
		}
		if err != nil {
			err = errors.Wrapf(err, "failed to extract plugin %s to %s", meta, dir)
			return
		}
		if len(extractedFiles) == 0 {
			//nolint:lll
			err = errors.Errorf("no files extracted from plugin %s: check the package definition of this dependency against the release assets", meta)
			return
		}
		print.Verb(meta, "extracted", len(extractedFiles), "plugin files to", dir)

		for source, target := range extractedFiles {
			for _, plugin := range resource.Plugins {
				print.Verb(meta, "checking resource source", source, "against plugin", plugin)
				if source == plugin {
					files = append(files, run.Plugin(filepath.Base(target)))
				}
			}
		}
	} else {
		print.Verb(meta, "plugin resource is a single file")
		base := filepath.Base(filename)
		if pluginDestDir == "" {
			return nil, errors.New("pluginDestDir is required when plugins=true")
		}
		finalDir := filepath.Join(dir, pluginDestDir)
		destination := filepath.Join(finalDir, base)

		err = fs.EnsureDir(finalDir, fs.PermDirShared)
		if err != nil {
			err = errors.Wrapf(err, "failed to create path for plugin resource %s to %s", filename, destination)
			return
		}

		err = util.CopyFile(filename, destination)
		if err != nil {
			err = errors.Wrapf(err, "failed to copy non-archive file %s to %s", filename, destination)
			return
		}
		files = []run.Plugin{run.Plugin(base)}
	}

	return files, err
}

func detectArchiveExt(filename string) string {
	f, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer f.Close()

	var b [4]byte
	_, err = f.Read(b[:])
	if err != nil {
		return ""
	}

	if b[0] == 'P' && b[1] == 'K' {
		return ".zip"
	}

	if b[0] == 0x1f && b[1] == 0x8b {
		return ".gz"
	}

	return ""
}

// EnsureVersionedPluginCached ensures that a plugin exists in the cache
func EnsureVersionedPluginCached(
	ctx context.Context,
	meta versioning.DependencyMeta,
	platform,
	version,
	cacheDir string,
	noCache bool,
	gh *github.Client,
) (
	filename string,
	resource *pkgresource.Resource,
	err error,
) {
	hit := false
	// only pull from cache if there is a version tag specified
	if !noCache && meta.Tag != "" {
		hit, filename, resource, err = PluginFromCache(meta, platform, version, cacheDir)
		if err != nil {
			err = errors.Wrapf(err, "failed to get plugin %s from cache", meta)
			return
		}
	}
	if !hit {
		if meta.Tag == "" {
			//nolint:lll
			print.Info("Downloading newest plugin because no version is specified. Consider specifying a version for this dependency.")
		}

		filename, resource, err = PluginFromNet(ctx, gh, meta, platform, version, cacheDir)
		if err != nil {
			err = errors.Wrapf(err, "failed to get plugin %s from net", meta)
			return
		}
	}

	return filename, resource, nil
}

// PluginFromCache tries to grab a plugin asset from the cache, `hit` indicates if it was successful
func PluginFromCache(
	meta versioning.DependencyMeta,
	platform string,
	version string,
	cacheDir string,
) (hit bool, filename string, resource *pkgresource.Resource, err error) {
	print.Verb("getting plugin resource from cache", meta)

	pkg, err := pawnpackage.GetCachedPackage(meta, cacheDir)
	if err != nil {
		print.Verb("cache hit failed while trying to get cached package:", err)
		err = nil
		hit = false
		return
	}
	if pkg.Format == "" {
		return
	}

	resource, err = GetResource(pkg.Resources, platform, version)
	if err != nil {
		return
	}

	matcher, err := regexp.Compile(resource.Name)
	if err != nil {
		err = errors.Wrap(err, "resource name is not a valid regular expression")
		return
	}

	ghr := infraresource.NewGitHubReleaseResource(meta, matcher, infraresource.ResourceTypePlugin, nil)
	ghr.SetCacheDir(cacheDir)

	var ok bool
	ok, filename = ghr.Cached(meta.Tag)
	if !ok {
		return
	}

	hit = true

	return hit, filename, resource, nil
}

// PluginFromNet downloads a plugin from the given metadata to the cache directory
func PluginFromNet(
	ctx context.Context,
	gh *github.Client,
	meta versioning.DependencyMeta,
	platform string,
	version string,
	cacheDir string,
) (filename string, resource *pkgresource.Resource, err error) {
	print.Info(meta, "downloading plugin resource for", platform)

	pkg, err := pawnpackage.GetRemotePackage(ctx, gh, meta)
	if err != nil {
		err = errors.Wrap(err, "failed to get remote package definition file")
		return
	}

	resource, err = GetResource(pkg.Resources, platform, version)
	if err != nil {
		return
	}

	matcher, err := regexp.Compile(resource.Name)
	if err != nil {
		err = errors.Wrap(err, "resource name is not a valid regular expression")
		return
	}

	downloader := infraresource.NewGitHubReleaseResource(meta, matcher, infraresource.ResourceTypePlugin, gh)
	downloader.SetCacheDir(cacheDir)

	requestedVersion := meta.Tag
	if requestedVersion == "" {
		requestedVersion = "latest"
	}
	if err = downloader.Ensure(ctx, requestedVersion, ""); err != nil {
		return
	}

	actualVersion := requestedVersion
	if actualVersion == "latest" {
		actualVersion = downloader.Version()
	}
	_, filename = downloader.Cached(actualVersion)
	if filename == "" {
		err = errors.New("failed to locate downloaded asset")
		return
	}

	print.Verb(meta, "downloaded", filename, "to cache")

	return filename, resource, nil
}

// GetResource searches a list of resources for one that matches the given platform
func GetResource(resources []pkgresource.Resource, platform string, version string) (*pkgresource.Resource, error) {
	if version == "" {
		version = "0.3.7"
	}

	found := false
	var tmp *pkgresource.Resource
	for _, resource := range resources {
		res := resource
		if res.Platform == platform {
			if res.Version == version {
				tmp = &res
				found = true
				break
			}
		}
	}
	if !found {
		for _, resource := range resources {
			res := resource
			if res.Platform == platform && res.Version == "" {
				print.Verb("no resource matching version: ", version, ", falling back to the first resource matching platform: ", platform)
				tmp = &res
				found = true
				break
			}
		}
	}
	if !found {
		return nil, errors.Errorf("plugin does not provide binaries for target platform %s and/or version %s", platform, version)
	}

	if err := tmp.Validate(); err != nil {
		return nil, errors.Wrap(err, "matching resource found but is invalid")
	}

	return tmp, nil
}

// GetResourcePath returns a path where a resource should be stored given the metadata
func GetResourcePath(meta versioning.DependencyMeta) (path string) {
	tag := meta.Tag
	if tag == "" {
		tag = "latest"
	}
	return filepath.Join("plugins", meta.Repo, tag)
}
