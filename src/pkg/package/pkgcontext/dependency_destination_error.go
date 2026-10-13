package pkgcontext

import (
	"fmt"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// DependencyDestinationConflictError identifies dependencies that require the same directory.
type DependencyDestinationConflictError struct {
	First     versioning.DependencyMeta
	Second    versioning.DependencyMeta
	Directory string
}

func (err *DependencyDestinationConflictError) Error() string {
	return fmt.Sprintf("dependencies %s and %s share the installation directory %s", err.First, err.Second, err.Directory)
}
