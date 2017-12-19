package types

import (
	"github.com/pkg/errors"
)

// Resource represents a resource associated with a package
type Resource struct {
	Name     string            `json:"name"`     // filename pattern of the resource
	Platform string            `json:"platform"` // target platform, if empty the resource is always used but if this is set and does not match the runtime OS, the resource is ignored
	Archive  bool              `json:"archive"`  // is this resource an archive file or just a single file?
	Includes []string          `json:"includes"` // if archive: paths to directories containing .inc files for the compiler
	Plugins  []string          `json:"plugins"`  // if archive: paths to plugin binaries, either .so or .dll
	Files    map[string]string `json:"files"`    // if archive: path-to-path map of any other files, keys are paths inside the archive and values are extraction paths relative to the sampctl working directory
}

// Validate checks for missing fields
func (res Resource) Validate() (err error) {
	if res.Name == "" {
		return errors.New("missing name field in resource")
	}
	if res.Platform == "" {
		return errors.New("missing platform field in resource")
	}
	return
}
