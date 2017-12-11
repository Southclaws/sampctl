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
	errs := ValidateServerDir(*cfg.dir, *cfg.Version)
	if errs != nil {
		fmt.Println(errs)
		err = GetServerPackage(*cfg.Endpoint, *cfg.Version, *cfg.dir)
		if err != nil {
			return errors.Wrap(err, "failed to get runtime package")
		}
	}

	err = cfg.GenerateServerCfg(*cfg.dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate server.cfg")
	}

	return
}

// ValidateWorkspace compares a Config to a directory and checks that all the declared gamemodes,
// filterscripts and plugins are present.
func (cfg Config) ValidateWorkspace(dir string) (errs []error) {
	for _, gamemode := range cfg.Gamemodes {
		fullpath := filepath.Join(dir, "gamemodes", gamemode+".amx")
		if !util.Exists(fullpath) {
			errs = append(errs, errors.Errorf("gamemode '%s' is missing its .amx file from the gamemodes directory", gamemode))
		}
	}
	for _, filterscript := range cfg.Filterscripts {
		fullpath := filepath.Join(dir, "filterscripts", filterscript+".amx")
		if !util.Exists(fullpath) {
			errs = append(errs, errors.Errorf("filterscript '%s' is missing its .amx file from the filterscripts directory", filterscript))
		}
	}
	var ext string
	switch runtime.GOOS {
	case "windows":
		ext = ".dll"
	case "linux", "darwin":
		ext = ".so"
	default:
		errs = append(errs, errors.New("unsupported platform"))
	}
	for _, plugin := range cfg.Plugins {
		fullpath := filepath.Join(dir, "plugins", string(plugin)+ext)
		if !util.Exists(fullpath) {
			errs = append(errs, errors.Errorf("plugin '%s' is missing its %s file from the plugins directory", plugin, ext))
		}
	}
	return
}

// ValidateServerDir ensures the dir has all the necessary files to run a server, it also performs an MD5
// checksum against the binary to prevent running anything unwanted.
func ValidateServerDir(dir, version string) (err error) {
	errs := []string{}
	if !util.Exists(filepath.Join(dir, getNpcBinary())) {
		errs = append(errs, "missing npc binary")
	}
	if !util.Exists(filepath.Join(dir, getAnnounceBinary())) {
		errs = append(errs, "missing announce binary")
	}
	if !util.Exists(filepath.Join(dir, getServerBinary())) {
		errs = append(errs, "missing server binary")
	} else {
		// now perform an md5 on the server
		ok, err := matchesChecksum(filepath.Join(dir, getServerBinary()), version)
		if err != nil {
			errs = append(errs, "failed to match checksum")
		} else if !ok {
			errs = append(errs, fmt.Sprintf("existing binary does not match checksum for version %s", version))
		}
	}

	if errs != nil {
		err = errors.New(strings.Join(errs, ", "))
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
