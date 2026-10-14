package runtime

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

// adjustForOS adds the plugin extension and uses the installed filename's case.
func adjustForOS(dir, platform string, cfg *run.Runtime) error {
	if (platform != "linux" && platform != "darwin") || len(cfg.Plugins) == 0 {
		return nil
	}

	actualPlugins, err := getPlugins(filepath.Join(dir, getPluginDirectory()), cfg.Platform)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to detect installed plugins")
	}

	for i, declared := range cfg.Plugins {
		ext := filepath.Ext(string(declared))
		if ext != "" {
			declared = run.Plugin(strings.TrimSuffix(string(declared), ext))
		}
		for _, actual := range actualPlugins {
			if strings.EqualFold(string(declared), actual) && string(declared) != actual {
				declared = run.Plugin(actual)
				break
			}
		}
		cfg.Plugins[i] = declared + ".so"
	}

	return nil
}

func getPlugins(dir, platform string) (result []string, err error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var ext string
	switch platform {
	case "windows":
		ext = ".dll"
	case "linux", "darwin":
		ext = ".so"
	default:
		return nil, errors.Errorf("unsupported OS %s", platform)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) == ext {
			result = append(result, strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())))
		}
	}

	return result, nil
}
