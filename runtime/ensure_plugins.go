package runtime

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// EnsurePlugins validates and downloads plugin binary files
func EnsurePlugins(ctx context.Context, gh *github.Client, cfg *types.Runtime, cacheDir string, noCache bool) (err error) {
	pluginsDir := util.FullPath(filepath.Join(cfg.WorkingDir, "plugins"))

	err = os.MkdirAll(pluginsDir, 0700)
	if err != nil {
		return errors.Wrap(err, "failed to create runtime plugins directory")
	}

	fileExt := pluginExtForFile(cfg.Platform)

	var (
		newPlugins = []types.Plugin{}
		files      []types.Plugin
	)

	for _, plugin := range cfg.PluginDeps {
		print.Verb("plugin", plugin, "is a package dependency")
		files, err = EnsureVersionedPlugin(ctx, gh, plugin, cfg.WorkingDir, cfg.Platform, cacheDir, true, false, noCache)
		if err != nil {
			print.Warn("failed to ensure plugin", plugin, err)
			err = nil
			continue
		}
		newPlugins = append(newPlugins, files...)
	}

	added := make(map[types.Plugin]struct{})

	// trim extensions for plugins list, they are added later by GenerateServerCFG if needed
	for _, plugin := range newPlugins {
		pluginName := types.Plugin(strings.TrimSuffix(string(plugin), fileExt))
		if _, ok := added[pluginName]; ok {
			continue
		}

		print.Verb("adding runtime plugin", pluginName)
		cfg.Plugins = append([]types.Plugin{pluginName}, cfg.Plugins...)
		added[pluginName] = struct{}{}
	}

	return
}

// EnsureVersionedPlugin automatically downloads a plugin binary from its github releases page
func EnsureVersionedPlugin(
	ctx context.Context,
	gh *github.Client,
	meta versioning.DependencyMeta,
	dir string,
	platform string,
	cacheDir string,
	plugins bool,
	includes bool,
	noCache bool,
) (files []types.Plugin, err error) {
	filename, resource, err := EnsureVersionedPluginCached(ctx, meta, platform, cacheDir, noCache, gh)
	if err != nil {
		return
	}

	print.Verb(meta, "retrieved package to file:", filename)

	if resource.Archive {
		var (
			ext    = filepath.Ext(filename)
			method download.ExtractFunc
		)
		if ext == ".zip" {
			method = download.Unzip
		} else if ext == ".gz" {
			method = download.Untar
		} else {
			err = errors.Errorf("unsupported archive format: %s", filename)
			return
		}

		paths := make(map[string]string)

		// get plugins
		if plugins {
			for _, plugin := range resource.Plugins {
				paths[plugin] = "plugins/"
			}
		}

		// get include directories
		if includes {
			for _, include := range resource.Includes {
				paths[include] = ""
			}
		}

		// get additional files
		for src, dest := range resource.Files {
			paths[src] = dest
		}

		var extractedFiles map[string]string
		extractedFiles, err = method(filename, dir, paths)
		if err != nil {
			err = errors.Wrapf(err, "failed to extract plugin %s to %s", meta, dir)
			return
		}

		for source, target := range extractedFiles {
			for _, plugin := range resource.Plugins {
				if source == plugin {
					files = append(files, types.Plugin(filepath.Base(target)))
				}
			}
		}
	} else {
		base := filepath.Base(filename)
		destination := filepath.Join(dir, "plugins", base)
		err = util.CopyFile(filename, destination)
		if err != nil {
			err = errors.Wrapf(err, "failed to copy non-archive file %s to %s", filename, destination)
			return
		}
		files = []types.Plugin{types.Plugin(base)}
	}

	return
}

// EnsureVersionedPluginCached ensures that a plugin exists in the cache
func EnsureVersionedPluginCached(
	ctx context.Context,
	meta versioning.DependencyMeta,
	platform,
	cacheDir string,
	noCache bool,
	gh *github.Client,
) (
	filename string,
	resource types.Resource,
	err error,
) {
	hit := false
	// only pull from cache if there is a version tag specified
	if !noCache && meta.Tag != "" {
		hit, filename, resource, err = PluginFromCache(meta, platform, cacheDir)
		if err != nil {
			err = errors.Wrapf(err, "failed to get plugin %s from cache", meta)
			return
		}
	}
	if !hit {
		if meta.Tag == "" {
			print.Info("Downloading newest plugin because no version is specified. Consider specifying a version for this dependency.")
		}

		filename, resource, err = PluginFromNet(ctx, gh, meta, platform, cacheDir)
		if err != nil {
			err = errors.Wrapf(err, "failed to get plugin %s from net", meta)
			return
		}
	}

	return
}

// PluginFromCache tries to grab a plugin asset from the cache, `hit` indicates if it was successful
func PluginFromCache(meta versioning.DependencyMeta, platform, cacheDir string) (hit bool, filename string, resource types.Resource, err error) {
	resourcePath := filepath.Join(cacheDir, GetResourcePath(meta))

	print.Verb("getting plugin resource from cache", meta, resourcePath)

	pkg, err := types.GetCachedPackage(meta, cacheDir)
	if err != nil {
		print.Verb("cache hit failed while trying to get cached package:", err)
		err = nil
		hit = false
		return
	}

	files, err := ioutil.ReadDir(resourcePath)
	if err != nil {
		print.Verb("cache hit failed while trying to read cached plugin folder:", err)
		err = nil
		hit = false
		return
	}

	resource, err = GetResourceForPlatform(pkg.Resources, platform)
	if err != nil {
		return
	}

	matcher, err := regexp.Compile(resource.Name)
	if err != nil {
		err = errors.Wrap(err, "resource name is not a valid regular expression")
		return
	}

	found := false
	name := ""
	for _, file := range files {
		name = file.Name()
		if matcher.MatchString(name) {
			found = true
			break
		}
	}
	if !found {
		return
	}

	hit = true
	filename = filepath.Join(resourcePath, name)
	return
}

// PluginFromNet downloads a plugin from the given metadata to the cache directory
func PluginFromNet(ctx context.Context, gh *github.Client, meta versioning.DependencyMeta, platform, cacheDir string) (filename string, resource types.Resource, err error) {
	print.Info(meta, "downloading plugin resource for", platform)

	resourcePathOnly := GetResourcePath(meta)
	resourcePath := filepath.Join(cacheDir, resourcePathOnly)

	err = os.MkdirAll(resourcePath, 0700)
	if err != nil {
		err = errors.Wrap(err, "failed to create cache directory for package resources")
		return
	}

	pkg, err := types.GetCachedPackage(meta, cacheDir)
	if err != nil {
		pkg, err = types.GetRemotePackage(ctx, gh, meta)
		if err != nil {
			err = errors.Wrap(err, "failed to get remote package definition file")
			return
		}
	}

	resource, err = GetResourceForPlatform(pkg.Resources, platform)
	if err != nil {
		return
	}

	matcher, err := regexp.Compile(resource.Name)
	if err != nil {
		err = errors.Wrap(err, "resource name is not a valid regular expression")
		return
	}

	filename, _, err = download.ReleaseAssetByPattern(ctx, gh, meta, matcher, resourcePathOnly, "", cacheDir)
	if err != nil {
		return
	}

	print.Verb(meta, "downloaded", filename, "to cache")

	return
}

// GetResourceForPlatform searches a list of resources for one that matches the given platform
func GetResourceForPlatform(resources []types.Resource, platform string) (resource types.Resource, err error) {
	var tmp *types.Resource
	for _, res := range resources {
		if res.Platform == platform {
			tmp = &res
			break
		}
	}
	if tmp == nil {
		err = errors.Errorf("plugin does not provide binaries for target platform %s", platform)
		return
	}

	err = tmp.Validate()
	if err != nil {
		err = errors.Wrap(err, "matching resource found but is invalid")
		return
	}
	resource = *tmp
	return
}

// GetResourcePath returns a path where a resource should be stored given the metadata
func GetResourcePath(meta versioning.DependencyMeta) (path string) {
	tag := meta.Tag
	if tag == "" {
		tag = "latest"
	}
	return filepath.Join("plugins", meta.Repo, tag)
}
