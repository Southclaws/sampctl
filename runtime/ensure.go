package runtime

import (
	"fmt"

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
