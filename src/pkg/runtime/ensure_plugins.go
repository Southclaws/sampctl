package runtime

import (
	"context"
	iofs "io/fs"
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
	pkgresource "github.com/Southclaws/sampctl/src/pkg/package/resource"
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
	Context        context.Context
	GitHub         *github.Client
	Meta           versioning.DependencyMeta
	Dir            string
	Platform       string
	Version        string
	CacheDir       string
	PluginDestDir  string
	Plugins        bool
	Includes       bool
	NoCache        bool
	IgnorePatterns []string
}

// EnsureVersionedPluginCachedRequest describes a cache lookup for a versioned plugin asset.
type EnsureVersionedPluginCachedRequest struct {
	Context  context.Context
	Meta     versioning.DependencyMeta
	Platform string
	Version  string
	CacheDir string
	NoCache  bool
	GitHub   *github.Client
}

// PluginFetchRequest describes a plugin asset fetch from the network.
type PluginFetchRequest struct {
	Context  context.Context
	GitHub   *github.Client
	Meta     versioning.DependencyMeta
	Platform string
	Version  string
	CacheDir string
}

// EnsurePlugins validates and downloads plugin binary files.
func EnsurePlugins(request EnsurePluginsRequest) (err error) {
	if err := fs.EnsurePackageLayout(request.Config.WorkingDir, request.Config.IsOpenMP()); err != nil {
		return err
	}

	fileExt := pluginExtForFile(request.Config.Platform)

	var files []run.Plugin

	addedPlugins := make(map[run.Plugin]struct{})
	addedComponents := make(map[run.Plugin]struct{})

	for _, plugin := range request.Config.PluginDeps {
		// Local scheme dependencies (plugin://local/... / component://local/...) are already present
		// in the workspace; they should not be downloaded from the network.
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

	print.Verb(request.Meta, "retrieved package to file:", filename)

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
				return nil, errors.New("pluginDestDir is required when plugins=true")
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
			print.Verb(request.Meta, "marking misc file path", src, "for extraction to", dest)
			paths[src] = dest
		}

		if len(request.IgnorePatterns) > 0 {
			print.Verb(request.Meta, "using", len(request.IgnorePatterns), "ignore pattern(s) for extraction")
		}

		var extractedFiles map[string]string
		switch ext {
		case ".zip":
			extractedFiles, err = download.UnzipWithIgnore(filename, request.Dir, paths, request.IgnorePatterns)
		case ".gz":
			extractedFiles, err = download.UntarWithIgnore(filename, request.Dir, paths, request.IgnorePatterns)
		default:
			err = errors.Errorf("unsupported archive format: %s", filename)
			return
		}
		if err != nil {
			err = errors.Wrapf(err, "failed to extract plugin %s to %s", request.Meta, request.Dir)
			return
		}
		if len(extractedFiles) == 0 {
			//nolint:lll
			err = errors.Errorf("no files extracted from plugin %s: check the package definition of this dependency against the release assets", request.Meta)
			return
		}
		print.Verb(request.Meta, "extracted", len(extractedFiles), "plugin files to", request.Dir)

		for source, target := range extractedFiles {
			for _, plugin := range resource.Plugins {
				print.Verb(request.Meta, "checking resource source", source, "against plugin", plugin)
				if source == plugin {
					files = append(files, run.Plugin(filepath.Base(target)))
				}
			}
		}
	} else {
		print.Verb(request.Meta, "plugin resource is a single file")
		base := filepath.Base(filename)
		if request.PluginDestDir == "" {
			return nil, errors.New("pluginDestDir is required when plugins=true")
		}
		finalDir := filepath.Join(request.Dir, request.PluginDestDir)
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

	var b [4]byte
	_, err = f.Read(b[:])
	if errClose := f.Close(); err != nil || errClose != nil {
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
func EnsureVersionedPluginCached(request EnsureVersionedPluginCachedRequest) (
	filename string,
	resource *pkgresource.Resource,
	err error,
) {
	hit := false
	if !request.NoCache {
		hit, filename, resource, err = PluginFromCache(request.Meta, request.Platform, request.Version, request.CacheDir)
		if err != nil {
			err = errors.Wrapf(err, "failed to get plugin %s from cache", request.Meta)
			return
		}
	}
	if !hit {
		if !hasExplicitDependencyReference(request.Meta) {
			//nolint:lll
			print.Info("Downloading newest plugin because no version is specified. Consider specifying a version for this dependency.")
		}

		filename, resource, err = PluginFromNet(PluginFetchRequest{
			Context:  request.Context,
			GitHub:   request.GitHub,
			Meta:     request.Meta,
			Platform: request.Platform,
			Version:  request.Version,
			CacheDir: request.CacheDir,
		})
		if err != nil {
			err = errors.Wrapf(err, "failed to get plugin %s from net", request.Meta)
			return
		}
	}

	return filename, resource, nil
}

func hasExplicitDependencyReference(meta versioning.DependencyMeta) bool {
	return meta.Tag != "" || meta.Branch != "" || meta.Commit != ""
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

	localFilename, found := cachedPackageResourceAsset(meta.CachePath(cacheDir), matcher)
	if found {
		hit = true
		return hit, localFilename, resource, nil
	}

	var ok bool
	ok, filename = ghr.Cached(meta.Tag)
	if !ok {
		return
	}

	hit = true

	return hit, filename, resource, nil
}

func cachedPackageResourceAsset(cachePath string, matcher *regexp.Regexp) (string, bool) {
	if matcher == nil || !fs.Exists(cachePath) {
		return "", false
	}

	var matched string
	err := filepath.WalkDir(cachePath, func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if matcher.MatchString(filepath.Base(path)) {
			matched = path
			return iofs.SkipAll
		}
		return nil
	})
	if err != nil || matched == "" {
		return "", false
	}

	return matched, true
}

// PluginFromNet downloads a plugin from the given metadata to the cache directory
func PluginFromNet(request PluginFetchRequest) (filename string, resource *pkgresource.Resource, err error) {
	print.Info(request.Meta, "downloading plugin resource for", request.Platform)

	pkg, err := pawnpackage.GetRemotePackage(request.Context, request.GitHub, request.Meta)
	if err != nil {
		err = errors.Wrap(err, "failed to get remote package definition file")
		return
	}

	resource, err = GetResource(pkg.Resources, request.Platform, request.Version)
	if err != nil {
		return
	}

	matcher, err := regexp.Compile(resource.Name)
	if err != nil {
		err = errors.Wrap(err, "resource name is not a valid regular expression")
		return
	}

	downloader := infraresource.NewGitHubReleaseResource(request.Meta, matcher, infraresource.ResourceTypePlugin, request.GitHub)
	downloader.SetCacheDir(request.CacheDir)

	requestedVersion := request.Meta.Tag
	if requestedVersion == "" {
		requestedVersion = "latest"
	}
	if err = downloader.Ensure(request.Context, requestedVersion, ""); err != nil {
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

	print.Verb(request.Meta, "downloaded", filename, "to cache")

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
