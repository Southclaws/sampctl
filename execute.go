package main

import (
	"github.com/pkg/errors"
)

// Execute handles the actual running of the server process - it collects log output too
func Execute(endpoint, version, dir string) (err error) {
	empty, errs := validate(dir, version)
	if errs != nil {
		return errors.Errorf("directory in invalid state: %v", errs)
	}
	if empty {
		err := GetPackage(endpoint, version, dir)
		if err != nil {
			return errors.Wrap(err, "failed to get server package")
		}
	}

	server, err := NewConfigFromEnvironment(dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate config from environment")
	}

	err = server.GenerateServerCfg(dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate server.cfg")
	}

	// create a Server object from config/env vars
	// generate server.cfg
	// execute platform binary in goroutine
	// collect stdout and print in other goroutine
	// in future, add options to send to syslog, database or something
	// also, parse log depending on the log_format setting, pull out time/date and maybe in future, allow custom regex for grouping outputs
	return
}
