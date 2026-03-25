package config

import (
	"encoding/json"
	"os"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func writeConfigFile(path string, cfg Config) error {
	contents, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, contents, fs.PermFileShared)
}
