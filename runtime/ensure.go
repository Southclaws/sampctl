package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Southclaws/sampctl/util"
	"github.com/pkg/errors"
)

// Ensure will make sure a Config's dir is representative of the held configuration.
// If any of the following are missing or mismatching, they will be automatically downloaded:
// - Server binaries (server, announce, npc)
// - Plugin binaries
// and a `server.cfg` is generated based on the contents of the Config fields.
func (cfg Config) Ensure() (err error) {
	err = cfg.EnsureBinaries()
	if err != nil {
		return
	}

	// err = cfg.ResolvePlugins()
	// if err != nil {
	// 	return
	// }

	err = cfg.ValidateWorkspace()
	if err != nil {
		return errors.Wrap(err, "configuration contains errors")
	}

	err = cfg.GenerateServerCfg(*cfg.dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate server.cfg")
	}

	return
}

// ValidateWorkspace compares a Config to a directory and checks that all the declared gamemodes,
// filterscripts and plugins are present.
func (cfg Config) ValidateWorkspace() (err error) {
	errs := []string{}

	for _, gamemode := range cfg.Gamemodes {
		fullpath := filepath.Join(*cfg.dir, "gamemodes", gamemode+".amx")
		if !util.Exists(fullpath) {
			errs = append(errs, fmt.Sprintf("gamemode '%s' is missing its .amx file from the gamemodes directory", gamemode))
		}
	}
	for _, filterscript := range cfg.Filterscripts {
		fullpath := filepath.Join(*cfg.dir, "filterscripts", filterscript+".amx")
		if !util.Exists(fullpath) {
			errs = append(errs, fmt.Sprintf("filterscript '%s' is missing its .amx file from the filterscripts directory", filterscript))
		}
	}

	var ext string
	switch runtime.GOOS {
	case "windows":
		ext = ".dll"
	case "linux", "darwin":
		ext = ".so"
	}

	for _, plugin := range cfg.Plugins {
		fullpath := filepath.Join(*cfg.dir, "plugins", string(plugin)+ext)
		if !util.Exists(fullpath) {
			errs = append(errs, fmt.Sprintf("plugin '%s' is missing its %s file from the plugins directory", plugin, ext))
		}
	}

	if len(errs) > 0 {
		err = errors.New(strings.Join(errs, ", "))
	}

	return
}

// EnsureBinaries ensures the dir has all the necessary files to run a server, it also performs an MD5
// checksum against the binary to prevent running anything unwanted.
func (cfg Config) EnsureBinaries() (err error) {
	missing := false

	if !util.Exists(filepath.Join(*cfg.dir, getNpcBinary())) {
		missing = true
	}
	if !util.Exists(filepath.Join(*cfg.dir, getAnnounceBinary())) {
		missing = true
	}
	if !util.Exists(filepath.Join(*cfg.dir, getServerBinary())) {
		missing = true
	}

	if missing {
		err = GetServerPackage(*cfg.Endpoint, *cfg.Version, *cfg.dir)
		if err != nil {
			return errors.Wrap(err, "failed to get runtime package")
		}
	}

	ok, err := MatchesChecksum(filepath.Join(*cfg.dir, getServerBinary()), *cfg.Version)
	if err != nil {
		return errors.Wrap(err, "failed to match checksum")
	} else if !ok {
		return errors.Errorf("existing binary does not match checksum for version %s", *cfg.Version)
	}

	return
}

// CreateServerDirectories simply creates the necessary gamemodes and filterscripts directories
func CreateServerDirectories(dir string) (err error) {
	err = os.MkdirAll(filepath.Join(dir, "gamemodes"), 0755)
	if err != nil {
		return
	}
	err = os.MkdirAll(filepath.Join(dir, "filterscripts"), 0755)
	return
}
