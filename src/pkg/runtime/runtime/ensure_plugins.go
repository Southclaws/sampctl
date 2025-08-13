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

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/resource"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// EnsurePlugins validates and downloads plugin binary files
func EnsurePlugins(
	ctx context.Context,
	gh *github.Client,
	cfg *run.Runtime,
	cacheDir string,
	noCache bool,
) (err error) {

	fileExt := pluginExtForFile(cfg.Platform)

	var (
		newPlugins = []run.Plugin{}
		files      []run.Plugin
	)

	for _, plugin := range cfg.PluginDeps {
		files, err = EnsureVersionedPlugin(ctx, gh, plugin, cfg.WorkingDir, cfg.Platform, cfg.Version, cacheDir, true, false, noCache)
		if err != nil {
			print.Warn("failed to ensure plugin", plugin, err)
			err = nil
			continue
		}
		print.Verb("adding dependency", plugin, "files", files, "to plugins list")
		newPlugins = append(newPlugins, files...)
	}

	added := make(map[run.Plugin]struct{})

	// trim extensions for plugins list, they are added later by GenerateServerCFG if needed
	for _, plugin := range newPlugins {
		pluginName := run.Plugin(strings.TrimSuffix(string(plugin), fileExt))
		if _, ok := added[pluginName]; ok {
			continue
		}

		print.Verb("adding plugin by local filename", pluginName)
		cfg.Plugins = append(cfg.Plugins, pluginName)
		added[pluginName] = struct{}{}
	}

	if len(added) != 0 {
		pluginsDir := util.FullPath(filepath.Join(cfg.WorkingDir, "plugins"))
		err = os.MkdirAll(pluginsDir, 0700)
		if err != nil {
			return errors.Wrap(err, "failed to create runtime plugins directory")
		}
	}

	return err
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
	plugins bool,
	includes bool,
	noCache bool,
) (files []run.Plugin, err error) {
	filename, resource, err := EnsureVersionedPluginCached(ctx, meta, platform, version, cacheDir, noCache, gh)
	if err != nil {
		return
	}

	print.Verb(meta, "retrieved package to file:", filename)

	if resource.Archive {
		print.Verb(meta, "plugin resource is an archive")
		var (
			ext    = filepath.Ext(filename)
			method download.ExtractFunc
		)
		switch ext {
		case ".zip":
			method = download.Unzip
		case ".gz":
			method = download.Untar
		default:
			err = errors.Errorf("unsupported archive format: %s", filename)
			return
		}

		paths := make(map[string]string)

		// get plugins
		if plugins {
			for _, plugin := range resource.Plugins {
				print.Verb(meta, "marking plugin path", plugin, "for extraction to ./plugins/")
				paths[plugin] = "plugins/"
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
			print.Verb(meta, "marking misc file path", src, "for extraction to", dest)
			paths[src] = dest
		}

		var extractedFiles map[string]string
		extractedFiles, err = method(filename, dir, paths)
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
		finalDir := filepath.Join(dir, "plugins")
		destination := filepath.Join(finalDir, base)

		err = os.MkdirAll(finalDir, 0700)
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
	resource *resource.Resource,
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
) (hit bool, filename string, resource *resource.Resource, err error) {
	resourcePath := filepath.Join(cacheDir, GetResourcePath(meta))
	print.Verb("getting plugin resource from cache", meta, resourcePath)

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

	files, err := ioutil.ReadDir(resourcePath)
	if err != nil {
		print.Verb("cache hit failed while trying to read cached plugin folder:", err)
		err = nil
		hit = false
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
) (filename string, resource *resource.Resource, err error) {
	print.Info(meta, "downloading plugin resource for", platform)

	resourcePathOnly := GetResourcePath(meta)
	resourcePath := filepath.Join(cacheDir, resourcePathOnly)

	err = os.MkdirAll(resourcePath, 0700)
	if err != nil {
		err = errors.Wrap(err, "failed to create cache directory for package resources")
		return
	}

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

	filename, _, err = download.ReleaseAssetByPattern(ctx, gh, meta, matcher, resourcePathOnly, "", cacheDir)
	if err != nil {
		return
	}

	print.Verb(meta, "downloaded", filename, "to cache")

	return filename, resource, nil
}

// GetResource searches a list of resources for one that matches the given platform
func GetResource(resources []resource.Resource, platform string, version string) (*resource.Resource, error) {
	if version == "" {
		version = "0.3.7"
	}

	found := false
	var tmp *resource.Resource
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
