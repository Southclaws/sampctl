package runtime

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

func getAmxFiles(dir string) (result []string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) == ".amx" {
			result = append(result, strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())))
		}
	}
	return
}

func getPlugins(dir, platform string) (result []string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	var ext string
	if platform == "windows" {
		ext = ".dll"
	} else if platform == "linux" || platform == "darwin" {
		ext = ".so"
	} else {
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
