package runtime

import (
	"context"
	stderrors "errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
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

func extractPluginArchive(
	request EnsureVersionedPluginRequest,
	filename, ext string,
	paths map[string]string,
) (_ map[string]string, err error) {
	if _, err := runtimeFilePath(request.Dir, ".sampctl-plugin-stage"); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(request.Dir, 0o755); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(request.Dir, ".sampctl-plugin-")
	if err != nil {
		return nil, err
	}
	defer func() { err = stderrors.Join(err, os.RemoveAll(stage)) }()
	stagedPaths := make(map[string]string, len(paths))
	keys := make([]string, 0, len(paths))
	for source := range paths {
		keys = append(keys, source)
	}
	sort.Strings(keys)
	for index, source := range keys {
		stagedPaths[source] = fmt.Sprintf("%d/", index)
	}
	var extracted map[string]string
	switch ext {
	case ".zip":
		extracted, err = download.Unzip(filename, stage, stagedPaths)
	case ".gz":
		extracted, err = download.Untar(filename, stage, stagedPaths)
	default:
		return nil, errors.Errorf("unsupported archive format: %s", filename)
	}
	if err != nil {
		return nil, err
	}
	if err := request.Context.Err(); err != nil {
		return nil, err
	}
	targets := make(map[string]string, len(extracted))
	type stagedFile struct {
		path   string
		target string
	}
	var stagedFiles []stagedFile
	for source, staged := range extracted {
		matches := []string{staged}
		if paths[source] == "" || strings.HasSuffix(paths[source], "/") {
			// The extractor returns only the last match for each pattern, but
			// directory mappings must install every file in its staging directory.
			entries, err := os.ReadDir(filepath.Dir(staged))
			if err != nil {
				return nil, err
			}
			matches = nil
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				matches = append(matches, filepath.Join(filepath.Dir(staged), entry.Name()))
			}
		}
		for _, path := range matches {
			relative, err := filepath.Rel(stage, path)
			if err != nil {
				return nil, err
			}
			if _, err := runtimeFilePath(stage, relative); err != nil {
				return nil, err
			}
			target := paths[source]
			if target == "" {
				target = filepath.Base(path)
			} else if strings.HasSuffix(target, "/") {
				target = filepath.Join(target, filepath.Base(path))
			}
			root := request.Dir
			if !filepath.IsLocal(target) {
				if request.RestrictFilesToDir {
					return nil, errors.Wrapf(os.ErrInvalid, "plugin destination %q must stay inside its installation", target)
				}
				// Explicit resource mappings may target files outside the runtime directory.
				if !filepath.IsAbs(target) {
					target = filepath.Join(root, target)
				}
				root, target = filepath.Dir(target), filepath.Base(target)
			}
			final, err := runtimeFilePath(root, target)
			if err != nil {
				return nil, err
			}
			if ignoredRuntimePlugin(final, request.IgnorePatterns) {
				continue
			}
			stagedFiles = append(stagedFiles, stagedFile{path: path, target: final})
			if _, recorded := targets[source]; !recorded || path == staged {
				targets[source] = final
			}
		}
	}
	for _, file := range stagedFiles {
		info, err := os.Lstat(file.path)
		if err != nil {
			return nil, err
		}
		if err := copyFileWithModeContext(request.Context, file.path, file.target, info.Mode()); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

func ignoredRuntimePlugin(path string, patterns []string) bool {
	if _, err := os.Lstat(path); err != nil {
		return false
	}
	for _, pattern := range patterns {
		if matched, err := filepath.Match(pattern, filepath.Base(path)); err == nil && matched {
			return true
		}
		if matched, err := filepath.Match(pattern, path); err == nil && matched {
			return true
		}
	}
	return false
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
			print.Info(request.Meta, "downloading the latest release because no version is specified. Pin a release tag to keep the plugin version fixed.")
		}

		filename, resource, err = pluginFromNet(PluginFetchRequest{
			Context:  request.Context,
			GitHub:   request.GitHub,
			Meta:     request.Meta,
			Platform: request.Platform,
			Version:  request.Version,
			CacheDir: request.CacheDir,
		}, resource)
		if err != nil {
			err = errors.Wrapf(err, "failed to download plugin %s", request.Meta)
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
		print.Verb("could not load cached plugin package:", err)
		err = nil
		hit = false
		return
	}
	if pkg.Format == "" {
		return
	}

	resource, err = GetResource(pkg.Resources, platform, version)
	if err != nil {
		var missing *PluginResourceNotFoundError
		if stderrors.As(err, &missing) {
			missing.Meta = meta
		}
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
	return pluginFromNet(request, nil)
}

func pluginFromNet(request PluginFetchRequest, cachedResource *pkgresource.Resource) (filename string, resource *pkgresource.Resource, err error) {
	print.Info(request.Meta, "downloading plugin resource for", request.Platform)

	resource = cachedResource
	if resource == nil {
		pkg, fetchErr := pawnpackage.GetRemotePackage(request.Context, request.GitHub, request.Meta)
		if fetchErr != nil {
			return "", nil, errors.Wrap(fetchErr, "failed to get remote package definition file")
		}

		resource, err = GetResource(pkg.Resources, request.Platform, request.Version)
		if err != nil {
			var missing *PluginResourceNotFoundError
			if stderrors.As(err, &missing) {
				missing.Meta = request.Meta
			}
			return
		}
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
				print.Verb("using the default resource for", platform, "because none matches version", version)
				tmp = &res
				found = true
				break
			}
		}
	}
	if !found {
		return nil, &PluginResourceNotFoundError{Platform: platform, Version: version}
	}

	if err := tmp.Validate(); err != nil {
		return nil, errors.Wrap(err, "plugin resource is invalid")
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
