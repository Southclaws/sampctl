package resource

import (
	"crypto/md5" //nolint
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
)

// Resource represents a resource associated with a package
// nolint:lll
type Resource struct {
	Name     string            `json:"name,omitempty" yaml:"name,omitempty"`         // filename pattern of the resource
	Platform string            `json:"platform,omitempty" yaml:"platform,omitempty"` // target platform, if empty the resource is always used but if this is set and does not match the runtime OS, the resource is ignored
	Version  string            `json:"version,omitempty" yaml:"version,omitempty"`   // which server version this resource belongs to
	Archive  bool              `json:"archive,omitempty" yaml:"archive,omitempty"`   // is this resource an archive file or just a single file?
	Includes []string          `json:"includes,omitempty" yaml:"includes,omitempty"` // if archive: paths to directories containing .inc files for the compiler
	Plugins  []string          `json:"plugins,omitempty" yaml:"plugins,omitempty"`   // if archive: paths to plugin binaries, either .so or .dll
	Files    map[string]string `json:"files,omitempty" yaml:"files,omitempty"`       // if archive: path-to-path map of any other files, keys are paths inside the archive and values are extraction paths relative to the sampctl working directory
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

// Path returns a file path for a resource based on a hash of the label
// nolint
func (res Resource) Path(repo string) (path string) {
	sum := md5.Sum([]byte(res.Name))
	return filepath.Join(".resources", fmt.Sprintf("%s-%x", repo, sum[:3]))
}
