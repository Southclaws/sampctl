package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// EnsurePlugins validates and downloads plugin binary files
func EnsurePlugins(cfg *types.Runtime, cacheDir string, noCache bool) (err error) {
	fmt.Println("ensuring runtime plugins", cfg.Plugins)

	pluginsDir := util.FullPath(filepath.Join(cfg.WorkingDir, "plugins"))

	err = os.MkdirAll(pluginsDir, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create runtime plugins directory")
	}

	fileExt := pluginExtForFile(cfg.Platform)

	var (
		newPlugins = []types.Plugin{}
		files      = []types.Plugin{}
		meta       versioning.DependencyMeta
	)

	for _, plugin := range cfg.Plugins {
		meta, err = plugin.AsDep()
		if err != nil {
			fmt.Println("plugin", plugin, "is a local plugin")
			fullpath := filepath.Join(pluginsDir, string(plugin)+fileExt)
			if !util.Exists(fullpath) {
				fmt.Println(fmt.Sprintf("plugin '%s' is missing its %s file from the plugins directory", plugin, fileExt))
			}
			newPlugins = append(newPlugins, plugin)
		} else {
			fmt.Println("plugin", plugin, "is a package dependency")
			files, err = EnsureVersionedPlugin(*cfg, meta, cacheDir, noCache)
			if err != nil {
				fmt.Println(fmt.Sprintf("plugin '%s' failed to ensure: %v", plugin, err))
			}
			newPlugins = append(newPlugins, files...)
		}
		err = nil
	}

	cfg.Plugins = []types.Plugin{}

	// trim extensions for plugins list, they are added later by GenerateServerCFG if needed
	for _, plugin := range newPlugins {
		cfg.Plugins = append(cfg.Plugins, types.Plugin(strings.TrimSuffix(string(plugin), fileExt)))
	}

	return
}

// EnsureVersionedPlugin automatically downloads a plugin binary from its github releases page
func EnsureVersionedPlugin(cfg types.Runtime, meta versioning.DependencyMeta, cacheDir string, noCache bool) (files []types.Plugin, err error) {
	var (
		hit      bool
		filename string
		resource types.Resource
	)
	if !noCache {
		hit, filename, resource, err = PluginFromCache(meta, cfg.Platform, cacheDir)
		if err != nil {
			err = errors.Wrapf(err, "failed to get plugin %s from cache", meta)
			return
		}
	}
	if !hit {
		filename, resource, err = PluginFromNet(meta, cfg.Platform, cacheDir)
		if err != nil {
			err = errors.Wrapf(err, "failed to get plugin %s from net", meta)
			return
		}
	}

	fmt.Println("retrieved package", meta, "resource file:", filename)

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
	for _, plugin := range resource.Plugins {
		filename := filepath.Base(plugin)
		paths[plugin] = filepath.Join("plugins", filename)
		files = append(files, types.Plugin(filename))
	}

	// get additional files
	for src, dest := range resource.Files {
		paths[src] = dest
	}

	err = method(filename, cfg.WorkingDir, paths)

	return
}

// PluginFromCache tries to grab a plugin asset from the cache, `hit` indicates if it was successful
func PluginFromCache(meta versioning.DependencyMeta, platform, cacheDir string) (hit bool, filename string, resource types.Resource, err error) {
	resourcePath := filepath.Join(cacheDir, GetResourcePath(meta))

	pkg, err := types.PackageFromDir(resourcePath)
	if err != nil {
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

	files, err := ioutil.ReadDir(resourcePath)
	if err != nil {
		err = errors.Wrap(err, "failed to read cache directory for package")
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
func PluginFromNet(meta versioning.DependencyMeta, platform, cacheDir string) (filename string, resource types.Resource, err error) {
	resourcePath := filepath.Join(cacheDir, GetResourcePath(meta))

	fmt.Println("downloading remote plugin to", resourcePath)

	err = os.MkdirAll(resourcePath, 0755)
	if err != nil {
		err = errors.Wrap(err, "failed to create cache directory for package resources")
		return
	}

	pkg, err := GetPluginRemotePackage(meta)
	if err != nil {
		err = errors.Wrap(err, "failed to get remote package definition file")
		return
	}

	pkgJSON, err := json.Marshal(pkg)
	if err != nil {
		err = errors.Wrap(err, "failed to encode package to json")
		return
	}

	err = ioutil.WriteFile(filepath.Join(resourcePath, "pawn.json"), pkgJSON, 0755)
	if err != nil {
		err = errors.Wrap(err, "failed to write package file to cache")
		if err != nil {
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

	filename, err = DownloadResource(meta, matcher, platform, cacheDir)
	if err != nil {
		return
	}

	return
}

// GetPluginRemotePackage attempts to get a package definition for the given dependency meta
// it first checks the repository itself, if that fails it falls back to using the sampctl central
// plugin metadata repository
func GetPluginRemotePackage(meta versioning.DependencyMeta) (pkg types.Package, err error) {
	resp, err := http.Get(fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/master/pawn.json", meta.User, meta.Repo))
	if err != nil {
		return
	}

	if resp.StatusCode == 200 {
		var contents []byte
		contents, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		err = json.Unmarshal(contents, &pkg)
		return
	}

	resp, err = http.Get(fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/master/pawn.yaml", meta.User, meta.Repo))
	if err != nil {
		return
	}

	if resp.StatusCode == 200 {
		var contents []byte
		contents, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		err = yaml.Unmarshal(contents, &pkg)
		return
	}

	fmt.Printf("https://raw.githubusercontent.com/sampctl/plugins/master/%s-%s.json\n", meta.User, meta.Repo)
	resp, err = http.Get(fmt.Sprintf("https://raw.githubusercontent.com/sampctl/plugins/master/%s-%s.json", meta.User, meta.Repo))
	if err != nil {
		return
	}

	if resp.StatusCode == 200 {
		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&pkg)
		return
	}

	err = errors.New("could not find plugin package definition")

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
		err = errors.New("plugin does not provide binaries for target platform")
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

// DownloadResource downloads a resource file, which is a GitHub release asset
func DownloadResource(meta versioning.DependencyMeta, matcher *regexp.Regexp, platform, cacheDir string) (filename string, err error) {
	resourcePath := GetResourcePath(meta)

	var (
		client = github.NewClient(nil)
		asset  *github.ReleaseAsset
		assets []string
	)

	var release *github.RepositoryRelease
	if meta.Version == "" {
		release, _, err = client.Repositories.GetLatestRelease(context.Background(), meta.User, meta.Repo)
	} else {
		release, _, err = client.Repositories.GetReleaseByTag(context.Background(), meta.User, meta.Repo, meta.Version)
	}
	if err != nil {
		return
	}

	for _, a := range release.Assets {
		if matcher.MatchString(*a.Name) {
			asset = &a
			break
		}
		assets = append(assets, *a.Name)
	}
	if asset == nil {
		err = errors.Errorf("resource matcher '%s' does not match any release assets from '%v'", matcher, assets)
		return
	}

	u, _ := url.Parse(*asset.BrowserDownloadURL)
	outputFile := filepath.Join(resourcePath, filepath.Base(u.Path))

	filename, err = download.FromNet(*asset.BrowserDownloadURL, cacheDir, outputFile)
	return
}

// GetResourcePath returns a path where a resource should be stored given the metadata
func GetResourcePath(meta versioning.DependencyMeta) (path string) {
	version := meta.Version
	if version == "" {
		version = "latest"
	}
	return filepath.Join("plugins", meta.Repo, version)
}
