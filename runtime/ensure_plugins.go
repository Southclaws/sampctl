package runtime

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// EnsurePlugins validates and downloads plugin binary files
func EnsurePlugins(cfg *types.Runtime) (err error) {
	ext := pluginExtensionForOS(runtime.GOOS)

	errs := []string{}
	for _, plugin := range cfg.Plugins {
		dep, err := plugin.AsDep()
		if err != nil {
			fullpath := filepath.Join(*cfg.WorkingDir, "plugins", string(plugin)+ext)
			if !util.Exists(fullpath) {
				errs = append(errs, fmt.Sprintf("plugin '%s' is missing its %s file from the plugins directory", plugin, ext))
			}
		} else {
			err = EnsureVersionedPlugin(*cfg, dep)
			if err != nil {
				errs = append(errs, fmt.Sprintf("plugin '%s' failed to ensure: %v", plugin, err))
			}
		}
	}
	if len(errs) > 0 {
		err = errors.New(strings.Join(errs, ", "))
	}

	return
}

// EnsureVersionedPlugin automatically downloads a plugin binary from its github releases page
func EnsureVersionedPlugin(cfg types.Runtime, dep versioning.DependencyMeta) (err error) {
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
		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&pkg)
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

	resp, err = http.Get(fmt.Sprintf("https://raw.githubusercontent.com/sampctl/plugins/master/%s-%s.json", meta.User, meta.Repo))
	if err != nil {
		return
	}

	if resp.StatusCode == 200 {
		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&pkg)
		return
	}

	err = errors.New("repository does not contain a pawn.json or pawn.yaml file")

	return
}
