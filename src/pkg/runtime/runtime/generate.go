package runtime

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
	"github.com/pkg/errors"
)

// adjustForOS quickly does some tweaks depending on the OS such as .so plugin extension on linux
func adjustForOS(dir, os string, cfg *run.Runtime) {
	if os == "linux" || os == "darwin" {
		if len(cfg.Plugins) > 0 {
			actualPlugins := getPlugins(filepath.Join(dir, getPluginDirectory()), cfg.Platform)

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
}

func getPlugins(dir, platform string) (result []string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	var ext string
	switch platform {
	case "windows":
		ext = ".dll"
	case "linux", "darwin":
		ext = ".so"
	default:
		panic(errors.Errorf("unsupported OS %s", platform))
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) == ext {
			result = append(result, strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())))
		}
	}
	return
}
