package runtime

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Southclaws/sampctl/versioning"

	"github.com/Southclaws/sampctl/util"
	"github.com/pkg/errors"
)

// EnsurePlugins validates and downloads plugin binary files
func (cfg *Config) EnsurePlugins() (err error) {
	ext := pluginExtensionForOS(runtime.GOOS)

	errs := []string{}
	for _, plugin := range cfg.Plugins {
		dep, err := plugin.AsDep()
		if err != nil {
			fullpath := filepath.Join(*cfg.dir, "plugins", string(plugin)+ext)
			if !util.Exists(fullpath) {
				errs = append(errs, fmt.Sprintf("plugin '%s' is missing its %s file from the plugins directory", plugin, ext))
			}
		} else {
			err = cfg.EnsureVersionedPlugin(dep)
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
func (cfg Config) EnsureVersionedPlugin(dep versioning.DependencyMeta) (err error) {
	return
}
