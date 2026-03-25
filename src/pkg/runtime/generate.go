package runtime

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

// adjustForOS quickly does some tweaks depending on the OS such as .so plugin extension on linux
func adjustForOS(dir, platform string, cfg *run.Runtime) error {
	if platform == "linux" || platform == "darwin" {
		if len(cfg.Plugins) > 0 {
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
					// if the declared plugin matches the found plugin case-insensitively but does match
					// case sensitively...
					if strings.EqualFold(string(declared), actual) && string(declared) != actual {
						// update the array index to use the actual filename
						declared = run.Plugin(actual)
						break
					}
				}
				cfg.Plugins[i] = declared + ".so"
			}
		}
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
