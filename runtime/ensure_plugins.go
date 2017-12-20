package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// EnsurePlugins validates and downloads plugin binary files
func EnsurePlugins(cfg *types.Runtime, cacheDir string) (err error) {
	fmt.Println("ensuring runtime plugins", cfg.Plugins)

	pluginsDir := util.FullPath(filepath.Join(cfg.WorkingDir, "plugins"))

	err = os.MkdirAll(pluginsDir, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create runtime plugins directory")
	}

	fileExt := pluginExtForFile(cfg.Platform)

	var (
		errs       = []string{}
		newPlugins = []types.Plugin{}
		files      = []types.Plugin{}
	)

	for _, plugin := range cfg.Plugins {
		meta, err := plugin.AsDep()
		if err != nil {
			fmt.Println("plugin", plugin, "is a local plugin")
			fullpath := filepath.Join(pluginsDir, string(plugin)+fileExt)
			if !util.Exists(fullpath) {
				errs = append(errs, fmt.Sprintf("plugin '%s' is missing its %s file from the plugins directory", plugin, fileExt))
			}
			newPlugins = append(newPlugins, plugin)
		} else {
			fmt.Println("plugin", plugin, "is a package dependency")
			files, err = EnsureVersionedPlugin(*cfg, meta, cacheDir)
			if err != nil {
				errs = append(errs, fmt.Sprintf("plugin '%s' failed to ensure: %v", plugin, err))
			}
			newPlugins = append(newPlugins, files...)
		}
	}
	if len(errs) > 0 {
		err = errors.New(strings.Join(errs, ", "))
	}

	cfg.Plugins = []types.Plugin{}

	// trim extensions for plugins list, they are added later by GenerateServerCFG if needed
	for _, plugin := range newPlugins {
		cfg.Plugins = append(cfg.Plugins, types.Plugin(strings.TrimSuffix(string(plugin), fileExt)))
	}

	return
}

// EnsureVersionedPlugin automatically downloads a plugin binary from its github releases page
func EnsureVersionedPlugin(cfg types.Runtime, meta versioning.DependencyMeta, cacheDir string) (files []types.Plugin, err error) {
	files, err = PluginFromNet(meta, cfg.Platform, cfg.WorkingDir, cacheDir)
	return
}

// PluginFromNet downloads a plugin from the given metadata to the cache directory
func PluginFromNet(meta versioning.DependencyMeta, platform, workingDir, cacheDir string) (files []types.Plugin, err error) {
	pkg, err := GetPluginRemotePackage(meta)
	if err != nil {
		return
	}

	var resource *types.Resource

	for _, res := range pkg.Resources {
		if res.Platform == platform {
			resource = &res
			break
		}
	}
	if resource == nil {
		err = errors.New("plugin does not provide binaries for target platform")
		return
	}

	err = resource.Validate()
	if err != nil {
		return
	}

	matcher, err := regexp.Compile(resource.Name)
	if err != nil {
		err = errors.Wrap(err, "resource name is not a valid regular expression")
		return
	}

	client := github.NewClient(nil)

	var release *github.RepositoryRelease
	if meta.Version == "" {
		release, _, err = client.Repositories.GetLatestRelease(context.Background(), meta.User, meta.Repo)
	} else {
		release, _, err = client.Repositories.GetReleaseByTag(context.Background(), meta.User, meta.Repo, meta.Version)
	}
	if err != nil {
		return
	}

	var (
		asset  *github.ReleaseAsset
		assets []string
	)
	for _, a := range release.Assets {
		if matcher.MatchString(*a.Name) {
			asset = &a
			break
		}
		assets = append(assets, *a.Name)
	}
	if asset == nil {
		err = errors.Errorf("resource name '%s' does not match any release assets from '%v'", resource.Name, assets)
		return
	}

	// download url should be valid from github api
	// nolint
	u, _ := url.Parse(*asset.BrowserDownloadURL)
	filename := filepath.Base(u.Path)

	fullPath, err := download.FromNet(*asset.BrowserDownloadURL, cacheDir, filename)
	if err != nil {
		return
	}

	var method download.ExtractFunc
	if filepath.Ext(filename) == ".zip" {
		method = download.Unzip
	} else if filepath.Ext(filename) == ".gz" {
		method = download.Untar
	} else {
		err = errors.Errorf("unsupported archive format: %s", filepath.Ext(filename))
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

	err = method(fullPath, workingDir, paths)

	return
}

// GetPluginRemotePackage attempts to get a package definition for the given dependency meta
// it first checks the repository itself, if that fails it falls back to using the sampctl central
// plugin metadata repository
func GetPluginRemotePackage(meta versioning.DependencyMeta) (pkg types.Package, err error) {
	// resp, err := http.Get(fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/master/pawn.json", meta.User, meta.Repo))
	// if err != nil {
	// 	return
	// }

	// if resp.StatusCode == 200 {
	// 	var contents []byte
	// 	contents, err = ioutil.ReadAll(resp.Body)
	// 	if err != nil {
	// 		return
	// 	}
	// 	err = json.Unmarshal(contents, &pkg)
	// 	return
	// }

	// resp, err = http.Get(fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/master/pawn.yaml", meta.User, meta.Repo))
	// if err != nil {
	// 	return
	// }

	// if resp.StatusCode == 200 {
	// 	var contents []byte
	// 	contents, err = ioutil.ReadAll(resp.Body)
	// 	if err != nil {
	// 		return
	// 	}
	// 	err = yaml.Unmarshal(contents, &pkg)
	// 	return
	// }

	resp, err := http.Get(fmt.Sprintf("https://raw.githubusercontent.com/sampctl/plugins/master/%s-%s.json", meta.User, meta.Repo))
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
